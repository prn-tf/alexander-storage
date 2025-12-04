// Package postgres provides PostgreSQL database utilities.
package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"github.com/prn-tf/alexander-storage/internal/config"
)

// DB wraps a pgx connection pool with additional functionality.
type DB struct {
	Pool   *pgxpool.Pool
	logger zerolog.Logger
}

// NewDB creates a new database connection pool.
func NewDB(ctx context.Context, cfg config.DatabaseConfig, logger zerolog.Logger) (*DB, error) {
	poolConfig, err := pgxpool.ParseConfig(cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("failed to parse database config: %w", err)
	}

	// Configure pool settings
	poolConfig.MaxConns = int32(cfg.MaxOpenConns)
	poolConfig.MinConns = int32(cfg.MaxIdleConns)
	poolConfig.MaxConnLifetime = cfg.ConnMaxLifetime
	poolConfig.MaxConnIdleTime = cfg.ConnMaxIdleTime

	// Configure connection settings
	poolConfig.ConnConfig.ConnectTimeout = 10 * time.Second

	// Add query tracer for debugging (optional)
	if logger.GetLevel() <= zerolog.DebugLevel {
		poolConfig.ConnConfig.Tracer = &queryTracer{logger: logger}
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Verify connection
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	logger.Info().
		Str("host", cfg.Host).
		Int("port", cfg.Port).
		Str("database", cfg.Database).
		Int("max_conns", cfg.MaxOpenConns).
		Msg("connected to PostgreSQL")

	return &DB{
		Pool:   pool,
		logger: logger,
	}, nil
}

// Close closes the database connection pool.
func (db *DB) Close() error {
	db.Pool.Close()
	db.logger.Info().Msg("database connection pool closed")
	return nil
}

// Ping checks the database connection.
func (db *DB) Ping(ctx context.Context) error {
	return db.Pool.Ping(ctx)
}

// Health checks the database connection health.
func (db *DB) Health(ctx context.Context) error {
	return db.Pool.Ping(ctx)
}

// Stats returns connection pool statistics.
func (db *DB) Stats() *pgxpool.Stat {
	return db.Pool.Stat()
}

// BeginTx starts a new transaction with the given options.
func (db *DB) BeginTx(ctx context.Context, opts pgx.TxOptions) (pgx.Tx, error) {
	return db.Pool.BeginTx(ctx, opts)
}

// WithTx executes a function within a transaction.
// If the function returns an error, the transaction is rolled back.
// Otherwise, the transaction is committed.
func (db *DB) WithTx(ctx context.Context, opts pgx.TxOptions, fn func(tx pgx.Tx) error) error {
	tx, err := db.Pool.BeginTx(ctx, opts)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback(ctx)
			panic(p)
		}
	}()

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(ctx); rbErr != nil {
			return fmt.Errorf("tx error: %v, rollback error: %w", err, rbErr)
		}
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// queryTracer implements pgx.QueryTracer for debug logging.
type queryTracer struct {
	logger zerolog.Logger
}

type traceQueryCtxKey struct{}

type traceQueryData struct {
	sql       string
	args      []any
	startTime time.Time
}

func (t *queryTracer) TraceQueryStart(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	return context.WithValue(ctx, traceQueryCtxKey{}, &traceQueryData{
		sql:       data.SQL,
		args:      data.Args,
		startTime: time.Now(),
	})
}

func (t *queryTracer) TraceQueryEnd(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryEndData) {
	queryData, ok := ctx.Value(traceQueryCtxKey{}).(*traceQueryData)
	if !ok {
		return
	}

	duration := time.Since(queryData.startTime)

	event := t.logger.Debug().
		Str("sql", queryData.sql).
		Dur("duration", duration).
		Str("command_tag", data.CommandTag.String())

	if data.Err != nil {
		event.Err(data.Err)
	}

	event.Msg("query executed")
}

// Querier is an interface that both pgxpool.Pool and pgx.Tx implement.
// This allows repositories to work with both.
type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Ensure both Pool and Tx implement Querier
var (
	_ Querier = (*pgxpool.Pool)(nil)
	_ Querier = (pgx.Tx)(nil)
)
