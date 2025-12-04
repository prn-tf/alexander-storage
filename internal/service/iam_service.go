// Package service provides business logic services for Alexander Storage.
package service

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog"

	"github.com/prn-tf/alexander-storage/internal/auth"
	"github.com/prn-tf/alexander-storage/internal/domain"
	"github.com/prn-tf/alexander-storage/internal/pkg/crypto"
	"github.com/prn-tf/alexander-storage/internal/repository"
)

const (
	// MaxAccessKeysPerUser is the maximum number of access keys a user can have.
	MaxAccessKeysPerUser = 5
)

// IAMService handles IAM operations (access key management).
type IAMService struct {
	accessKeyRepo repository.AccessKeyRepository
	userRepo      repository.UserRepository
	encryptor     *crypto.Encryptor
	logger        zerolog.Logger
}

// NewIAMService creates a new IAMService.
func NewIAMService(
	accessKeyRepo repository.AccessKeyRepository,
	userRepo repository.UserRepository,
	encryptor *crypto.Encryptor,
	logger zerolog.Logger,
) *IAMService {
	return &IAMService{
		accessKeyRepo: accessKeyRepo,
		userRepo:      userRepo,
		encryptor:     encryptor,
		logger:        logger.With().Str("service", "iam").Logger(),
	}
}

// CreateAccessKeyInput contains the data needed to create an access key.
type CreateAccessKeyInput struct {
	UserID      int64
	Description string
	ExpiresAt   *time.Time
}

// CreateAccessKeyOutput contains the result of creating an access key.
// Note: SecretKey is only available at creation time and should be shown to the user once.
type CreateAccessKeyOutput struct {
	AccessKeyID string
	SecretKey   string // Plaintext - only shown once!
	AccessKey   *domain.AccessKey
}

