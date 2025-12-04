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
	"github.com/prn-tf/alexander-storage/internal/lock"
	"github.com/prn-tf/alexander-storage/internal/repository"
	"github.com/prn-tf/alexander-storage/internal/storage"
)

// MultipartService handles multipart upload operations.
type MultipartService struct {
	multipartRepo repository.MultipartUploadRepository
	objectRepo    repository.ObjectRepository
	blobRepo      repository.BlobRepository
	bucketRepo    repository.BucketRepository
	storage       storage.Backend
	locker        lock.Locker
	logger        zerolog.Logger
}

// NewMultipartService creates a new MultipartService.
func NewMultipartService(
	multipartRepo repository.MultipartUploadRepository,
	objectRepo repository.ObjectRepository,
	blobRepo repository.BlobRepository,
	bucketRepo repository.BucketRepository,
	storage storage.Backend,
	locker lock.Locker,
	logger zerolog.Logger,
) *MultipartService {
	return &MultipartService{
		multipartRepo: multipartRepo,
		objectRepo:    objectRepo,
		blobRepo:      blobRepo,
		bucketRepo:    bucketRepo,
		storage:       storage,
		locker:        locker,
		logger:        logger.With().Str("service", "multipart").Logger(),
	}
}

// =============================================================================
// Input/Output Structs
// =============================================================================

// InitiateMultipartUploadInput contains the data needed to initiate a multipart upload.
type InitiateMultipartUploadInput struct {
	BucketName   string
	Key          string
	ContentType  string
	Metadata     map[string]string
	StorageClass domain.StorageClass
	OwnerID      int64
}

// InitiateMultipartUploadOutput contains the result of initiating a multipart upload.
type InitiateMultipartUploadOutput struct {
	Bucket   string
	Key      string
	UploadID string
}

// UploadPartInput contains the data needed to upload a part.
type UploadPartInput struct {
	BucketName string
	Key        string
	UploadID   string
	PartNumber int
	Body       io.Reader
	Size       int64
	OwnerID    int64
}

// UploadPartOutput contains the result of uploading a part.
type UploadPartOutput struct {
	ETag string
}

// CompleteMultipartUploadInput contains the data needed to complete a multipart upload.
type CompleteMultipartUploadInput struct {
	BucketName string
	Key        string
	UploadID   string
	Parts      []domain.CompletedPart
	OwnerID    int64
}

// CompleteMultipartUploadOutput contains the result of completing a multipart upload.
type CompleteMultipartUploadOutput struct {
	Location  string
	Bucket    string
	Key       string
	ETag      string
	VersionID string
}

// AbortMultipartUploadInput contains the data needed to abort a multipart upload.
type AbortMultipartUploadInput struct {
	BucketName string
	Key        string
	UploadID   string
	OwnerID    int64
}

// ListMultipartUploadsInput contains the data needed to list multipart uploads.
type ListMultipartUploadsInput struct {
	BucketName     string
	Prefix         string
	Delimiter      string
	KeyMarker      string
	UploadIDMarker string
	MaxUploads     int
	OwnerID        int64
}

// ListMultipartUploadsOutput contains the result of listing multipart uploads.
type ListMultipartUploadsOutput struct {
	Bucket             string
	KeyMarker          string
	UploadIDMarker     string
	NextKeyMarker      string
	NextUploadIDMarker string
	Prefix             string
	Delimiter          string
	MaxUploads         int
	IsTruncated        bool
	Uploads            []MultipartUploadInfo
	CommonPrefixes     []string
}

// MultipartUploadInfo represents a multipart upload in list output.
type MultipartUploadInfo struct {
	Key          string
	UploadID     string
	Initiated    time.Time
	StorageClass domain.StorageClass
}

// ListPartsInput contains the data needed to list parts.
type ListPartsInput struct {
	BucketName       string
	Key              string
	UploadID         string
	PartNumberMarker int
	MaxParts         int
	OwnerID          int64
}

// ListPartsOutput contains the result of listing parts.
type ListPartsOutput struct {
	Bucket               string
	Key                  string
	UploadID             string
	PartNumberMarker     int
	NextPartNumberMarker int
	MaxParts             int
	IsTruncated          bool
	Parts                []PartInfo
	StorageClass         domain.StorageClass
}

