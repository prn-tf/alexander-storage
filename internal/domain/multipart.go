// Package domain contains the core business entities for Alexander Storage.
package domain

import (
	"time"

	"github.com/google/uuid"
)

// MultipartStatus represents the status of a multipart upload.
type MultipartStatus string

const (
	// MultipartStatusInProgress indicates the upload is active.
	MultipartStatusInProgress MultipartStatus = "InProgress"

	// MultipartStatusCompleted indicates the upload was successfully completed.
	MultipartStatusCompleted MultipartStatus = "Completed"

	// MultipartStatusAborted indicates the upload was aborted.
	MultipartStatusAborted MultipartStatus = "Aborted"
)

// MultipartUpload represents an in-progress multipart upload.
// Multipart uploads allow uploading large objects in parts.
type MultipartUpload struct {
	// ID is the unique identifier for this multipart upload (upload ID).
	ID uuid.UUID `json:"upload_id"`

	// BucketID is the ID of the bucket where the object will be created.
	BucketID int64 `json:"bucket_id"`

	// Key is the object key for the final object.
	Key string `json:"key"`

	// InitiatorID is the ID of the user who initiated the upload.
	InitiatorID int64 `json:"initiator_id"`

	// Status is the current status of the upload.
	Status MultipartStatus `json:"status"`

	// StorageClass is the storage tier for the final object.
	StorageClass StorageClass `json:"storage_class"`

	// Metadata contains user-defined metadata for the final object.
	Metadata map[string]string `json:"metadata,omitempty"`

	// InitiatedAt is when the multipart upload was initiated.
	InitiatedAt time.Time `json:"initiated_at"`

	// ExpiresAt is when the incomplete upload will be garbage collected.
	// Default: 7 days after initiation.
	ExpiresAt time.Time `json:"expires_at"`

	// CompletedAt is when the upload was completed (if applicable).
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// NewMultipartUpload creates a new MultipartUpload.
func NewMultipartUpload(bucketID int64, key string, initiatorID int64) *MultipartUpload {
	now := time.Now().UTC()
	return &MultipartUpload{
		ID:           uuid.New(),
		BucketID:     bucketID,
		Key:          key,
		InitiatorID:  initiatorID,
		Status:       MultipartStatusInProgress,
		StorageClass: StorageClassStandard,
		Metadata:     make(map[string]string),
		InitiatedAt:  now,
		ExpiresAt:    now.Add(7 * 24 * time.Hour), // 7 days
	}
}

// IsExpired returns true if the upload has expired.
func (m *MultipartUpload) IsExpired() bool {
	return time.Now().UTC().After(m.ExpiresAt)
}

// IsActive returns true if the upload is still in progress.
func (m *MultipartUpload) IsActive() bool {
	return m.Status == MultipartStatusInProgress && !m.IsExpired()
}

// UploadPart represents a single part of a multipart upload.
type UploadPart struct {
	// ID is the unique identifier for this part record.
	ID int64 `json:"id"`

	// UploadID is the ID of the parent multipart upload.
	UploadID uuid.UUID `json:"upload_id"`

	// PartNumber is the part number (1-10000).
	PartNumber int `json:"part_number"`

	// ContentHash is the SHA-256 hash of the part content.
	// This references a blob in the blobs table.
	ContentHash string `json:"content_hash"`

	// Size is the size of the part in bytes.
	Size int64 `json:"size"`

	// ETag is the entity tag for this part (typically MD5).
	ETag string `json:"etag"`

	// CreatedAt is when this part was uploaded.
	CreatedAt time.Time `json:"created_at"`
}

// NewUploadPart creates a new UploadPart.
func NewUploadPart(uploadID uuid.UUID, partNumber int, contentHash, etag string, size int64) *UploadPart {
	return &UploadPart{
		UploadID:    uploadID,
		PartNumber:  partNumber,
		ContentHash: contentHash,
		Size:        size,
		ETag:        etag,
		CreatedAt:   time.Now().UTC(),
	}
}

// ValidatePartNumber checks if the part number is valid (1-10000).
func ValidatePartNumber(partNumber int) error {
	if partNumber < 1 || partNumber > 10000 {
		return ErrInvalidPartNumber
	}
	return nil
}

// PartInfo is a summary of part information returned in list operations.
type PartInfo struct {
	PartNumber   int       `json:"part_number"`
	Size         int64     `json:"size"`
	ETag         string    `json:"etag"`
	LastModified time.Time `json:"last_modified"`
}

// CompletedPart is used in CompleteMultipartUpload to identify parts to combine.
type CompletedPart struct {
	PartNumber int    `json:"part_number"`
	ETag       string `json:"etag"`
}

// MultipartUploadInfo is a summary returned in ListMultipartUploads.
type MultipartUploadInfo struct {
	UploadID     string       `json:"upload_id"`
	Key          string       `json:"key"`
	Initiated    time.Time    `json:"initiated"`
	StorageClass StorageClass `json:"storage_class"`
	Owner        *OwnerInfo   `json:"owner,omitempty"`
	Initiator    *OwnerInfo   `json:"initiator,omitempty"`
}
