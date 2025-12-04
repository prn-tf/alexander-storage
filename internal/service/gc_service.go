// Package service provides business logic services for Alexander Storage.
package service

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/prn-tf/alexander-storage/internal/lock"
	"github.com/prn-tf/alexander-storage/internal/metrics"
	"github.com/prn-tf/alexander-storage/internal/repository"
	"github.com/prn-tf/alexander-storage/internal/storage"
)

// GarbageCollector handles cleanup of orphan blobs.
type GarbageCollector struct {
	blobRepo repository.BlobRepository
	storage  storage.Backend
	locker   lock.Locker
	metrics  *metrics.Metrics
	logger   zerolog.Logger
	config   GCConfig

	// Control
	mu       sync.Mutex
	running  bool
	stopChan chan struct{}
	doneChan chan struct{}
}

// GCConfig contains garbage collection configuration.
type GCConfig struct {
	// Enabled determines if GC runs automatically.
	Enabled bool

	// Interval is how often to run garbage collection.
	Interval time.Duration

	// GracePeriod is how long to wait before deleting orphan blobs.
	// This prevents race conditions during uploads.
	GracePeriod time.Duration

	// BatchSize is the maximum number of blobs to process per run.
	BatchSize int

	// DryRun logs what would be deleted without actually deleting.
	DryRun bool
}

// DefaultGCConfig returns sensible defaults.
func DefaultGCConfig() GCConfig {
	return GCConfig{
		Enabled:     true,
		Interval:    1 * time.Hour,
		GracePeriod: 24 * time.Hour,
		BatchSize:   1000,
		DryRun:      false,
	}
}

// NewGarbageCollector creates a new garbage collector.
func NewGarbageCollector(
	blobRepo repository.BlobRepository,
	storage storage.Backend,
	locker lock.Locker,
	m *metrics.Metrics,
	logger zerolog.Logger,
	config GCConfig,
) *GarbageCollector {
	return &GarbageCollector{
		blobRepo: blobRepo,
		storage:  storage,
		locker:   locker,
		metrics:  m,
		logger:   logger.With().Str("service", "gc").Logger(),
		config:   config,
		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
	}
}

// Start begins the garbage collection scheduler.
func (gc *GarbageCollector) Start() {
	gc.mu.Lock()
	if gc.running {
		gc.mu.Unlock()
		return
	}
	gc.running = true
	gc.mu.Unlock()

	gc.logger.Info().
		Dur("interval", gc.config.Interval).
		Dur("grace_period", gc.config.GracePeriod).
		Int("batch_size", gc.config.BatchSize).
		Bool("dry_run", gc.config.DryRun).
		Msg("Starting garbage collector")

	go gc.runLoop()
}

// Stop stops the garbage collection scheduler.
func (gc *GarbageCollector) Stop() {
	gc.mu.Lock()
	if !gc.running {
		gc.mu.Unlock()
		return
	}
	gc.running = false
	gc.mu.Unlock()

	close(gc.stopChan)
	<-gc.doneChan

	gc.logger.Info().Msg("Garbage collector stopped")
}

// runLoop is the main garbage collection loop.
func (gc *GarbageCollector) runLoop() {
	defer close(gc.doneChan)

	// Run immediately on start
	gc.runOnce()

	ticker := time.NewTicker(gc.config.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			gc.runOnce()
		case <-gc.stopChan:
			return
		}
	}
}

// RunOnce executes a single garbage collection run.
// This can be called manually or by the scheduler.
func (gc *GarbageCollector) RunOnce(ctx context.Context) GCResult {
	return gc.runWithContext(ctx)
}

// runOnce is called by the scheduler loop.
func (gc *GarbageCollector) runOnce() {
	ctx := context.Background()
	gc.runWithContext(ctx)
}

// GCResult contains the result of a garbage collection run.
type GCResult struct {
	// BlobsDeleted is the number of blobs deleted.
	BlobsDeleted int

	// BytesFreed is the total bytes freed.
	BytesFreed int64

	// Errors is the number of errors encountered.
	Errors int

	// Duration is how long the run took.
	Duration time.Duration

	// OrphanBlobsRemaining is the approximate number of orphan blobs still pending.
	OrphanBlobsRemaining int
}

