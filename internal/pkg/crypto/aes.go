// Package crypto provides cryptographic utilities for Alexander Storage.
// This includes AES-256-GCM encryption for secret keys and key generation.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

const (
	// KeySize is the size of the AES-256 key in bytes.
	KeySize = 32

	// NonceSize is the size of the GCM nonce in bytes.
	NonceSize = 12

	// AccessKeyIDLength is the length of AWS-style access key IDs.
	AccessKeyIDLength = 20

	// SecretKeyLength is the length of AWS-style secret keys.
	SecretKeyLength = 40
)

// Errors
var (
	// ErrInvalidKeySize indicates the encryption key is not 32 bytes.
	ErrInvalidKeySize = errors.New("encryption key must be 32 bytes (256 bits)")

	// ErrInvalidCiphertext indicates the ciphertext is malformed or too short.
	ErrInvalidCiphertext = errors.New("invalid ciphertext: too short or malformed")

	// ErrDecryptionFailed indicates decryption failed (wrong key or corrupted data).
	ErrDecryptionFailed = errors.New("decryption failed: authentication error")
)

// Encryptor provides AES-256-GCM encryption and decryption.
type Encryptor struct {
	gcm cipher.AEAD
}

// NewEncryptor creates a new Encryptor with the given master key.
// The key must be exactly 32 bytes (256 bits).
func NewEncryptor(masterKey []byte) (*Encryptor, error) {
	if len(masterKey) != KeySize {
		return nil, ErrInvalidKeySize
	}

	block, err := aes.NewCipher(masterKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	return &Encryptor{gcm: gcm}, nil
}

// NewEncryptorFromHex creates a new Encryptor from a hex-encoded master key.
// The key must be 64 hex characters (32 bytes).
func NewEncryptorFromHex(hexKey string) (*Encryptor, error) {
	key, err := ParseHexKey(hexKey)
	if err != nil {
		return nil, err
	}
	return NewEncryptor(key)
}

// Encrypt encrypts the plaintext and returns base64-encoded ciphertext.
// The ciphertext format is: base64(nonce || ciphertext || tag)
func (e *Encryptor) Encrypt(plaintext []byte) (string, error) {
	// Generate random nonce
	nonce := make([]byte, NonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt with GCM (includes authentication tag)
	ciphertext := e.gcm.Seal(nonce, nonce, plaintext, nil)

	// Return base64-encoded result
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts base64-encoded ciphertext and returns plaintext.
// Expects format: base64(nonce || ciphertext || tag)
func (e *Encryptor) Decrypt(encoded string) ([]byte, error) {
	// Decode from base64
	ciphertext, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64: %w", err)
	}

	// Validate minimum length (nonce + at least 1 byte + tag)
	minLength := NonceSize + 1 + e.gcm.Overhead()
	if len(ciphertext) < minLength {
		return nil, ErrInvalidCiphertext
	}

	// Extract nonce and encrypted data
	nonce := ciphertext[:NonceSize]
	encryptedData := ciphertext[NonceSize:]

	// Decrypt and verify
	plaintext, err := e.gcm.Open(nil, nonce, encryptedData, nil)
	if err != nil {
		return nil, ErrDecryptionFailed
	}

	return plaintext, nil
}

// EncryptString encrypts a string and returns base64-encoded ciphertext.
func (e *Encryptor) EncryptString(plaintext string) (string, error) {
	return e.Encrypt([]byte(plaintext))
}

// DecryptString decrypts base64-encoded ciphertext and returns a string.
func (e *Encryptor) DecryptString(encoded string) (string, error) {
	plaintext, err := e.Decrypt(encoded)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}
