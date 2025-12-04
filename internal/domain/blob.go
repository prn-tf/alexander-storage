// Package domain contains the core business entities for Alexander Storage.
package domain

import (
	"path/filepath"
	"time"
)

// Blob represents a content-addressable storage entry.
// Blobs are stored by their SHA-256 hash, enabling deduplication.
// Multiple objects can reference the same blob.
type Blob struct {
	// ContentHash is the SHA-256 hash of the content (64 hex characters).
	// This serves as the primary key and storage identifier.
	ContentHash string `json:"content_hash"`

	// Size is the size of the blob in bytes.
	Size int64 `json:"size"`

	// StoragePath is the path where the blob is stored on disk.
	// Format: /{base}/{first2chars}/{next2chars}/{fullhash}
	// Example: /data/ab/cd/abcdef1234567890...
	StoragePath string `json:"storage_path"`

	// RefCount is the number of objects referencing this blob.
	// When RefCount reaches 0, the blob can be garbage collected.
	RefCount int32 `json:"ref_count"`

	// CreatedAt is the timestamp when the blob was first stored.
	CreatedAt time.Time `json:"created_at"`

	// LastAccessed is the timestamp when the blob was last read.
	LastAccessed time.Time `json:"last_accessed"`
}

// NewBlob creates a new Blob with the given hash and size.
// The storage path is computed from the hash using 2-level sharding.
func NewBlob(contentHash string, size int64, basePath string) *Blob {
	now := time.Now().UTC()
	return &Blob{
		ContentHash:  contentHash,
		Size:         size,
		StoragePath:  ComputeStoragePath(basePath, contentHash),
		RefCount:     1,
		CreatedAt:    now,
		LastAccessed: now,
	}
}

// ComputeStoragePath generates the storage path for a blob using 2-level directory sharding.
// This distributes files across directories to avoid filesystem limitations.
//
// Example:
//
//	hash: "abcdef1234567890..."
//	basePath: "/data"
//	result: "/data/ab/cd/abcdef1234567890..."
func ComputeStoragePath(basePath, contentHash string) string {
	if len(contentHash) < 4 {
		return filepath.Join(basePath, contentHash)
	}

	level1 := contentHash[0:2]
	level2 := contentHash[2:4]

	return filepath.Join(basePath, level1, level2, contentHash)
}

// IsOrphan returns true if no objects reference this blob.
func (b *Blob) IsOrphan() bool {
	return b.RefCount <= 0
}

// CanGarbageCollect returns true if the blob is orphaned and old enough.
func (b *Blob) CanGarbageCollect(gracePeriod time.Duration) bool {
	if !b.IsOrphan() {
		return false
	}

	// Don't delete blobs that were just created (might be in-progress upload)
	return time.Since(b.CreatedAt) > gracePeriod
}
