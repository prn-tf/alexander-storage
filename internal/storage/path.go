// Package storage defines interfaces for blob storage backends.
package storage

import (
	"path/filepath"
)

// PathConfig holds configuration for storage path generation.
type PathConfig struct {
	// BasePath is the root directory for blob storage.
	BasePath string

	// ShardLevels is the number of directory levels for sharding.
	// Default: 2 (e.g., /ab/cd/abcdef...)
	ShardLevels int

	// ShardWidth is the number of characters per shard level.
	// Default: 2 (e.g., ab, cd)
	ShardWidth int
}

// DefaultPathConfig returns the default path configuration.
func DefaultPathConfig(basePath string) PathConfig {
	return PathConfig{
		BasePath:    basePath,
		ShardLevels: 2,
		ShardWidth:  2,
	}
}

// ComputePath generates the storage path for a content hash.
// Uses directory sharding to distribute files across directories.
//
// Example with default config (2 levels, 2 chars each):
//
//	hash: "abcdef1234567890..."
//	basePath: "/data"
//	result: "/data/ab/cd/abcdef1234567890..."
func ComputePath(config PathConfig, contentHash string) string {
	// Validate hash length
	minLength := config.ShardLevels * config.ShardWidth
	if len(contentHash) < minLength {
		return filepath.Join(config.BasePath, contentHash)
	}

	// Build path components
	components := make([]string, 0, config.ShardLevels+2)
	components = append(components, config.BasePath)

	// Add shard directories
	offset := 0
	for i := 0; i < config.ShardLevels; i++ {
		components = append(components, contentHash[offset:offset+config.ShardWidth])
		offset += config.ShardWidth
	}

	// Add full hash as filename
	components = append(components, contentHash)

	return filepath.Join(components...)
}

// ComputeDefaultPath generates the storage path using default configuration.
// Convenience function for the common case of 2-level, 2-char sharding.
func ComputeDefaultPath(basePath, contentHash string) string {
	return ComputePath(DefaultPathConfig(basePath), contentHash)
}

// GetShardDirs returns the shard directory components for a hash.
// Useful for creating directory structure before storing.
//
// Example:
//
//	hash: "abcdef..."
//	result: ["ab", "cd"]
func GetShardDirs(config PathConfig, contentHash string) []string {
	minLength := config.ShardLevels * config.ShardWidth
	if len(contentHash) < minLength {
		return nil
	}

	dirs := make([]string, config.ShardLevels)
	offset := 0
	for i := 0; i < config.ShardLevels; i++ {
		dirs[i] = contentHash[offset : offset+config.ShardWidth]
		offset += config.ShardWidth
	}

	return dirs
}

// GetShardPath returns the directory path for a hash (without the filename).
// Useful for checking or creating the directory structure.
//
// Example:
//
//	hash: "abcdef..."
//	basePath: "/data"
//	result: "/data/ab/cd"
func GetShardPath(config PathConfig, contentHash string) string {
	dirs := GetShardDirs(config, contentHash)
	if dirs == nil {
		return config.BasePath
	}

	components := make([]string, 0, len(dirs)+1)
	components = append(components, config.BasePath)
	components = append(components, dirs...)

	return filepath.Join(components...)
}
