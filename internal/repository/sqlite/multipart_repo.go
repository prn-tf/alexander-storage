package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/prn-tf/alexander-storage/internal/domain"
	"github.com/prn-tf/alexander-storage/internal/repository"
)

// multipartRepository implements repository.MultipartUploadRepository for SQLite.
type multipartRepository struct {
	db *DB
}

// NewMultipartRepository creates a new SQLite multipart repository.
func NewMultipartRepository(db *DB) repository.MultipartUploadRepository {
	return &multipartRepository{db: db}
}

// Create creates a new multipart upload.
func (r *multipartRepository) Create(ctx context.Context, upload *domain.MultipartUpload) error {
	query := `
		INSERT INTO multipart_uploads (id, bucket_id, key, initiator_id, status, storage_class, metadata, initiated_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	var metadataJSON string
	if upload.Metadata != nil {
		data, _ := json.Marshal(upload.Metadata)
		metadataJSON = string(data)
	} else {
		metadataJSON = "{}"
	}

	_, err := r.db.ExecContext(ctx, query,
		upload.ID.String(),
		upload.BucketID,
		upload.Key,
		upload.InitiatorID,
		upload.Status,
		upload.StorageClass,
		metadataJSON,
		upload.InitiatedAt.Format(time.RFC3339),
		upload.ExpiresAt.Format(time.RFC3339),
	)

	if err != nil {
		return fmt.Errorf("failed to create multipart upload: %w", err)
	}

	return nil
}

// GetByID retrieves a multipart upload by ID.
func (r *multipartRepository) GetByID(ctx context.Context, uploadID uuid.UUID) (*domain.MultipartUpload, error) {
	query := `
		SELECT id, bucket_id, key, initiator_id, status, storage_class, metadata, initiated_at, expires_at, completed_at
		FROM multipart_uploads
		WHERE id = ?
	`

	upload := &domain.MultipartUpload{}
	var idStr string
	var metadataJSON string
	var initiatedAt, expiresAt string
	var completedAt sql.NullString

	err := r.db.QueryRowContext(ctx, query, uploadID.String()).Scan(
		&idStr,
		&upload.BucketID,
		&upload.Key,
		&upload.InitiatorID,
		&upload.Status,
		&upload.StorageClass,
		&metadataJSON,
		&initiatedAt,
		&expiresAt,
		&completedAt,
	)

	if err != nil {
		if isNoRows(err) {
			return nil, domain.ErrMultipartUploadNotFound
		}
		return nil, fmt.Errorf("failed to get multipart upload: %w", err)
	}

	upload.ID = uuid.MustParse(idStr)
	upload.InitiatedAt, _ = time.Parse(time.RFC3339, initiatedAt)
	upload.ExpiresAt, _ = time.Parse(time.RFC3339, expiresAt)
	if completedAt.Valid {
		t, _ := time.Parse(time.RFC3339, completedAt.String)
		upload.CompletedAt = &t
	}
	if metadataJSON != "" {
		json.Unmarshal([]byte(metadataJSON), &upload.Metadata)
	}

	return upload, nil
}

// List returns multipart uploads for a bucket.
func (r *multipartRepository) List(ctx context.Context, bucketID int64, opts repository.MultipartListOptions) (*repository.MultipartListResult, error) {
	maxUploads := opts.MaxUploads
	if maxUploads <= 0 {
		maxUploads = 1000
	}

	query := `
		SELECT id, key, initiated_at, storage_class
		FROM multipart_uploads
		WHERE bucket_id = ? AND status = ?
			AND (? = '' OR key LIKE ? || '%')
			AND (? = '' OR key > ?)
		ORDER BY key ASC, initiated_at ASC
		LIMIT ?
	`

	rows, err := r.db.QueryContext(ctx, query,
		bucketID,
		domain.MultipartStatusInProgress,
		opts.Prefix, opts.Prefix,
		opts.KeyMarker, opts.KeyMarker,
		maxUploads+1,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list uploads: %w", err)
	}
	defer rows.Close()

	var uploads []*domain.MultipartUploadInfo
	for rows.Next() {
		info := &domain.MultipartUploadInfo{}
		var uploadIDStr string
		var initiatedAt string

		err := rows.Scan(
			&uploadIDStr,
			&info.Key,
			&initiatedAt,
			&info.StorageClass,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan upload: %w", err)
		}
		info.UploadID = uploadIDStr
		info.Initiated, _ = time.Parse(time.RFC3339, initiatedAt)
		uploads = append(uploads, info)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating uploads: %w", err)
	}

	result := &repository.MultipartListResult{}

	if len(uploads) > maxUploads {
		result.IsTruncated = true
		result.NextKeyMarker = uploads[maxUploads-1].Key
		result.NextUploadIDMarker = uploads[maxUploads-1].UploadID
		result.Uploads = uploads[:maxUploads]
	} else {
		result.Uploads = uploads
	}

	return result, nil
}

// UpdateStatus updates the status of a multipart upload.
func (r *multipartRepository) UpdateStatus(ctx context.Context, uploadID uuid.UUID, status domain.MultipartStatus) error {
	var query string
	var err error

	if status == domain.MultipartStatusCompleted {
		query = `UPDATE multipart_uploads SET status = ?, completed_at = ? WHERE id = ?`
		_, err = r.db.ExecContext(ctx, query, status, time.Now().UTC().Format(time.RFC3339), uploadID.String())
	} else {
		query = `UPDATE multipart_uploads SET status = ? WHERE id = ?`
		_, err = r.db.ExecContext(ctx, query, status, uploadID.String())
	}

	if err != nil {
		return fmt.Errorf("failed to update upload status: %w", err)
	}

	return nil
}

// Delete deletes a multipart upload.
func (r *multipartRepository) Delete(ctx context.Context, uploadID uuid.UUID) error {
	return r.db.WithTx(ctx, func(tx *sql.Tx) error {
		// Delete parts first
		_, err := tx.ExecContext(ctx, `DELETE FROM upload_parts WHERE upload_id = ?`, uploadID.String())
		if err != nil {
			return fmt.Errorf("failed to delete upload parts: %w", err)
		}

		// Delete upload
		result, err := tx.ExecContext(ctx, `DELETE FROM multipart_uploads WHERE id = ?`, uploadID.String())
		if err != nil {
			return fmt.Errorf("failed to delete upload: %w", err)
		}

		rowsAffected, _ := result.RowsAffected()
		if rowsAffected == 0 {
			return domain.ErrMultipartUploadNotFound
		}

		return nil
	})
}

// DeleteExpired deletes expired multipart uploads.
func (r *multipartRepository) DeleteExpired(ctx context.Context) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)

	// First delete parts for expired uploads
	_, err := r.db.ExecContext(ctx, `
		DELETE FROM upload_parts 
		WHERE upload_id IN (
			SELECT id FROM multipart_uploads 
			WHERE status = ? AND expires_at < ?
		)
	`, domain.MultipartStatusInProgress, now)
	if err != nil {
		return 0, fmt.Errorf("failed to delete expired parts: %w", err)
	}

	// Then delete expired uploads
	result, err := r.db.ExecContext(ctx, `
		DELETE FROM multipart_uploads 
		WHERE status = ? AND expires_at < ?
	`, domain.MultipartStatusInProgress, now)
	if err != nil {
		return 0, fmt.Errorf("failed to delete expired uploads: %w", err)
	}

	return result.RowsAffected()
}

// CreatePart creates a new upload part.
func (r *multipartRepository) CreatePart(ctx context.Context, part *domain.UploadPart) error {
	// SQLite uses INSERT OR REPLACE for upsert
	query := `
		INSERT INTO upload_parts (upload_id, part_number, content_hash, size, etag, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(upload_id, part_number) DO UPDATE SET
			content_hash = excluded.content_hash,
			size = excluded.size,
			etag = excluded.etag,
			created_at = excluded.created_at
	`

	result, err := r.db.ExecContext(ctx, query,
		part.UploadID.String(),
		part.PartNumber,
		part.ContentHash,
		part.Size,
		part.ETag,
		part.CreatedAt.Format(time.RFC3339),
	)

	if err != nil {
		return fmt.Errorf("failed to create upload part: %w", err)
	}

	id, _ := result.LastInsertId()
	part.ID = id

	return nil
}

// GetPart retrieves a specific part.
func (r *multipartRepository) GetPart(ctx context.Context, uploadID uuid.UUID, partNumber int) (*domain.UploadPart, error) {
	query := `
		SELECT id, upload_id, part_number, content_hash, size, etag, created_at
		FROM upload_parts
		WHERE upload_id = ? AND part_number = ?
	`

	part := &domain.UploadPart{}
	var uploadIDStr string
	var createdAt string

	err := r.db.QueryRowContext(ctx, query, uploadID.String(), partNumber).Scan(
		&part.ID,
		&uploadIDStr,
		&part.PartNumber,
		&part.ContentHash,
		&part.Size,
		&part.ETag,
		&createdAt,
	)

	if err != nil {
		if isNoRows(err) {
			return nil, domain.ErrPartNotFound
		}
		return nil, fmt.Errorf("failed to get upload part: %w", err)
	}

	part.UploadID = uuid.MustParse(uploadIDStr)
	part.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)

	return part, nil
}

// ListParts returns all parts for an upload.
func (r *multipartRepository) ListParts(ctx context.Context, uploadID uuid.UUID, opts repository.PartListOptions) (*repository.PartListResult, error) {
	maxParts := opts.MaxParts
	if maxParts <= 0 {
		maxParts = 1000
	}

	query := `
		SELECT part_number, size, etag, created_at
		FROM upload_parts
		WHERE upload_id = ? AND part_number > ?
		ORDER BY part_number ASC
		LIMIT ?
	`

	rows, err := r.db.QueryContext(ctx, query, uploadID.String(), opts.PartNumberMarker, maxParts+1)
	if err != nil {
		return nil, fmt.Errorf("failed to list parts: %w", err)
	}
	defer rows.Close()

	var parts []*domain.PartInfo
	for rows.Next() {
		part := &domain.PartInfo{}
		var createdAt string

		err := rows.Scan(
			&part.PartNumber,
			&part.Size,
			&part.ETag,
			&createdAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan part: %w", err)
		}
		part.LastModified, _ = time.Parse(time.RFC3339, createdAt)
		parts = append(parts, part)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating parts: %w", err)
	}

	result := &repository.PartListResult{}

	if len(parts) > maxParts {
		result.IsTruncated = true
		result.NextPartNumberMarker = parts[maxParts-1].PartNumber
		result.Parts = parts[:maxParts]
	} else {
		result.Parts = parts
	}

	return result, nil
}

// DeleteParts deletes all parts for an upload.
func (r *multipartRepository) DeleteParts(ctx context.Context, uploadID uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM upload_parts WHERE upload_id = ?`, uploadID.String())
	if err != nil {
		return fmt.Errorf("failed to delete parts: %w", err)
	}
	return nil
}

