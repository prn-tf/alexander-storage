// Package main is the entry point for the Alexander Storage server.
// Alexander Storage is an enterprise-grade, S3-compatible object storage system.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/prn-tf/alexander-storage/internal/auth"
	"github.com/prn-tf/alexander-storage/internal/config"
	"github.com/prn-tf/alexander-storage/internal/handler"
	"github.com/prn-tf/alexander-storage/internal/pkg/crypto"
	"github.com/prn-tf/alexander-storage/internal/repository/postgres"
	"github.com/prn-tf/alexander-storage/internal/service"
	"github.com/prn-tf/alexander-storage/internal/storage"
	"github.com/prn-tf/alexander-storage/internal/storage/filesystem"
)

// Version information (set at build time)
var (
	Version   = "dev"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

func main() {
	// Initialize logger
	zerolog.TimeFieldFormat = time.RFC3339Nano
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})

	log.Info().
		Str("version", Version).
		Str("build_time", BuildTime).
		Str("git_commit", GitCommit).
		Msg("Starting Alexander Storage Server")

	// Load configuration
	cfg, err := config.Load("")
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load configuration")
	}

	// Set log level
	level, err := zerolog.ParseLevel(cfg.Logging.Level)
	if err != nil {
		level = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(level)

	// Initialize database connection
	ctx := context.Background()
	db, err := postgres.NewDB(ctx, cfg.Database, log.Logger)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to database")
	}
	defer db.Close()

	log.Info().Msg("Connected to database")

	// Initialize repositories
	userRepo := postgres.NewUserRepository(db)
	accessKeyRepo := postgres.NewAccessKeyRepository(db)
	bucketRepo := postgres.NewBucketRepository(db)
	objectRepo := postgres.NewObjectRepository(db)
	blobRepo := postgres.NewBlobRepository(db)

	// Initialize encryptor
	encryptionKey, err := cfg.Auth.GetEncryptionKey()
	if err != nil {
		log.Fatal().Err(err).Msg("Invalid encryption key")
	}
	encryptor, err := crypto.NewEncryptor(encryptionKey)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize encryptor")
	}

	// Initialize storage backend
	storageBackend, err := initStorageBackend(cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize storage backend")
	}

	// Initialize services
	iamService := service.NewIAMService(accessKeyRepo, userRepo, encryptor, log.Logger)
	bucketService := service.NewBucketService(bucketRepo, log.Logger)
	objectService := service.NewObjectService(bucketRepo, objectRepo, blobRepo, storageBackend, log.Logger)

	// Initialize auth middleware
	accessKeyStore := service.NewAccessKeyStoreAdapter(iamService)
	authConfig := auth.Config{
		Region:         cfg.Auth.Region,
		Service:        cfg.Auth.Service,
		AllowAnonymous: false,
		SkipPaths:      []string{"/health"},
	}
	authMiddleware := handler.CreateAuthMiddleware(accessKeyStore, authConfig)

	// Initialize handlers
	bucketHandler := handler.NewBucketHandler(bucketService, log.Logger)
	objectHandler := handler.NewObjectHandler(objectService, log.Logger)

	// Initialize router
	router := handler.NewRouter(handler.RouterConfig{
		BucketHandler:  bucketHandler,
		ObjectHandler:  objectHandler,
		AuthMiddleware: authMiddleware,
		Logger:         log.Logger,
	})

	// Create HTTP server
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      router.Handler(),
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	// Start server in goroutine
	go func() {
		log.Info().
			Int("port", cfg.Server.Port).
			Str("region", cfg.Auth.Region).
			Msg("Server listening")

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("Server failed")
		}
	}()

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Info().Msg("Shutting down server...")

	// Graceful shutdown with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("Server shutdown error")
	}

	log.Info().Msg("Server stopped")
}

// initStorageBackend initializes the storage backend based on configuration.
func initStorageBackend(cfg *config.Config) (storage.Backend, error) {
	// For now, we only support filesystem backend
	// TODO: Add support for other backends (S3, Azure Blob, etc.)
	return filesystem.NewStorage(cfg.Storage.DataDir)
}
