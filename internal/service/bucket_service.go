// Package service provides business logic services for Alexander Storage.
package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/rs/zerolog"

	"github.com/prn-tf/alexander-storage/internal/auth"
	"github.com/prn-tf/alexander-storage/internal/domain"
	"github.com/prn-tf/alexander-storage/internal/repository"
)

// BucketService handles bucket operations.
type BucketService struct {
	bucketRepo repository.BucketRepository
	logger     zerolog.Logger
}

// NewBucketService creates a new BucketService.
func NewBucketService(
	bucketRepo repository.BucketRepository,
	logger zerolog.Logger,
) *BucketService {
	return &BucketService{
		bucketRepo: bucketRepo,
		logger:     logger.With().Str("service", "bucket").Logger(),
	}
}

// =============================================================================
// Input/Output Structs
// =============================================================================

// CreateBucketInput contains the data needed to create a bucket.
type CreateBucketInput struct {
	OwnerID int64
	Name    string
	Region  string
}

// CreateBucketOutput contains the result of creating a bucket.
type CreateBucketOutput struct {
	Bucket *domain.Bucket
}

// GetBucketInput contains the data needed to get a bucket.
type GetBucketInput struct {
	Name    string
	OwnerID int64 // For ownership verification
}

// GetBucketOutput contains the result of getting a bucket.
type GetBucketOutput struct {
	Bucket *domain.Bucket
}

// ListBucketsInput contains the data needed to list buckets.
type ListBucketsInput struct {
	OwnerID int64
}

// ListBucketsOutput contains the result of listing buckets.
type ListBucketsOutput struct {
	Buckets []*domain.Bucket
}

// DeleteBucketInput contains the data needed to delete a bucket.
type DeleteBucketInput struct {
	Name    string
	OwnerID int64 // For ownership verification
}

// HeadBucketInput contains the data needed to check bucket existence.
type HeadBucketInput struct {
	Name    string
	OwnerID int64 // For ownership verification
}

// HeadBucketOutput contains the result of checking bucket existence.
type HeadBucketOutput struct {
	Exists bool
	Region string
}

// GetBucketVersioningInput contains the data needed to get bucket versioning.
type GetBucketVersioningInput struct {
	Name    string
	OwnerID int64
}

// GetBucketVersioningOutput contains the versioning status.
type GetBucketVersioningOutput struct {
	Status domain.VersioningStatus
}

// PutBucketVersioningInput contains the data needed to set bucket versioning.
type PutBucketVersioningInput struct {
	Name    string
	OwnerID int64
	Status  domain.VersioningStatus
}

// =============================================================================
// Service Methods
// =============================================================================

