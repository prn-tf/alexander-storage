// Package repository defines data access interfaces for Alexander Storage.
package repository

import (
	"context"
	"time"
)

// =============================================================================
// Cache Interface (Redis)
// =============================================================================

// Cache defines the interface for caching operations.
// Primarily implemented using Redis for distributed caching.
type Cache interface {
	// Get retrieves a value by key.
	// Returns ErrCacheMiss if the key doesn't exist.
	Get(ctx context.Context, key string) ([]byte, error)

	// Set stores a value with an optional TTL.
	// If ttl is 0, the value doesn't expire.
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error

	// SetNX sets a value only if the key doesn't exist.
	// Returns true if the value was set, false if the key already exists.
	SetNX(ctx context.Context, key string, value []byte, ttl time.Duration) (bool, error)

	// Delete removes a value by key.
	Delete(ctx context.Context, key string) error

	// Exists checks if a key exists.
	Exists(ctx context.Context, key string) (bool, error)

	// Expire sets or updates the TTL for a key.
	Expire(ctx context.Context, key string, ttl time.Duration) error

	// TTL returns the remaining TTL for a key.
	// Returns -1 if the key doesn't exist, -2 if no TTL is set.
	TTL(ctx context.Context, key string) (time.Duration, error)

	// GetMulti retrieves multiple values by keys.
	GetMulti(ctx context.Context, keys []string) (map[string][]byte, error)

	// SetMulti stores multiple values.
	SetMulti(ctx context.Context, items map[string][]byte, ttl time.Duration) error

	// DeleteMulti removes multiple values.
	DeleteMulti(ctx context.Context, keys ...string) error

	// Increment atomically increments an integer value.
	Increment(ctx context.Context, key string, delta int64) (int64, error)

	// Decrement atomically decrements an integer value.
	Decrement(ctx context.Context, key string, delta int64) (int64, error)
}

// CacheError represents a cache error type.
type CacheError string

const (
	// ErrCacheMiss indicates the key was not found in cache.
	ErrCacheMiss CacheError = "cache miss"

	// ErrCacheUnavailable indicates the cache is unavailable.
	ErrCacheUnavailable CacheError = "cache unavailable"
)

func (e CacheError) Error() string {
	return string(e)
}

// =============================================================================
// Distributed Lock Interface (Redis)
// =============================================================================

// DistributedLock defines the interface for distributed locking.
// Used to coordinate operations across multiple server instances.
type DistributedLock interface {
	// Acquire attempts to acquire a lock.
	// Returns true if the lock was acquired, false if it's held by another process.
	// The lock will automatically expire after the specified TTL.
	Acquire(ctx context.Context, key string, ttl time.Duration) (bool, error)

	// AcquireWithRetry attempts to acquire a lock with retries.
	// Will retry up to maxRetries times with retryDelay between attempts.
	AcquireWithRetry(ctx context.Context, key string, ttl time.Duration, maxRetries int, retryDelay time.Duration) (bool, error)

	// Release releases a lock.
	// Returns true if the lock was released, false if it wasn't held.
	Release(ctx context.Context, key string) (bool, error)

	// Extend extends the TTL of a held lock.
	// Returns true if the lock was extended, false if it's not held.
	Extend(ctx context.Context, key string, ttl time.Duration) (bool, error)

	// IsHeld checks if the lock is currently held.
	IsHeld(ctx context.Context, key string) (bool, error)
}

// Lock is a convenience wrapper for a specific lock instance.
type Lock struct {
	locker DistributedLock
	key    string
	held   bool
}

// NewLock creates a new Lock instance.
func NewLock(locker DistributedLock, key string) *Lock {
	return &Lock{
		locker: locker,
		key:    key,
		held:   false,
	}
}

// Acquire attempts to acquire the lock.
func (l *Lock) Acquire(ctx context.Context, ttl time.Duration) (bool, error) {
	acquired, err := l.locker.Acquire(ctx, l.key, ttl)
	if err != nil {
		return false, err
	}
	l.held = acquired
	return acquired, nil
}

// Release releases the lock.
func (l *Lock) Release(ctx context.Context) error {
	if !l.held {
		return nil
	}
	_, err := l.locker.Release(ctx, l.key)
	l.held = false
	return err
}

// Extend extends the lock TTL.
func (l *Lock) Extend(ctx context.Context, ttl time.Duration) error {
	if !l.held {
		return nil
	}
	extended, err := l.locker.Extend(ctx, l.key, ttl)
	if err != nil {
		return err
	}
	if !extended {
		l.held = false
	}
	return nil
}

// IsHeld returns whether the lock is held.
func (l *Lock) IsHeld() bool {
	return l.held
}

// =============================================================================
// Common Lock Keys
// =============================================================================

// LockKey generates a lock key for common scenarios.
type LockKey struct{}

// ObjectUpload returns a lock key for object upload operations.
// Prevents concurrent uploads to the same object.
func (LockKey) ObjectUpload(bucketID int64, key string) string {
	return "lock:object:upload:" + formatBucketKey(bucketID, key)
}

// MultipartUpload returns a lock key for multipart upload operations.
func (LockKey) MultipartUpload(uploadID string) string {
	return "lock:multipart:" + uploadID
}

// BlobGC returns a lock key for blob garbage collection.
func (LockKey) BlobGC() string {
	return "lock:gc:blob"
}

// MultipartGC returns a lock key for multipart upload cleanup.
func (LockKey) MultipartGC() string {
	return "lock:gc:multipart"
}

// formatBucketKey formats a bucket ID and key into a string.
func formatBucketKey(bucketID int64, key string) string {
	return string(rune(bucketID)) + ":" + key
}

// =============================================================================
// Common Cache Keys
// =============================================================================

// CacheKey generates cache keys for common scenarios.
type CacheKey struct{}

// Bucket returns a cache key for bucket metadata.
func (CacheKey) Bucket(name string) string {
	return "cache:bucket:" + name
}

// BucketByID returns a cache key for bucket metadata by ID.
func (CacheKey) BucketByID(id int64) string {
	return "cache:bucket:id:" + string(rune(id))
}

// AccessKey returns a cache key for access key metadata.
func (CacheKey) AccessKey(accessKeyID string) string {
	return "cache:accesskey:" + accessKeyID
}

// ObjectMeta returns a cache key for object metadata.
func (CacheKey) ObjectMeta(bucketID int64, key string) string {
	return "cache:object:" + formatBucketKey(bucketID, key)
}

// UserByID returns a cache key for user metadata.
func (CacheKey) UserByID(id int64) string {
	return "cache:user:id:" + string(rune(id))
}
