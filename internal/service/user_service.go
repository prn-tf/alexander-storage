// Package service provides business logic services for Alexander Storage.
package service

import (
	"context"
	"fmt"
	"net/mail"
	"time"

	"github.com/rs/zerolog"
	"golang.org/x/crypto/bcrypt"

	"github.com/prn-tf/alexander-storage/internal/domain"
	"github.com/prn-tf/alexander-storage/internal/repository"
)

// UserService handles user management operations.
type UserService struct {
	userRepo repository.UserRepository
	logger   zerolog.Logger
}

// NewUserService creates a new UserService.
func NewUserService(userRepo repository.UserRepository, logger zerolog.Logger) *UserService {
	return &UserService{
		userRepo: userRepo,
		logger:   logger.With().Str("service", "user").Logger(),
	}
}

// CreateUserInput contains the data needed to create a new user.
type CreateUserInput struct {
	Username string
	Email    string
	Password string
	IsAdmin  bool
}

// CreateUserOutput contains the result of creating a user.
type CreateUserOutput struct {
	User *domain.User
}

// Create creates a new user account.
func (s *UserService) Create(ctx context.Context, input CreateUserInput) (*CreateUserOutput, error) {
	// Validate input
	if err := s.validateCreateInput(input); err != nil {
		return nil, err
	}

	// Check if username already exists
	exists, err := s.userRepo.ExistsByUsername(ctx, input.Username)
	if err != nil {
		s.logger.Error().Err(err).Str("username", input.Username).Msg("failed to check username existence")
		return nil, fmt.Errorf("%w: %v", ErrInternalError, err)
	}
	if exists {
		return nil, fmt.Errorf("%w: username '%s'", ErrUserAlreadyExists, input.Username)
	}

	// Check if email already exists
	exists, err = s.userRepo.ExistsByEmail(ctx, input.Email)
	if err != nil {
		s.logger.Error().Err(err).Str("email", input.Email).Msg("failed to check email existence")
		return nil, fmt.Errorf("%w: %v", ErrInternalError, err)
	}
	if exists {
		return nil, fmt.Errorf("%w: email '%s'", ErrUserAlreadyExists, input.Email)
	}

	// Hash password
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		s.logger.Error().Err(err).Msg("failed to hash password")
		return nil, fmt.Errorf("%w: failed to hash password", ErrInternalError)
	}

	// Create user
	user := domain.NewUser(input.Username, input.Email, string(passwordHash))
	user.IsAdmin = input.IsAdmin

	if err := s.userRepo.Create(ctx, user); err != nil {
		s.logger.Error().Err(err).Str("username", input.Username).Msg("failed to create user")
		return nil, fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	s.logger.Info().
		Int64("user_id", user.ID).
		Str("username", user.Username).
		Bool("is_admin", user.IsAdmin).
		Msg("user created")

	return &CreateUserOutput{User: user}, nil
}

// Authenticate verifies user credentials and returns the user.
func (s *UserService) Authenticate(ctx context.Context, username, password string) (*domain.User, error) {
	user, err := s.userRepo.GetByUsername(ctx, username)
	if err != nil {
		// Log but don't expose whether username exists
		s.logger.Debug().Str("username", username).Msg("user not found during authentication")
		return nil, ErrInvalidCredentials
	}

	if !user.CanAuthenticate() {
		s.logger.Debug().Str("username", username).Msg("inactive user attempted authentication")
		return nil, ErrUserInactive
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		s.logger.Debug().Str("username", username).Msg("invalid password during authentication")
		return nil, ErrInvalidCredentials
	}

	s.logger.Info().
		Int64("user_id", user.ID).
		Str("username", user.Username).
		Msg("user authenticated")

	return user, nil
}

// GetByID retrieves a user by ID.
func (s *UserService) GetByID(ctx context.Context, id int64) (*domain.User, error) {
	user, err := s.userRepo.GetByID(ctx, id)
	if err != nil {
		if err == repository.ErrNotFound {
			return nil, ErrUserNotFound
		}
		s.logger.Error().Err(err).Int64("user_id", id).Msg("failed to get user")
		return nil, fmt.Errorf("%w: %v", ErrInternalError, err)
	}
	return user, nil
}

