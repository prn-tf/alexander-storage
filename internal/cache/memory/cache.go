// Package memory provides an in-memory cache implementation.
// This is suitable for single-node deployments where Redis is not available.
package memory

import (
	"context"
	"sync"
	"time"

	"github.com/prn-tf/alexander-storage/internal/repository"
)

// Cache implements repository.Cache using in-memory storage.
// This is NOT suitable for distributed deployments.
type Cache struct {
	mu      sync.RWMutex
	items   map[string]*cacheItem
	stopCh  chan struct{}
	stopped bool
}

// cacheItem represents a single cached item.
type cacheItem struct {
	value     []byte
	expiresAt time.Time
	noExpiry  bool
}

// isExpired checks if the item has expired.
func (i *cacheItem) isExpired() bool {
	if i.noExpiry {
		return false
	}
	return time.Now().After(i.expiresAt)
}

// NewCache creates a new in-memory cache.
func NewCache() *Cache {
	c := &Cache{
		items:  make(map[string]*cacheItem),
		stopCh: make(chan struct{}),
	}

	// Start cleanup goroutine.
	go c.cleanupLoop()

	return c
}

// cleanupLoop periodically removes expired items.
func (c *Cache) cleanupLoop() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.cleanup()
		}
	}
}

// cleanup removes expired items.
func (c *Cache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for key, item := range c.items {
		if item.isExpired() {
			delete(c.items, key)
		}
	}
}

// Stop stops the cleanup goroutine.
func (c *Cache) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.stopped {
		close(c.stopCh)
		c.stopped = true
	}
}

// Get retrieves a value by key.
func (c *Cache) Get(ctx context.Context, key string) ([]byte, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	item, exists := c.items[key]
	if !exists {
		return nil, repository.ErrCacheMiss
	}

	if item.isExpired() {
		return nil, repository.ErrCacheMiss
	}

	// Return a copy to prevent mutation.
	result := make([]byte, len(item.value))
	copy(result, item.value)
	return result, nil
}

// Set stores a value with an optional TTL.
func (c *Cache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Make a copy of the value.
	valueCopy := make([]byte, len(value))
	copy(valueCopy, value)

	item := &cacheItem{
		value: valueCopy,
	}

	if ttl > 0 {
		item.expiresAt = time.Now().Add(ttl)
	} else {
		item.noExpiry = true
	}

	c.items[key] = item
	return nil
}

// SetNX sets a value only if the key doesn't exist.
func (c *Cache) SetNX(ctx context.Context, key string, value []byte, ttl time.Duration) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if key exists and is not expired.
	if item, exists := c.items[key]; exists && !item.isExpired() {
		return false, nil
	}

	// Make a copy of the value.
	valueCopy := make([]byte, len(value))
	copy(valueCopy, value)

	item := &cacheItem{
		value: valueCopy,
	}

	if ttl > 0 {
		item.expiresAt = time.Now().Add(ttl)
	} else {
		item.noExpiry = true
	}

	c.items[key] = item
	return true, nil
}

// Delete removes a value by key.
func (c *Cache) Delete(ctx context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.items, key)
	return nil
}

// Exists checks if a key exists.
func (c *Cache) Exists(ctx context.Context, key string) (bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	item, exists := c.items[key]
	if !exists {
		return false, nil
	}

	return !item.isExpired(), nil
}

// Expire sets or updates the TTL for a key.
func (c *Cache) Expire(ctx context.Context, key string, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	item, exists := c.items[key]
	if !exists {
		return nil
	}

	if ttl > 0 {
		item.expiresAt = time.Now().Add(ttl)
		item.noExpiry = false
	} else {
		item.noExpiry = true
	}

	return nil
}

// TTL returns the remaining TTL for a key.
func (c *Cache) TTL(ctx context.Context, key string) (time.Duration, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	item, exists := c.items[key]
	if !exists {
		return -1, nil
	}

	if item.noExpiry {
		return -2, nil
	}

	remaining := time.Until(item.expiresAt)
	if remaining < 0 {
		return -1, nil
	}

	return remaining, nil
}

// GetMulti retrieves multiple values by keys.
func (c *Cache) GetMulti(ctx context.Context, keys []string) (map[string][]byte, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make(map[string][]byte)
	for _, key := range keys {
		item, exists := c.items[key]
		if exists && !item.isExpired() {
			valueCopy := make([]byte, len(item.value))
			copy(valueCopy, item.value)
			result[key] = valueCopy
		}
	}

	return result, nil
}

// SetMulti stores multiple values.
func (c *Cache) SetMulti(ctx context.Context, items map[string][]byte, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for key, value := range items {
		valueCopy := make([]byte, len(value))
		copy(valueCopy, value)

		item := &cacheItem{
			value: valueCopy,
		}

		if ttl > 0 {
			item.expiresAt = time.Now().Add(ttl)
		} else {
			item.noExpiry = true
		}

		c.items[key] = item
	}

	return nil
}

// DeleteMulti removes multiple values.
func (c *Cache) DeleteMulti(ctx context.Context, keys ...string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, key := range keys {
		delete(c.items, key)
	}

	return nil
}

// Increment atomically increments an integer value.
func (c *Cache) Increment(ctx context.Context, key string, delta int64) (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var current int64
	if item, exists := c.items[key]; exists && !item.isExpired() {
		// Parse current value as int64.
		if len(item.value) == 8 {
			current = int64(item.value[0]) | int64(item.value[1])<<8 | int64(item.value[2])<<16 | int64(item.value[3])<<24 |
				int64(item.value[4])<<32 | int64(item.value[5])<<40 | int64(item.value[6])<<48 | int64(item.value[7])<<56
		}
	}

	newValue := current + delta

	// Store as bytes.
	bytes := make([]byte, 8)
	bytes[0] = byte(newValue)
	bytes[1] = byte(newValue >> 8)
	bytes[2] = byte(newValue >> 16)
	bytes[3] = byte(newValue >> 24)
	bytes[4] = byte(newValue >> 32)
	bytes[5] = byte(newValue >> 40)
	bytes[6] = byte(newValue >> 48)
	bytes[7] = byte(newValue >> 56)

	c.items[key] = &cacheItem{
		value:    bytes,
		noExpiry: true,
	}

	return newValue, nil
}

// Decrement atomically decrements an integer value.
func (c *Cache) Decrement(ctx context.Context, key string, delta int64) (int64, error) {
	return c.Increment(ctx, key, -delta)
}

// Ensure Cache implements repository.Cache.
var _ repository.Cache = (*Cache)(nil)