// CreateBucket creates a new bucket.
func (s *BucketService) CreateBucket(ctx context.Context, input CreateBucketInput) (*CreateBucketOutput, error) {
	// Validate bucket name
	if err := domain.ValidateBucketName(input.Name); err != nil {
		return nil, err
	}

	// Check if bucket already exists
	exists, err := s.bucketRepo.ExistsByName(ctx, input.Name)
	if err != nil {
		s.logger.Error().Err(err).Str("bucket", input.Name).Msg("failed to check bucket existence")
		return nil, fmt.Errorf("%w: %v", ErrInternalError, err)
	}
	if exists {
		return nil, domain.ErrBucketAlreadyExists
	}

	// Set default region if not specified
	region := input.Region
	if region == "" {
		region = "us-east-1"
	}

	// Create bucket
	bucket := &domain.Bucket{
		OwnerID:    input.OwnerID,
		Name:       input.Name,
		Region:     region,
		Versioning: domain.VersioningDisabled,
		ObjectLock: false,
		CreatedAt:  time.Now().UTC(),
	}

	if err := s.bucketRepo.Create(ctx, bucket); err != nil {
		if errors.Is(err, domain.ErrBucketAlreadyExists) {
			return nil, domain.ErrBucketAlreadyExists
		}
		s.logger.Error().Err(err).Str("bucket", input.Name).Msg("failed to create bucket")
		return nil, fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	s.logger.Info().
		Int64("owner_id", input.OwnerID).
		Str("bucket", input.Name).
		Str("region", region).
		Msg("bucket created")

	return &CreateBucketOutput{
		Bucket: bucket,
	}, nil
}

// GetBucket retrieves a bucket by name.
func (s *BucketService) GetBucket(ctx context.Context, input GetBucketInput) (*GetBucketOutput, error) {
	bucket, err := s.bucketRepo.GetByName(ctx, input.Name)
	if err != nil {
		if errors.Is(err, domain.ErrBucketNotFound) {
			return nil, domain.ErrBucketNotFound
		}
		s.logger.Error().Err(err).Str("bucket", input.Name).Msg("failed to get bucket")
		return nil, fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	// Verify ownership if OwnerID is specified
	if input.OwnerID > 0 && bucket.OwnerID != input.OwnerID {
		return nil, ErrBucketAccessDenied
	}

	return &GetBucketOutput{
		Bucket: bucket,
	}, nil
}

// ListBuckets returns all buckets for a user.
func (s *BucketService) ListBuckets(ctx context.Context, input ListBucketsInput) (*ListBucketsOutput, error) {
	buckets, err := s.bucketRepo.List(ctx, input.OwnerID)
	if err != nil {
		s.logger.Error().Err(err).Int64("owner_id", input.OwnerID).Msg("failed to list buckets")
		return nil, fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	return &ListBucketsOutput{
		Buckets: buckets,
	}, nil
}

// DeleteBucket deletes a bucket.
func (s *BucketService) DeleteBucket(ctx context.Context, input DeleteBucketInput) error {
	// Get bucket to verify it exists and check ownership
	bucket, err := s.bucketRepo.GetByName(ctx, input.Name)
	if err != nil {
		if errors.Is(err, domain.ErrBucketNotFound) {
			return domain.ErrBucketNotFound
		}
		s.logger.Error().Err(err).Str("bucket", input.Name).Msg("failed to get bucket")
		return fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	// Verify ownership
	if input.OwnerID > 0 && bucket.OwnerID != input.OwnerID {
		return ErrBucketAccessDenied
	}

	// Check if bucket is empty
	isEmpty, err := s.bucketRepo.IsEmpty(ctx, bucket.ID)
	if err != nil {
		s.logger.Error().Err(err).Str("bucket", input.Name).Msg("failed to check if bucket is empty")
		return fmt.Errorf("%w: %v", ErrInternalError, err)
	}
	if !isEmpty {
		return domain.ErrBucketNotEmpty
	}

	// Delete bucket
	if err := s.bucketRepo.Delete(ctx, bucket.ID); err != nil {
		s.logger.Error().Err(err).Str("bucket", input.Name).Msg("failed to delete bucket")
		return fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	s.logger.Info().
		Int64("owner_id", input.OwnerID).
		Str("bucket", input.Name).
		Msg("bucket deleted")

	return nil
}

// HeadBucket checks if a bucket exists and returns its region.
func (s *BucketService) HeadBucket(ctx context.Context, input HeadBucketInput) (*HeadBucketOutput, error) {
	bucket, err := s.bucketRepo.GetByName(ctx, input.Name)
	if err != nil {
		if errors.Is(err, domain.ErrBucketNotFound) {
			return &HeadBucketOutput{
				Exists: false,
			}, nil
		}
		s.logger.Error().Err(err).Str("bucket", input.Name).Msg("failed to check bucket")
		return nil, fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	// Verify ownership if OwnerID is specified
	if input.OwnerID > 0 && bucket.OwnerID != input.OwnerID {
		return nil, ErrBucketAccessDenied
	}

	return &HeadBucketOutput{
		Exists: true,
		Region: bucket.Region,
	}, nil
}

// GetBucketVersioning retrieves the versioning status of a bucket.
func (s *BucketService) GetBucketVersioning(ctx context.Context, input GetBucketVersioningInput) (*GetBucketVersioningOutput, error) {
	bucket, err := s.bucketRepo.GetByName(ctx, input.Name)
	if err != nil {
		if errors.Is(err, domain.ErrBucketNotFound) {
			return nil, domain.ErrBucketNotFound
		}
		s.logger.Error().Err(err).Str("bucket", input.Name).Msg("failed to get bucket")
		return nil, fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	// Verify ownership
	if input.OwnerID > 0 && bucket.OwnerID != input.OwnerID {
		return nil, ErrBucketAccessDenied
	}

	return &GetBucketVersioningOutput{
		Status: bucket.Versioning,
	}, nil
}

// PutBucketVersioning sets the versioning status of a bucket.
func (s *BucketService) PutBucketVersioning(ctx context.Context, input PutBucketVersioningInput) error {
	// Validate versioning status
	if input.Status != domain.VersioningEnabled && input.Status != domain.VersioningSuspended {
		return ErrInvalidVersioningStatus
	}

	// Get bucket to verify it exists and check ownership
	bucket, err := s.bucketRepo.GetByName(ctx, input.Name)
	if err != nil {
		if errors.Is(err, domain.ErrBucketNotFound) {
			return domain.ErrBucketNotFound
		}
		s.logger.Error().Err(err).Str("bucket", input.Name).Msg("failed to get bucket")
		return fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	// Verify ownership
	if input.OwnerID > 0 && bucket.OwnerID != input.OwnerID {
		return ErrBucketAccessDenied
	}

	// Update versioning status
	if err := s.bucketRepo.UpdateVersioning(ctx, bucket.ID, input.Status); err != nil {
		s.logger.Error().Err(err).Str("bucket", input.Name).Msg("failed to update versioning")
		return fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	s.logger.Info().
		Str("bucket", input.Name).
		Str("versioning", string(input.Status)).
		Msg("bucket versioning updated")

	return nil
}

// GetBucketACL retrieves the ACL for a bucket.
func (s *BucketService) GetBucketACL(ctx context.Context, bucketName string) (domain.BucketACL, error) {
	acl, err := s.bucketRepo.GetACLByName(ctx, bucketName)
	if err != nil {
		if errors.Is(err, domain.ErrBucketNotFound) {
			return "", nil // Return empty string for not found
		}
		return "", fmt.Errorf("%w: %v", ErrInternalError, err)
	}
	return acl, nil
}

// =============================================================================
// BucketACLAdapter
// =============================================================================

// BucketACLAdapter adapts BucketService to implement auth.BucketACLChecker interface.
type BucketACLAdapter struct {
	bucketService *BucketService
}

// NewBucketACLAdapter creates a new adapter.
func NewBucketACLAdapter(bucketService *BucketService) *BucketACLAdapter {
	return &BucketACLAdapter{bucketService: bucketService}
}

// GetBucketACL implements auth.BucketACLChecker.
func (a *BucketACLAdapter) GetBucketACL(ctx context.Context, bucketName string) (string, error) {
	acl, err := a.bucketService.GetBucketACL(ctx, bucketName)
	if err != nil {
		return "", err
	}
	return string(acl), nil
}

// Ensure BucketACLAdapter implements auth.BucketACLChecker
var _ auth.BucketACLChecker = (*BucketACLAdapter)(nil)
