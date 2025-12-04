// Package repository defines data access interfaces for Alexander Storage.
// These interfaces abstract database operations, allowing for different implementations
// (PostgreSQL, in-memory for testing, etc.) while keeping the service layer clean.
package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/prn-tf/alexander-storage/internal/domain"
)

// =============================================================================
// User Repository
// =============================================================================

// UserRepository defines the interface for user data access.
type UserRepository interface {
	// Create creates a new user.
	Create(ctx context.Context, user *domain.User) error

	// GetByID retrieves a user by ID.
	GetByID(ctx context.Context, id int64) (*domain.User, error)

	// GetByUsername retrieves a user by username.
	GetByUsername(ctx context.Context, username string) (*domain.User, error)

	// GetByEmail retrieves a user by email.
	GetByEmail(ctx context.Context, email string) (*domain.User, error)

	// Update updates an existing user.
	Update(ctx context.Context, user *domain.User) error

	// Delete deletes a user by ID.
	Delete(ctx context.Context, id int64) error

	// List returns all users with pagination.
	List(ctx context.Context, opts ListOptions) (*ListResult[domain.User], error)

	// ExistsByUsername checks if a user with the given username exists.
	ExistsByUsername(ctx context.Context, username string) (bool, error)

	// ExistsByEmail checks if a user with the given email exists.
	ExistsByEmail(ctx context.Context, email string) (bool, error)
}

// =============================================================================
// Access Key Repository
// =============================================================================

// AccessKeyRepository defines the interface for access key data access.
type AccessKeyRepository interface {
	// Create creates a new access key.
	Create(ctx context.Context, key *domain.AccessKey) error

	// GetByID retrieves an access key by ID.
	GetByID(ctx context.Context, id int64) (*domain.AccessKey, error)

	// GetByAccessKeyID retrieves an access key by access key ID (the 20-char identifier).
	GetByAccessKeyID(ctx context.Context, accessKeyID string) (*domain.AccessKey, error)

	// GetActiveByAccessKeyID retrieves an active, non-expired access key.
	// This is the primary method used for authentication.
	GetActiveByAccessKeyID(ctx context.Context, accessKeyID string) (*domain.AccessKey, error)

	// ListByUserID returns all access keys for a user.
	ListByUserID(ctx context.Context, userID int64) ([]*domain.AccessKey, error)

	// Update updates an existing access key.
	Update(ctx context.Context, key *domain.AccessKey) error

	// UpdateLastUsed updates the last_used_at timestamp.
	UpdateLastUsed(ctx context.Context, id int64) error

	// Delete deletes an access key by ID.
	Delete(ctx context.Context, id int64) error

	// DeleteByAccessKeyID deletes an access key by access key ID.
	DeleteByAccessKeyID(ctx context.Context, accessKeyID string) error

	// DeleteExpired deletes all expired access keys.
	DeleteExpired(ctx context.Context) (int64, error)
}

// =============================================================================
// Bucket Repository
// =============================================================================

// BucketRepository defines the interface for bucket data access.
type BucketRepository interface {
	// Create creates a new bucket.
	Create(ctx context.Context, bucket *domain.Bucket) error

	// GetByID retrieves a bucket by ID.
	GetByID(ctx context.Context, id int64) (*domain.Bucket, error)

	// GetByName retrieves a bucket by name.
	GetByName(ctx context.Context, name string) (*domain.Bucket, error)

	// List returns all buckets for a user (or all if userID is 0).
	List(ctx context.Context, userID int64) ([]*domain.Bucket, error)

	// Update updates an existing bucket.
	Update(ctx context.Context, bucket *domain.Bucket) error

	// UpdateVersioning updates the versioning status of a bucket.
	UpdateVersioning(ctx context.Context, id int64, status domain.VersioningStatus) error

	// Delete deletes a bucket by ID.
	Delete(ctx context.Context, id int64) error

	// DeleteByName deletes a bucket by name.
	DeleteByName(ctx context.Context, name string) error

	// ExistsByName checks if a bucket with the given name exists.
	ExistsByName(ctx context.Context, name string) (bool, error)

	// IsEmpty checks if a bucket contains any objects.
	IsEmpty(ctx context.Context, id int64) (bool, error)
}

// =============================================================================
// Blob Repository (Content-Addressable Storage Metadata)
// =============================================================================

