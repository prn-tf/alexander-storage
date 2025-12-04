// Package domain contains the core business entities for Alexander Storage.
// These are pure Go structs with no external dependencies, representing
// the fundamental concepts of the object storage system.
package domain

import (
	"time"
)

// User represents a registered user in the system.
// Users own buckets and can have multiple access keys for API authentication.
type User struct {
	// ID is the unique identifier for the user (auto-generated).
	ID int64 `json:"id"`

	// Username is the unique username for login and display.
	// Constraints: 3-255 characters.
	Username string `json:"username"`

	// Email is the unique email address for the user.
	Email string `json:"email"`

	// PasswordHash is the bcrypt hash of the user's password.
	// This should never be exposed in API responses.
	PasswordHash string `json:"-"`

	// IsActive indicates whether the user account is active.
	// Inactive users cannot authenticate or perform any operations.
	IsActive bool `json:"is_active"`

	// IsAdmin indicates whether the user has administrative privileges.
	// Admins can manage other users and perform system-wide operations.
	IsAdmin bool `json:"is_admin"`

	// CreatedAt is the timestamp when the user was created.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is the timestamp when the user was last updated.
	UpdatedAt time.Time `json:"updated_at"`
}

// NewUser creates a new User with default values.
func NewUser(username, email, passwordHash string) *User {
	now := time.Now().UTC()
	return &User{
		Username:     username,
		Email:        email,
		PasswordHash: passwordHash,
		IsActive:     true,
		IsAdmin:      false,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

// CanAuthenticate returns true if the user is allowed to authenticate.
func (u *User) CanAuthenticate() bool {
	return u.IsActive
}
