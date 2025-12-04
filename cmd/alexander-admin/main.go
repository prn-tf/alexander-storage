// Package main is the entry point for the Alexander Storage admin CLI.
// This tool provides administrative commands for managing users, access keys, and system operations.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/prn-tf/alexander-storage/internal/config"
	"github.com/prn-tf/alexander-storage/internal/domain"
	"github.com/prn-tf/alexander-storage/internal/lock"
	"github.com/prn-tf/alexander-storage/internal/pkg/crypto"
	"github.com/prn-tf/alexander-storage/internal/repository"
	"github.com/prn-tf/alexander-storage/internal/repository/postgres"
	"github.com/prn-tf/alexander-storage/internal/repository/sqlite"
	"github.com/prn-tf/alexander-storage/internal/service"
	"github.com/prn-tf/alexander-storage/internal/storage/filesystem"
)

// Version information (set at build time)
var (
	Version   = "dev"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "version":
		printVersion()

	case "user":
		handleUserCommand(os.Args[2:])

	case "accesskey":
		handleAccessKeyCommand(os.Args[2:])

	case "bucket":
		handleBucketCommand(os.Args[2:])

	case "gc":
		handleGCCommand(os.Args[2:])

	case "help", "-h", "--help":
		printUsage()

	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printVersion() {
	fmt.Printf("Alexander Storage Admin CLI\n")
	fmt.Printf("Version: %s\n", Version)
	fmt.Printf("Build Time: %s\n", BuildTime)
	fmt.Printf("Git Commit: %s\n", GitCommit)
}

func printUsage() {
	fmt.Println(`Alexander Storage Admin CLI

Usage:
  alexander-admin <command> [arguments]

Commands:
  user        Manage users (create, list, delete, update)
  accesskey   Manage access keys (create, list, revoke)
  bucket      Manage buckets (list, delete, set-versioning)
  gc          Run garbage collection for orphan blobs
  version     Print version information
  help        Show this help message

Examples:
  alexander-admin user create --username admin --email admin@example.com --admin
  alexander-admin user list
  alexander-admin accesskey create --user-id 1
  alexander-admin accesskey list --user-id 1
  alexander-admin bucket list
  alexander-admin gc run --dry-run

Use "alexander-admin <command> --help" for more information about a command.`)
}

// =============================================================================
// Initialization Helpers
// =============================================================================

type adminContext struct {
	ctx       context.Context
	cfg       *config.Config
	repos     *repository.Repositories
	encryptor *crypto.Encryptor
	dbCloser  func()
	logger    zerolog.Logger
}

func initAdminContext() (*adminContext, error) {
	// Initialize logger
	zerolog.TimeFieldFormat = time.RFC3339
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})

	// Load configuration
	cfg, err := config.Load("")
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration: %w", err)
	}

	// Set log level
	level, err := zerolog.ParseLevel(cfg.Logging.Level)
	if err != nil {
		level = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(level)

	ctx := context.Background()
	var repos *repository.Repositories
	var dbCloser func()

	if cfg.Database.Driver == "sqlite" {
		// SQLite mode
		if err := os.MkdirAll(filepath.Dir(cfg.Database.Path), 0755); err != nil {
			return nil, fmt.Errorf("failed to create database directory: %w", err)
		}

		sqliteDB, err := sqlite.NewDB(ctx, sqlite.Config{
			Path:            cfg.Database.Path,
			MaxOpenConns:    cfg.Database.MaxOpenConns,
			MaxIdleConns:    cfg.Database.MaxIdleConns,
			ConnMaxLifetime: cfg.Database.ConnMaxLifetime,
			JournalMode:     cfg.Database.JournalMode,
			BusyTimeout:     cfg.Database.BusyTimeout,
			CacheSize:       cfg.Database.CacheSize,
			SynchronousMode: cfg.Database.SynchronousMode,
		}, log.Logger)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to SQLite: %w", err)
		}
		dbCloser = func() { sqliteDB.Close() }

		// Run migrations
		if err := sqliteDB.Migrate(ctx); err != nil {
			dbCloser()
			return nil, fmt.Errorf("failed to run migrations: %w", err)
		}

		repos = &repository.Repositories{
			User:      sqlite.NewUserRepository(sqliteDB),
			AccessKey: sqlite.NewAccessKeyRepository(sqliteDB),
			Bucket:    sqlite.NewBucketRepository(sqliteDB),
			Object:    sqlite.NewObjectRepository(sqliteDB),
			Blob:      sqlite.NewBlobRepository(sqliteDB),
			Multipart: sqlite.NewMultipartRepository(sqliteDB),
		}
	} else {
		// PostgreSQL mode
		pgDB, err := postgres.NewDB(ctx, cfg.Database, log.Logger)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to PostgreSQL: %w", err)
		}
		dbCloser = func() { pgDB.Close() }

		repos = &repository.Repositories{
			User:      postgres.NewUserRepository(pgDB),
			AccessKey: postgres.NewAccessKeyRepository(pgDB),
			Bucket:    postgres.NewBucketRepository(pgDB),
			Object:    postgres.NewObjectRepository(pgDB),
			Blob:      postgres.NewBlobRepository(pgDB),
			Multipart: postgres.NewMultipartRepository(pgDB),
		}
	}

	// Initialize encryptor
	encryptionKey, err := cfg.Auth.GetEncryptionKey()
	if err != nil {
		dbCloser()
		return nil, fmt.Errorf("invalid encryption key: %w", err)
	}
	encryptor, err := crypto.NewEncryptor(encryptionKey)
	if err != nil {
		dbCloser()
		return nil, fmt.Errorf("failed to initialize encryptor: %w", err)
	}

	return &adminContext{
		ctx:       ctx,
		cfg:       cfg,
		repos:     repos,
		encryptor: encryptor,
		dbCloser:  dbCloser,
		logger:    log.Logger,
	}, nil
}

