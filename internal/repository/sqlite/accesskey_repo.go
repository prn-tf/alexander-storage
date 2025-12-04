package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/prn-tf/alexander-storage/internal/domain"
	"github.com/prn-tf/alexander-storage/internal/repository"
)

// accessKeyRepository implements repository.AccessKeyRepository for SQLite.
type accessKeyRepository struct {
	db *DB
}

// NewAccessKeyRepository creates a new SQLite access key repository.
func NewAccessKeyRepository(db *DB) repository.AccessKeyRepository {
	return &accessKeyRepository{db: db}
}

// Create creates a new access key.
func (r *accessKeyRepository) Create(ctx context.Context, key *domain.AccessKey) error {
	query := `
		INSERT INTO access_keys (user_id, access_key_id, encrypted_secret, description, status, created_at, expires_at, last_used_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	var expiresAt, lastUsedAt sql.NullString
	if key.ExpiresAt != nil {
		expiresAt = sql.NullString{String: key.ExpiresAt.Format(time.RFC3339), Valid: true}
	}
	if key.LastUsedAt != nil {
		lastUsedAt = sql.NullString{String: key.LastUsedAt.Format(time.RFC3339), Valid: true}
	}

	result, err := r.db.ExecContext(ctx, query,
		key.UserID,
		key.AccessKeyID,
		key.EncryptedSecret,
		key.Description,
		key.Status,
		key.CreatedAt.Format(time.RFC3339),
		expiresAt,
		lastUsedAt,
	)

	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: access key ID already exists", domain.ErrAccessKeyNotFound)
		}
		return fmt.Errorf("failed to create access key: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert ID: %w", err)
	}
	key.ID = id

	return nil
}

// GetByID retrieves an access key by ID.
func (r *accessKeyRepository) GetByID(ctx context.Context, id int64) (*domain.AccessKey, error) {
	query := `
		SELECT id, user_id, access_key_id, encrypted_secret, description, status, created_at, expires_at, last_used_at
		FROM access_keys
		WHERE id = ?
	`
	return r.scanAccessKey(r.db.QueryRowContext(ctx, query, id))
}

// GetByAccessKeyID retrieves an access key by access key ID (20-char identifier).
func (r *accessKeyRepository) GetByAccessKeyID(ctx context.Context, accessKeyID string) (*domain.AccessKey, error) {
	query := `
		SELECT id, user_id, access_key_id, encrypted_secret, description, status, created_at, expires_at, last_used_at
		FROM access_keys
		WHERE access_key_id = ?
	`
	return r.scanAccessKey(r.db.QueryRowContext(ctx, query, accessKeyID))
}

// GetActiveByAccessKeyID retrieves an active, non-expired access key.
func (r *accessKeyRepository) GetActiveByAccessKeyID(ctx context.Context, accessKeyID string) (*domain.AccessKey, error) {
	query := `
		SELECT id, user_id, access_key_id, encrypted_secret, description, status, created_at, expires_at, last_used_at
		FROM access_keys
		WHERE access_key_id = ? 
			AND status = ? 
			AND (expires_at IS NULL OR expires_at > ?)
	`
	return r.scanAccessKey(r.db.QueryRowContext(ctx, query, accessKeyID, domain.AccessKeyStatusActive, time.Now().UTC().Format(time.RFC3339)))
}

// scanAccessKey scans a single access key row.
func (r *accessKeyRepository) scanAccessKey(row *sql.Row) (*domain.AccessKey, error) {
	key := &domain.AccessKey{}
	var createdAt string
	var expiresAt, lastUsedAt sql.NullString
	var description sql.NullString

	err := row.Scan(
		&key.ID,
		&key.UserID,
		&key.AccessKeyID,
		&key.EncryptedSecret,
		&description,
		&key.Status,
		&createdAt,
		&expiresAt,
		&lastUsedAt,
	)

	if err != nil {
		if isNoRows(err) {
			return nil, domain.ErrAccessKeyNotFound
		}
		return nil, fmt.Errorf("failed to scan access key: %w", err)
	}

	key.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	if description.Valid {
		key.Description = description.String
	}
	if expiresAt.Valid {
		t, _ := time.Parse(time.RFC3339, expiresAt.String)
		key.ExpiresAt = &t
	}
	if lastUsedAt.Valid {
		t, _ := time.Parse(time.RFC3339, lastUsedAt.String)
		key.LastUsedAt = &t
	}

	return key, nil
}

// ListByUserID retrieves all access keys for a user.
func (r *accessKeyRepository) ListByUserID(ctx context.Context, userID int64) ([]*domain.AccessKey, error) {
	query := `
		SELECT id, user_id, access_key_id, encrypted_secret, description, status, created_at, expires_at, last_used_at
		FROM access_keys
		WHERE user_id = ?
		ORDER BY created_at DESC
	`

	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list access keys: %w", err)
	}
	defer rows.Close()

	var keys []*domain.AccessKey
	for rows.Next() {
		key := &domain.AccessKey{}
		var createdAt string
		var expiresAt, lastUsedAt, description sql.NullString

		err := rows.Scan(
			&key.ID,
			&key.UserID,
			&key.AccessKeyID,
			&key.EncryptedSecret,
			&description,
			&key.Status,
			&createdAt,
			&expiresAt,
			&lastUsedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan access key: %w", err)
		}

		key.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		if description.Valid {
			key.Description = description.String
		}
		if expiresAt.Valid {
			t, _ := time.Parse(time.RFC3339, expiresAt.String)
			key.ExpiresAt = &t
		}
		if lastUsedAt.Valid {
			t, _ := time.Parse(time.RFC3339, lastUsedAt.String)
			key.LastUsedAt = &t
		}

		keys = append(keys, key)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating access keys: %w", err)
	}

	return keys, nil
}

// Update updates an existing access key.
func (r *accessKeyRepository) Update(ctx context.Context, key *domain.AccessKey) error {
	query := `
		UPDATE access_keys
		SET description = ?, status = ?, expires_at = ?
		WHERE id = ?
	`

	var expiresAt sql.NullString
	if key.ExpiresAt != nil {
		expiresAt = sql.NullString{String: key.ExpiresAt.Format(time.RFC3339), Valid: true}
	}

	result, err := r.db.ExecContext(ctx, query,
		key.Description,
		key.Status,
		expiresAt,
		key.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update access key: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return domain.ErrAccessKeyNotFound
	}

	return nil
}

// UpdateLastUsed updates the last_used_at timestamp.
func (r *accessKeyRepository) UpdateLastUsed(ctx context.Context, id int64) error {
	query := `UPDATE access_keys SET last_used_at = ? WHERE id = ?`
	_, err := r.db.ExecContext(ctx, query, time.Now().UTC().Format(time.RFC3339), id)
	if err != nil {
		return fmt.Errorf("failed to update last used: %w", err)
	}
	return nil
}

// Delete deletes an access key by ID.
func (r *accessKeyRepository) Delete(ctx context.Context, id int64) error {
	query := `DELETE FROM access_keys WHERE id = ?`

	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete access key: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return domain.ErrAccessKeyNotFound
	}

	return nil
}

// DeleteByAccessKeyID deletes an access key by access key ID.
func (r *accessKeyRepository) DeleteByAccessKeyID(ctx context.Context, accessKeyID string) error {
	query := `DELETE FROM access_keys WHERE access_key_id = ?`

	result, err := r.db.ExecContext(ctx, query, accessKeyID)
	if err != nil {
		return fmt.Errorf("failed to delete access key: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return domain.ErrAccessKeyNotFound
	}

	return nil
}

// DeleteExpired deletes all expired access keys.
func (r *accessKeyRepository) DeleteExpired(ctx context.Context) (int64, error) {
	query := `DELETE FROM access_keys WHERE expires_at IS NOT NULL AND expires_at < ?`

	result, err := r.db.ExecContext(ctx, query, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return 0, fmt.Errorf("failed to delete expired access keys: %w", err)
	}

	return result.RowsAffected()
}

// Ensure accessKeyRepository implements repository.AccessKeyRepository.
var _ repository.AccessKeyRepository = (*accessKeyRepository)(nil)
