// Package service provides business logic services for Alexander Storage.
package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/prn-tf/alexander-storage/internal/domain"
	"github.com/prn-tf/alexander-storage/internal/lock"
	"github.com/prn-tf/alexander-storage/internal/metrics"
	"github.com/prn-tf/alexander-storage/internal/repository"
)

// LifecycleService handles object lifecycle rules and expiration.
type LifecycleService struct {
	lifecycleRepo repository.LifecycleRepository
	objectRepo    repository.ObjectRepository
	bucketRepo    repository.BucketRepository
	blobRepo      repository.BlobRepository
	locker        lock.Locker
	metrics       *metrics.Metrics
	logger        zerolog.Logger
	config        LifecycleConfig

	// Scheduler control
	mu       sync.Mutex
	running  bool
	stopChan chan struct{}
	doneChan chan struct{}
}

// LifecycleConfig contains lifecycle service configuration.
type LifecycleConfig struct {
	// Enabled determines if lifecycle scheduler runs automatically.
	Enabled bool

	// Interval is how often to run lifecycle evaluation.
	Interval time.Duration

	// BatchSize is the maximum number of objects to process per bucket per run.
	BatchSize int

	// DryRun logs what would be deleted without actually deleting.
	DryRun bool
}

// DefaultLifecycleConfig returns sensible defaults.
func DefaultLifecycleConfig() LifecycleConfig {
	return LifecycleConfig{
		Enabled:   true,
		Interval:  1 * time.Hour,
		BatchSize: 1000,
		DryRun:    false,
	}
}

// NewLifecycleService creates a new lifecycle service.
func NewLifecycleService(
	lifecycleRepo repository.LifecycleRepository,
	objectRepo repository.ObjectRepository,
	bucketRepo repository.BucketRepository,
	blobRepo repository.BlobRepository,
	locker lock.Locker,
	m *metrics.Metrics,
	logger zerolog.Logger,
	config LifecycleConfig,
) *LifecycleService {
	return &LifecycleService{
		lifecycleRepo: lifecycleRepo,
		objectRepo:    objectRepo,
		bucketRepo:    bucketRepo,
		blobRepo:      blobRepo,
		locker:        locker,
		metrics:       m,
		logger:        logger.With().Str("service", "lifecycle").Logger(),
		config:        config,
		stopChan:      make(chan struct{}),
		doneChan:      make(chan struct{}),
	}
}

// CreateRuleInput contains data to create a lifecycle rule.
type CreateRuleInput struct {
	BucketName     string
	RuleID         string // User-defined rule ID
	Prefix         string
	ExpirationDays int
	Status         string // "Enabled" or "Disabled"
}

// CreateRule creates a new lifecycle rule for a bucket.
func (s *LifecycleService) CreateRule(ctx context.Context, input CreateRuleInput) (*domain.LifecycleRule, error) {
	// Get bucket
	bucket, err := s.bucketRepo.GetByName(ctx, input.BucketName)
	if err != nil {
		if err == repository.ErrNotFound {
			return nil, fmt.Errorf("bucket not found: %s", input.BucketName)
		}
		s.logger.Error().Err(err).Str("bucket", input.BucketName).Msg("failed to get bucket")
		return nil, fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	// Create rule
	rule := domain.NewLifecycleRule(bucket.ID, input.RuleID)
	rule.Prefix = input.Prefix
	expDays := input.ExpirationDays
	rule.ExpirationDays = &expDays
	if input.Status != "" {
		rule.Status = domain.LifecycleStatus(input.Status)
	}

	// Validate rule
	if err := rule.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidLifecycleRule, err)
	}

	// Check for duplicate rule ID in bucket
	existing, err := s.lifecycleRepo.ListByBucket(ctx, bucket.ID)
	if err != nil && err != repository.ErrNotFound {
		s.logger.Error().Err(err).Msg("failed to check existing rules")
		return nil, fmt.Errorf("%w: %v", ErrInternalError, err)
	}
	for _, r := range existing {
		if r.RuleID == input.RuleID {
			return nil, fmt.Errorf("%w: rule ID '%s' already exists", ErrLifecycleRuleAlreadyExists, input.RuleID)
		}
	}

	if err := s.lifecycleRepo.Create(ctx, rule); err != nil {
		s.logger.Error().Err(err).Msg("failed to create lifecycle rule")
		return nil, fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	s.logger.Info().
		Str("bucket", input.BucketName).
		Str("rule_id", input.RuleID).
		Int("expiration_days", input.ExpirationDays).
		Msg("lifecycle rule created")

	return rule, nil
}

