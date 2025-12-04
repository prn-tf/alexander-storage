// Package service provides business logic services for Alexander Storage.
package service

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/prn-tf/alexander-storage/internal/domain"
	"github.com/prn-tf/alexander-storage/internal/repository"
	"github.com/prn-tf/alexander-storage/internal/storage"
)

// ObjectService handles object operations.
type ObjectService struct {
	objectRepo repository.ObjectRepository
	blobRepo   repository.BlobRepository
	bucketRepo repository.BucketRepository
	storage    storage.Backend
	logger     zerolog.Logger
}

// NewObjectService creates a new ObjectService.
func NewObjectService(
	objectRepo repository.ObjectRepository,
	blobRepo repository.BlobRepository,
	bucketRepo repository.BucketRepository,
	storage storage.Backend,
	logger zerolog.Logger,
) *ObjectService {
	return &ObjectService{
		objectRepo: objectRepo,
		blobRepo:   blobRepo,
		bucketRepo: bucketRepo,
		storage:    storage,
		logger:     logger.With().Str("service", "object").Logger(),
	}
}

// =============================================================================
// Input/Output Structs
// =============================================================================

// PutObjectInput contains the data needed to store an object.
type PutObjectInput struct {
	BucketName  string
	Key         string
	Body        io.Reader
	Size        int64
	ContentType string
	Metadata    map[string]string
	OwnerID     int64
}

// PutObjectOutput contains the result of storing an object.
type PutObjectOutput struct {
	ETag      string
	VersionID string
}

// GetObjectInput contains the data needed to retrieve an object.
type GetObjectInput struct {
	BucketName string
	Key        string
	VersionID  string // Optional
	OwnerID    int64
	Range      *ByteRange // Optional
}

// ByteRange represents a byte range for partial content requests.
type ByteRange struct {
	Start int64
	End   int64
}

// GetObjectOutput contains the result of retrieving an object.
type GetObjectOutput struct {
	Body          io.ReadCloser
	ContentLength int64
	ContentType   string
	ETag          string
	LastModified  time.Time
	VersionID     string
	Metadata      map[string]string
	ContentRange  string // For range requests
}

// HeadObjectInput contains the data needed to get object metadata.
type HeadObjectInput struct {
	BucketName string
	Key        string
	VersionID  string // Optional
	OwnerID    int64
}

// HeadObjectOutput contains object metadata.
type HeadObjectOutput struct {
	ContentLength int64
	ContentType   string
	ETag          string
	LastModified  time.Time
	VersionID     string
	Metadata      map[string]string
	StorageClass  domain.StorageClass
}

// DeleteObjectInput contains the data needed to delete an object.
type DeleteObjectInput struct {
	BucketName string
	Key        string
	VersionID  string // Optional - if provided, deletes specific version
	OwnerID    int64
}

// DeleteObjectOutput contains the result of deleting an object.
type DeleteObjectOutput struct {
	DeleteMarker          bool
	VersionID             string
	DeleteMarkerVersionID string
}

// ListObjectsInput contains the data needed to list objects.
type ListObjectsInput struct {
	BucketName        string
	Prefix            string
	Delimiter         string
	MaxKeys           int
	Marker            string // v1
	StartAfter        string // v2
	ContinuationToken string // v2
	OwnerID           int64
}

// ListObjectsOutput contains the result of listing objects.
type ListObjectsOutput struct {
	Name                  string
	Prefix                string
	Delimiter             string
	MaxKeys               int
	IsTruncated           bool
	Contents              []ObjectInfo
	CommonPrefixes        []string
	NextMarker            string // v1
	NextContinuationToken string // v2
	KeyCount              int
}

// ObjectInfo represents an object in list output.
type ObjectInfo struct {
	Key          string
	LastModified time.Time
	ETag         string
	Size         int64
	StorageClass domain.StorageClass
}

// CopyObjectInput contains the data needed to copy an object.
type CopyObjectInput struct {
	SourceBucket      string
	SourceKey         string
	SourceVersionID   string // Optional
	DestBucket        string
	DestKey           string
	ContentType       string            // Optional - override content type
	Metadata          map[string]string // Optional - new metadata
	MetadataDirective string            // COPY or REPLACE
	OwnerID           int64
}

// CopyObjectOutput contains the result of copying an object.
type CopyObjectOutput struct {
	ETag         string
	LastModified time.Time
	VersionID    string
}

