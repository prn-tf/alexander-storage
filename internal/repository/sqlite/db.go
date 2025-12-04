// Package sqlite provides SQLite database utilities for embedded deployments.
// This package uses modernc.org/sqlite, a pure Go SQLite implementation that
// doesn't require CGO, making it ideal for cross-platform single-binary deployments.
package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"

	"github.com/rs/zerolog"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Config holds SQLite connection settings.
type Config struct {
	// Path is the path to the SQLite database file.
	// Use ":memory:" for in-memory database.
	Path string

	// MaxOpenConns sets the maximum number of open connections.
	MaxOpenConns int

	// MaxIdleConns sets the maximum number of idle connections.
	MaxIdleConns int

	// ConnMaxLifetime sets the maximum connection lifetime.
	ConnMaxLifetime time.Duration

	// JournalMode sets the SQLite journal mode (WAL recommended for concurrency).
	JournalMode string

	// BusyTimeout sets the busy timeout in milliseconds.
	BusyTimeout int

	// CacheSize sets the page cache size (negative = KB, positive = pages).
	CacheSize int

	// SynchronousMode sets the synchronous mode (NORMAL, FULL, OFF).
	SynchronousMode string
}

// DefaultConfig returns a default SQLite configuration.
func DefaultConfig(dbPath string) Config {
	return Config{
		Path:            dbPath,
		MaxOpenConns:    1, // SQLite works best with single writer
		MaxIdleConns:    1,
		ConnMaxLifetime: time.Hour,
		JournalMode:     "WAL",
		BusyTimeout:     5000,  // 5 seconds
		CacheSize:       -2000, // 2MB
		SynchronousMode: "NORMAL",
	}
}

// DB wraps a sql.DB connection for SQLite.
type DB struct {
	db     *sql.DB
	logger zerolog.Logger
	path   string
}

// NewDB creates a new SQLite database connection.
func NewDB(ctx context.Context, cfg Config, logger zerolog.Logger) (*DB, error) {
	// Build connection string with pragmas
	connStr := cfg.Path
	if cfg.Path != ":memory:" {
		// Ensure directory exists
		dir := filepath.Dir(cfg.Path)
		if dir != "." && dir != "" {
			// Directory creation should be handled by caller
		}
	}

	// Add pragmas to connection string
	connStr = fmt.Sprintf(
		"%s?_journal_mode=%s&_busy_timeout=%d&_cache_size=%d&_synchronous=%s&_foreign_keys=ON",
		cfg.Path,
		cfg.JournalMode,
		cfg.BusyTimeout,
		cfg.CacheSize,
		cfg.SynchronousMode,
	)

	db, err := sql.Open("sqlite", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open SQLite database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	// Verify connection
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping SQLite database: %w", err)
	}

	logger.Info().
		Str("path", cfg.Path).
		Str("journal_mode", cfg.JournalMode).
		Int("max_conns", cfg.MaxOpenConns).
		Msg("connected to SQLite database")

	return &DB{
		db:     db,
		logger: logger,
		path:   cfg.Path,
	}, nil
}

// Close closes the database connection.
func (db *DB) Close() error {
	db.logger.Info().Msg("closing SQLite connection")
	return db.db.Close()
}

// Ping checks the database connection.
func (db *DB) Ping(ctx context.Context) error {
	return db.db.PingContext(ctx)
}

// Health checks the database connection health.
func (db *DB) Health(ctx context.Context) error {
	return db.Ping(ctx)
}

// DB returns the underlying sql.DB.
func (db *DB) DB() *sql.DB {
	return db.db
}

// BeginTx starts a new transaction.
func (db *DB) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	return db.db.BeginTx(ctx, opts)
}

// WithTx executes a function within a transaction.
// If the function returns an error, the transaction is rolled back.
// Otherwise, the transaction is committed.
func (db *DB) WithTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
	}()

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("tx error: %v, rollback error: %w", err, rbErr)
		}
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// ExecContext executes a query without returning rows.
func (db *DB) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return db.db.ExecContext(ctx, query, args...)
}

// QueryContext executes a query that returns rows.
func (db *DB) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return db.db.QueryContext(ctx, query, args...)
}

// QueryRowContext executes a query that returns a single row.
func (db *DB) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return db.db.QueryRowContext(ctx, query, args...)
}

// Migrate runs database migrations.
func (db *DB) Migrate(ctx context.Context) error {
	// Create migrations table if not exists
	_, err := db.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TEXT NOT NULL DEFAULT (datetime('now'))
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	// Get current version
	var currentVersion int
	err = db.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&currentVersion)
	if err != nil {
		return fmt.Errorf("failed to get current migration version: %w", err)
	}

	db.logger.Info().Int("current_version", currentVersion).Msg("checking migrations")

	// For now, we apply all migrations in a single file
	// In a production system, you'd iterate through migration files
	if currentVersion < 1 {
		// Read and execute the init migration
		migration, err := migrationsFS.ReadFile("migrations/000001_init.up.sql")
		if err != nil {
			// If embedded migrations not found, try to continue (migrations may be applied externally)
			db.logger.Warn().Msg("embedded migrations not found, skipping auto-migration")
			return nil
		}

		if _, err := db.db.ExecContext(ctx, string(migration)); err != nil {
			return fmt.Errorf("failed to apply migration 1: %w", err)
		}

		if _, err := db.db.ExecContext(ctx, `INSERT INTO schema_migrations (version) VALUES (1)`); err != nil {
			return fmt.Errorf("failed to record migration: %w", err)
		}

		db.logger.Info().Int("version", 1).Msg("applied migration")
	}

	return nil
}
