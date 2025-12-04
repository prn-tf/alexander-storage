// Package domain contains the core business entities for Alexander Storage.
package domain

import (
	"time"

	"github.com/google/uuid"
)

// StorageClass represents the storage tier for an object.
type StorageClass string

const (
	// StorageClassStandard is the default storage class for frequently accessed data.
	StorageClassStandard StorageClass = "STANDARD"

	// StorageClassReducedRedundancy is for non-critical, reproducible data.
	StorageClassReducedRedundancy StorageClass = "REDUCED_REDUNDANCY"

	// StorageClassGlacier is for archived data with retrieval times in minutes to hours.
	StorageClassGlacier StorageClass = "GLACIER"

	// StorageClassDeepArchive is for rarely accessed data with retrieval times in hours.
	StorageClassDeepArchive StorageClass = "DEEP_ARCHIVE"
)

// Object represents an S3-compatible object stored in a bucket.
// Objects support versioning - each version has a unique version ID.
type Object struct {
	// ID is the unique identifier for this object version.
	ID int64 `json:"id"`

	// BucketID is the ID of the bucket containing this object.
	BucketID int64 `json:"bucket_id"`

	// Key is the object key (path) within the bucket.
	// Maximum length: 1024 characters.
	Key string `json:"key"`

	// VersionID is the unique identifier for this version.
	// For non-versioned buckets, this is still set but not exposed.
	VersionID uuid.UUID `json:"version_id"`

	// IsLatest indicates whether this is the current version.
	// Only one version per bucket+key can have IsLatest=true.
	IsLatest bool `json:"is_latest"`

	// IsDeleteMarker indicates whether this is a delete marker.
	// Delete markers have no content and represent a "deleted" state in versioned buckets.
	IsDeleteMarker bool `json:"is_delete_marker"`

	// ContentHash is the SHA-256 hash of the object content.
	// This references a blob in the blobs table.
	// NULL for delete markers.
	ContentHash *string `json:"content_hash,omitempty"`

	// Size is the size of the object in bytes.
	// 0 for delete markers.
	Size int64 `json:"size"`

	// ContentType is the MIME type of the object.
	ContentType string `json:"content_type"`

	// ETag is the entity tag, typically MD5 hash or composite hash for multipart.
	// Format for multipart: "{md5}-{partCount}"
	ETag string `json:"etag"`

	// StorageClass is the storage tier for this object.
	StorageClass StorageClass `json:"storage_class"`

	// Metadata contains user-defined metadata (x-amz-meta-* headers).
	Metadata map[string]string `json:"metadata,omitempty"`

	// CreatedAt is the timestamp when this version was created.
	CreatedAt time.Time `json:"created_at"`

	// DeletedAt is the timestamp when this version was deleted.
	// Only set for hard deletes (specific version deletion).
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
}

// NewObject creates a new Object with default values.
func NewObject(bucketID int64, key, contentHash, contentType, etag string, size int64) *Object {
	return &Object{
		BucketID:       bucketID,
		Key:            key,
		VersionID:      uuid.New(),
		IsLatest:       true,
		IsDeleteMarker: false,
		ContentHash:    &contentHash,
		Size:           size,
		ContentType:    contentType,
		ETag:           etag,
		StorageClass:   StorageClassStandard,
		Metadata:       make(map[string]string),
		CreatedAt:      time.Now().UTC(),
	}
}

// NewDeleteMarker creates a delete marker object.
// Delete markers are used in versioned buckets to indicate deletion.
func NewDeleteMarker(bucketID int64, key string) *Object {
	return &Object{
		BucketID:       bucketID,
		Key:            key,
		VersionID:      uuid.New(),
		IsLatest:       true,
		IsDeleteMarker: true,
		ContentHash:    nil,
		Size:           0,
		ContentType:    "",
		ETag:           "",
		StorageClass:   StorageClassStandard,
		Metadata:       make(map[string]string),
		CreatedAt:      time.Now().UTC(),
	}
}

// IsDeleted returns true if the object is effectively deleted.
// This is true if it's a delete marker or has been hard-deleted.
func (o *Object) IsDeleted() bool {
	return o.IsDeleteMarker || o.DeletedAt != nil
}

// GetVersionIDString returns the version ID as a string.
// Returns "null" for null version (suspended versioning).
func (o *Object) GetVersionIDString() string {
	if o.VersionID == uuid.Nil {
		return "null"
	}
	return o.VersionID.String()
}

// ObjectInfo is a summary of object metadata returned in list operations.
type ObjectInfo struct {
	Key          string       `json:"key"`
	VersionID    string       `json:"version_id,omitempty"`
	IsLatest     bool         `json:"is_latest,omitempty"`
	Size         int64        `json:"size"`
	ETag         string       `json:"etag"`
	LastModified time.Time    `json:"last_modified"`
	StorageClass StorageClass `json:"storage_class"`
	Owner        *OwnerInfo   `json:"owner,omitempty"`
}

// OwnerInfo contains information about the object owner.
type OwnerInfo struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
}

// ObjectVersion is used in ListObjectVersions responses.
type ObjectVersion struct {
	Key            string       `json:"key"`
	VersionID      string       `json:"version_id"`
	IsLatest       bool         `json:"is_latest"`
	IsDeleteMarker bool         `json:"is_delete_marker"`
	Size           int64        `json:"size"`
	ETag           string       `json:"etag"`
	LastModified   time.Time    `json:"last_modified"`
	StorageClass   StorageClass `json:"storage_class"`
	Owner          *OwnerInfo   `json:"owner,omitempty"`
}
