package repository

import "errors"

// Repository errors
var (
	// ErrNotFound indicates the requested entity was not found.
	ErrNotFound = errors.New("not found")
)

// Cache and lock errors
var (
	// ErrCacheMiss indicates the key was not found in cache.
	ErrCacheMiss = errors.New("cache miss")

	// ErrCacheUnavailable indicates the cache is unavailable.
	ErrCacheUnavailable = errors.New("cache unavailable")

	// ErrLockNotAcquired indicates the lock could not be acquired.
	ErrLockNotAcquired = errors.New("lock not acquired")

	// ErrLockNotOwned indicates the operation failed because we don't own the lock.
	ErrLockNotOwned = errors.New("lock not owned")

	// ErrLockExpired indicates the lock has expired.
	ErrLockExpired = errors.New("lock expired")
)
