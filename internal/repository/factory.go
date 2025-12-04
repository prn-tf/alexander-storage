// Package repository provides data access layer for Alexander Storage.
// This file contains factory functions to create repositories based on configuration.
package repository

import (
	"context"
	"fmt"

	"github.com/rs/zerolog"

	"github.com/prn-tf/alexander-storage/internal/config"
)

// Repositories holds all repository instances.
type Repositories struct {
	User      UserRepository
	AccessKey AccessKeyRepository
	Bucket    BucketRepository
	Object    ObjectRepository
	Blob      BlobRepository
	Multipart MultipartUploadRepository
}

// DatabaseHealth is an interface for database health checks.
// This interface satisfies handler.DatabaseChecker for health endpoints.
type DatabaseHealth interface {
	Ping(ctx context.Context) error
	Health(ctx context.Context) error
	Close() error
}

// Factory creates repositories based on configuration.
type Factory struct {
	cfg    config.DatabaseConfig
	logger zerolog.Logger
}

// NewFactory creates a new repository factory.
func NewFactory(cfg config.DatabaseConfig, logger zerolog.Logger) *Factory {
	return &Factory{
		cfg:    cfg,
		logger: logger,
	}
}

// Driver returns the configured database driver.
func (f *Factory) Driver() string {
	return f.cfg.Driver
}

// IsEmbedded returns true if using embedded database.
func (f *Factory) IsEmbedded() bool {
	return f.cfg.IsEmbedded()
}

// CreateRepositoriesResult contains the created repositories and database connection.
type CreateRepositoriesResult struct {
	Repos    *Repositories
	Database DatabaseHealth
}

// CreatePostgres creates PostgreSQL repositories.
// This is a placeholder - the actual implementation is in the postgres package.
func CreatePostgres(ctx context.Context, cfg config.DatabaseConfig, logger zerolog.Logger) (*CreateRepositoriesResult, error) {
	return nil, fmt.Errorf("PostgreSQL factory not implemented in this package - use postgres.NewDB directly")
}

// CreateSQLite creates SQLite repositories.
// This is a placeholder - the actual implementation is in the sqlite package.
func CreateSQLite(ctx context.Context, cfg config.DatabaseConfig, logger zerolog.Logger) (*CreateRepositoriesResult, error) {
	return nil, fmt.Errorf("SQLite factory not implemented in this package - use sqlite.NewDB directly")
}