// GetByUsername retrieves a user by username.
func (s *UserService) GetByUsername(ctx context.Context, username string) (*domain.User, error) {
	user, err := s.userRepo.GetByUsername(ctx, username)
	if err != nil {
		if err == repository.ErrNotFound {
			return nil, ErrUserNotFound
		}
		s.logger.Error().Err(err).Str("username", username).Msg("failed to get user")
		return nil, fmt.Errorf("%w: %v", ErrInternalError, err)
	}
	return user, nil
}

// UpdatePasswordInput contains the data needed to update a password.
type UpdatePasswordInput struct {
	UserID      int64
	OldPassword string
	NewPassword string
}

// UpdatePassword changes a user's password.
func (s *UserService) UpdatePassword(ctx context.Context, input UpdatePasswordInput) error {
	// Get user
	user, err := s.userRepo.GetByID(ctx, input.UserID)
	if err != nil {
		if err == repository.ErrNotFound {
			return ErrUserNotFound
		}
		return fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	// Verify old password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(input.OldPassword)); err != nil {
		return ErrInvalidCredentials
	}

	// Validate new password
	if len(input.NewPassword) < 8 {
		return ErrInvalidPassword
	}

	// Hash new password
	newHash, err := bcrypt.GenerateFromPassword([]byte(input.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("%w: failed to hash password", ErrInternalError)
	}

	// Update user
	user.PasswordHash = string(newHash)
	user.UpdatedAt = time.Now().UTC()

	if err := s.userRepo.Update(ctx, user); err != nil {
		return fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	s.logger.Info().Int64("user_id", user.ID).Msg("password updated")
	return nil
}

// SetActive sets the active status of a user.
func (s *UserService) SetActive(ctx context.Context, userID int64, isActive bool) error {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		if err == repository.ErrNotFound {
			return ErrUserNotFound
		}
		return fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	user.IsActive = isActive
	user.UpdatedAt = time.Now().UTC()

	if err := s.userRepo.Update(ctx, user); err != nil {
		return fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	s.logger.Info().
		Int64("user_id", user.ID).
		Bool("is_active", isActive).
		Msg("user active status updated")

	return nil
}

// SetAdmin sets the admin status of a user.
func (s *UserService) SetAdmin(ctx context.Context, userID int64, isAdmin bool) error {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		if err == repository.ErrNotFound {
			return ErrUserNotFound
		}
		return fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	user.IsAdmin = isAdmin
	user.UpdatedAt = time.Now().UTC()

	if err := s.userRepo.Update(ctx, user); err != nil {
		return fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	s.logger.Info().
		Int64("user_id", user.ID).
		Bool("is_admin", isAdmin).
		Msg("user admin status updated")

	return nil
}

// Delete deletes a user account.
func (s *UserService) Delete(ctx context.Context, userID int64) error {
	if err := s.userRepo.Delete(ctx, userID); err != nil {
		if err == repository.ErrNotFound {
			return ErrUserNotFound
		}
		return fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	s.logger.Info().Int64("user_id", userID).Msg("user deleted")
	return nil
}

// ListUsersInput contains pagination options for listing users.
type ListUsersInput struct {
	Limit  int
	Offset int
}

// ListUsersOutput contains the result of listing users.
type ListUsersOutput struct {
	Users      []*domain.User
	TotalCount int64
}

// List returns all users with pagination.
func (s *UserService) List(ctx context.Context, input ListUsersInput) (*ListUsersOutput, error) {
	if input.Limit <= 0 {
		input.Limit = 20
	}
	if input.Limit > 100 {
		input.Limit = 100
	}

	result, err := s.userRepo.List(ctx, repository.ListOptions{
		Limit:  input.Limit,
		Offset: input.Offset,
	})
	if err != nil {
		s.logger.Error().Err(err).Msg("failed to list users")
		return nil, fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	return &ListUsersOutput{
		Users:      result.Items,
		TotalCount: result.Total,
	}, nil
}

// validateCreateInput validates the input for creating a user.
func (s *UserService) validateCreateInput(input CreateUserInput) error {
	// Validate username
	if len(input.Username) < 3 || len(input.Username) > 255 {
		return ErrInvalidUsername
	}

	// Validate email
	if _, err := mail.ParseAddress(input.Email); err != nil {
		return ErrInvalidEmail
	}

	// Validate password
	if len(input.Password) < 8 {
		return ErrInvalidPassword
	}

	return nil
}