// =============================================================================
// User Commands
// =============================================================================

func handleUserCommand(args []string) {
	if len(args) == 0 {
		printUserUsage()
		os.Exit(1)
	}

	subcommand := args[0]
	subArgs := args[1:]

	switch subcommand {
	case "create":
		userCreate(subArgs)
	case "list":
		userList(subArgs)
	case "get":
		userGet(subArgs)
	case "delete":
		userDelete(subArgs)
	case "help", "-h", "--help":
		printUserUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown user subcommand: %s\n", subcommand)
		printUserUsage()
		os.Exit(1)
	}
}

func printUserUsage() {
	fmt.Println(`User management commands

Usage:
  alexander-admin user <subcommand> [arguments]

Subcommands:
  create      Create a new user
  list        List all users
  get         Get user details by ID or username
  delete      Delete a user

Examples:
  alexander-admin user create --username admin --email admin@example.com --admin
  alexander-admin user list
  alexander-admin user get --id 1
  alexander-admin user delete --id 1`)
}

func userCreate(args []string) {
	fs := flag.NewFlagSet("user create", flag.ExitOnError)
	username := fs.String("username", "", "Username (required)")
	email := fs.String("email", "", "Email address (required)")
	password := fs.String("password", "", "Password (leave empty for auto-generated)")
	isAdmin := fs.Bool("admin", false, "Grant admin privileges")
	jsonOutput := fs.Bool("json", false, "Output in JSON format")

	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	if *username == "" || *email == "" {
		fmt.Fprintln(os.Stderr, "Error: --username and --email are required")
		fs.Usage()
		os.Exit(1)
	}

	adminCtx, err := initAdminContext()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer adminCtx.dbCloser()

	userService := service.NewUserService(adminCtx.repos.User, adminCtx.logger)

	// Auto-generate password if not provided
	actualPassword := *password
	if actualPassword == "" {
		actualPassword = generateSecurePassword(16)
	}

	output, err := userService.Create(adminCtx.ctx, service.CreateUserInput{
		Username: *username,
		Email:    *email,
		Password: actualPassword,
		IsAdmin:  *isAdmin,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating user: %v\n", err)
		os.Exit(1)
	}

	if *jsonOutput {
		result := map[string]interface{}{
			"id":       output.User.ID,
			"username": output.User.Username,
			"email":    output.User.Email,
			"is_admin": output.User.IsAdmin,
			"password": actualPassword,
		}
		jsonBytes, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(jsonBytes))
	} else {
		fmt.Printf("User created successfully!\n")
		fmt.Printf("  ID:       %d\n", output.User.ID)
		fmt.Printf("  Username: %s\n", output.User.Username)
		fmt.Printf("  Email:    %s\n", output.User.Email)
		fmt.Printf("  Admin:    %v\n", output.User.IsAdmin)
		if *password == "" {
			fmt.Printf("  Password: %s\n", actualPassword)
			fmt.Println("\n⚠️  Save this password - it won't be shown again!")
		}
	}
}

