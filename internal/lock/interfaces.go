// Package lock provides distributed and local locking abstractions.
// For single-node deployments, memory-based locks are used.
// For distributed deployments, Redis-based locks can be used.
package lock

import (
	"context"
	"time"
)

// Locker defines the interface for distributed/local locking.
// This abstraction allows switching between in-memory locks (single-node)
// and Redis-based locks (distributed) without changing business logic.
type Locker interface {
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
	locker Locker
	key    string
	held   bool
}

// NewLock creates a new Lock instance.
func NewLock(locker Locker, key string) *Lock {
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

// Keys provides lock key generation for common scenarios.
var Keys = lockKeys{}

type lockKeys struct{}

// ObjectUpload returns a lock key for object upload operations.
// Prevents concurrent uploads to the same object.
func (lockKeys) ObjectUpload(bucketID int64, key string) string {
	return "lock:object:upload:" + formatBucketKey(bucketID, key)
}

// MultipartUpload returns a lock key for multipart upload operations.
func (lockKeys) MultipartUpload(uploadID string) string {
	return "lock:multipart:" + uploadID
}

// BlobGC returns a lock key for blob garbage collection.
func (lockKeys) BlobGC() string {
	return "lock:gc:blob"
}

// MultipartGC returns a lock key for multipart upload cleanup.
func (lockKeys) MultipartGC() string {
	return "lock:gc:multipart"
}

// formatBucketKey formats a bucket ID and key into a string.
func formatBucketKey(bucketID int64, key string) string {
	return string(rune(bucketID)) + ":" + key
}
