package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/prn-tf/alexander-storage/internal/domain"
	"github.com/prn-tf/alexander-storage/internal/repository"
)

// userRepository implements repository.UserRepository for SQLite.
type userRepository struct {
	db *DB
}

// NewUserRepository creates a new SQLite user repository.
func NewUserRepository(db *DB) repository.UserRepository {
	return &userRepository{db: db}
}

// Create creates a new user.
func (r *userRepository) Create(ctx context.Context, user *domain.User) error {
	query := `
		INSERT INTO users (username, email, password_hash, is_active, is_admin, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`

	result, err := r.db.ExecContext(ctx, query,
		user.Username,
		user.Email,
		user.PasswordHash,
		boolToInt(user.IsActive),
		boolToInt(user.IsAdmin),
		user.CreatedAt.Format(time.RFC3339),
		user.UpdatedAt.Format(time.RFC3339),
	)

	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: username or email already exists", domain.ErrUserAlreadyExists)
		}
		return fmt.Errorf("failed to create user: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert ID: %w", err)
	}
	user.ID = id

	return nil
}

// GetByID retrieves a user by ID.
func (r *userRepository) GetByID(ctx context.Context, id int64) (*domain.User, error) {
	query := `
		SELECT id, username, email, password_hash, is_active, is_admin, created_at, updated_at
		FROM users
		WHERE id = ?
	`

	user := &domain.User{}
	var isActive, isAdmin int
	var createdAt, updatedAt string
	
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.PasswordHash,
		&isActive,
		&isAdmin,
		&createdAt,
		&updatedAt,
	)

	if err != nil {
		if isNoRows(err) {
			return nil, domain.ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to get user by ID: %w", err)
	}

	user.IsActive = isActive != 0
	user.IsAdmin = isAdmin != 0
	user.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	user.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

	return user, nil
}

// GetByUsername retrieves a user by username.
func (r *userRepository) GetByUsername(ctx context.Context, username string) (*domain.User, error) {
	query := `
		SELECT id, username, email, password_hash, is_active, is_admin, created_at, updated_at
		FROM users
		WHERE username = ?
	`

	user := &domain.User{}
	var isActive, isAdmin int
	var createdAt, updatedAt string

	err := r.db.QueryRowContext(ctx, query, username).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.PasswordHash,
		&isActive,
		&isAdmin,
		&createdAt,
		&updatedAt,
	)

	if err != nil {
		if isNoRows(err) {
			return nil, domain.ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to get user by username: %w", err)
	}

	user.IsActive = isActive != 0
	user.IsAdmin = isAdmin != 0
	user.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	user.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

	return user, nil
}

// GetByEmail retrieves a user by email.
func (r *userRepository) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	query := `
		SELECT id, username, email, password_hash, is_active, is_admin, created_at, updated_at
		FROM users
		WHERE email = ?
	`

	user := &domain.User{}
	var isActive, isAdmin int
	var createdAt, updatedAt string

	err := r.db.QueryRowContext(ctx, query, email).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.PasswordHash,
		&isActive,
		&isAdmin,
		&createdAt,
		&updatedAt,
	)

	if err != nil {
		if isNoRows(err) {
			return nil, domain.ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to get user by email: %w", err)
	}

	user.IsActive = isActive != 0
	user.IsAdmin = isAdmin != 0
	user.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	user.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

	return user, nil
}

// Update updates an existing user.
func (r *userRepository) Update(ctx context.Context, user *domain.User) error {
	user.UpdatedAt = time.Now().UTC()

	query := `
		UPDATE users
		SET username = ?, email = ?, password_hash = ?, is_active = ?, is_admin = ?, updated_at = ?
		WHERE id = ?
	`

	result, err := r.db.ExecContext(ctx, query,
		user.Username,
		user.Email,
		user.PasswordHash,
		boolToInt(user.IsActive),
		boolToInt(user.IsAdmin),
		user.UpdatedAt.Format(time.RFC3339),
		user.ID,
	)

	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: username or email already exists", domain.ErrUserAlreadyExists)
		}
		return fmt.Errorf("failed to update user: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return domain.ErrUserNotFound
	}

	return nil
}

// Delete deletes a user by ID.
func (r *userRepository) Delete(ctx context.Context, id int64) error {
	query := `DELETE FROM users WHERE id = ?`

	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return domain.ErrUserNotFound
	}

	return nil
}

// List returns all users with pagination.
func (r *userRepository) List(ctx context.Context, opts repository.ListOptions) (*repository.ListResult[domain.User], error) {
	countQuery := `SELECT COUNT(*) FROM users`
	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery).Scan(&total); err != nil {
		return nil, fmt.Errorf("failed to count users: %w", err)
	}

	query := `
		SELECT id, username, email, password_hash, is_active, is_admin, created_at, updated_at
		FROM users
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`

	rows, err := r.db.QueryContext(ctx, query, opts.Limit, opts.Offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}
	defer rows.Close()

	var users []*domain.User
	for rows.Next() {
		user := &domain.User{}
		var isActive, isAdmin int
		var createdAt, updatedAt string
		
		err := rows.Scan(
			&user.ID,
			&user.Username,
			&user.Email,
			&user.PasswordHash,
			&isActive,
			&isAdmin,
			&createdAt,
			&updatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan user: %w", err)
		}
		
		user.IsActive = isActive != 0
		user.IsAdmin = isAdmin != 0
		user.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		user.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		
		users = append(users, user)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating users: %w", err)
	}

	return &repository.ListResult[domain.User]{
		Items:  users,
		Total:  total,
		Offset: opts.Offset,
		Limit:  opts.Limit,
	}, nil
}

// ExistsByUsername checks if a user with the given username exists.
func (r *userRepository) ExistsByUsername(ctx context.Context, username string) (bool, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE username = ?`, username).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check username existence: %w", err)
	}
	return count > 0, nil
}

// ExistsByEmail checks if a user with the given email exists.
func (r *userRepository) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE email = ?`, email).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check email existence: %w", err)
	}
	return count > 0, nil
}

// boolToInt converts a boolean to an integer (SQLite doesn't have native boolean).
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// scanNullString handles nullable string columns.
func scanNullString(ns sql.NullString) *string {
	if ns.Valid {
		return &ns.String
	}
	return nil
}

// Ensure userRepository implements repository.UserRepository.
var _ repository.UserRepository = (*userRepository)(nil)
