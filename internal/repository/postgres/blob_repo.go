package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/prn-tf/alexander-storage/internal/domain"
	"github.com/prn-tf/alexander-storage/internal/repository"
)

// blobRepository implements repository.BlobRepository.
type blobRepository struct {
	db *DB
}

// NewBlobRepository creates a new PostgreSQL blob repository.
func NewBlobRepository(db *DB) repository.BlobRepository {
	return &blobRepository{db: db}
}

// UpsertWithRefIncrement creates a new blob or increments ref_count if it exists.
// Returns (isNew, error) where isNew indicates if a new blob was created.
// New blobs are marked as encrypted by default (SSE-S3).
func (r *blobRepository) UpsertWithRefIncrement(ctx context.Context, contentHash string, size int64, storagePath string) (bool, error) {
	// Use PostgreSQL's INSERT ... ON CONFLICT DO UPDATE for atomic upsert
	// New blobs are encrypted by default (is_encrypted = true)
	query := `
		INSERT INTO blobs (content_hash, size, storage_path, ref_count, is_encrypted, created_at)
		VALUES ($1, $2, $3, 1, true, $4)
		ON CONFLICT (content_hash) DO UPDATE
		SET ref_count = blobs.ref_count + 1
		RETURNING (xmax = 0) AS is_new
	`

	var isNew bool
	err := r.db.Pool.QueryRow(ctx, query, contentHash, size, storagePath, time.Now().UTC()).Scan(&isNew)
	if err != nil {
		return false, fmt.Errorf("failed to upsert blob: %w", err)
	}

	return isNew, nil
}

// UpsertEncrypted creates a new encrypted blob or increments ref_count if it exists.
// Returns (isNew, error) where isNew indicates if a new blob was created.
func (r *blobRepository) UpsertEncrypted(ctx context.Context, contentHash string, size int64, storagePath string, encryptionIV string) (bool, error) {
	query := `
		INSERT INTO blobs (content_hash, size, storage_path, ref_count, is_encrypted, encryption_iv, created_at)
		VALUES ($1, $2, $3, 1, true, $4, $5)
		ON CONFLICT (content_hash) DO UPDATE
		SET ref_count = blobs.ref_count + 1
		RETURNING (xmax = 0) AS is_new
	`

	var isNew bool
	err := r.db.Pool.QueryRow(ctx, query, contentHash, size, storagePath, encryptionIV, time.Now().UTC()).Scan(&isNew)
	if err != nil {
		return false, fmt.Errorf("failed to upsert encrypted blob: %w", err)
	}

	return isNew, nil
}

// GetByHash retrieves a blob by its content hash (primary key).
func (r *blobRepository) GetByHash(ctx context.Context, contentHash string) (*domain.Blob, error) {
	query := `
		SELECT content_hash, size, storage_path, ref_count, is_encrypted, created_at, last_accessed
		FROM blobs
		WHERE content_hash = $1
	`

	blob := &domain.Blob{}
	err := r.db.Pool.QueryRow(ctx, query, contentHash).Scan(
		&blob.ContentHash,
		&blob.Size,
		&blob.StoragePath,
		&blob.RefCount,
		&blob.IsEncrypted,
		&blob.CreatedAt,
		&blob.LastAccessed,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrBlobNotFound
		}
		return nil, fmt.Errorf("failed to get blob by hash: %w", err)
	}

	return blob, nil
}

// IncrementRef atomically increments the reference count.
func (r *blobRepository) IncrementRef(ctx context.Context, contentHash string) error {
	query := `
		UPDATE blobs
		SET ref_count = ref_count + 1
		WHERE content_hash = $1
	`

	result, err := r.db.Pool.Exec(ctx, query, contentHash)
	if err != nil {
		return fmt.Errorf("failed to increment ref count: %w", err)
	}

	if result.RowsAffected() == 0 {
		return domain.ErrBlobNotFound
	}

	return nil
}

// DecrementRef atomically decrements the reference count.
// Returns the new reference count.
func (r *blobRepository) DecrementRef(ctx context.Context, contentHash string) (int32, error) {
	query := `
		UPDATE blobs
		SET ref_count = ref_count - 1
		WHERE content_hash = $1
		RETURNING ref_count
	`

	var newRefCount int32
	err := r.db.Pool.QueryRow(ctx, query, contentHash).Scan(&newRefCount)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, domain.ErrBlobNotFound
		}
		return 0, fmt.Errorf("failed to decrement ref count: %w", err)
	}

	return newRefCount, nil
}

// GetRefCount returns the current reference count for a blob.
func (r *blobRepository) GetRefCount(ctx context.Context, contentHash string) (int32, error) {
	var refCount int32
	err := r.db.Pool.QueryRow(ctx, `SELECT ref_count FROM blobs WHERE content_hash = $1`, contentHash).Scan(&refCount)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, domain.ErrBlobNotFound
		}
		return 0, fmt.Errorf("failed to get ref count: %w", err)
	}
	return refCount, nil
}

