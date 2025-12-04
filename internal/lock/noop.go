package lock

import (
	"context"
	"time"
)

// NoOpLocker is a no-operation locker that always succeeds.
// Use this when locking is not needed (e.g., single-threaded tests).
type NoOpLocker struct{}

// NewNoOpLocker creates a new no-op locker.
func NewNoOpLocker() *NoOpLocker {
	return &NoOpLocker{}
}

// Acquire always returns true (lock acquired).
func (n *NoOpLocker) Acquire(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	return true, ctx.Err()
}

// AcquireWithRetry always returns true (lock acquired).
func (n *NoOpLocker) AcquireWithRetry(ctx context.Context, key string, ttl time.Duration, maxRetries int, retryDelay time.Duration) (bool, error) {
	return true, ctx.Err()
}

// Release always returns true (lock released).
func (n *NoOpLocker) Release(ctx context.Context, key string) (bool, error) {
	return true, ctx.Err()
}

// Extend always returns true (lock extended).
func (n *NoOpLocker) Extend(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	return true, ctx.Err()
}

// IsHeld always returns false (no lock held in no-op mode).
func (n *NoOpLocker) IsHeld(ctx context.Context, key string) (bool, error) {
	return false, ctx.Err()
}

// Ensure NoOpLocker implements Locker.
var _ Locker = (*NoOpLocker)(nil)