// =============================================================================
// Service Methods
// =============================================================================

// PutObject stores an object in the specified bucket.
func (s *ObjectService) PutObject(ctx context.Context, input PutObjectInput) (*PutObjectOutput, error) {
	// Validate key
	if err := validateObjectKey(input.Key); err != nil {
		return nil, err
	}

	// Get bucket
	bucket, err := s.bucketRepo.GetByName(ctx, input.BucketName)
	if err != nil {
		if errors.Is(err, domain.ErrBucketNotFound) {
			return nil, domain.ErrBucketNotFound
		}
		s.logger.Error().Err(err).Str("bucket", input.BucketName).Msg("failed to get bucket")
		return nil, fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	// Check ownership
	if input.OwnerID > 0 && bucket.OwnerID != input.OwnerID {
		return nil, ErrBucketAccessDenied
	}

	// Store content in CAS storage
	contentHash, err := s.storage.Store(ctx, input.Body, input.Size)
	if err != nil {
		s.logger.Error().Err(err).Str("key", input.Key).Msg("failed to store content")
		return nil, fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	// Get storage path for blob
	storagePath := s.storage.GetPath(contentHash)

	// Upsert blob metadata (handles deduplication via ref_count)
	_, err = s.blobRepo.UpsertWithRefIncrement(ctx, contentHash, input.Size, storagePath)
	if err != nil {
		s.logger.Error().Err(err).Str("content_hash", contentHash).Msg("failed to upsert blob")
		return nil, fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	// Calculate ETag (MD5 of content hash for simplicity, or we could stream MD5)
	etag := calculateETag(contentHash)

	// Set default content type
	contentType := input.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	// For non-versioned buckets, mark existing object as not latest
	if bucket.Versioning != domain.VersioningEnabled {
		// Get existing object to decrement its blob ref_count
		existingObj, err := s.objectRepo.GetByKey(ctx, bucket.ID, input.Key)
		if err == nil && existingObj.ContentHash != nil {
			// Decrement ref count for old blob
			_, _ = s.blobRepo.DecrementRef(ctx, *existingObj.ContentHash)
		}
		// Mark existing as not latest (or delete for non-versioned)
		_ = s.objectRepo.MarkNotLatest(ctx, bucket.ID, input.Key)
	}

	// Create new object
	obj := domain.NewObject(bucket.ID, input.Key, contentHash, contentType, etag, input.Size)
	if input.Metadata != nil {
		obj.Metadata = input.Metadata
	}

	if err := s.objectRepo.Create(ctx, obj); err != nil {
		s.logger.Error().Err(err).Str("key", input.Key).Msg("failed to create object")
		return nil, fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	s.logger.Info().
		Str("bucket", input.BucketName).
		Str("key", input.Key).
		Int64("size", input.Size).
		Str("etag", etag).
		Msg("object stored")

	return &PutObjectOutput{
		ETag:      etag,
		VersionID: obj.GetVersionIDString(),
	}, nil
}

// GetObject retrieves an object from the specified bucket.
func (s *ObjectService) GetObject(ctx context.Context, input GetObjectInput) (*GetObjectOutput, error) {
	// Get bucket
	bucket, err := s.bucketRepo.GetByName(ctx, input.BucketName)
	if err != nil {
		if errors.Is(err, domain.ErrBucketNotFound) {
			return nil, domain.ErrBucketNotFound
		}
		return nil, fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	// Check ownership
	if input.OwnerID > 0 && bucket.OwnerID != input.OwnerID {
		return nil, ErrBucketAccessDenied
	}

	// Get object
	var obj *domain.Object
	if input.VersionID != "" && input.VersionID != "null" {
		versionUUID, err := uuid.Parse(input.VersionID)
		if err != nil {
			return nil, domain.ErrInvalidVersionID
		}
		obj, err = s.objectRepo.GetByKeyAndVersion(ctx, bucket.ID, input.Key, versionUUID)
	} else {
		obj, err = s.objectRepo.GetByKey(ctx, bucket.ID, input.Key)
	}

	if err != nil {
		if errors.Is(err, domain.ErrObjectNotFound) {
			return nil, domain.ErrObjectNotFound
		}
		return nil, fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	// Check if it's a delete marker
	if obj.IsDeleteMarker {
		return nil, domain.ErrObjectDeleted
	}

	if obj.ContentHash == nil {
		return nil, domain.ErrObjectNotFound
	}

	// Retrieve content from storage
	var reader io.ReadCloser
	var contentLength int64
	var contentRange string

	if input.Range != nil {
		// Check if storage supports range reads
		rangeReader, ok := s.storage.(RangeReader)
		if !ok {
			return nil, fmt.Errorf("storage backend does not support range requests")
		}
		// Range request
		length := input.Range.End - input.Range.Start + 1
		reader, err = rangeReader.RetrieveRange(ctx, *obj.ContentHash, input.Range.Start, length)
		contentLength = length
		contentRange = fmt.Sprintf("bytes %d-%d/%d", input.Range.Start, input.Range.End, obj.Size)
	} else {
		reader, err = s.storage.Retrieve(ctx, *obj.ContentHash)
		contentLength = obj.Size
	}

	if err != nil {
		if errors.Is(err, storage.ErrBlobNotFound) {
			return nil, domain.ErrObjectNotFound
		}
		return nil, fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	return &GetObjectOutput{
		Body:          reader,
		ContentLength: contentLength,
		ContentType:   obj.ContentType,
		ETag:          obj.ETag,
		LastModified:  obj.CreatedAt,
		VersionID:     obj.GetVersionIDString(),
		Metadata:      obj.Metadata,
		ContentRange:  contentRange,
	}, nil
}

// HeadObject retrieves object metadata without the body.
func (s *ObjectService) HeadObject(ctx context.Context, input HeadObjectInput) (*HeadObjectOutput, error) {
	// Get bucket
	bucket, err := s.bucketRepo.GetByName(ctx, input.BucketName)
	if err != nil {
		if errors.Is(err, domain.ErrBucketNotFound) {
			return nil, domain.ErrBucketNotFound
		}
		return nil, fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	// Check ownership
	if input.OwnerID > 0 && bucket.OwnerID != input.OwnerID {
		return nil, ErrBucketAccessDenied
	}

	// Get object
	var obj *domain.Object
	if input.VersionID != "" && input.VersionID != "null" {
		versionUUID, err := uuid.Parse(input.VersionID)
		if err != nil {
			return nil, domain.ErrInvalidVersionID
		}
		obj, err = s.objectRepo.GetByKeyAndVersion(ctx, bucket.ID, input.Key, versionUUID)
	} else {
		obj, err = s.objectRepo.GetByKey(ctx, bucket.ID, input.Key)
	}

	if err != nil {
		if errors.Is(err, domain.ErrObjectNotFound) {
			return nil, domain.ErrObjectNotFound
		}
		return nil, fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	// Check if it's a delete marker
	if obj.IsDeleteMarker {
		return nil, domain.ErrObjectDeleted
	}

	return &HeadObjectOutput{
		ContentLength: obj.Size,
		ContentType:   obj.ContentType,
		ETag:          obj.ETag,
		LastModified:  obj.CreatedAt,
		VersionID:     obj.GetVersionIDString(),
		Metadata:      obj.Metadata,
		StorageClass:  obj.StorageClass,
	}, nil
}

// DeleteObject deletes an object or creates a delete marker.
func (s *ObjectService) DeleteObject(ctx context.Context, input DeleteObjectInput) (*DeleteObjectOutput, error) {
	// Get bucket
	bucket, err := s.bucketRepo.GetByName(ctx, input.BucketName)
	if err != nil {
		if errors.Is(err, domain.ErrBucketNotFound) {
			return nil, domain.ErrBucketNotFound
		}
		return nil, fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	// Check ownership
	if input.OwnerID > 0 && bucket.OwnerID != input.OwnerID {
		return nil, ErrBucketAccessDenied
	}

	// If versioning is enabled and no version specified, create delete marker
	if bucket.IsVersioningEnabled() && input.VersionID == "" {
		deleteMarker := domain.NewDeleteMarker(bucket.ID, input.Key)

		// Mark current version as not latest
		_ = s.objectRepo.MarkNotLatest(ctx, bucket.ID, input.Key)

		if err := s.objectRepo.Create(ctx, deleteMarker); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInternalError, err)
		}

		s.logger.Info().
			Str("bucket", input.BucketName).
			Str("key", input.Key).
			Str("version_id", deleteMarker.GetVersionIDString()).
			Msg("delete marker created")

		return &DeleteObjectOutput{
			DeleteMarker:          true,
			DeleteMarkerVersionID: deleteMarker.GetVersionIDString(),
		}, nil
	}

	// Delete specific version or non-versioned object
	var obj *domain.Object
	if input.VersionID != "" && input.VersionID != "null" {
		versionUUID, err := uuid.Parse(input.VersionID)
		if err != nil {
			return nil, domain.ErrInvalidVersionID
		}
		obj, err = s.objectRepo.GetByKeyAndVersion(ctx, bucket.ID, input.Key, versionUUID)
	} else {
		obj, err = s.objectRepo.GetByKey(ctx, bucket.ID, input.Key)
	}

	if err != nil {
		if errors.Is(err, domain.ErrObjectNotFound) {
			// S3 returns success even if object doesn't exist
			return &DeleteObjectOutput{}, nil
		}
		return nil, fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	// Decrement blob ref count if object has content
	if obj.ContentHash != nil {
		if _, err := s.blobRepo.DecrementRef(ctx, *obj.ContentHash); err != nil {
			s.logger.Error().Err(err).Str("content_hash", *obj.ContentHash).Msg("failed to decrement ref count")
		}
	}

	// Delete the object record
	if err := s.objectRepo.Delete(ctx, obj.ID); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	s.logger.Info().
		Str("bucket", input.BucketName).
		Str("key", input.Key).
		Msg("object deleted")

	return &DeleteObjectOutput{
		DeleteMarker: obj.IsDeleteMarker,
		VersionID:    obj.GetVersionIDString(),
	}, nil
}

// ListObjects lists objects in a bucket (v1 and v2 compatible).
func (s *ObjectService) ListObjects(ctx context.Context, input ListObjectsInput) (*ListObjectsOutput, error) {
	// Get bucket
	bucket, err := s.bucketRepo.GetByName(ctx, input.BucketName)
	if err != nil {
		if errors.Is(err, domain.ErrBucketNotFound) {
			return nil, domain.ErrBucketNotFound
		}
		return nil, fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	// Check ownership
	if input.OwnerID > 0 && bucket.OwnerID != input.OwnerID {
		return nil, ErrBucketAccessDenied
	}

	// Set defaults
	maxKeys := input.MaxKeys
	if maxKeys <= 0 {
		maxKeys = 1000
	}
	if maxKeys > 1000 {
		maxKeys = 1000
	}

	// Determine start key
	startAfter := input.StartAfter
	if startAfter == "" {
		startAfter = input.Marker
	}
	if startAfter == "" && input.ContinuationToken != "" {
		// Decode continuation token (simple base64 of key)
		startAfter = decodeContinuationToken(input.ContinuationToken)
	}

	// List objects from repository
	result, err := s.objectRepo.List(ctx, bucket.ID, repository.ObjectListOptions{
		Prefix:     input.Prefix,
		Delimiter:  input.Delimiter,
		StartAfter: startAfter,
		MaxKeys:    maxKeys,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	// Convert to output format
	contents := make([]ObjectInfo, len(result.Objects))
	for i, obj := range result.Objects {
		contents[i] = ObjectInfo{
			Key:          obj.Key,
			LastModified: obj.LastModified,
			ETag:         obj.ETag,
			Size:         obj.Size,
			StorageClass: obj.StorageClass,
		}
	}

	output := &ListObjectsOutput{
		Name:           input.BucketName,
		Prefix:         input.Prefix,
		Delimiter:      input.Delimiter,
		MaxKeys:        maxKeys,
		IsTruncated:    result.IsTruncated,
		Contents:       contents,
		CommonPrefixes: result.CommonPrefixes,
		KeyCount:       result.KeyCount,
	}

	if result.IsTruncated && len(contents) > 0 {
		lastKey := contents[len(contents)-1].Key
		output.NextMarker = lastKey
		output.NextContinuationToken = encodeContinuationToken(lastKey)
	}

	return output, nil
}

// CopyObject copies an object within or between buckets.
func (s *ObjectService) CopyObject(ctx context.Context, input CopyObjectInput) (*CopyObjectOutput, error) {
	// Get source bucket
	sourceBucket, err := s.bucketRepo.GetByName(ctx, input.SourceBucket)
	if err != nil {
		if errors.Is(err, domain.ErrBucketNotFound) {
			return nil, domain.ErrBucketNotFound
		}
		return nil, fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	// Check source ownership
	if input.OwnerID > 0 && sourceBucket.OwnerID != input.OwnerID {
		return nil, ErrBucketAccessDenied
	}

	// Get destination bucket
	destBucket, err := s.bucketRepo.GetByName(ctx, input.DestBucket)
	if err != nil {
		if errors.Is(err, domain.ErrBucketNotFound) {
			return nil, domain.ErrBucketNotFound
		}
		return nil, fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	// Check destination ownership
	if input.OwnerID > 0 && destBucket.OwnerID != input.OwnerID {
		return nil, ErrBucketAccessDenied
	}

	// Get source object
	var sourceObj *domain.Object
	if input.SourceVersionID != "" && input.SourceVersionID != "null" {
		versionUUID, err := uuid.Parse(input.SourceVersionID)
		if err != nil {
			return nil, domain.ErrInvalidVersionID
		}
		sourceObj, err = s.objectRepo.GetByKeyAndVersion(ctx, sourceBucket.ID, input.SourceKey, versionUUID)
	} else {
		sourceObj, err = s.objectRepo.GetByKey(ctx, sourceBucket.ID, input.SourceKey)
	}

	if err != nil {
		if errors.Is(err, domain.ErrObjectNotFound) {
			return nil, domain.ErrObjectNotFound
		}
		return nil, fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	if sourceObj.IsDeleteMarker || sourceObj.ContentHash == nil {
		return nil, domain.ErrObjectNotFound
	}

	// Validate destination key
	if err := validateObjectKey(input.DestKey); err != nil {
		return nil, err
	}

	// Increment blob ref count (same content, new object)
	if err := s.blobRepo.IncrementRef(ctx, *sourceObj.ContentHash); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	// Determine content type and metadata
	contentType := sourceObj.ContentType
	metadata := sourceObj.Metadata
	if input.MetadataDirective == "REPLACE" {
		if input.ContentType != "" {
			contentType = input.ContentType
		}
		if input.Metadata != nil {
			metadata = input.Metadata
		}
	}

	// Mark existing destination as not latest
	if destBucket.Versioning != domain.VersioningEnabled {
		existingObj, err := s.objectRepo.GetByKey(ctx, destBucket.ID, input.DestKey)
		if err == nil && existingObj.ContentHash != nil {
			_, _ = s.blobRepo.DecrementRef(ctx, *existingObj.ContentHash)
		}
		_ = s.objectRepo.MarkNotLatest(ctx, destBucket.ID, input.DestKey)
	}

	// Create new object
	newObj := domain.NewObject(destBucket.ID, input.DestKey, *sourceObj.ContentHash, contentType, sourceObj.ETag, sourceObj.Size)
	newObj.Metadata = metadata
	newObj.StorageClass = sourceObj.StorageClass

	if err := s.objectRepo.Create(ctx, newObj); err != nil {
		// Rollback ref count increment
		_, _ = s.blobRepo.DecrementRef(ctx, *sourceObj.ContentHash)
		return nil, fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	s.logger.Info().
		Str("source_bucket", input.SourceBucket).
		Str("source_key", input.SourceKey).
		Str("dest_bucket", input.DestBucket).
		Str("dest_key", input.DestKey).
		Msg("object copied")

	return &CopyObjectOutput{
		ETag:         newObj.ETag,
		LastModified: newObj.CreatedAt,
		VersionID:    newObj.GetVersionIDString(),
	}, nil
}

// =============================================================================
// Helper Functions
// =============================================================================

// validateObjectKey validates an S3 object key.
func validateObjectKey(key string) error {
	if key == "" {
		return domain.ErrObjectKeyEmpty
	}
	if len(key) > 1024 {
		return domain.ErrObjectKeyTooLong
	}
	return nil
}

// calculateETag generates an ETag from the content hash.
// For simple uploads, we use MD5 of the SHA256 hash.
func calculateETag(contentHash string) string {
	hash := md5.Sum([]byte(contentHash))
	return fmt.Sprintf("\"%s\"", hex.EncodeToString(hash[:]))
}

// encodeContinuationToken encodes a key as a continuation token.
func encodeContinuationToken(key string) string {
	// Simple encoding - in production, might want to encrypt/sign
	return key
}

// decodeContinuationToken decodes a continuation token to a key.
func decodeContinuationToken(token string) string {
	return token
}

// RangeReader is an interface for storage backends that support range reads.
type RangeReader interface {
	RetrieveRange(ctx context.Context, contentHash string, offset, length int64) (io.ReadCloser, error)
}