// BlobRepository defines the interface for blob metadata access.
// This manages the reference counting for content-addressable storage.
type BlobRepository interface {
	// UpsertWithRefIncrement creates a new blob or increments ref_count if it exists.
	// This is an atomic operation that handles deduplication.
	// Returns (isNew, error) where isNew indicates if a new blob was created.
	UpsertWithRefIncrement(ctx context.Context, contentHash string, size int64, storagePath string) (isNew bool, err error)

	// GetByHash retrieves a blob by its content hash.
	GetByHash(ctx context.Context, contentHash string) (*domain.Blob, error)

	// IncrementRef atomically increments the reference count.
	IncrementRef(ctx context.Context, contentHash string) error

	// DecrementRef atomically decrements the reference count.
	// Returns the new reference count (0 means blob can be garbage collected).
	DecrementRef(ctx context.Context, contentHash string) (newRefCount int32, err error)

	// GetRefCount returns the current reference count for a blob.
	GetRefCount(ctx context.Context, contentHash string) (int32, error)

	// Exists checks if a blob with the given hash exists.
	Exists(ctx context.Context, contentHash string) (bool, error)

	// Delete deletes a blob by its content hash.
	// Should only be called when ref_count is 0.
	Delete(ctx context.Context, contentHash string) error

	// ListOrphans returns blobs with ref_count = 0 that are older than the grace period.
	// Used by garbage collection.
	ListOrphans(ctx context.Context, gracePeriod time.Duration, limit int) ([]*domain.Blob, error)

	// DeleteOrphans deletes orphan blobs older than the grace period.
	// Returns the list of deleted blobs (for physical file cleanup).
	DeleteOrphans(ctx context.Context, gracePeriod time.Duration, limit int) ([]*domain.Blob, error)

	// UpdateLastAccessed updates the last_accessed timestamp.
	UpdateLastAccessed(ctx context.Context, contentHash string) error
}

// =============================================================================
// Object Repository
// =============================================================================

// ObjectRepository defines the interface for object data access.
type ObjectRepository interface {
	// Create creates a new object.
	Create(ctx context.Context, obj *domain.Object) error

	// GetByID retrieves an object by ID.
	GetByID(ctx context.Context, id int64) (*domain.Object, error)

	// GetByKey retrieves the latest version of an object by bucket ID and key.
	GetByKey(ctx context.Context, bucketID int64, key string) (*domain.Object, error)

	// GetByKeyAndVersion retrieves a specific version of an object.
	GetByKeyAndVersion(ctx context.Context, bucketID int64, key string, versionID uuid.UUID) (*domain.Object, error)

	// List returns objects in a bucket with pagination and optional prefix filtering.
	List(ctx context.Context, bucketID int64, opts ObjectListOptions) (*ObjectListResult, error)

	// ListVersions returns all versions of objects in a bucket.
	ListVersions(ctx context.Context, bucketID int64, opts ObjectListOptions) (*ObjectVersionListResult, error)

	// Update updates an existing object.
	Update(ctx context.Context, obj *domain.Object) error

	// MarkNotLatest marks an object as not the latest version.
	// Used when creating a new version.
	MarkNotLatest(ctx context.Context, bucketID int64, key string) error

	// Delete hard-deletes an object by ID.
	Delete(ctx context.Context, id int64) error

	// DeleteAllVersions deletes all versions of an object.
	DeleteAllVersions(ctx context.Context, bucketID int64, key string) error

	// CountByBucket returns the number of objects in a bucket.
	CountByBucket(ctx context.Context, bucketID int64) (int64, error)

	// GetContentHashForVersion retrieves the content hash for a specific version.
	// Used for ref_count management.
	GetContentHashForVersion(ctx context.Context, bucketID int64, key string, versionID uuid.UUID) (*string, error)
}

// ObjectListOptions contains options for listing objects.
type ObjectListOptions struct {
	// Prefix filters objects by key prefix.
	Prefix string

	// Delimiter groups keys by delimiter (e.g., "/" for directory-like listing).
	Delimiter string

	// StartAfter lists objects after this key (for pagination).
	StartAfter string

	// ContinuationToken for pagination (opaque token from previous response).
	ContinuationToken string

	// MaxKeys is the maximum number of keys to return.
	MaxKeys int
}

// ObjectListResult contains the result of a list objects operation.
type ObjectListResult struct {
	// Objects is the list of objects.
	Objects []*domain.ObjectInfo

	// CommonPrefixes contains grouped key prefixes (when using delimiter).
	CommonPrefixes []string

	// IsTruncated indicates if there are more results.
	IsTruncated bool

	// NextContinuationToken is the token for the next page.
	NextContinuationToken string

	// KeyCount is the number of keys returned.
	KeyCount int
}

// ObjectVersionListResult contains the result of a list object versions operation.
type ObjectVersionListResult struct {
	// Versions is the list of object versions.
	Versions []*domain.ObjectVersion

	// DeleteMarkers is the list of delete markers.
	DeleteMarkers []*domain.ObjectVersion

	// CommonPrefixes contains grouped key prefixes.
	CommonPrefixes []string

	// IsTruncated indicates if there are more results.
	IsTruncated bool

	// NextKeyMarker is the key marker for the next page.
	NextKeyMarker string

	// NextVersionIDMarker is the version ID marker for the next page.
	NextVersionIDMarker string
}

