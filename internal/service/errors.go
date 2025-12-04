// Package service provides business logic services for Alexander Storage.
package service

import "errors"

// Common service errors.
var (
	// User errors
	ErrUserNotFound       = errors.New("user not found")
	ErrUserAlreadyExists  = errors.New("user already exists")
	ErrUserInactive       = errors.New("user is inactive")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrInvalidPassword    = errors.New("invalid password: must be at least 8 characters")
	ErrInvalidUsername    = errors.New("invalid username: must be 3-255 characters")
	ErrInvalidEmail       = errors.New("invalid email format")

	// Access key errors
	ErrAccessKeyNotFound      = errors.New("access key not found")
	ErrAccessKeyInactive      = errors.New("access key is inactive")
	ErrAccessKeyExpired       = errors.New("access key has expired")
	ErrMaxAccessKeysReached   = errors.New("maximum number of access keys reached")
	ErrAccessKeyAlreadyExists = errors.New("access key already exists")

	// Presigned URL errors
	ErrInvalidExpiration     = errors.New("invalid expiration: must be between 1 second and 7 days")
	ErrPresignedURLExpired   = errors.New("presigned URL has expired")
	ErrInvalidPresignedURL   = errors.New("invalid presigned URL")
	ErrMissingRequiredParams = errors.New("missing required parameters")

	// General errors
	ErrEncryptionFailed = errors.New("encryption failed")
	ErrDecryptionFailed = errors.New("decryption failed")
	ErrInternalError    = errors.New("internal server error")
)
