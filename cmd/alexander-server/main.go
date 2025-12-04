// Package main is the entry point for the Alexander Storage server.
// Alexander Storage is an enterprise-grade, S3-compatible object storage system.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/prn-tf/alexander-storage/internal/auth"
	"github.com/prn-tf/alexander-storage/internal/cache/memory"
	"github.com/prn-tf/alexander-storage/internal/config"
	"github.com/prn-tf/alexander-storage/internal/handler"
	"github.com/prn-tf/alexander-storage/internal/lock"
	"github.com/prn-tf/alexander-storage/internal/metrics"
	"github.com/prn-tf/alexander-storage/internal/middleware"
	"github.com/prn-tf/alexander-storage/internal/pkg/crypto"
	"github.com/prn-tf/alexander-storage/internal/repository"
	"github.com/prn-tf/alexander-storage/internal/repository/postgres"
	"github.com/prn-tf/alexander-storage/internal/repository/sqlite"
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

	// Initialize database and repositories based on driver
	ctx := context.Background()
	var repos *repository.Repositories
	var dbCloser func()
	var dbHealth repository.DatabaseHealth

	if cfg.Database.Driver == "sqlite" {
		// SQLite / Embedded mode
		log.Info().Str("driver", "sqlite").Str("path", cfg.Database.Path).Msg("Using embedded SQLite database")

		// Ensure directory exists for SQLite database
		if err := os.MkdirAll(filepath.Dir(cfg.Database.Path), 0755); err != nil {
			log.Fatal().Err(err).Msg("Failed to create database directory")
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
			log.Fatal().Err(err).Msg("Failed to connect to SQLite database")
		}
		dbCloser = func() { sqliteDB.Close() }
		dbHealth = sqliteDB

		// Run migrations
		if err := sqliteDB.Migrate(ctx); err != nil {
			log.Fatal().Err(err).Msg("Failed to run SQLite migrations")
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
		// PostgreSQL mode (default)
		log.Info().Str("driver", "postgres").Str("host", cfg.Database.Host).Msg("Using PostgreSQL database")

		pgDB, err := postgres.NewDB(ctx, cfg.Database, log.Logger)
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to connect to PostgreSQL database")
		}
		dbCloser = func() { pgDB.Close() }
		dbHealth = pgDB

		repos = &repository.Repositories{
			User:      postgres.NewUserRepository(pgDB),
			AccessKey: postgres.NewAccessKeyRepository(pgDB),
			Bucket:    postgres.NewBucketRepository(pgDB),
			Object:    postgres.NewObjectRepository(pgDB),
			Blob:      postgres.NewBlobRepository(pgDB),
			Multipart: postgres.NewMultipartRepository(pgDB),
		}
	}
	defer dbCloser()

	log.Info().Msg("Connected to database")

	// Initialize cache and lock based on mode
	var memCache *memory.Cache
	var locker lock.Locker

	if !cfg.Redis.Enabled || cfg.Database.IsEmbedded() {
		// Single-node mode: use in-memory cache and locks
		log.Info().Msg("Using in-memory cache and locks (single-node mode)")
		memCache = memory.NewCache()
		locker = lock.NewMemoryLocker()
		defer memCache.Stop()
	} else {
		// Distributed mode: Redis would be used here
		// For now, still use memory-based (Redis integration can be added)
		log.Info().Msg("Redis enabled but using in-memory fallback")
		memCache = memory.NewCache()
		locker = lock.NewMemoryLocker()
		defer memCache.Stop()
	}

	// Silence unused variable warning for cache (will be used for metadata caching in future)
	_ = memCache

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
	storageBackend, err := initStorageBackend(cfg, log.Logger)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize storage backend")
	}

	// Initialize services
	iamService := service.NewIAMService(repos.AccessKey, repos.User, encryptor, log.Logger)
	bucketService := service.NewBucketService(repos.Bucket, log.Logger)
	objectService := service.NewObjectService(repos.Object, repos.Blob, repos.Bucket, storageBackend, locker, log.Logger)
	multipartService := service.NewMultipartService(repos.Multipart, repos.Object, repos.Blob, repos.Bucket, storageBackend, locker, log.Logger)

	// Initialize metrics
	var m *metrics.Metrics
	if cfg.Metrics.Enabled {
		m = metrics.New()
		log.Info().Int("port", cfg.Metrics.Port).Msg("Prometheus metrics enabled")
	}

	// Initialize garbage collector
	var gc *service.GarbageCollector
	if cfg.GC.Enabled {
		gc = service.NewGarbageCollector(
			repos.Blob,
			storageBackend,
			locker,
			m,
			log.Logger,
			service.GCConfig{
				Enabled:     cfg.GC.Enabled,
				Interval:    cfg.GC.Interval,
				GracePeriod: cfg.GC.GracePeriod,
				BatchSize:   cfg.GC.BatchSize,
				DryRun:      cfg.GC.DryRun,
			},
		)
		gc.Start()
		defer gc.Stop()
		log.Info().
			Dur("interval", cfg.GC.Interval).
			Dur("grace_period", cfg.GC.GracePeriod).
			Msg("Garbage collector started")
	}

	// Initialize rate limiter
	var rateLimiter *middleware.RateLimiter
	if cfg.RateLimit.Enabled {
		rateLimiter = middleware.NewRateLimiter(
			middleware.RateLimiterConfig{
				RequestsPerSecond: cfg.RateLimit.RequestsPerSecond,
				BurstSize:         cfg.RateLimit.BurstSize,
				Enabled:           cfg.RateLimit.Enabled,
				CleanupInterval:   5 * time.Minute,
			},
			m,
			log.Logger,
		)
		defer rateLimiter.Stop()
		log.Info().
			Float64("requests_per_second", cfg.RateLimit.RequestsPerSecond).
			Int("burst_size", cfg.RateLimit.BurstSize).
			Msg("Rate limiting enabled")
	}

	// Initialize tracing middleware
	tracing := middleware.NewTracing(m, log.Logger)

	// Initialize auth middleware
	accessKeyStore := service.NewAccessKeyStoreAdapter(iamService)
	authConfig := auth.Config{
		Region:         cfg.Auth.Region,
		Service:        cfg.Auth.Service,
		AllowAnonymous: false,
		SkipPaths:      []string{"/health", "/healthz", "/readyz"},
	}
	authMiddleware := handler.CreateAuthMiddleware(accessKeyStore, authConfig)

	// Initialize handlers
	bucketHandler := handler.NewBucketHandler(bucketService, log.Logger)
	objectHandler := handler.NewObjectHandler(objectService, log.Logger)
	multipartHandler := handler.NewMultipartHandler(multipartService, log.Logger)

	// Initialize health checker
	healthChecker := handler.NewHealthChecker(handler.HealthCheckerConfig{
		DatabaseChecker: dbHealth,
		StorageBackend:  storageBackend,
		Logger:          log.Logger,
		CacheTTL:        5 * time.Second,
	})

	// Initialize router
	router := handler.NewRouter(handler.RouterConfig{
		BucketHandler:    bucketHandler,
		ObjectHandler:    objectHandler,
		MultipartHandler: multipartHandler,
		HealthChecker:    healthChecker,
		AuthMiddleware:   authMiddleware,
		RateLimiter:      rateLimiter,
		Tracing:          tracing,
		Metrics:          m,
		Logger:           log.Logger,
	})

	// Create HTTP server
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      router.Handler(),
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	// Start metrics server if enabled
	var metricsServer *http.Server
	if cfg.Metrics.Enabled {
		metricsMux := http.NewServeMux()
		metricsMux.Handle(cfg.Metrics.Path, metrics.Handler())
		metricsServer = &http.Server{
			Addr:    fmt.Sprintf(":%d", cfg.Metrics.Port),
			Handler: metricsMux,
		}
		go func() {
			log.Info().
				Int("port", cfg.Metrics.Port).
				Str("path", cfg.Metrics.Path).
				Msg("Metrics server listening")
			if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Error().Err(err).Msg("Metrics server failed")
			}
		}()
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

	// Shutdown metrics server first
	if metricsServer != nil {
		if err := metricsServer.Shutdown(shutdownCtx); err != nil {
			log.Error().Err(err).Msg("Metrics server shutdown error")
		}
	}

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("Server shutdown error")
	}

	log.Info().Msg("Server stopped")
}

// initStorageBackend initializes the storage backend based on configuration.
func initStorageBackend(cfg *config.Config, logger zerolog.Logger) (storage.Backend, error) {
	// For now, we only support filesystem backend
	// TODO: Add support for other backends (S3, Azure Blob, etc.)
	return filesystem.NewStorage(filesystem.Config{
		DataDir: cfg.Storage.DataDir,
		TempDir: cfg.Storage.TempDir,
	}, logger)
}