// =============================================================================
// Multipart Upload Repository
// =============================================================================

// MultipartUploadRepository defines the interface for multipart upload data access.
type MultipartUploadRepository interface {
	// Create creates a new multipart upload.
	Create(ctx context.Context, upload *domain.MultipartUpload) error

	// GetByID retrieves a multipart upload by ID.
	GetByID(ctx context.Context, uploadID uuid.UUID) (*domain.MultipartUpload, error)

	// List returns multipart uploads for a bucket.
	List(ctx context.Context, bucketID int64, opts MultipartListOptions) (*MultipartListResult, error)

	// UpdateStatus updates the status of a multipart upload.
	UpdateStatus(ctx context.Context, uploadID uuid.UUID, status domain.MultipartStatus) error

	// Delete deletes a multipart upload.
	Delete(ctx context.Context, uploadID uuid.UUID) error

	// DeleteExpired deletes expired multipart uploads.
	DeleteExpired(ctx context.Context) (int64, error)

	// --- Part operations ---

	// CreatePart creates a new upload part.
	CreatePart(ctx context.Context, part *domain.UploadPart) error

	// GetPart retrieves a specific part.
	GetPart(ctx context.Context, uploadID uuid.UUID, partNumber int) (*domain.UploadPart, error)

	// ListParts returns all parts for an upload.
	ListParts(ctx context.Context, uploadID uuid.UUID, opts PartListOptions) (*PartListResult, error)

	// DeleteParts deletes all parts for an upload.
	DeleteParts(ctx context.Context, uploadID uuid.UUID) error

	// GetPartsForCompletion returns parts in order for completing the upload.
	GetPartsForCompletion(ctx context.Context, uploadID uuid.UUID, partNumbers []int) ([]*domain.UploadPart, error)
}

// MultipartListOptions contains options for listing multipart uploads.
type MultipartListOptions struct {
	// Prefix filters uploads by key prefix.
	Prefix string

	// Delimiter groups keys by delimiter.
	Delimiter string

	// KeyMarker lists uploads after this key.
	KeyMarker string

	// UploadIDMarker lists uploads after this upload ID (within same key).
	UploadIDMarker string

	// MaxUploads is the maximum number of uploads to return.
	MaxUploads int
}

// MultipartListResult contains the result of a list multipart uploads operation.
type MultipartListResult struct {
	// Uploads is the list of multipart uploads.
	Uploads []*domain.MultipartUploadInfo

	// CommonPrefixes contains grouped key prefixes.
	CommonPrefixes []string

	// IsTruncated indicates if there are more results.
	IsTruncated bool

	// NextKeyMarker is the key marker for the next page.
	NextKeyMarker string

	// NextUploadIDMarker is the upload ID marker for the next page.
	NextUploadIDMarker string
}

// PartListOptions contains options for listing parts.
type PartListOptions struct {
	// PartNumberMarker lists parts after this part number.
	PartNumberMarker int

	// MaxParts is the maximum number of parts to return.
	MaxParts int
}

// PartListResult contains the result of a list parts operation.
type PartListResult struct {
	// Parts is the list of parts.
	Parts []*domain.PartInfo

	// IsTruncated indicates if there are more results.
	IsTruncated bool

	// NextPartNumberMarker is the part number marker for the next page.
	NextPartNumberMarker int
}

// =============================================================================
// Common Types
// =============================================================================

// ListOptions contains common pagination options.
type ListOptions struct {
	// Offset is the number of records to skip.
	Offset int

	// Limit is the maximum number of records to return.
	Limit int

	// OrderBy specifies the sort order.
	OrderBy string

	// Descending specifies descending order if true.
	Descending bool
}

// ListResult is a generic paginated list result.
type ListResult[T any] struct {
	// Items is the list of items.
	Items []*T

	// Total is the total number of items (without pagination).
	Total int64

	// Offset is the current offset.
	Offset int

	// Limit is the current limit.
	Limit int
}

// =============================================================================
// Transaction Support
// =============================================================================

// TxManager defines the interface for transaction management.
type TxManager interface {
	// WithTx executes the given function within a transaction.
	// If the function returns an error, the transaction is rolled back.
	// If the function succeeds, the transaction is committed.
	WithTx(ctx context.Context, fn func(ctx context.Context) error) error

	// WithTxOptions executes the given function within a transaction with options.
	WithTxOptions(ctx context.Context, opts TxOptions, fn func(ctx context.Context) error) error
}

// TxOptions contains transaction options.
type TxOptions struct {
	// IsolationLevel specifies the isolation level.
	IsolationLevel string

	// ReadOnly specifies if the transaction is read-only.
	ReadOnly bool
}
