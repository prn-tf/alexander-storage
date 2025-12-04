package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/prn-tf/alexander-storage/internal/domain"
	"github.com/prn-tf/alexander-storage/internal/repository"
)

// objectRepository implements repository.ObjectRepository for SQLite.
type objectRepository struct {
	db *DB
}

// NewObjectRepository creates a new SQLite object repository.
func NewObjectRepository(db *DB) repository.ObjectRepository {
	return &objectRepository{db: db}
}

// Create creates a new object.
func (r *objectRepository) Create(ctx context.Context, obj *domain.Object) error {
	query := `
		INSERT INTO objects (bucket_id, key, version_id, is_latest, is_delete_marker, 
			content_hash, size, content_type, etag, storage_class, metadata, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	var metadataJSON string
	if obj.Metadata != nil {
		data, _ := json.Marshal(obj.Metadata)
		metadataJSON = string(data)
	} else {
		metadataJSON = "{}"
	}

	result, err := r.db.ExecContext(ctx, query,
		obj.BucketID,
		obj.Key,
		obj.VersionID.String(),
		boolToInt(obj.IsLatest),
		boolToInt(obj.IsDeleteMarker),
		obj.ContentHash,
		obj.Size,
		obj.ContentType,
		obj.ETag,
		obj.StorageClass,
		metadataJSON,
		obj.CreatedAt.Format(time.RFC3339),
	)

	if err != nil {
		return fmt.Errorf("failed to create object: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert ID: %w", err)
	}
	obj.ID = id

	return nil
}

// GetByID retrieves an object by ID.
func (r *objectRepository) GetByID(ctx context.Context, id int64) (*domain.Object, error) {
	query := `
		SELECT id, bucket_id, key, version_id, is_latest, is_delete_marker, 
			content_hash, size, content_type, etag, storage_class, metadata, created_at, deleted_at
		FROM objects
		WHERE id = ?
	`
	return r.scanObject(r.db.QueryRowContext(ctx, query, id))
}

// GetByKey retrieves the latest version of an object by bucket ID and key.
func (r *objectRepository) GetByKey(ctx context.Context, bucketID int64, key string) (*domain.Object, error) {
	query := `
		SELECT id, bucket_id, key, version_id, is_latest, is_delete_marker, 
			content_hash, size, content_type, etag, storage_class, metadata, created_at, deleted_at
		FROM objects
		WHERE bucket_id = ? AND key = ? AND is_latest = 1 AND deleted_at IS NULL
	`
	return r.scanObject(r.db.QueryRowContext(ctx, query, bucketID, key))
}

// GetByKeyAndVersion retrieves a specific version of an object.
func (r *objectRepository) GetByKeyAndVersion(ctx context.Context, bucketID int64, key string, versionID uuid.UUID) (*domain.Object, error) {
	query := `
		SELECT id, bucket_id, key, version_id, is_latest, is_delete_marker, 
			content_hash, size, content_type, etag, storage_class, metadata, created_at, deleted_at
		FROM objects
		WHERE bucket_id = ? AND key = ? AND version_id = ?
	`
	return r.scanObject(r.db.QueryRowContext(ctx, query, bucketID, key, versionID.String()))
}

// scanObject scans a single object row.
func (r *objectRepository) scanObject(row *sql.Row) (*domain.Object, error) {
	obj := &domain.Object{}
	var versionIDStr string
	var isLatest, isDeleteMarker int
	var contentHash sql.NullString
	var etag sql.NullString
	var metadataJSON string
	var createdAt string
	var deletedAt sql.NullString

	err := row.Scan(
		&obj.ID,
		&obj.BucketID,
		&obj.Key,
		&versionIDStr,
		&isLatest,
		&isDeleteMarker,
		&contentHash,
		&obj.Size,
		&obj.ContentType,
		&etag,
		&obj.StorageClass,
		&metadataJSON,
		&createdAt,
		&deletedAt,
	)

	if err != nil {
		if isNoRows(err) {
			return nil, domain.ErrObjectNotFound
		}
		return nil, fmt.Errorf("failed to scan object: %w", err)
	}

	obj.VersionID = uuid.MustParse(versionIDStr)
	obj.IsLatest = isLatest != 0
	obj.IsDeleteMarker = isDeleteMarker != 0
	if contentHash.Valid {
		obj.ContentHash = &contentHash.String
	}
	if etag.Valid {
		obj.ETag = &etag.String
	}
	if metadataJSON != "" {
		json.Unmarshal([]byte(metadataJSON), &obj.Metadata)
	}
	obj.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	if deletedAt.Valid {
		t, _ := time.Parse(time.RFC3339, deletedAt.String)
		obj.DeletedAt = &t
	}

	return obj, nil
}

// List returns objects in a bucket with pagination and optional prefix filtering.
func (r *objectRepository) List(ctx context.Context, bucketID int64, opts repository.ObjectListOptions) (*repository.ObjectListResult, error) {
	maxKeys := opts.MaxKeys
	if maxKeys <= 0 {
		maxKeys = 1000
	}

	query := `
		SELECT key, version_id, is_latest, size, etag, created_at, storage_class
		FROM objects
		WHERE bucket_id = ? AND is_latest = 1 AND deleted_at IS NULL
			AND (? = '' OR key LIKE ? || '%')
			AND (? = '' OR key > ?)
		ORDER BY key ASC
		LIMIT ?
	`

	rows, err := r.db.QueryContext(ctx, query, bucketID, opts.Prefix, opts.Prefix, opts.StartAfter, opts.StartAfter, maxKeys+1)
	if err != nil {
		return nil, fmt.Errorf("failed to list objects: %w", err)
	}
	defer rows.Close()

	var objects []*domain.ObjectInfo
	for rows.Next() {
		obj := &domain.ObjectInfo{}
		var versionIDStr string
		var etag sql.NullString
		var createdAt string

		err := rows.Scan(
			&obj.Key,
			&versionIDStr,
			&obj.IsLatest,
			&obj.Size,
			&etag,
			&createdAt,
			&obj.StorageClass,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan object: %w", err)
		}
		obj.VersionID = versionIDStr
		if etag.Valid {
			obj.ETag = etag.String
		}
		obj.LastModified, _ = time.Parse(time.RFC3339, createdAt)
		objects = append(objects, obj)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating objects: %w", err)
	}

	result := &repository.ObjectListResult{
		KeyCount: len(objects),
	}

	if len(objects) > maxKeys {
		result.IsTruncated = true
		result.NextContinuationToken = objects[maxKeys-1].Key
		result.Objects = objects[:maxKeys]
	} else {
		result.Objects = objects
	}

	return result, nil
}

// ListVersions returns all versions of objects in a bucket.
func (r *objectRepository) ListVersions(ctx context.Context, bucketID int64, opts repository.ObjectListOptions) (*repository.ObjectVersionListResult, error) {
	maxKeys := opts.MaxKeys
	if maxKeys <= 0 {
		maxKeys = 1000
	}

	query := `
		SELECT key, version_id, is_latest, is_delete_marker, size, etag, created_at, storage_class
		FROM objects
		WHERE bucket_id = ? AND deleted_at IS NULL
			AND (? = '' OR key LIKE ? || '%')
			AND (? = '' OR key > ?)
		ORDER BY key ASC, created_at DESC
		LIMIT ?
	`

	rows, err := r.db.QueryContext(ctx, query, bucketID, opts.Prefix, opts.Prefix, opts.StartAfter, opts.StartAfter, maxKeys+1)
	if err != nil {
		return nil, fmt.Errorf("failed to list versions: %w", err)
	}
	defer rows.Close()

	var versions []*domain.ObjectVersion
	var deleteMarkers []*domain.ObjectVersion

	for rows.Next() {
		ver := &domain.ObjectVersion{}
		var versionIDStr string
		var isLatest, isDeleteMarker int
		var etag sql.NullString
		var createdAt string

		err := rows.Scan(
			&ver.Key,
			&versionIDStr,
			&isLatest,
			&isDeleteMarker,
			&ver.Size,
			&etag,
			&createdAt,
			&ver.StorageClass,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan version: %w", err)
		}
		ver.VersionID = versionIDStr
		ver.IsLatest = isLatest != 0
		ver.IsDeleteMarker = isDeleteMarker != 0
		if etag.Valid {
			ver.ETag = etag.String
		}
		ver.LastModified, _ = time.Parse(time.RFC3339, createdAt)

		if ver.IsDeleteMarker {
			deleteMarkers = append(deleteMarkers, ver)
		} else {
			versions = append(versions, ver)
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating versions: %w", err)
	}

	result := &repository.ObjectVersionListResult{
		Versions:      versions,
		DeleteMarkers: deleteMarkers,
	}

	totalCount := len(versions) + len(deleteMarkers)
	if totalCount > maxKeys {
		result.IsTruncated = true
	}

	return result, nil
}

// Update updates an existing object.
func (r *objectRepository) Update(ctx context.Context, obj *domain.Object) error {
	var metadataJSON string
	if obj.Metadata != nil {
		data, _ := json.Marshal(obj.Metadata)
		metadataJSON = string(data)
	} else {
		metadataJSON = "{}"
	}

	query := `
		UPDATE objects
		SET content_type = ?, metadata = ?, storage_class = ?
		WHERE id = ?
	`

	result, err := r.db.ExecContext(ctx, query,
		obj.ContentType,
		metadataJSON,
		obj.StorageClass,
		obj.ID,
	)

	if err != nil {
		return fmt.Errorf("failed to update object: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return domain.ErrObjectNotFound
	}

	return nil
}

// MarkNotLatest marks an object as not the latest version.
func (r *objectRepository) MarkNotLatest(ctx context.Context, bucketID int64, key string) error {
	query := `
		UPDATE objects
		SET is_latest = 0
		WHERE bucket_id = ? AND key = ? AND is_latest = 1
	`

	_, err := r.db.ExecContext(ctx, query, bucketID, key)
	if err != nil {
		return fmt.Errorf("failed to mark as not latest: %w", err)
	}

	return nil
}

// Delete soft-deletes an object by ID.
func (r *objectRepository) Delete(ctx context.Context, id int64) error {
	query := `UPDATE objects SET deleted_at = ? WHERE id = ?`

	result, err := r.db.ExecContext(ctx, query, time.Now().UTC().Format(time.RFC3339), id)
	if err != nil {
		return fmt.Errorf("failed to delete object: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return domain.ErrObjectNotFound
	}

	return nil
}

// DeleteAllVersions deletes all versions of an object.
func (r *objectRepository) DeleteAllVersions(ctx context.Context, bucketID int64, key string) error {
	query := `UPDATE objects SET deleted_at = ? WHERE bucket_id = ? AND key = ?`

	_, err := r.db.ExecContext(ctx, query, time.Now().UTC().Format(time.RFC3339), bucketID, key)
	if err != nil {
		return fmt.Errorf("failed to delete all versions: %w", err)
	}

	return nil
}

// CountByBucket returns the number of objects in a bucket.
func (r *objectRepository) CountByBucket(ctx context.Context, bucketID int64) (int64, error) {
	var count int64
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM objects WHERE bucket_id = ? AND is_latest = 1 AND deleted_at IS NULL`,
		bucketID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count objects: %w", err)
	}
	return count, nil
}

// GetContentHashForVersion retrieves the content hash for a specific version.
func (r *objectRepository) GetContentHashForVersion(ctx context.Context, bucketID int64, key string, versionID uuid.UUID) (*string, error) {
	var contentHash sql.NullString
	err := r.db.QueryRowContext(ctx,
		`SELECT content_hash FROM objects WHERE bucket_id = ? AND key = ? AND version_id = ?`,
		bucketID, key, versionID.String(),
	).Scan(&contentHash)
	if err != nil {
		if isNoRows(err) {
			return nil, domain.ErrObjectNotFound
		}
		return nil, fmt.Errorf("failed to get content hash: %w", err)
	}
	if contentHash.Valid {
		return &contentHash.String, nil
	}
	return nil, nil
}

// Ensure objectRepository implements repository.ObjectRepository.
var _ repository.ObjectRepository = (*objectRepository)(nil)
