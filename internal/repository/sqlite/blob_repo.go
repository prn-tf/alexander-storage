package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/prn-tf/alexander-storage/internal/domain"
	"github.com/prn-tf/alexander-storage/internal/repository"
)

// blobRepository implements repository.BlobRepository for SQLite.
type blobRepository struct {
	db *DB
}

// NewBlobRepository creates a new SQLite blob repository.
func NewBlobRepository(db *DB) repository.BlobRepository {
	return &blobRepository{db: db}
}

// UpsertWithRefIncrement creates a new blob or increments ref_count if it exists.
// Returns (isNew, error) where isNew indicates if a new blob was created.
func (r *blobRepository) UpsertWithRefIncrement(ctx context.Context, contentHash string, size int64, storagePath string) (bool, error) {
	// SQLite uses INSERT OR REPLACE/INSERT ON CONFLICT
	// We need to first check if exists to determine if new

	var existingRefCount int32
	err := r.db.QueryRowContext(ctx,
		`SELECT ref_count FROM blobs WHERE content_hash = ?`,
		contentHash,
	).Scan(&existingRefCount)

	if err != nil {
		if isNoRows(err) {
			// New blob - insert it
			query := `
				INSERT INTO blobs (content_hash, size, storage_path, ref_count, created_at, last_accessed)
				VALUES (?, ?, ?, 1, ?, ?)
			`
			now := time.Now().UTC().Format(time.RFC3339)
			_, err := r.db.ExecContext(ctx, query, contentHash, size, storagePath, now, now)
			if err != nil {
				return false, fmt.Errorf("failed to insert blob: %w", err)
			}
			return true, nil
		}
		return false, fmt.Errorf("failed to check blob existence: %w", err)
	}

	// Existing blob - increment ref_count
	query := `
		UPDATE blobs
		SET ref_count = ref_count + 1, last_accessed = ?
		WHERE content_hash = ?
	`
	_, err = r.db.ExecContext(ctx, query, time.Now().UTC().Format(time.RFC3339), contentHash)
	if err != nil {
		return false, fmt.Errorf("failed to increment blob ref_count: %w", err)
	}

	return false, nil
}

// GetByHash retrieves a blob by its content hash.
func (r *blobRepository) GetByHash(ctx context.Context, contentHash string) (*domain.Blob, error) {
	query := `
		SELECT content_hash, size, storage_path, ref_count, created_at, last_accessed
		FROM blobs
		WHERE content_hash = ?
	`

	blob := &domain.Blob{}
	var createdAt, lastAccessed string

	err := r.db.QueryRowContext(ctx, query, contentHash).Scan(
		&blob.ContentHash,
		&blob.Size,
		&blob.StoragePath,
		&blob.RefCount,
		&createdAt,
		&lastAccessed,
	)

	if err != nil {
		if isNoRows(err) {
			return nil, domain.ErrBlobNotFound
		}
		return nil, fmt.Errorf("failed to get blob by hash: %w", err)
	}

	blob.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	blob.LastAccessed, _ = time.Parse(time.RFC3339, lastAccessed)

	return blob, nil
}

