// Package crypto provides cryptographic utilities for Alexander Storage.
package crypto

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// Character sets for key generation
const (
	// accessKeyChars contains characters used in access key IDs (uppercase alphanumeric).
	accessKeyChars = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

	// secretKeyChars contains characters used in secret keys (alphanumeric + special).
	secretKeyChars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
)

// Key generation errors
var (
	// ErrInvalidHexKey indicates the hex key is malformed or wrong length.
	ErrInvalidHexKey = errors.New("invalid hex key: must be 64 hex characters (32 bytes)")
)

// GenerateAccessKeyID generates a random 20-character access key ID.
// Format: Uppercase alphanumeric, similar to AWS access key IDs.
// Example: "AKIAIOSFODNN7EXAMPLE"
func GenerateAccessKeyID() (string, error) {
	return generateRandomString(AccessKeyIDLength, accessKeyChars)
}

// GenerateSecretKey generates a random 40-character secret key.
// Format: Mixed case alphanumeric with +/, similar to AWS secret keys.
// Example: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
func GenerateSecretKey() (string, error) {
	return generateRandomString(SecretKeyLength, secretKeyChars)
}

// GenerateMasterKey generates a random 32-byte master key for AES-256.
// Returns the key as a 64-character hex string.
func GenerateMasterKey() (string, error) {
	key := make([]byte, KeySize)
	if _, err := rand.Read(key); err != nil {
		return "", fmt.Errorf("failed to generate master key: %w", err)
	}
	return hex.EncodeToString(key), nil
}

// ParseHexKey parses a hex-encoded key string into bytes.
// Expects 64 hex characters (32 bytes).
func ParseHexKey(hexKey string) ([]byte, error) {
	// Remove any whitespace
	hexKey = strings.TrimSpace(hexKey)

	// Validate length
	if len(hexKey) != KeySize*2 {
		return nil, ErrInvalidHexKey
	}

	// Decode hex
	key, err := hex.DecodeString(hexKey)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidHexKey, err)
	}

	return key, nil
}

// generateRandomString generates a random string of the specified length
// using characters from the provided character set.
func generateRandomString(length int, charset string) (string, error) {
	result := make([]byte, length)
	charsetLen := len(charset)

	// Generate random bytes
	randomBytes := make([]byte, length)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	// Map to charset
	for i := 0; i < length; i++ {
		result[i] = charset[int(randomBytes[i])%charsetLen]
	}

	return string(result), nil
}

// GenerateAccessKeyPair generates a new access key ID and secret key pair.
// Returns the access key ID, plaintext secret key, and any error.
func GenerateAccessKeyPair() (accessKeyID, secretKey string, err error) {
	accessKeyID, err = GenerateAccessKeyID()
	if err != nil {
		return "", "", fmt.Errorf("failed to generate access key ID: %w", err)
	}

	secretKey, err = GenerateSecretKey()
	if err != nil {
		return "", "", fmt.Errorf("failed to generate secret key: %w", err)
	}

	return accessKeyID, secretKey, nil
}
