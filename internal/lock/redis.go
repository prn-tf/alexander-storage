// Package lock provides distributed and local locking abstractions.
package lock

import (
	"context"
	"time"

	"github.com/prn-tf/alexander-storage/internal/repository"
)

// RedisLocker implements Locker using Redis distributed lock.
// This wraps the repository.DistributedLock interface to implement lock.Locker.
type RedisLocker struct {
	distributedLock repository.DistributedLock
}

// NewRedisLocker creates a new RedisLocker wrapping a DistributedLock implementation.
func NewRedisLocker(dl repository.DistributedLock) *RedisLocker {
	return &RedisLocker{
		distributedLock: dl,
	}
}

// Acquire attempts to acquire a lock.
// Returns true if the lock was acquired, false if it's held by another process.
func (l *RedisLocker) Acquire(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	return l.distributedLock.Acquire(ctx, key, ttl)
}

// AcquireWithRetry attempts to acquire a lock with retries.
func (l *RedisLocker) AcquireWithRetry(ctx context.Context, key string, ttl time.Duration, maxRetries int, retryDelay time.Duration) (bool, error) {
	return l.distributedLock.AcquireWithRetry(ctx, key, ttl, maxRetries, retryDelay)
}

// Release releases a lock.
func (l *RedisLocker) Release(ctx context.Context, key string) (bool, error) {
	return l.distributedLock.Release(ctx, key)
}

// Extend extends the TTL of a held lock.
func (l *RedisLocker) Extend(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	return l.distributedLock.Extend(ctx, key, ttl)
}

// IsHeld checks if the lock is currently held.
func (l *RedisLocker) IsHeld(ctx context.Context, key string) (bool, error) {
	return l.distributedLock.IsHeld(ctx, key)
}

// Ensure RedisLocker implements Locker
var _ Locker = (*RedisLocker)(nil)
