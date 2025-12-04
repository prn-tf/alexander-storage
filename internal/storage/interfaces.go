// Package storage defines interfaces for blob storage backends.
// The storage layer is responsible for persisting and retrieving raw object data.
// It implements Content-Addressable Storage (CAS) for deduplication.
package storage

import (
	"context"
	"io"
)

// Backend defines the interface for storage backends.
// Implementations can include local filesystem, NAS, S3, or other storage systems.
// The interface is designed to be stateless and support horizontal scaling.
type Backend interface {
	// Store stores content from a reader and returns the content hash (SHA-256).
	// The content is stored at a location derived from its hash.
	// If the content already exists (same hash), no new file is created.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeouts
	//   - reader: Source of the content to store
	//   - size: Expected size in bytes (for validation and preallocation)
	//
	// Returns:
	//   - contentHash: SHA-256 hash of the content (64 hex characters)
	//   - err: Error if storage fails
	Store(ctx context.Context, reader io.Reader, size int64) (contentHash string, err error)

	// Retrieve retrieves content by its hash.
	// Returns a ReadCloser that must be closed after use.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeouts
	//   - contentHash: SHA-256 hash of the content to retrieve
	//
	// Returns:
	//   - io.ReadCloser: Stream of the content (caller must close)
	//   - err: ErrBlobNotFound if content doesn't exist, or other error
	Retrieve(ctx context.Context, contentHash string) (io.ReadCloser, error)

	// Delete removes content by its hash.
	// This should only be called when reference count reaches zero.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeouts
	//   - contentHash: SHA-256 hash of the content to delete
	//
	// Returns:
	//   - err: Error if deletion fails (ErrBlobNotFound if content doesn't exist)
	Delete(ctx context.Context, contentHash string) error

	// Exists checks if content with the given hash exists.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeouts
	//   - contentHash: SHA-256 hash to check
	//
	// Returns:
	//   - bool: true if content exists, false otherwise
	//   - err: Error if check fails
	Exists(ctx context.Context, contentHash string) (bool, error)

	// GetSize returns the size of stored content.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeouts
	//   - contentHash: SHA-256 hash of the content
	//
	// Returns:
	//   - int64: Size in bytes
	//   - err: ErrBlobNotFound if content doesn't exist
	GetSize(ctx context.Context, contentHash string) (int64, error)

	// GetPath returns the storage path for a content hash.
	// This is useful for debugging and direct access scenarios.
	//
	// Parameters:
	//   - contentHash: SHA-256 hash of the content
	//
	// Returns:
	//   - string: Full path to the content in storage
	GetPath(contentHash string) string
}

// ContentAddressableStorage extends Backend with reference counting support.
// This interface is used when deduplication tracking is managed by the storage layer.
// In our architecture, reference counting is handled by PostgreSQL, but this interface
// allows for alternative implementations where the storage backend tracks references.
type ContentAddressableStorage interface {
	Backend

	// StoreWithDedup stores content and handles deduplication.
	// Returns whether this is a new blob or an existing one.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeouts
	//   - reader: Source of the content
	//   - size: Expected size in bytes
	//
	// Returns:
	//   - contentHash: SHA-256 hash of the content
	//   - isNew: true if this is a new blob, false if it already existed
	//   - err: Error if storage fails
	StoreWithDedup(ctx context.Context, reader io.Reader, size int64) (contentHash string, isNew bool, err error)

	// Stats returns storage statistics.
	Stats(ctx context.Context) (*StorageStats, error)
}

// StorageStats contains storage backend statistics.
type StorageStats struct {
	// TotalBlobs is the number of unique blobs stored.
	TotalBlobs int64 `json:"total_blobs"`

	// TotalSize is the total size of all blobs in bytes.
	TotalSize int64 `json:"total_size"`

	// UsedSpace is the actual disk space used (may differ from TotalSize due to filesystem overhead).
	UsedSpace int64 `json:"used_space"`

	// FreeSpace is the available disk space in bytes.
	FreeSpace int64 `json:"free_space"`

	// OrphanBlobs is the number of blobs with zero references.
	OrphanBlobs int64 `json:"orphan_blobs"`

	// OrphanSize is the total size of orphan blobs.
	OrphanSize int64 `json:"orphan_size"`
}

// PartStorage provides operations for multipart upload parts.
// Parts are stored temporarily and combined into a final blob upon completion.
type PartStorage interface {
	// StorePart stores a multipart upload part.
	//
	// Parameters:
	//   - ctx: Context for cancellation
	//   - uploadID: The multipart upload ID
	//   - partNumber: Part number (1-10000)
	//   - reader: Source of the part content
	//   - size: Expected size in bytes
	//
	// Returns:
	//   - contentHash: SHA-256 hash of the part
	//   - err: Error if storage fails
	StorePart(ctx context.Context, uploadID string, partNumber int, reader io.Reader, size int64) (contentHash string, err error)

	// GetPart retrieves a stored part.
	//
	// Parameters:
	//   - ctx: Context for cancellation
	//   - uploadID: The multipart upload ID
	//   - partNumber: Part number to retrieve
	//
	// Returns:
	//   - io.ReadCloser: Stream of part content
	//   - err: Error if retrieval fails
	GetPart(ctx context.Context, uploadID string, partNumber int) (io.ReadCloser, error)

	// CombineParts combines parts into a single blob.
	// This is called during CompleteMultipartUpload.
	//
	// Parameters:
	//   - ctx: Context for cancellation
	//   - uploadID: The multipart upload ID
	//   - parts: Ordered list of part numbers to combine
	//
	// Returns:
	//   - contentHash: SHA-256 hash of the combined content
	//   - size: Total size of the combined content
	//   - err: Error if combination fails
	CombineParts(ctx context.Context, uploadID string, parts []int) (contentHash string, size int64, err error)

	// CleanupParts removes all parts for an upload.
	// Called after completion or abort.
	//
	// Parameters:
	//   - ctx: Context for cancellation
	//   - uploadID: The multipart upload ID
	//
	// Returns:
	//   - err: Error if cleanup fails
	CleanupParts(ctx context.Context, uploadID string) error
}
