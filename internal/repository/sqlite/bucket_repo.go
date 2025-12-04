package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/prn-tf/alexander-storage/internal/domain"
	"github.com/prn-tf/alexander-storage/internal/repository"
)

// bucketRepository implements repository.BucketRepository for SQLite.
type bucketRepository struct {
	db *DB
}

// NewBucketRepository creates a new SQLite bucket repository.
func NewBucketRepository(db *DB) repository.BucketRepository {
	return &bucketRepository{db: db}
}

// Create creates a new bucket.
func (r *bucketRepository) Create(ctx context.Context, bucket *domain.Bucket) error {
	query := `
		INSERT INTO buckets (owner_id, name, region, versioning, object_lock, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`

	result, err := r.db.ExecContext(ctx, query,
		bucket.OwnerID,
		bucket.Name,
		bucket.Region,
		bucket.Versioning,
		boolToInt(bucket.ObjectLock),
		bucket.CreatedAt.Format(time.RFC3339),
	)

	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: %s", domain.ErrBucketAlreadyExists, bucket.Name)
		}
		return fmt.Errorf("failed to create bucket: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert ID: %w", err)
	}
	bucket.ID = id

	return nil
}

// GetByID retrieves a bucket by ID.
func (r *bucketRepository) GetByID(ctx context.Context, id int64) (*domain.Bucket, error) {
	query := `
		SELECT id, owner_id, name, region, versioning, object_lock, created_at
		FROM buckets
		WHERE id = ?
	`

	bucket := &domain.Bucket{}
	var objectLock int
	var createdAt string

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&bucket.ID,
		&bucket.OwnerID,
		&bucket.Name,
		&bucket.Region,
		&bucket.Versioning,
		&objectLock,
		&createdAt,
	)

	if err != nil {
		if isNoRows(err) {
			return nil, domain.ErrBucketNotFound
		}
		return nil, fmt.Errorf("failed to get bucket by ID: %w", err)
	}

	bucket.ObjectLock = objectLock != 0
	bucket.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)

	return bucket, nil
}

// GetByName retrieves a bucket by name.
func (r *bucketRepository) GetByName(ctx context.Context, name string) (*domain.Bucket, error) {
	query := `
		SELECT id, owner_id, name, region, versioning, object_lock, created_at
		FROM buckets
		WHERE name = ?
	`

	bucket := &domain.Bucket{}
	var objectLock int
	var createdAt string

	err := r.db.QueryRowContext(ctx, query, name).Scan(
		&bucket.ID,
		&bucket.OwnerID,
		&bucket.Name,
		&bucket.Region,
		&bucket.Versioning,
		&objectLock,
		&createdAt,
	)

	if err != nil {
		if isNoRows(err) {
			return nil, domain.ErrBucketNotFound
		}
		return nil, fmt.Errorf("failed to get bucket by name: %w", err)
	}

	bucket.ObjectLock = objectLock != 0
	bucket.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)

	return bucket, nil
}

// List returns all buckets for a user (or all if userID is 0).
func (r *bucketRepository) List(ctx context.Context, userID int64) ([]*domain.Bucket, error) {
	var query string
	var args []interface{}

	if userID > 0 {
		query = `
			SELECT id, owner_id, name, region, versioning, object_lock, created_at
			FROM buckets
			WHERE owner_id = ?
			ORDER BY name ASC
		`
		args = []interface{}{userID}
	} else {
		query = `
			SELECT id, owner_id, name, region, versioning, object_lock, created_at
			FROM buckets
			ORDER BY name ASC
		`
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list buckets: %w", err)
	}
	defer rows.Close()

	var buckets []*domain.Bucket
	for rows.Next() {
		bucket := &domain.Bucket{}
		var objectLock int
		var createdAt string

		err := rows.Scan(
			&bucket.ID,
			&bucket.OwnerID,
			&bucket.Name,
			&bucket.Region,
			&bucket.Versioning,
			&objectLock,
			&createdAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan bucket: %w", err)
		}

		bucket.ObjectLock = objectLock != 0
		bucket.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)

		buckets = append(buckets, bucket)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating buckets: %w", err)
	}

	return buckets, nil
}

// Update updates an existing bucket.
func (r *bucketRepository) Update(ctx context.Context, bucket *domain.Bucket) error {
	query := `
		UPDATE buckets
		SET versioning = ?, object_lock = ?
		WHERE id = ?
	`

	result, err := r.db.ExecContext(ctx, query,
		bucket.Versioning,
		boolToInt(bucket.ObjectLock),
		bucket.ID,
	)

	if err != nil {
		return fmt.Errorf("failed to update bucket: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return domain.ErrBucketNotFound
	}

	return nil
}

// UpdateVersioning updates the versioning status of a bucket.
func (r *bucketRepository) UpdateVersioning(ctx context.Context, id int64, status domain.VersioningStatus) error {
	query := `UPDATE buckets SET versioning = ? WHERE id = ?`

	result, err := r.db.ExecContext(ctx, query, status, id)
	if err != nil {
		return fmt.Errorf("failed to update versioning status: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return domain.ErrBucketNotFound
	}

	return nil
}

// Delete deletes a bucket by ID.
func (r *bucketRepository) Delete(ctx context.Context, id int64) error {
	query := `DELETE FROM buckets WHERE id = ?`

	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete bucket: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return domain.ErrBucketNotFound
	}

	return nil
}

// DeleteByName deletes a bucket by name.
func (r *bucketRepository) DeleteByName(ctx context.Context, name string) error {
	query := `DELETE FROM buckets WHERE name = ?`

	result, err := r.db.ExecContext(ctx, query, name)
	if err != nil {
		return fmt.Errorf("failed to delete bucket: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return domain.ErrBucketNotFound
	}

	return nil
}

// ExistsByName checks if a bucket with the given name exists.
func (r *bucketRepository) ExistsByName(ctx context.Context, name string) (bool, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM buckets WHERE name = ?`, name).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check bucket existence: %w", err)
	}
	return count > 0, nil
}

// IsEmpty checks if a bucket contains any objects.
func (r *bucketRepository) IsEmpty(ctx context.Context, id int64) (bool, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM objects WHERE bucket_id = ? LIMIT 1`, id).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check if bucket is empty: %w", err)
	}
	return count == 0, nil
}

// Ensure bucketRepository implements repository.BucketRepository.
var _ repository.BucketRepository = (*bucketRepository)(nil)