// runWithContext executes garbage collection with the given context.
func (gc *GarbageCollector) runWithContext(ctx context.Context) GCResult {
	start := time.Now()
	result := GCResult{}

	gc.logger.Debug().Msg("Starting garbage collection run")

	// Acquire distributed lock to prevent concurrent GC runs
	lockKey := lock.Keys.BlobGC()
	lockTTL := gc.config.Interval / 2 // Lock expires before next scheduled run
	if lockTTL < 5*time.Minute {
		lockTTL = 5 * time.Minute
	}

	acquired, err := gc.locker.Acquire(ctx, lockKey, lockTTL)
	if err != nil {
		gc.logger.Error().Err(err).Msg("Failed to acquire GC lock")
		result.Errors++
		result.Duration = time.Since(start)
		return result
	}
	if !acquired {
		gc.logger.Debug().Msg("GC lock held by another process, skipping run")
		result.Duration = time.Since(start)
		return result
	}
	defer func() {
		if _, err := gc.locker.Release(ctx, lockKey); err != nil {
			gc.logger.Error().Err(err).Msg("Failed to release GC lock")
		}
	}()

	// Get orphan blobs
	orphans, err := gc.blobRepo.ListOrphans(ctx, gc.config.GracePeriod, gc.config.BatchSize)
	if err != nil {
		gc.logger.Error().Err(err).Msg("Failed to list orphan blobs")
		result.Errors++
		result.Duration = time.Since(start)
		return result
	}

	if len(orphans) == 0 {
		gc.logger.Debug().Msg("No orphan blobs found")
		result.Duration = time.Since(start)
		if gc.metrics != nil {
			gc.metrics.GCLastRunTime.SetToCurrentTime()
		}
		return result
	}

	gc.logger.Info().
		Int("count", len(orphans)).
		Msg("Found orphan blobs for cleanup")

	// Update metrics with orphan count
	if gc.metrics != nil {
		gc.metrics.GCOrphanBlobs.Set(float64(len(orphans)))
	}

	// Process each orphan blob
	for _, blob := range orphans {
		if gc.config.DryRun {
			gc.logger.Info().
				Str("content_hash", blob.ContentHash).
				Int64("size", blob.Size).
				Msg("[DRY RUN] Would delete orphan blob")
			result.BlobsDeleted++
			result.BytesFreed += blob.Size
			continue
		}

		// Delete from storage first
		if err := gc.storage.Delete(ctx, blob.ContentHash); err != nil {
			if !storage.IsNotFound(err) {
				gc.logger.Error().
					Err(err).
					Str("content_hash", blob.ContentHash).
					Msg("Failed to delete blob from storage")
				result.Errors++
				continue
			}
			// Blob already deleted from storage, continue to delete from DB
		}

		// Delete from database
		if err := gc.blobRepo.Delete(ctx, blob.ContentHash); err != nil {
			gc.logger.Error().
				Err(err).
				Str("content_hash", blob.ContentHash).
				Msg("Failed to delete blob from database")
			result.Errors++
			continue
		}

		gc.logger.Debug().
			Str("content_hash", blob.ContentHash).
			Int64("size", blob.Size).
			Msg("Deleted orphan blob")

		result.BlobsDeleted++
		result.BytesFreed += blob.Size
	}

	result.Duration = time.Since(start)

	// Check if there might be more orphans
	if len(orphans) == gc.config.BatchSize {
		// There might be more, check again
		remaining, _ := gc.blobRepo.ListOrphans(ctx, gc.config.GracePeriod, 1)
		result.OrphanBlobsRemaining = len(remaining)
		if len(remaining) > 0 {
			gc.logger.Info().Msg("More orphan blobs remain for next run")
		}
	}

	// Record metrics
	if gc.metrics != nil {
		gc.metrics.RecordGCRun(result.Duration.Seconds(), result.BlobsDeleted, result.BytesFreed)
		gc.metrics.GCLastRunTime.SetToCurrentTime()
		if result.OrphanBlobsRemaining == 0 && len(orphans) < gc.config.BatchSize {
			gc.metrics.GCOrphanBlobs.Set(0)
		}
	}

	gc.logger.Info().
		Int("blobs_deleted", result.BlobsDeleted).
		Int64("bytes_freed", result.BytesFreed).
		Int("errors", result.Errors).
		Dur("duration", result.Duration).
		Msg("Garbage collection run completed")

	return result
}

// CleanupExpiredMultipartUploads cleans up expired multipart uploads.
// This is called separately from blob GC.
func (gc *GarbageCollector) CleanupExpiredMultipartUploads(ctx context.Context, multipartRepo repository.MultipartUploadRepository) (int64, error) {
	deleted, err := multipartRepo.DeleteExpired(ctx)
	if err != nil {
		gc.logger.Error().Err(err).Msg("Failed to delete expired multipart uploads")
		return 0, err
	}

	if deleted > 0 {
		gc.logger.Info().
			Int64("count", deleted).
			Msg("Deleted expired multipart uploads")
	}

	return deleted, nil
}

// CleanupExpiredAccessKeys cleans up expired access keys.
func (gc *GarbageCollector) CleanupExpiredAccessKeys(ctx context.Context, accessKeyRepo repository.AccessKeyRepository) (int64, error) {
	deleted, err := accessKeyRepo.DeleteExpired(ctx)
	if err != nil {
		gc.logger.Error().Err(err).Msg("Failed to delete expired access keys")
		return 0, err
	}

	if deleted > 0 {
		gc.logger.Info().
			Int64("count", deleted).
			Msg("Deleted expired access keys")
	}

	return deleted, nil
}

// GetStats returns current GC statistics.
func (gc *GarbageCollector) GetStats(ctx context.Context) (*GCStats, error) {
	orphans, err := gc.blobRepo.ListOrphans(ctx, gc.config.GracePeriod, gc.config.BatchSize+1)
	if err != nil {
		return nil, err
	}

	var totalSize int64
	for _, blob := range orphans {
		totalSize += blob.Size
	}

	hasMore := len(orphans) > gc.config.BatchSize
	if hasMore {
		orphans = orphans[:gc.config.BatchSize]
	}

	return &GCStats{
		OrphanBlobCount: len(orphans),
		OrphanBlobSize:  totalSize,
		HasMoreOrphans:  hasMore,
		GracePeriod:     gc.config.GracePeriod,
		NextRunIn:       gc.config.Interval,
	}, nil
}

// GCStats contains garbage collection statistics.
type GCStats struct {
	OrphanBlobCount int
	OrphanBlobSize  int64
	HasMoreOrphans  bool
	GracePeriod     time.Duration
	NextRunIn       time.Duration
}