// Exists checks if a blob with the given hash exists.
func (r *blobRepository) Exists(ctx context.Context, contentHash string) (bool, error) {
	var exists bool
	err := r.db.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM blobs WHERE content_hash = $1)`, contentHash).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check blob existence: %w", err)
	}
	return exists, nil
}

// Delete deletes a blob by its content hash.
func (r *blobRepository) Delete(ctx context.Context, contentHash string) error {
	query := `DELETE FROM blobs WHERE content_hash = $1 AND ref_count <= 0`

	result, err := r.db.Pool.Exec(ctx, query, contentHash)
	if err != nil {
		return fmt.Errorf("failed to delete blob: %w", err)
	}

	if result.RowsAffected() == 0 {
		return domain.ErrBlobNotFound
	}

	return nil
}

// ListOrphans returns blobs with ref_count = 0 that are older than the grace period.
func (r *blobRepository) ListOrphans(ctx context.Context, gracePeriod time.Duration, limit int) ([]*domain.Blob, error) {
	query := `
		SELECT content_hash, size, storage_path, ref_count, is_encrypted, created_at, last_accessed
		FROM blobs
		WHERE ref_count <= 0 AND created_at < $1
		ORDER BY created_at ASC
		LIMIT $2
	`

	cutoff := time.Now().UTC().Add(-gracePeriod)
	rows, err := r.db.Pool.Query(ctx, query, cutoff, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list orphan blobs: %w", err)
	}
	defer rows.Close()

	var blobs []*domain.Blob
	for rows.Next() {
		blob := &domain.Blob{}
		err := rows.Scan(
			&blob.ContentHash,
			&blob.Size,
			&blob.StoragePath,
			&blob.RefCount,
			&blob.IsEncrypted,
			&blob.CreatedAt,
			&blob.LastAccessed,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan blob: %w", err)
		}
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
		return nil, nil
	}

	// Delete them
	query := `
		DELETE FROM blobs
		WHERE ref_count <= 0 AND created_at < $1
	`

	cutoff := time.Now().UTC().Add(-gracePeriod)
	_, err = r.db.Pool.Exec(ctx, query, cutoff)
	if err != nil {
		return nil, fmt.Errorf("failed to delete orphan blobs: %w", err)
	}

	return blobs, nil
}

// UpdateLastAccessed updates the last_accessed timestamp.
func (r *blobRepository) UpdateLastAccessed(ctx context.Context, contentHash string) error {
	query := `UPDATE blobs SET last_accessed = $2 WHERE content_hash = $1`

	result, err := r.db.Pool.Exec(ctx, query, contentHash, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("failed to update last accessed: %w", err)
	}

	if result.RowsAffected() == 0 {
		return domain.ErrBlobNotFound
	}

	return nil
}

// UpdateEncrypted marks a blob as encrypted with the given IV (SSE-S3 migration).
func (r *blobRepository) UpdateEncrypted(ctx context.Context, contentHash string, encryptionIV string) error {
	query := `UPDATE blobs SET is_encrypted = true, encryption_iv = $2 WHERE content_hash = $1`

	result, err := r.db.Pool.Exec(ctx, query, contentHash, encryptionIV)
	if err != nil {
		return fmt.Errorf("failed to update encrypted flag: %w", err)
	}

	if result.RowsAffected() == 0 {
		return domain.ErrBlobNotFound
	}

	return nil
}

// ListUnencrypted returns unencrypted blobs for migration.
func (r *blobRepository) ListUnencrypted(ctx context.Context, limit int) ([]*domain.Blob, error) {
	query := `
		SELECT content_hash, size, storage_path, ref_count, is_encrypted, created_at, last_accessed
		FROM blobs
		WHERE is_encrypted = false
		ORDER BY created_at ASC
		LIMIT $1
	`

	rows, err := r.db.Pool.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list unencrypted blobs: %w", err)
	}
	defer rows.Close()

	var blobs []*domain.Blob
	for rows.Next() {
		blob := &domain.Blob{}
		err := rows.Scan(
			&blob.ContentHash,
			&blob.Size,
			&blob.StoragePath,
			&blob.RefCount,
			&blob.IsEncrypted,
			&blob.CreatedAt,
			&blob.LastAccessed,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan blob: %w", err)
		}
		blobs = append(blobs, blob)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating blobs: %w", err)
	}

	return blobs, nil
}

// IsEncrypted checks if a blob is stored encrypted.
func (r *blobRepository) IsEncrypted(ctx context.Context, contentHash string) (bool, error) {
	var isEncrypted bool
	err := r.db.Pool.QueryRow(ctx, `SELECT is_encrypted FROM blobs WHERE content_hash = $1`, contentHash).Scan(&isEncrypted)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, domain.ErrBlobNotFound
		}
		return false, fmt.Errorf("failed to check encryption status: %w", err)
	}
	return isEncrypted, nil
}

// GetEncryptionStatus returns the encryption status and IV for a blob.
func (r *blobRepository) GetEncryptionStatus(ctx context.Context, contentHash string) (isEncrypted bool, encryptionIV string, err error) {
	var iv *string

	err = r.db.Pool.QueryRow(ctx,
		`SELECT is_encrypted, encryption_iv FROM blobs WHERE content_hash = $1`,
		contentHash,
	).Scan(&isEncrypted, &iv)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, "", domain.ErrBlobNotFound
		}
		return false, "", fmt.Errorf("failed to get encryption status: %w", err)
	}

	if iv != nil {
		encryptionIV = *iv
	}
	return isEncrypted, encryptionIV, nil
}

// Ensure blobRepository implements repository.BlobRepository
var _ repository.BlobRepository = (*blobRepository)(nil)