// GetPartsForCompletion returns parts in order for completing the upload.
func (r *multipartRepository) GetPartsForCompletion(ctx context.Context, uploadID uuid.UUID, partNumbers []int) ([]*domain.UploadPart, error) {
	if len(partNumbers) == 0 {
		return []*domain.UploadPart{}, nil
	}

	// Build IN clause for SQLite (doesn't support ANY)
	placeholders := make([]string, len(partNumbers))
	args := make([]interface{}, len(partNumbers)+1)
	args[0] = uploadID.String()
	for i, pn := range partNumbers {
		placeholders[i] = "?"
		args[i+1] = pn
	}

	query := fmt.Sprintf(`
		SELECT id, upload_id, part_number, content_hash, size, etag, created_at
		FROM upload_parts
		WHERE upload_id = ? AND part_number IN (%s)
		ORDER BY part_number ASC
	`, strings.Join(placeholders, ","))

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get parts for completion: %w", err)
	}
	defer rows.Close()

	var parts []*domain.UploadPart
	for rows.Next() {
		part := &domain.UploadPart{}
		var uploadIDStr string
		var createdAt string

		err := rows.Scan(
			&part.ID,
			&uploadIDStr,
			&part.PartNumber,
			&part.ContentHash,
			&part.Size,
			&part.ETag,
			&createdAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan part: %w", err)
		}

		part.UploadID = uuid.MustParse(uploadIDStr)
		part.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		parts = append(parts, part)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating parts: %w", err)
	}

	return parts, nil
}

// Ensure multipartRepository implements repository.MultipartUploadRepository.
var _ repository.MultipartUploadRepository = (*multipartRepository)(nil)