// IncrementRef atomically increments the reference count.
func (r *blobRepository) IncrementRef(ctx context.Context, contentHash string) error {
	query := `
		UPDATE blobs
		SET ref_count = ref_count + 1
		WHERE content_hash = ?
	`

	result, err := r.db.ExecContext(ctx, query, contentHash)
	if err != nil {
		return fmt.Errorf("failed to increment ref count: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return domain.ErrBlobNotFound
	}

	return nil
}

// DecrementRef atomically decrements the reference count.
// Returns the new reference count.
func (r *blobRepository) DecrementRef(ctx context.Context, contentHash string) (int32, error) {
	// SQLite doesn't support RETURNING in all versions, so we need two queries
	query := `
		UPDATE blobs
		SET ref_count = ref_count - 1
		WHERE content_hash = ?
	`

	result, err := r.db.ExecContext(ctx, query, contentHash)
	if err != nil {
		return 0, fmt.Errorf("failed to decrement ref count: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return 0, domain.ErrBlobNotFound
	}

	// Get the new ref_count
	var newRefCount int32
	err = r.db.QueryRowContext(ctx,
		`SELECT ref_count FROM blobs WHERE content_hash = ?`,
		contentHash,
	).Scan(&newRefCount)
	if err != nil {
		return 0, fmt.Errorf("failed to get new ref count: %w", err)
	}

	return newRefCount, nil
}

// GetRefCount returns the current reference count for a blob.
func (r *blobRepository) GetRefCount(ctx context.Context, contentHash string) (int32, error) {
	var refCount int32
	err := r.db.QueryRowContext(ctx,
		`SELECT ref_count FROM blobs WHERE content_hash = ?`,
		contentHash,
	).Scan(&refCount)
	if err != nil {
		if isNoRows(err) {
			return 0, domain.ErrBlobNotFound
		}
		return 0, fmt.Errorf("failed to get ref count: %w", err)
	}
	return refCount, nil
}

// Exists checks if a blob with the given hash exists.
func (r *blobRepository) Exists(ctx context.Context, contentHash string) (bool, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM blobs WHERE content_hash = ?`,
		contentHash,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check blob existence: %w", err)
	}
	return count > 0, nil
}

// Delete deletes a blob by its content hash.
func (r *blobRepository) Delete(ctx context.Context, contentHash string) error {
	query := `DELETE FROM blobs WHERE content_hash = ? AND ref_count <= 0`

	result, err := r.db.ExecContext(ctx, query, contentHash)
	if err != nil {
		return fmt.Errorf("failed to delete blob: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return domain.ErrBlobNotFound
	}

	return nil
}

// ListOrphans returns blobs with ref_count = 0 that are older than the grace period.
func (r *blobRepository) ListOrphans(ctx context.Context, gracePeriod time.Duration, limit int) ([]*domain.Blob, error) {
	cutoff := time.Now().UTC().Add(-gracePeriod).Format(time.RFC3339)

	query := `
		SELECT content_hash, size, storage_path, ref_count, created_at, last_accessed
		FROM blobs
		WHERE ref_count <= 0 AND created_at < ?
		ORDER BY created_at ASC
		LIMIT ?
	`

	rows, err := r.db.QueryContext(ctx, query, cutoff, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list orphan blobs: %w", err)
	}
	defer rows.Close()

	var blobs []*domain.Blob
	for rows.Next() {
		blob := &domain.Blob{}
		var createdAt, lastAccessed string

		err := rows.Scan(
			&blob.ContentHash,
			&blob.Size,
			&blob.StoragePath,
			&blob.RefCount,
			&createdAt,
			&lastAccessed,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan blob: %w", err)
		}

		blob.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		blob.LastAccessed, _ = time.Parse(time.RFC3339, lastAccessed)

		blobs = append(blobs, blob)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating blobs: %w", err)
	}

	return blobs, nil
}

// DeleteOrphans deletes orphan blobs older than the grace period.
func (r *blobRepository) DeleteOrphans(ctx context.Context, gracePeriod time.Duration, limit int) ([]*domain.Blob, error) {
	// First get the blobs to be deleted
	blobs, err := r.ListOrphans(ctx, gracePeriod, limit)
	if err != nil {
		return nil, err
	}

	if len(blobs) == 0 {
		return blobs, nil
	}

	// Delete them
	for _, blob := range blobs {
		if err := r.Delete(ctx, blob.ContentHash); err != nil {
			// Log but continue
			continue
		}
	}

	return blobs, nil
}

// UpdateLastAccessed updates the last_accessed timestamp.
func (r *blobRepository) UpdateLastAccessed(ctx context.Context, contentHash string) error {
	query := `UPDATE blobs SET last_accessed = ? WHERE content_hash = ?`
	_, err := r.db.ExecContext(ctx, query, time.Now().UTC().Format(time.RFC3339), contentHash)
	if err != nil {
		return fmt.Errorf("failed to update last accessed: %w", err)
	}
	return nil
}

// Ensure blobRepository implements repository.BlobRepository.
var _ repository.BlobRepository = (*blobRepository)(nil)