// PartInfo represents a part in list output.
type PartInfo struct {
	PartNumber   int
	LastModified time.Time
	ETag         string
	Size         int64
}

// =============================================================================
// Service Methods
// =============================================================================

// InitiateMultipartUpload starts a new multipart upload.
func (s *MultipartService) InitiateMultipartUpload(ctx context.Context, input InitiateMultipartUploadInput) (*InitiateMultipartUploadOutput, error) {
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

	// Create multipart upload
	upload := domain.NewMultipartUpload(bucket.ID, input.Key, input.OwnerID)
	if input.StorageClass != "" {
		upload.StorageClass = input.StorageClass
	}
	if input.Metadata != nil {
		upload.Metadata = input.Metadata
	}

	if err := s.multipartRepo.Create(ctx, upload); err != nil {
		s.logger.Error().Err(err).Str("key", input.Key).Msg("failed to create multipart upload")
		return nil, fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	s.logger.Info().
		Str("bucket", input.BucketName).
		Str("key", input.Key).
		Str("upload_id", upload.ID.String()).
		Msg("multipart upload initiated")

	return &InitiateMultipartUploadOutput{
		Bucket:   input.BucketName,
		Key:      input.Key,
		UploadID: upload.ID.String(),
	}, nil
}

// UploadPart uploads a part of a multipart upload.
func (s *MultipartService) UploadPart(ctx context.Context, input UploadPartInput) (*UploadPartOutput, error) {
	// Validate part number
	if err := domain.ValidatePartNumber(input.PartNumber); err != nil {
		return nil, err
	}

	// Validate part size (5MB minimum except for last part, 5GB maximum)
	const minPartSize = 5 * 1024 * 1024        // 5MB
	const maxPartSize = 5 * 1024 * 1024 * 1024 // 5GB

	if input.Size > maxPartSize {
		return nil, domain.ErrPartTooLarge
	}

	// Parse upload ID
	uploadID, err := uuid.Parse(input.UploadID)
	if err != nil {
		return nil, domain.ErrMultipartUploadNotFound
	}

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

	// Get multipart upload
	upload, err := s.multipartRepo.GetByID(ctx, uploadID)
	if err != nil {
		if errors.Is(err, domain.ErrMultipartUploadNotFound) {
			return nil, domain.ErrMultipartUploadNotFound
		}
		return nil, fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	// Verify upload is for the correct bucket and key
	if upload.BucketID != bucket.ID || upload.Key != input.Key {
		return nil, domain.ErrMultipartUploadNotFound
	}

	// Check upload status
	if upload.Status != domain.MultipartStatusInProgress {
		if upload.Status == domain.MultipartStatusCompleted {
			return nil, domain.ErrMultipartUploadCompleted
		}
		return nil, domain.ErrMultipartUploadAborted
	}

	// Check if upload is expired
	if upload.IsExpired() {
		return nil, domain.ErrMultipartUploadExpired
	}

	// Store part content in CAS storage
	contentHash, err := s.storage.Store(ctx, input.Body, input.Size)
	if err != nil {
		s.logger.Error().Err(err).Int("part", input.PartNumber).Msg("failed to store part content")
		return nil, fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	// Get storage path for blob
	storagePath := s.storage.GetPath(contentHash)

	// Upsert blob metadata
	_, err = s.blobRepo.UpsertWithRefIncrement(ctx, contentHash, input.Size, storagePath)
	if err != nil {
		s.logger.Error().Err(err).Str("content_hash", contentHash).Msg("failed to upsert blob")
		return nil, fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	// Calculate ETag (MD5 of content hash)
	etag := calculatePartETag(contentHash)

	// Create/update part record
	part := domain.NewUploadPart(uploadID, input.PartNumber, contentHash, etag, input.Size)
	if err := s.multipartRepo.CreatePart(ctx, part); err != nil {
		s.logger.Error().Err(err).Int("part", input.PartNumber).Msg("failed to create part record")
		return nil, fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	s.logger.Info().
		Str("upload_id", input.UploadID).
		Int("part_number", input.PartNumber).
		Int64("size", input.Size).
		Msg("part uploaded")

	return &UploadPartOutput{
		ETag: etag,
	}, nil
}

// CompleteMultipartUpload completes a multipart upload by combining all parts.
func (s *MultipartService) CompleteMultipartUpload(ctx context.Context, input CompleteMultipartUploadInput) (*CompleteMultipartUploadOutput, error) {
	// Validate parts provided
	if len(input.Parts) == 0 {
		return nil, domain.ErrNoPartsProvided
	}

	// Parse upload ID
	uploadID, err := uuid.Parse(input.UploadID)
	if err != nil {
		return nil, domain.ErrMultipartUploadNotFound
	}

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

	// Get multipart upload
	upload, err := s.multipartRepo.GetByID(ctx, uploadID)
	if err != nil {
		if errors.Is(err, domain.ErrMultipartUploadNotFound) {
			return nil, domain.ErrMultipartUploadNotFound
		}
		return nil, fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	// Verify upload is for the correct bucket and key
	if upload.BucketID != bucket.ID || upload.Key != input.Key {
		return nil, domain.ErrMultipartUploadNotFound
	}

	// Check upload status
	if upload.Status != domain.MultipartStatusInProgress {
		if upload.Status == domain.MultipartStatusCompleted {
			return nil, domain.ErrMultipartUploadCompleted
		}
		return nil, domain.ErrMultipartUploadAborted
	}

	// Validate part numbers are in ascending order
	for i := 1; i < len(input.Parts); i++ {
		if input.Parts[i].PartNumber <= input.Parts[i-1].PartNumber {
			return nil, domain.ErrInvalidPartOrder
		}
	}

	// Get part numbers from input
	partNumbers := make([]int, len(input.Parts))
	for i, p := range input.Parts {
		partNumbers[i] = p.PartNumber
	}

	// Get parts from database
	parts, err := s.multipartRepo.GetPartsForCompletion(ctx, uploadID, partNumbers)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	// Verify all requested parts exist and ETags match
	partMap := make(map[int]*domain.UploadPart)
	for _, p := range parts {
		partMap[p.PartNumber] = p
	}

	var totalSize int64
	etagParts := make([]string, len(input.Parts))
	orderedContentHashes := make([]string, len(input.Parts))
	for i, requestedPart := range input.Parts {
		storedPart, exists := partMap[requestedPart.PartNumber]
		if !exists {
			return nil, domain.ErrPartNotFound
		}
		if storedPart.ETag != requestedPart.ETag {
			return nil, domain.ErrPartETagMismatch
		}
		totalSize += storedPart.Size
		// Collect ETags for composite ETag calculation
		etagParts[i] = storedPart.ETag
		orderedContentHashes[i] = storedPart.ContentHash
	}

	// Calculate composite ETag (MD5 of concatenated part MD5s + "-" + partCount)
	compositeETag := calculateCompositeETag(etagParts)

	// Concatenate all parts into a single blob
	// Create a multi-reader that streams all parts sequentially
	contentHash, err := s.concatenateParts(ctx, orderedContentHashes, totalSize)
	if err != nil {
		s.logger.Error().Err(err).Str("upload_id", input.UploadID).Msg("failed to concatenate parts")
		return nil, fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	// Register the new combined blob
	storagePath := s.storage.GetPath(contentHash)
	_, err = s.blobRepo.UpsertWithRefIncrement(ctx, contentHash, totalSize, storagePath)
	if err != nil {
		s.logger.Error().Err(err).Str("content_hash", contentHash).Msg("failed to upsert combined blob")
		return nil, fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	// Handle versioning for destination bucket
	if bucket.IsVersioningEnabled() {
		_ = s.objectRepo.MarkNotLatest(ctx, bucket.ID, input.Key)
	} else {
		// Non-versioned: clean up existing object
		existingObj, err := s.objectRepo.GetByKey(ctx, bucket.ID, input.Key)
		if err == nil && existingObj.ContentHash != nil {
			_, _ = s.blobRepo.DecrementRef(ctx, *existingObj.ContentHash)
		}
		_ = s.objectRepo.MarkNotLatest(ctx, bucket.ID, input.Key)
	}

	// Create final object
	contentType := "application/octet-stream"
	if ct, ok := upload.Metadata["Content-Type"]; ok {
		contentType = ct
	}

	obj := domain.NewObject(bucket.ID, input.Key, contentHash, contentType, compositeETag, totalSize)
	obj.Metadata = upload.Metadata
	obj.StorageClass = upload.StorageClass

	if err := s.objectRepo.Create(ctx, obj); err != nil {
		s.logger.Error().Err(err).Str("key", input.Key).Msg("failed to create final object")
		return nil, fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	// Update upload status
	if err := s.multipartRepo.UpdateStatus(ctx, uploadID, domain.MultipartStatusCompleted); err != nil {
		s.logger.Error().Err(err).Str("upload_id", input.UploadID).Msg("failed to update upload status")
	}

	s.logger.Info().
		Str("bucket", input.BucketName).
		Str("key", input.Key).
		Str("upload_id", input.UploadID).
		Int64("total_size", totalSize).
		Int("part_count", len(input.Parts)).
		Msg("multipart upload completed")

	return &CompleteMultipartUploadOutput{
		Location:  fmt.Sprintf("/%s/%s", input.BucketName, input.Key),
		Bucket:    input.BucketName,
		Key:       input.Key,
		ETag:      compositeETag,
		VersionID: obj.GetVersionIDString(),
	}, nil
}

// AbortMultipartUpload aborts a multipart upload and cleans up parts.
func (s *MultipartService) AbortMultipartUpload(ctx context.Context, input AbortMultipartUploadInput) error {
	// Parse upload ID
	uploadID, err := uuid.Parse(input.UploadID)
	if err != nil {
		return domain.ErrMultipartUploadNotFound
	}

	// Get bucket
	bucket, err := s.bucketRepo.GetByName(ctx, input.BucketName)
	if err != nil {
		if errors.Is(err, domain.ErrBucketNotFound) {
			return domain.ErrBucketNotFound
		}
		return fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	// Check ownership
	if input.OwnerID > 0 && bucket.OwnerID != input.OwnerID {
		return ErrBucketAccessDenied
	}

	// Get multipart upload
	upload, err := s.multipartRepo.GetByID(ctx, uploadID)
	if err != nil {
		if errors.Is(err, domain.ErrMultipartUploadNotFound) {
			return domain.ErrMultipartUploadNotFound
		}
		return fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	// Verify upload is for the correct bucket and key
	if upload.BucketID != bucket.ID || upload.Key != input.Key {
		return domain.ErrMultipartUploadNotFound
	}

	// Get all parts to decrement blob ref counts
	partsResult, err := s.multipartRepo.ListParts(ctx, uploadID, repository.PartListOptions{MaxParts: 10000})
	if err == nil {
		for _, part := range partsResult.Parts {
			// Get full part info to get content hash
			fullPart, err := s.multipartRepo.GetPart(ctx, uploadID, part.PartNumber)
			if err == nil {
				_, _ = s.blobRepo.DecrementRef(ctx, fullPart.ContentHash)
			}
		}
	}

	// Delete multipart upload (cascades to parts)
	if err := s.multipartRepo.Delete(ctx, uploadID); err != nil {
		s.logger.Error().Err(err).Str("upload_id", input.UploadID).Msg("failed to delete multipart upload")
		return fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	s.logger.Info().
		Str("bucket", input.BucketName).
		Str("key", input.Key).
		Str("upload_id", input.UploadID).
		Msg("multipart upload aborted")

	return nil
}

// ListMultipartUploads lists in-progress multipart uploads.
func (s *MultipartService) ListMultipartUploads(ctx context.Context, input ListMultipartUploadsInput) (*ListMultipartUploadsOutput, error) {
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
	maxUploads := input.MaxUploads
	if maxUploads <= 0 {
		maxUploads = 1000
	}
	if maxUploads > 1000 {
		maxUploads = 1000
	}

	// List uploads
	result, err := s.multipartRepo.List(ctx, bucket.ID, repository.MultipartListOptions{
		Prefix:         input.Prefix,
		Delimiter:      input.Delimiter,
		KeyMarker:      input.KeyMarker,
		UploadIDMarker: input.UploadIDMarker,
		MaxUploads:     maxUploads,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	// Convert to output format
	uploads := make([]MultipartUploadInfo, len(result.Uploads))
	for i, u := range result.Uploads {
		uploads[i] = MultipartUploadInfo{
			Key:          u.Key,
			UploadID:     u.UploadID,
			Initiated:    u.Initiated,
			StorageClass: u.StorageClass,
		}
	}

	return &ListMultipartUploadsOutput{
		Bucket:             input.BucketName,
		KeyMarker:          input.KeyMarker,
		UploadIDMarker:     input.UploadIDMarker,
		NextKeyMarker:      result.NextKeyMarker,
		NextUploadIDMarker: result.NextUploadIDMarker,
		Prefix:             input.Prefix,
		Delimiter:          input.Delimiter,
		MaxUploads:         maxUploads,
		IsTruncated:        result.IsTruncated,
		Uploads:            uploads,
		CommonPrefixes:     result.CommonPrefixes,
	}, nil
}

// ListParts lists parts of a multipart upload.
func (s *MultipartService) ListParts(ctx context.Context, input ListPartsInput) (*ListPartsOutput, error) {
	// Parse upload ID
	uploadID, err := uuid.Parse(input.UploadID)
	if err != nil {
		return nil, domain.ErrMultipartUploadNotFound
	}

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

	// Get multipart upload
	upload, err := s.multipartRepo.GetByID(ctx, uploadID)
	if err != nil {
		if errors.Is(err, domain.ErrMultipartUploadNotFound) {
			return nil, domain.ErrMultipartUploadNotFound
		}
		return nil, fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	// Verify upload is for the correct bucket and key
	if upload.BucketID != bucket.ID || upload.Key != input.Key {
		return nil, domain.ErrMultipartUploadNotFound
	}

	// Set defaults
	maxParts := input.MaxParts
	if maxParts <= 0 {
		maxParts = 1000
	}
	if maxParts > 1000 {
		maxParts = 1000
	}

	// List parts
	result, err := s.multipartRepo.ListParts(ctx, uploadID, repository.PartListOptions{
		PartNumberMarker: input.PartNumberMarker,
		MaxParts:         maxParts,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	// Convert to output format
	parts := make([]PartInfo, len(result.Parts))
	for i, p := range result.Parts {
		parts[i] = PartInfo{
			PartNumber:   p.PartNumber,
			LastModified: p.LastModified,
			ETag:         p.ETag,
			Size:         p.Size,
		}
	}

	return &ListPartsOutput{
		Bucket:               input.BucketName,
		Key:                  input.Key,
		UploadID:             input.UploadID,
		PartNumberMarker:     input.PartNumberMarker,
		NextPartNumberMarker: result.NextPartNumberMarker,
		MaxParts:             maxParts,
		IsTruncated:          result.IsTruncated,
		Parts:                parts,
		StorageClass:         upload.StorageClass,
	}, nil
}

// =============================================================================
// Helper Functions
// =============================================================================

// calculatePartETag generates an ETag for a part.
func calculatePartETag(contentHash string) string {
	hash := md5.Sum([]byte(contentHash))
	return fmt.Sprintf("\"%s\"", hex.EncodeToString(hash[:]))
}

// calculateCompositeETag generates a composite ETag for multipart uploads.
// Format: "{md5-of-concatenated-part-etags}-{partCount}"
func calculateCompositeETag(partETags []string) string {
	h := md5.New()
	for _, etag := range partETags {
		// Strip quotes from ETag
		etag = etag[1 : len(etag)-1]
		data, _ := hex.DecodeString(etag)
		h.Write(data)
	}
	return fmt.Sprintf("\"%s-%d\"", hex.EncodeToString(h.Sum(nil)), len(partETags))
}

// concatenateParts concatenates multiple part blobs into a single new blob.
// It reads each part sequentially and writes them to a new combined blob.
// Returns the content hash of the combined blob.
func (s *MultipartService) concatenateParts(ctx context.Context, contentHashes []string, totalSize int64) (string, error) {
	// Create a multi-reader that streams all parts sequentially
	readers := make([]io.Reader, 0, len(contentHashes))
	closers := make([]io.Closer, 0, len(contentHashes))

	// Ensure all readers are closed on exit
	defer func() {
		for _, c := range closers {
			_ = c.Close()
		}
	}()

	// Open readers for each part
	for _, hash := range contentHashes {
		reader, err := s.storage.Retrieve(ctx, hash)
		if err != nil {
			return "", fmt.Errorf("failed to retrieve part %s: %w", hash, err)
		}
		readers = append(readers, reader)
		closers = append(closers, reader)
	}

	// Create a multi-reader that concatenates all parts
	multiReader := io.MultiReader(readers...)

	// Store the concatenated content
	contentHash, err := s.storage.Store(ctx, multiReader, totalSize)
	if err != nil {
		return "", fmt.Errorf("failed to store concatenated blob: %w", err)
	}

	return contentHash, nil
}
