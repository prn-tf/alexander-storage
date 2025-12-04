// Package domain contains the core business entities for Alexander Storage.
package domain

import (
	"time"
)

// AccessKeyStatus represents the status of an access key.
type AccessKeyStatus string

const (
	// AccessKeyStatusActive indicates the key can be used for authentication.
	AccessKeyStatusActive AccessKeyStatus = "Active"

	// AccessKeyStatusInactive indicates the key is disabled and cannot be used.
	AccessKeyStatusInactive AccessKeyStatus = "Inactive"
)

// AccessKey represents an AWS-style access key for API authentication.
// Each user can have multiple access keys for different applications or use cases.
type AccessKey struct {
	// ID is the unique identifier for the access key record.
	ID int64 `json:"id"`

	// UserID is the ID of the user who owns this access key.
	UserID int64 `json:"user_id"`

	// AccessKeyID is the public identifier (20 characters, AWS-style).
	// Example: "AKIAIOSFODNN7EXAMPLE"
	AccessKeyID string `json:"access_key_id"`

	// EncryptedSecret is the AES-256-GCM encrypted secret key.
	// The plaintext secret is 40 characters (AWS-style).
	// Stored as: base64(nonce || ciphertext || tag)
	EncryptedSecret string `json:"-"`

	// Description is an optional human-readable description of the key's purpose.
	Description string `json:"description,omitempty"`

	// Status indicates whether the key is active or inactive.
	Status AccessKeyStatus `json:"status"`

	// ExpiresAt is the optional expiration time for the key.
	// If nil, the key does not expire.
	ExpiresAt *time.Time `json:"expires_at,omitempty"`

	// CreatedAt is the timestamp when the key was created.
	CreatedAt time.Time `json:"created_at"`

	// LastUsedAt is the timestamp when the key was last used for authentication.
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

// NewAccessKey creates a new AccessKey with default values.
// The accessKeyID and encryptedSecret should be generated using the crypto package.
func NewAccessKey(userID int64, accessKeyID, encryptedSecret string) *AccessKey {
	return &AccessKey{
		UserID:          userID,
		AccessKeyID:     accessKeyID,
		EncryptedSecret: encryptedSecret,
		Status:          AccessKeyStatusActive,
		CreatedAt:       time.Now().UTC(),
	}
}

// IsValid returns true if the access key can be used for authentication.
func (ak *AccessKey) IsValid() bool {
	if ak.Status != AccessKeyStatusActive {
		return false
	}

	if ak.ExpiresAt != nil && time.Now().UTC().After(*ak.ExpiresAt) {
		return false
	}

	return true
}

// IsExpired returns true if the access key has expired.
func (ak *AccessKey) IsExpired() bool {
	if ak.ExpiresAt == nil {
		return false
	}
	return time.Now().UTC().After(*ak.ExpiresAt)
}

// AccessKeyCredentials holds both the access key ID and the plaintext secret.
// This is used when creating a new access key to return credentials to the user.
// The secret should only be shown once and never stored in plaintext.
type AccessKeyCredentials struct {
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
}