func userList(args []string) {
	fs := flag.NewFlagSet("user list", flag.ExitOnError)
	jsonOutput := fs.Bool("json", false, "Output in JSON format")
	limit := fs.Int("limit", 100, "Maximum number of users to return")
	offset := fs.Int("offset", 0, "Offset for pagination")

	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	adminCtx, err := initAdminContext()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer adminCtx.dbCloser()

	userService := service.NewUserService(adminCtx.repos.User, adminCtx.logger)

	output, err := userService.List(adminCtx.ctx, service.ListUsersInput{
		Limit:  *limit,
		Offset: *offset,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing users: %v\n", err)
		os.Exit(1)
	}

	if *jsonOutput {
		jsonBytes, _ := json.MarshalIndent(output.Users, "", "  ")
		fmt.Println(string(jsonBytes))
	} else {
		fmt.Printf("Users (total: %d):\n", output.TotalCount)
		fmt.Println(strings.Repeat("-", 80))
		fmt.Printf("%-8s %-20s %-30s %-8s %-10s\n", "ID", "Username", "Email", "Admin", "Active")
		fmt.Println(strings.Repeat("-", 80))
		for _, u := range output.Users {
			fmt.Printf("%-8d %-20s %-30s %-8v %-10v\n", u.ID, u.Username, u.Email, u.IsAdmin, u.IsActive)
		}
	}
}

func userGet(args []string) {
	fs := flag.NewFlagSet("user get", flag.ExitOnError)
	id := fs.Int64("id", 0, "User ID")
	username := fs.String("username", "", "Username")
	jsonOutput := fs.Bool("json", false, "Output in JSON format")

	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	if *id == 0 && *username == "" {
		fmt.Fprintln(os.Stderr, "Error: --id or --username is required")
		fs.Usage()
		os.Exit(1)
	}

	adminCtx, err := initAdminContext()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer adminCtx.dbCloser()

	userService := service.NewUserService(adminCtx.repos.User, adminCtx.logger)

	var user *domain.User
	if *id > 0 {
		user, err = userService.GetByID(adminCtx.ctx, *id)
	} else {
		user, err = userService.GetByUsername(adminCtx.ctx, *username)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting user: %v\n", err)
		os.Exit(1)
	}

	if *jsonOutput {
		jsonBytes, _ := json.MarshalIndent(user, "", "  ")
		fmt.Println(string(jsonBytes))
	} else {
		fmt.Printf("User Details:\n")
		fmt.Printf("  ID:         %d\n", user.ID)
		fmt.Printf("  Username:   %s\n", user.Username)
		fmt.Printf("  Email:      %s\n", user.Email)
		fmt.Printf("  Admin:      %v\n", user.IsAdmin)
		fmt.Printf("  Active:     %v\n", user.IsActive)
		fmt.Printf("  Created At: %s\n", user.CreatedAt.Format(time.RFC3339))
	}
}

func userDelete(args []string) {
	fs := flag.NewFlagSet("user delete", flag.ExitOnError)
	id := fs.Int64("id", 0, "User ID (required)")
	force := fs.Bool("force", false, "Skip confirmation")

	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	if *id == 0 {
		fmt.Fprintln(os.Stderr, "Error: --id is required")
		fs.Usage()
		os.Exit(1)
	}

	if !*force {
		fmt.Printf("Are you sure you want to delete user %d? (yes/no): ", *id)
		var confirm string
		fmt.Scanln(&confirm)
		if strings.ToLower(confirm) != "yes" {
			fmt.Println("Cancelled.")
			return
		}
	}

	adminCtx, err := initAdminContext()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer adminCtx.dbCloser()

	userService := service.NewUserService(adminCtx.repos.User, adminCtx.logger)

	if err := userService.Delete(adminCtx.ctx, *id); err != nil {
		fmt.Fprintf(os.Stderr, "Error deleting user: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("User %d deleted successfully.\n", *id)
}

// =============================================================================
// Access Key Commands
// =============================================================================

func handleAccessKeyCommand(args []string) {
	if len(args) == 0 {
		printAccessKeyUsage()
		os.Exit(1)
	}

	subcommand := args[0]
	subArgs := args[1:]

	switch subcommand {
	case "create":
		accessKeyCreate(subArgs)
	case "list":
		accessKeyList(subArgs)
	case "revoke":
		accessKeyRevoke(subArgs)
	case "help", "-h", "--help":
		printAccessKeyUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown accesskey subcommand: %s\n", subcommand)
		printAccessKeyUsage()
		os.Exit(1)
	}
}

func printAccessKeyUsage() {
	fmt.Println(`Access key management commands

Usage:
  alexander-admin accesskey <subcommand> [arguments]

Subcommands:
  create      Create a new access key for a user
  list        List access keys for a user
  revoke      Revoke an access key

Examples:
  alexander-admin accesskey create --user-id 1
  alexander-admin accesskey list --user-id 1
  alexander-admin accesskey revoke --access-key-id AKIAIOSFODNN7EXAMPLE`)
}

func accessKeyCreate(args []string) {
	fs := flag.NewFlagSet("accesskey create", flag.ExitOnError)
	userID := fs.Int64("user-id", 0, "User ID (required)")
	description := fs.String("description", "", "Description for the access key")
	expiresDays := fs.Int("expires-days", 0, "Days until expiration (0 = never)")
	jsonOutput := fs.Bool("json", false, "Output in JSON format")

	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	if *userID == 0 {
		fmt.Fprintln(os.Stderr, "Error: --user-id is required")
		fs.Usage()
		os.Exit(1)
	}

	adminCtx, err := initAdminContext()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer adminCtx.dbCloser()

	iamService := service.NewIAMService(adminCtx.repos.AccessKey, adminCtx.repos.User, adminCtx.encryptor, adminCtx.logger)

	var expiresAt *time.Time
	if *expiresDays > 0 {
		t := time.Now().AddDate(0, 0, *expiresDays)
		expiresAt = &t
	}

	output, err := iamService.CreateAccessKey(adminCtx.ctx, service.CreateAccessKeyInput{
		UserID:      *userID,
		Description: *description,
		ExpiresAt:   expiresAt,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating access key: %v\n", err)
		os.Exit(1)
	}

	if *jsonOutput {
		result := map[string]interface{}{
			"access_key_id":     output.AccessKeyID,
			"secret_access_key": output.SecretKey,
		}
		if expiresAt != nil {
			result["expires_at"] = expiresAt.Format(time.RFC3339)
		}
		jsonBytes, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(jsonBytes))
	} else {
		fmt.Printf("Access key created successfully!\n\n")
		fmt.Printf("  Access Key ID:     %s\n", output.AccessKeyID)
		fmt.Printf("  Secret Access Key: %s\n", output.SecretKey)
		if expiresAt != nil {
			fmt.Printf("  Expires At:        %s\n", expiresAt.Format(time.RFC3339))
		}
		fmt.Println("\n⚠️  Save the secret access key - it won't be shown again!")
	}
}

func accessKeyList(args []string) {
	fs := flag.NewFlagSet("accesskey list", flag.ExitOnError)
	userID := fs.Int64("user-id", 0, "User ID (required)")
	jsonOutput := fs.Bool("json", false, "Output in JSON format")

	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	if *userID == 0 {
		fmt.Fprintln(os.Stderr, "Error: --user-id is required")
		fs.Usage()
		os.Exit(1)
	}

	adminCtx, err := initAdminContext()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer adminCtx.dbCloser()

	iamService := service.NewIAMService(adminCtx.repos.AccessKey, adminCtx.repos.User, adminCtx.encryptor, adminCtx.logger)

	keys, err := iamService.ListAccessKeys(adminCtx.ctx, service.ListAccessKeysInput{
		UserID:     *userID,
		ActiveOnly: false,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing access keys: %v\n", err)
		os.Exit(1)
	}

	if *jsonOutput {
		jsonBytes, _ := json.MarshalIndent(keys, "", "  ")
		fmt.Println(string(jsonBytes))
	} else {
		fmt.Printf("Access Keys for User %d:\n", *userID)
		fmt.Println(strings.Repeat("-", 100))
		fmt.Printf("%-24s %-10s %-20s %-20s\n", "Access Key ID", "Status", "Created At", "Last Used")
		fmt.Println(strings.Repeat("-", 100))
		for _, k := range keys {
			lastUsed := "Never"
			if k.LastUsedAt != nil {
				lastUsed = k.LastUsedAt.Format("2006-01-02 15:04")
			}
			fmt.Printf("%-24s %-10s %-20s %-20s\n",
				k.AccessKeyID,
				k.Status,
				k.CreatedAt.Format("2006-01-02 15:04"),
				lastUsed,
			)
		}
	}
}

func accessKeyRevoke(args []string) {
	fs := flag.NewFlagSet("accesskey revoke", flag.ExitOnError)
	accessKeyID := fs.String("access-key-id", "", "Access Key ID (required)")
	force := fs.Bool("force", false, "Skip confirmation")

	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	if *accessKeyID == "" {
		fmt.Fprintln(os.Stderr, "Error: --access-key-id is required")
		fs.Usage()
		os.Exit(1)
	}

	if !*force {
		fmt.Printf("Are you sure you want to revoke access key %s? (yes/no): ", *accessKeyID)
		var confirm string
		fmt.Scanln(&confirm)
		if strings.ToLower(confirm) != "yes" {
			fmt.Println("Cancelled.")
			return
		}
	}

	adminCtx, err := initAdminContext()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer adminCtx.dbCloser()

	iamService := service.NewIAMService(adminCtx.repos.AccessKey, adminCtx.repos.User, adminCtx.encryptor, adminCtx.logger)

	if err := iamService.DeactivateAccessKey(adminCtx.ctx, *accessKeyID); err != nil {
		fmt.Fprintf(os.Stderr, "Error revoking access key: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Access key %s revoked successfully.\n", *accessKeyID)
}

// =============================================================================
// Bucket Commands
// =============================================================================

func handleBucketCommand(args []string) {
	if len(args) == 0 {
		printBucketUsage()
		os.Exit(1)
	}

	subcommand := args[0]
	subArgs := args[1:]

	switch subcommand {
	case "list":
		bucketList(subArgs)
	case "delete":
		bucketDelete(subArgs)
	case "set-versioning":
		bucketSetVersioning(subArgs)
	case "help", "-h", "--help":
		printBucketUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown bucket subcommand: %s\n", subcommand)
		printBucketUsage()
		os.Exit(1)
	}
}

func printBucketUsage() {
	fmt.Println(`Bucket management commands

Usage:
  alexander-admin bucket <subcommand> [arguments]

Subcommands:
  list            List all buckets
  delete          Delete a bucket (must be empty)
  set-versioning  Enable or disable versioning

Examples:
  alexander-admin bucket list
  alexander-admin bucket list --owner-id 1
  alexander-admin bucket delete --name my-bucket --force
  alexander-admin bucket set-versioning --name my-bucket --status enabled`)
}

func bucketList(args []string) {
	fs := flag.NewFlagSet("bucket list", flag.ExitOnError)
	ownerID := fs.Int64("owner-id", 0, "Filter by owner ID (0 = all)")
	jsonOutput := fs.Bool("json", false, "Output in JSON format")

	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	adminCtx, err := initAdminContext()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer adminCtx.dbCloser()

	bucketService := service.NewBucketService(adminCtx.repos.Bucket, adminCtx.logger)

	output, err := bucketService.ListBuckets(adminCtx.ctx, service.ListBucketsInput{
		OwnerID: *ownerID,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing buckets: %v\n", err)
		os.Exit(1)
	}

	if *jsonOutput {
		jsonBytes, _ := json.MarshalIndent(output.Buckets, "", "  ")
		fmt.Println(string(jsonBytes))
	} else {
		fmt.Printf("Buckets:\n")
		fmt.Println(strings.Repeat("-", 80))
		fmt.Printf("%-30s %-10s %-15s %-20s\n", "Name", "Owner ID", "Versioning", "Created At")
		fmt.Println(strings.Repeat("-", 80))
		for _, b := range output.Buckets {
			fmt.Printf("%-30s %-10d %-15s %-20s\n",
				b.Name,
				b.OwnerID,
				b.Versioning,
				b.CreatedAt.Format("2006-01-02 15:04"),
			)
		}
	}
}

func bucketDelete(args []string) {
	fs := flag.NewFlagSet("bucket delete", flag.ExitOnError)
	name := fs.String("name", "", "Bucket name (required)")
	force := fs.Bool("force", false, "Skip confirmation")

	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	if *name == "" {
		fmt.Fprintln(os.Stderr, "Error: --name is required")
		fs.Usage()
		os.Exit(1)
	}

	if !*force {
		fmt.Printf("Are you sure you want to delete bucket '%s'? (yes/no): ", *name)
		var confirm string
		fmt.Scanln(&confirm)
		if strings.ToLower(confirm) != "yes" {
			fmt.Println("Cancelled.")
			return
		}
	}

	adminCtx, err := initAdminContext()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer adminCtx.dbCloser()

	bucketService := service.NewBucketService(adminCtx.repos.Bucket, adminCtx.logger)

	// Use OwnerID 0 to bypass ownership check (admin operation)
	if err := bucketService.DeleteBucket(adminCtx.ctx, service.DeleteBucketInput{
		Name:    *name,
		OwnerID: 0,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "Error deleting bucket: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Bucket '%s' deleted successfully.\n", *name)
}

func bucketSetVersioning(args []string) {
	fs := flag.NewFlagSet("bucket set-versioning", flag.ExitOnError)
	name := fs.String("name", "", "Bucket name (required)")
	status := fs.String("status", "", "Versioning status: enabled or suspended (required)")

	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	if *name == "" || *status == "" {
		fmt.Fprintln(os.Stderr, "Error: --name and --status are required")
		fs.Usage()
		os.Exit(1)
	}

	*status = strings.ToLower(*status)
	if *status != "enabled" && *status != "suspended" {
		fmt.Fprintln(os.Stderr, "Error: --status must be 'enabled' or 'suspended'")
		os.Exit(1)
	}

	adminCtx, err := initAdminContext()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer adminCtx.dbCloser()

	bucketService := service.NewBucketService(adminCtx.repos.Bucket, adminCtx.logger)

	var versioningStatus domain.VersioningStatus
	if *status == "enabled" {
		versioningStatus = domain.VersioningEnabled
	} else {
		versioningStatus = domain.VersioningSuspended
	}

	if err := bucketService.PutBucketVersioning(adminCtx.ctx, service.PutBucketVersioningInput{
		Name:    *name,
		Status:  versioningStatus,
		OwnerID: 0, // Admin bypass
	}); err != nil {
		fmt.Fprintf(os.Stderr, "Error setting versioning: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Versioning %s for bucket '%s'.\n", *status, *name)
}

// =============================================================================
// GC Commands
// =============================================================================

func handleGCCommand(args []string) {
	if len(args) == 0 {
		printGCUsage()
		os.Exit(1)
	}

	subcommand := args[0]
	subArgs := args[1:]

	switch subcommand {
	case "run":
		gcRun(subArgs)
	case "status":
		gcStatus(subArgs)
	case "help", "-h", "--help":
		printGCUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown gc subcommand: %s\n", subcommand)
		printGCUsage()
		os.Exit(1)
	}
}

func printGCUsage() {
	fmt.Println(`Garbage collection commands

Usage:
  alexander-admin gc <subcommand> [arguments]

Subcommands:
  run       Run garbage collection manually
  status    Show orphan blob statistics

Examples:
  alexander-admin gc run --dry-run
  alexander-admin gc run --batch-size 500
  alexander-admin gc status`)
}

func gcRun(args []string) {
	fs := flag.NewFlagSet("gc run", flag.ExitOnError)
	dryRun := fs.Bool("dry-run", false, "Show what would be deleted without deleting")
	batchSize := fs.Int("batch-size", 1000, "Maximum blobs to process per run")
	gracePeriod := fs.Duration("grace-period", 24*time.Hour, "Grace period before deleting orphans")
	jsonOutput := fs.Bool("json", false, "Output in JSON format")

	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	adminCtx, err := initAdminContext()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer adminCtx.dbCloser()

	// Initialize storage backend
	storageCfg := adminCtx.cfg.Storage
	storageBackend, err := filesystem.NewStorage(filesystem.Config{
		DataDir: storageCfg.DataDir,
		TempDir: storageCfg.TempDir,
	}, adminCtx.logger)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing storage: %v\n", err)
		os.Exit(1)
	}

	// Create locker (use NoOp for CLI since we're running manually)
	locker := lock.NewNoOpLocker()

	gc := service.NewGarbageCollector(
		adminCtx.repos.Blob,
		storageBackend,
		locker,
		nil, // No metrics
		adminCtx.logger,
		service.GCConfig{
			Enabled:     true,
			Interval:    1 * time.Hour,
			GracePeriod: *gracePeriod,
			BatchSize:   *batchSize,
			DryRun:      *dryRun,
		},
	)

	if *dryRun {
		fmt.Println("Running garbage collection in DRY RUN mode (no actual deletions)...")
	} else {
		fmt.Println("Running garbage collection...")
	}

	result := gc.RunOnce(adminCtx.ctx)

	if *jsonOutput {
		jsonBytes, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(jsonBytes))
	} else {
		fmt.Printf("\nGC Result:\n")
		fmt.Printf("  Blobs Deleted:    %d\n", result.BlobsDeleted)
		fmt.Printf("  Bytes Freed:      %s\n", formatBytes(result.BytesFreed))
		fmt.Printf("  Errors:           %d\n", result.Errors)
		fmt.Printf("  Duration:         %s\n", result.Duration.Round(time.Millisecond))
		if result.OrphanBlobsRemaining > 0 {
			fmt.Printf("  Remaining Orphans: ~%d (run again to process more)\n", result.OrphanBlobsRemaining)
		}
	}
}

func gcStatus(args []string) {
	fs := flag.NewFlagSet("gc status", flag.ExitOnError)
	gracePeriod := fs.Duration("grace-period", 24*time.Hour, "Grace period for counting orphans")
	jsonOutput := fs.Bool("json", false, "Output in JSON format")

	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	adminCtx, err := initAdminContext()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer adminCtx.dbCloser()

	// List orphan blobs to get count
	orphans, err := adminCtx.repos.Blob.ListOrphans(adminCtx.ctx, *gracePeriod, 10000)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing orphans: %v\n", err)
		os.Exit(1)
	}

	var totalSize int64
	for _, b := range orphans {
		totalSize += b.Size
	}

	if *jsonOutput {
		result := map[string]interface{}{
			"orphan_count":    len(orphans),
			"orphan_size":     totalSize,
			"grace_period_ns": gracePeriod.Nanoseconds(),
		}
		jsonBytes, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(jsonBytes))
	} else {
		fmt.Printf("Garbage Collection Status:\n")
		fmt.Printf("  Orphan Blobs:  %d\n", len(orphans))
		fmt.Printf("  Orphan Size:   %s\n", formatBytes(totalSize))
		fmt.Printf("  Grace Period:  %s\n", *gracePeriod)
		if len(orphans) >= 10000 {
			fmt.Printf("\n  Note: Count may be higher (limited to 10000)\n")
		}
	}
}

// =============================================================================
// Utility Functions
// =============================================================================

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func generateSecurePassword(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
		time.Sleep(time.Nanosecond)
	}
	return string(b)
}