// CreateAccessKey creates a new access key for a user.
func (s *IAMService) CreateAccessKey(ctx context.Context, input CreateAccessKeyInput) (*CreateAccessKeyOutput, error) {
	// Verify user exists and is active
	user, err := s.userRepo.GetByID(ctx, input.UserID)
	if err != nil {
		if err == repository.ErrNotFound {
			return nil, ErrUserNotFound
		}
		s.logger.Error().Err(err).Int64("user_id", input.UserID).Msg("failed to get user")
		return nil, fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	if !user.IsActive {
		return nil, ErrUserInactive
	}

	// Check max access keys limit
	existingKeys, err := s.accessKeyRepo.ListByUserID(ctx, input.UserID)
	if err != nil {
		s.logger.Error().Err(err).Int64("user_id", input.UserID).Msg("failed to list user access keys")
		return nil, fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	activeCount := 0
	for _, key := range existingKeys {
		if key.Status == domain.AccessKeyStatusActive {
			activeCount++
		}
	}

	if activeCount >= MaxAccessKeysPerUser {
		return nil, ErrMaxAccessKeysReached
	}

	// Generate access key pair
	accessKeyID, secretKey, err := crypto.GenerateAccessKeyPair()
	if err != nil {
		s.logger.Error().Err(err).Msg("failed to generate access key pair")
		return nil, fmt.Errorf("%w: %v", ErrEncryptionFailed, err)
	}

	// Encrypt secret key
	encryptedSecret, err := s.encryptor.EncryptString(secretKey)
	if err != nil {
		s.logger.Error().Err(err).Msg("failed to encrypt secret key")
		return nil, fmt.Errorf("%w: %v", ErrEncryptionFailed, err)
	}

	// Create access key record
	accessKey := domain.NewAccessKey(input.UserID, accessKeyID, encryptedSecret)
	accessKey.Description = input.Description
	accessKey.ExpiresAt = input.ExpiresAt

	if err := s.accessKeyRepo.Create(ctx, accessKey); err != nil {
		s.logger.Error().Err(err).Str("access_key_id", accessKeyID).Msg("failed to create access key")
		return nil, fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	s.logger.Info().
		Int64("user_id", input.UserID).
		Str("access_key_id", accessKeyID).
		Msg("access key created")

	return &CreateAccessKeyOutput{
		AccessKeyID: accessKeyID,
		SecretKey:   secretKey, // Only time this is returned!
		AccessKey:   accessKey,
	}, nil
}

// GetAccessKey retrieves an access key by ID.
func (s *IAMService) GetAccessKey(ctx context.Context, accessKeyID string) (*domain.AccessKey, error) {
	key, err := s.accessKeyRepo.GetByAccessKeyID(ctx, accessKeyID)
	if err != nil {
		if err == repository.ErrNotFound {
			return nil, ErrAccessKeyNotFound
		}
		s.logger.Error().Err(err).Str("access_key_id", accessKeyID).Msg("failed to get access key")
		return nil, fmt.Errorf("%w: %v", ErrInternalError, err)
	}
	return key, nil
}

// ListAccessKeysInput contains filters for listing access keys.
type ListAccessKeysInput struct {
	UserID     int64
	ActiveOnly bool
}

// ListAccessKeys returns all access keys for a user.
func (s *IAMService) ListAccessKeys(ctx context.Context, input ListAccessKeysInput) ([]*domain.AccessKey, error) {
	keys, err := s.accessKeyRepo.ListByUserID(ctx, input.UserID)
	if err != nil {
		s.logger.Error().Err(err).Int64("user_id", input.UserID).Msg("failed to list access keys")
		return nil, fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	if input.ActiveOnly {
		activeKeys := make([]*domain.AccessKey, 0)
		for _, key := range keys {
			if key.IsValid() {
				activeKeys = append(activeKeys, key)
			}
		}
		return activeKeys, nil
	}

	return keys, nil
}

// DeactivateAccessKey deactivates an access key (soft delete).
func (s *IAMService) DeactivateAccessKey(ctx context.Context, accessKeyID string) error {
	key, err := s.accessKeyRepo.GetByAccessKeyID(ctx, accessKeyID)
	if err != nil {
		if err == repository.ErrNotFound {
			return ErrAccessKeyNotFound
		}
		return fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	key.Status = domain.AccessKeyStatusInactive

	if err := s.accessKeyRepo.Update(ctx, key); err != nil {
		s.logger.Error().Err(err).Str("access_key_id", accessKeyID).Msg("failed to deactivate access key")
		return fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	s.logger.Info().
		Str("access_key_id", accessKeyID).
		Int64("user_id", key.UserID).
		Msg("access key deactivated")

	return nil
}

// ActivateAccessKey activates a previously deactivated access key.
func (s *IAMService) ActivateAccessKey(ctx context.Context, accessKeyID string) error {
	key, err := s.accessKeyRepo.GetByAccessKeyID(ctx, accessKeyID)
	if err != nil {
		if err == repository.ErrNotFound {
			return ErrAccessKeyNotFound
		}
		return fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	key.Status = domain.AccessKeyStatusActive

	if err := s.accessKeyRepo.Update(ctx, key); err != nil {
		s.logger.Error().Err(err).Str("access_key_id", accessKeyID).Msg("failed to activate access key")
		return fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	s.logger.Info().
		Str("access_key_id", accessKeyID).
		Int64("user_id", key.UserID).
		Msg("access key activated")

	return nil
}

// DeleteAccessKey permanently deletes an access key.
func (s *IAMService) DeleteAccessKey(ctx context.Context, accessKeyID string) error {
	if err := s.accessKeyRepo.DeleteByAccessKeyID(ctx, accessKeyID); err != nil {
		if err == repository.ErrNotFound {
			return ErrAccessKeyNotFound
		}
		s.logger.Error().Err(err).Str("access_key_id", accessKeyID).Msg("failed to delete access key")
		return fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	s.logger.Info().Str("access_key_id", accessKeyID).Msg("access key deleted")
	return nil
}

// DeleteExpiredAccessKeys deletes all expired access keys (cleanup job).
func (s *IAMService) DeleteExpiredAccessKeys(ctx context.Context) (int64, error) {
	count, err := s.accessKeyRepo.DeleteExpired(ctx)
	if err != nil {
		s.logger.Error().Err(err).Msg("failed to delete expired access keys")
		return 0, fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	if count > 0 {
		s.logger.Info().Int64("count", count).Msg("deleted expired access keys")
	}

	return count, nil
}

// VerifyAccessKey verifies an access key is valid and returns the decrypted secret.
// This is used internally by the auth middleware.
func (s *IAMService) VerifyAccessKey(ctx context.Context, accessKeyID string) (*auth.AccessKeyInfo, error) {
	key, err := s.accessKeyRepo.GetActiveByAccessKeyID(ctx, accessKeyID)
	if err != nil {
		if err == repository.ErrNotFound {
			return nil, ErrAccessKeyNotFound
		}
		return nil, fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	// Check if key is valid (active and not expired)
	if !key.IsValid() {
		if key.Status != domain.AccessKeyStatusActive {
			return nil, ErrAccessKeyInactive
		}
		return nil, ErrAccessKeyExpired
	}

	// Decrypt secret key
	secretKey, err := s.encryptor.DecryptString(key.EncryptedSecret)
	if err != nil {
		s.logger.Error().Err(err).Str("access_key_id", accessKeyID).Msg("failed to decrypt secret key")
		return nil, fmt.Errorf("%w: %v", ErrDecryptionFailed, err)
	}

	return &auth.AccessKeyInfo{
		AccessKeyID: key.AccessKeyID,
		SecretKey:   secretKey,
		UserID:      key.UserID,
		IsActive:    key.Status == domain.AccessKeyStatusActive,
		ExpiresAt:   key.ExpiresAt,
	}, nil
}

// UpdateLastUsed updates the last used timestamp for an access key.
func (s *IAMService) UpdateLastUsed(ctx context.Context, accessKeyID string) error {
	key, err := s.accessKeyRepo.GetByAccessKeyID(ctx, accessKeyID)
	if err != nil {
		return err // Silent fail for async updates
	}

	return s.accessKeyRepo.UpdateLastUsed(ctx, key.ID)
}

// AccessKeyStoreAdapter adapts IAMService to implement auth.AccessKeyStore interface.
type AccessKeyStoreAdapter struct {
	iamService *IAMService
}

// NewAccessKeyStoreAdapter creates a new adapter.
func NewAccessKeyStoreAdapter(iamService *IAMService) *AccessKeyStoreAdapter {
	return &AccessKeyStoreAdapter{iamService: iamService}
}

// GetActiveAccessKey implements auth.AccessKeyStore.
func (a *AccessKeyStoreAdapter) GetActiveAccessKey(ctx context.Context, accessKeyID string) (*auth.AccessKeyInfo, error) {
	return a.iamService.VerifyAccessKey(ctx, accessKeyID)
}

// UpdateLastUsed implements auth.AccessKeyStore.
func (a *AccessKeyStoreAdapter) UpdateLastUsed(ctx context.Context, accessKeyID string) error {
	return a.iamService.UpdateLastUsed(ctx, accessKeyID)
}

// Ensure AccessKeyStoreAdapter implements auth.AccessKeyStore
var _ auth.AccessKeyStore = (*AccessKeyStoreAdapter)(nil)
