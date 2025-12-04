package lock

import (
	"context"
	"sync"
	"time"
)

// MemoryLocker implements Locker using in-memory locks.
// This is suitable for single-node deployments where distributed locking is not needed.
// The locks are NOT shared across process restarts or multiple instances.
type MemoryLocker struct {
	mu    sync.Mutex
	locks map[string]*lockEntry
}

// lockEntry represents a single lock.
type lockEntry struct {
	expiresAt time.Time
	token     string
}

// NewMemoryLocker creates a new in-memory locker.
func NewMemoryLocker() *MemoryLocker {
	ml := &MemoryLocker{
		locks: make(map[string]*lockEntry),
	}

	// Start a background goroutine to clean up expired locks.
	go ml.cleanupLoop()

	return ml
}

// cleanupLoop periodically removes expired locks.
func (m *MemoryLocker) cleanupLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		m.cleanup()
	}
}

// cleanup removes expired locks.
func (m *MemoryLocker) cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for key, entry := range m.locks {
		if now.After(entry.expiresAt) {
			delete(m.locks, key)
		}
	}
}

// generateToken creates a unique token for lock ownership.
func generateToken() string {
	return time.Now().Format(time.RFC3339Nano)
}

// Acquire attempts to acquire a lock.
func (m *MemoryLocker) Acquire(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	if ctx.Err() != nil {
		return false, ctx.Err()
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()

	// Check if lock exists and is not expired.
	if entry, exists := m.locks[key]; exists {
		if now.Before(entry.expiresAt) {
			// Lock is held by someone else.
			return false, nil
		}
		// Lock expired, we can take it.
	}

	// Acquire the lock.
	m.locks[key] = &lockEntry{
		expiresAt: now.Add(ttl),
		token:     generateToken(),
	}

	return true, nil
}

// AcquireWithRetry attempts to acquire a lock with retries.
func (m *MemoryLocker) AcquireWithRetry(ctx context.Context, key string, ttl time.Duration, maxRetries int, retryDelay time.Duration) (bool, error) {
	for i := 0; i <= maxRetries; i++ {
		acquired, err := m.Acquire(ctx, key, ttl)
		if err != nil {
			return false, err
		}
		if acquired {
			return true, nil
		}

		// Don't sleep on the last attempt.
		if i < maxRetries {
			select {
			case <-ctx.Done():
				return false, ctx.Err()
			case <-time.After(retryDelay):
				// Continue to next attempt.
			}
		}
	}
	return false, nil
}

// Release releases a lock.
func (m *MemoryLocker) Release(ctx context.Context, key string) (bool, error) {
	if ctx.Err() != nil {
		return false, ctx.Err()
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.locks[key]; exists {
		delete(m.locks, key)
		return true, nil
	}

	return false, nil
}

// Extend extends the TTL of a held lock.
func (m *MemoryLocker) Extend(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	if ctx.Err() != nil {
		return false, ctx.Err()
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	entry, exists := m.locks[key]
	if !exists {
		return false, nil
	}

	// Check if lock has expired.
	if time.Now().After(entry.expiresAt) {
		delete(m.locks, key)
		return false, nil
	}

	// Extend the lock.
	entry.expiresAt = time.Now().Add(ttl)
	return true, nil
}

// IsHeld checks if a lock is currently held.
func (m *MemoryLocker) IsHeld(ctx context.Context, key string) (bool, error) {
	if ctx.Err() != nil {
		return false, ctx.Err()
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	entry, exists := m.locks[key]
	if !exists {
		return false, nil
	}

	// Check if lock has expired.
	if time.Now().After(entry.expiresAt) {
		delete(m.locks, key)
		return false, nil
	}

	return true, nil
}

// Ensure MemoryLocker implements Locker.
var _ Locker = (*MemoryLocker)(nil)