// GetRules returns all lifecycle rules for a bucket.
func (s *LifecycleService) GetRules(ctx context.Context, bucketName string) ([]*domain.LifecycleRule, error) {
	bucket, err := s.bucketRepo.GetByName(ctx, bucketName)
	if err != nil {
		if err == repository.ErrNotFound {
			return nil, fmt.Errorf("bucket not found: %s", bucketName)
		}
		return nil, fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	rules, err := s.lifecycleRepo.ListByBucket(ctx, bucket.ID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	return rules, nil
}

// UpdateRule updates an existing lifecycle rule.
func (s *LifecycleService) UpdateRule(ctx context.Context, ruleID int64, expirationDays int, status string) error {
	rule, err := s.lifecycleRepo.GetByID(ctx, ruleID)
	if err != nil {
		if err == repository.ErrNotFound {
			return ErrLifecycleRuleNotFound
		}
		return fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	if expirationDays > 0 {
		rule.ExpirationDays = &expirationDays
	}
	if status != "" {
		rule.Status = domain.LifecycleStatus(status)
	}

	if err := rule.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidLifecycleRule, err)
	}

	rule.UpdatedAt = time.Now().UTC()

	if err := s.lifecycleRepo.Update(ctx, rule); err != nil {
		return fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	s.logger.Info().
		Int64("rule_id", ruleID).
		Int("expiration_days", *rule.ExpirationDays).
		Str("status", string(rule.Status)).
		Msg("lifecycle rule updated")

	return nil
}

// DeleteRule deletes a lifecycle rule.
func (s *LifecycleService) DeleteRule(ctx context.Context, ruleID int64) error {
	if err := s.lifecycleRepo.Delete(ctx, ruleID); err != nil {
		if err == repository.ErrNotFound {
			return ErrLifecycleRuleNotFound
		}
		return fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	s.logger.Info().Int64("rule_id", ruleID).Msg("lifecycle rule deleted")

	return nil
}

// DeleteRuleByName deletes a lifecycle rule by bucket name and rule ID string.
func (s *LifecycleService) DeleteRuleByName(ctx context.Context, bucketName, ruleIDStr string) error {
	// Get bucket
	bucket, err := s.bucketRepo.GetByName(ctx, bucketName)
	if err != nil {
		if err == repository.ErrNotFound {
			return fmt.Errorf("bucket not found: %s", bucketName)
		}
		return fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	// Find rule by ruleID string
	rules, err := s.lifecycleRepo.ListByBucket(ctx, bucket.ID)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	for _, rule := range rules {
		if rule.RuleID == ruleIDStr {
			return s.DeleteRule(ctx, rule.ID)
		}
	}

	return ErrLifecycleRuleNotFound
}

// Start begins the lifecycle scheduler.
func (s *LifecycleService) Start() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()

	s.logger.Info().
		Dur("interval", s.config.Interval).
		Int("batch_size", s.config.BatchSize).
		Bool("dry_run", s.config.DryRun).
		Msg("Starting lifecycle scheduler")

	go s.runLoop()
}

// Stop stops the lifecycle scheduler.
func (s *LifecycleService) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	s.mu.Unlock()

	close(s.stopChan)
	<-s.doneChan

	s.logger.Info().Msg("Lifecycle scheduler stopped")
}

// runLoop is the main lifecycle evaluation loop.
func (s *LifecycleService) runLoop() {
	defer close(s.doneChan)

	// Run immediately on start
	s.runOnce()

	ticker := time.NewTicker(s.config.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.runOnce()
		case <-s.stopChan:
			return
		}
	}
}

// LifecycleResult contains the result of a lifecycle evaluation run.
type LifecycleResult struct {
	ObjectsExpired   int
	BytesFreed       int64
	RulesEvaluated   int
	BucketsProcessed int
	Errors           int
	Duration         time.Duration
}

// RunOnce executes a single lifecycle evaluation run.
func (s *LifecycleService) RunOnce(ctx context.Context) LifecycleResult {
	return s.runWithContext(ctx)
}

// runOnce is called by the scheduler loop.
func (s *LifecycleService) runOnce() {
	ctx := context.Background()
	s.runWithContext(ctx)
}

// runWithContext executes lifecycle evaluation with the given context.
func (s *LifecycleService) runWithContext(ctx context.Context) LifecycleResult {
	start := time.Now()
	result := LifecycleResult{}

	s.logger.Debug().Msg("Starting lifecycle evaluation run")

	// Acquire distributed lock
	lockKey := "lifecycle:evaluation"
	lockTTL := s.config.Interval / 2
	if lockTTL < 5*time.Minute {
		lockTTL = 5 * time.Minute
	}

	acquired, err := s.locker.Acquire(ctx, lockKey, lockTTL)
	if err != nil {
		s.logger.Error().Err(err).Msg("Failed to acquire lifecycle lock")
		result.Errors++
		result.Duration = time.Since(start)
		return result
	}
	if !acquired {
		s.logger.Debug().Msg("Lifecycle lock held by another process, skipping run")
		result.Duration = time.Since(start)
		return result
	}
	defer func() {
		if _, err := s.locker.Release(ctx, lockKey); err != nil {
			s.logger.Error().Err(err).Msg("Failed to release lifecycle lock")
		}
	}()

	// Get all enabled rules
	rules, err := s.lifecycleRepo.ListAllEnabled(ctx)
	if err != nil {
		s.logger.Error().Err(err).Msg("Failed to list enabled rules")
		result.Errors++
		result.Duration = time.Since(start)
		return result
	}

	if len(rules) == 0 {
		s.logger.Debug().Msg("No enabled lifecycle rules found")
		result.Duration = time.Since(start)
		return result
	}

	// Group rules by bucket for efficient processing
	rulesByBucket := make(map[int64][]*domain.LifecycleRule)
	for _, rule := range rules {
		rulesByBucket[rule.BucketID] = append(rulesByBucket[rule.BucketID], rule)
	}

	result.RulesEvaluated = len(rules)
	result.BucketsProcessed = len(rulesByBucket)

	// Process each bucket
	for bucketID, bucketRules := range rulesByBucket {
		expired, bytes, errs := s.processBucketRules(ctx, bucketID, bucketRules)
		result.ObjectsExpired += expired
		result.BytesFreed += bytes
		result.Errors += errs
	}

	result.Duration = time.Since(start)

	if result.ObjectsExpired > 0 || result.Errors > 0 {
		s.logger.Info().
			Int("objects_expired", result.ObjectsExpired).
			Int64("bytes_freed", result.BytesFreed).
			Int("rules_evaluated", result.RulesEvaluated).
			Int("buckets_processed", result.BucketsProcessed).
			Int("errors", result.Errors).
			Dur("duration", result.Duration).
			Msg("Lifecycle evaluation completed")
	} else {
		s.logger.Debug().
			Int("rules_evaluated", result.RulesEvaluated).
			Dur("duration", result.Duration).
			Msg("Lifecycle evaluation completed, no objects expired")
	}

	return result
}

// processBucketRules evaluates lifecycle rules for a single bucket.
func (s *LifecycleService) processBucketRules(ctx context.Context, bucketID int64, rules []*domain.LifecycleRule) (expired int, bytesFreed int64, errors int) {
	// Get bucket info for logging
	bucket, err := s.bucketRepo.GetByID(ctx, bucketID)
	if err != nil {
		s.logger.Error().Err(err).Int64("bucket_id", bucketID).Msg("Failed to get bucket")
		return 0, 0, 1
	}

	for _, rule := range rules {
		e, b, errs := s.evaluateRule(ctx, bucket, rule)
		expired += e
		bytesFreed += b
		errors += errs
	}

	return expired, bytesFreed, errors
}

// evaluateRule evaluates a single lifecycle rule against objects in a bucket.
func (s *LifecycleService) evaluateRule(ctx context.Context, bucket *domain.Bucket, rule *domain.LifecycleRule) (expired int, bytesFreed int64, errors int) {
	// Skip rules without expiration
	if !rule.HasExpiration() {
		return 0, 0, 0
	}

	// Calculate expiration cutoff time
	cutoff := time.Now().UTC().AddDate(0, 0, -*rule.ExpirationDays)

	s.logger.Debug().
		Str("bucket", bucket.Name).
		Str("rule_id", rule.RuleID).
		Str("prefix", rule.Prefix).
		Int("expiration_days", *rule.ExpirationDays).
		Time("cutoff", cutoff).
		Msg("Evaluating lifecycle rule")

	// List objects matching prefix that are older than cutoff
	// For versioned buckets, we only expire the latest version
	objects, err := s.objectRepo.ListExpiredObjects(ctx, bucket.ID, rule.Prefix, cutoff, s.config.BatchSize)
	if err != nil {
		s.logger.Error().Err(err).Str("bucket", bucket.Name).Str("rule_id", rule.RuleID).Msg("Failed to list expired objects")
		return 0, 0, 1
	}

	for _, obj := range objects {
		if s.config.DryRun {
			s.logger.Info().
				Str("bucket", bucket.Name).
				Str("key", obj.Key).
				Time("created_at", obj.CreatedAt).
				Msg("[DRY RUN] Would expire object")
			expired++
			bytesFreed += obj.Size
			continue
		}

		// Delete the object
		if err := s.expireObject(ctx, bucket, obj); err != nil {
			s.logger.Error().Err(err).
				Str("bucket", bucket.Name).
				Str("key", obj.Key).
				Msg("Failed to expire object")
			errors++
			continue
		}

		expired++
		bytesFreed += obj.Size

		s.logger.Debug().
			Str("bucket", bucket.Name).
			Str("key", obj.Key).
			Int64("size", obj.Size).
			Msg("Object expired")
	}

	return expired, bytesFreed, errors
}

// expireObject deletes an object due to lifecycle expiration.
func (s *LifecycleService) expireObject(ctx context.Context, bucket *domain.Bucket, obj *domain.Object) error {
	// For versioned buckets, insert a delete marker
	// For non-versioned buckets, delete the object directly

	if bucket.Versioning == domain.VersioningEnabled {
		// Create delete marker
		deleteMarker := domain.NewDeleteMarker(bucket.ID, obj.Key)
		if err := s.objectRepo.Create(ctx, deleteMarker); err != nil {
			return fmt.Errorf("failed to create delete marker: %w", err)
		}

		// Mark previous version as not latest
		if err := s.objectRepo.MarkNotLatest(ctx, bucket.ID, obj.Key); err != nil {
			return fmt.Errorf("failed to mark not latest: %w", err)
		}
	} else {
		// Decrement blob reference count
		if obj.ContentHash != nil {
			if _, err := s.blobRepo.DecrementRef(ctx, *obj.ContentHash); err != nil {
				s.logger.Warn().Err(err).Str("content_hash", *obj.ContentHash).Msg("Failed to decrement blob ref")
			}
		}

		// Delete object
		if err := s.objectRepo.Delete(ctx, obj.ID); err != nil {
			return fmt.Errorf("failed to delete object: %w", err)
		}
	}

	return nil
}
