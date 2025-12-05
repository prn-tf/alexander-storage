package service

import (
	"context"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/prn-tf/alexander-storage/internal/domain"
)

// MockBucketRepository is a mock implementation of repository.BucketRepository.
type MockBucketRepository struct {
	buckets   map[string]*domain.Bucket
	nextID    int64
	objects   map[int64]int64 // bucketID -> object count
	createErr error
	getErr    error
	deleteErr error
}

func NewMockBucketRepository() *MockBucketRepository {
	return &MockBucketRepository{
		buckets: make(map[string]*domain.Bucket),
		objects: make(map[int64]int64),
		nextID:  1,
	}
}

func (m *MockBucketRepository) Create(ctx context.Context, bucket *domain.Bucket) error {
	if m.createErr != nil {
		return m.createErr
	}
	if _, exists := m.buckets[bucket.Name]; exists {
		return domain.ErrBucketAlreadyExists
	}
	bucket.ID = m.nextID
	m.nextID++
	m.buckets[bucket.Name] = bucket
	return nil
}

func (m *MockBucketRepository) GetByID(ctx context.Context, id int64) (*domain.Bucket, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	for _, b := range m.buckets {
		if b.ID == id {
			return b, nil
		}
	}
	return nil, domain.ErrBucketNotFound
}

func (m *MockBucketRepository) GetByName(ctx context.Context, name string) (*domain.Bucket, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	if b, exists := m.buckets[name]; exists {
		return b, nil
	}
	return nil, domain.ErrBucketNotFound
}

func (m *MockBucketRepository) List(ctx context.Context, userID int64) ([]*domain.Bucket, error) {
	var result []*domain.Bucket
	for _, b := range m.buckets {
		if userID == 0 || b.OwnerID == userID {
			result = append(result, b)
		}
	}
	return result, nil
}

func (m *MockBucketRepository) Update(ctx context.Context, bucket *domain.Bucket) error {
	if _, exists := m.buckets[bucket.Name]; !exists {
		return domain.ErrBucketNotFound
	}
	m.buckets[bucket.Name] = bucket
	return nil
}

func (m *MockBucketRepository) UpdateVersioning(ctx context.Context, id int64, status domain.VersioningStatus) error {
	for _, b := range m.buckets {
		if b.ID == id {
			b.Versioning = status
			return nil
		}
	}
	return domain.ErrBucketNotFound
}

func (m *MockBucketRepository) Delete(ctx context.Context, id int64) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	for name, b := range m.buckets {
		if b.ID == id {
			delete(m.buckets, name)
			return nil
		}
	}
	return domain.ErrBucketNotFound
}

func (m *MockBucketRepository) DeleteByName(ctx context.Context, name string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	if _, exists := m.buckets[name]; !exists {
		return domain.ErrBucketNotFound
	}
	delete(m.buckets, name)
	return nil
}

func (m *MockBucketRepository) ExistsByName(ctx context.Context, name string) (bool, error) {
	_, exists := m.buckets[name]
	return exists, nil
}

func (m *MockBucketRepository) IsEmpty(ctx context.Context, id int64) (bool, error) {
	count, exists := m.objects[id]
	if !exists {
		return true, nil
	}
	return count == 0, nil
}

func (m *MockBucketRepository) GetACLByName(ctx context.Context, name string) (domain.BucketACL, error) {
	if b, exists := m.buckets[name]; exists {
		return b.ACL, nil
	}
	return "", domain.ErrBucketNotFound
}

func (m *MockBucketRepository) UpdateACL(ctx context.Context, id int64, acl domain.BucketACL) error {
	for _, b := range m.buckets {
		if b.ID == id {
			b.ACL = acl
			return nil
		}
	}
	return domain.ErrBucketNotFound
}

// Helper to add objects to a bucket for testing
func (m *MockBucketRepository) AddObjects(bucketID int64, count int64) {
	m.objects[bucketID] = count
}

// =============================================================================
// Tests
// =============================================================================

func TestBucketService_CreateBucket(t *testing.T) {
	tests := []struct {
		name      string
		input     CreateBucketInput
		wantErr   error
		setupRepo func(*MockBucketRepository)
	}{
		{
			name: "success",
			input: CreateBucketInput{
				OwnerID: 1,
				Name:    "my-bucket",
				Region:  "us-east-1",
			},
			wantErr: nil,
		},
		{
			name: "success with default region",
			input: CreateBucketInput{
				OwnerID: 1,
				Name:    "my-bucket-2",
				Region:  "",
			},
			wantErr: nil,
		},
		{
			name: "invalid name - too short",
			input: CreateBucketInput{
				OwnerID: 1,
				Name:    "ab",
			},
			wantErr: domain.ErrBucketNameLength,
		},
		{
			name: "invalid name - uppercase",
			input: CreateBucketInput{
				OwnerID: 1,
				Name:    "MyBucket",
			},
			wantErr: domain.ErrBucketNameFormat,
		},
		{
			name: "already exists",
			input: CreateBucketInput{
				OwnerID: 1,
				Name:    "existing-bucket",
			},
			wantErr: domain.ErrBucketAlreadyExists,
			setupRepo: func(m *MockBucketRepository) {
				m.buckets["existing-bucket"] = &domain.Bucket{
					ID:      1,
					OwnerID: 1,
					Name:    "existing-bucket",
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := NewMockBucketRepository()
			if tt.setupRepo != nil {
				tt.setupRepo(repo)
			}

			logger := zerolog.Nop()
			svc := NewBucketService(repo, logger)

			output, err := svc.CreateBucket(context.Background(), tt.input)

			if tt.wantErr != nil {
				if err == nil {
					t.Errorf("expected error %v, got nil", tt.wantErr)
				} else if err != tt.wantErr {
					t.Errorf("expected error %v, got %v", tt.wantErr, err)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if output.Bucket == nil {
				t.Error("expected bucket in output")
				return
			}

			if output.Bucket.Name != tt.input.Name {
				t.Errorf("expected name %s, got %s", tt.input.Name, output.Bucket.Name)
			}

			if tt.input.Region == "" && output.Bucket.Region != "us-east-1" {
				t.Errorf("expected default region us-east-1, got %s", output.Bucket.Region)
			}
		})
	}
}

func TestBucketService_DeleteBucket(t *testing.T) {
	tests := []struct {
		name      string
		input     DeleteBucketInput
		wantErr   error
		setupRepo func(*MockBucketRepository)
	}{
		{
			name: "success",
			input: DeleteBucketInput{
				Name:    "my-bucket",
				OwnerID: 1,
			},
			wantErr: nil,
			setupRepo: func(m *MockBucketRepository) {
				m.buckets["my-bucket"] = &domain.Bucket{
					ID:      1,
					OwnerID: 1,
					Name:    "my-bucket",
				}
			},
		},
		{
			name: "not found",
			input: DeleteBucketInput{
				Name:    "non-existent",
				OwnerID: 1,
			},
			wantErr: domain.ErrBucketNotFound,
		},
		{
			name: "not empty",
			input: DeleteBucketInput{
				Name:    "non-empty-bucket",
				OwnerID: 1,
			},
			wantErr: domain.ErrBucketNotEmpty,
			setupRepo: func(m *MockBucketRepository) {
				m.buckets["non-empty-bucket"] = &domain.Bucket{
					ID:      1,
					OwnerID: 1,
					Name:    "non-empty-bucket",
				}
				m.AddObjects(1, 5) // 5 objects in bucket
			},
		},
		{
			name: "access denied - different owner",
			input: DeleteBucketInput{
				Name:    "other-user-bucket",
				OwnerID: 2,
			},
			wantErr: ErrBucketAccessDenied,
			setupRepo: func(m *MockBucketRepository) {
				m.buckets["other-user-bucket"] = &domain.Bucket{
					ID:      1,
					OwnerID: 1, // Different owner
					Name:    "other-user-bucket",
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := NewMockBucketRepository()
			if tt.setupRepo != nil {
				tt.setupRepo(repo)
			}

			logger := zerolog.Nop()
			svc := NewBucketService(repo, logger)

			err := svc.DeleteBucket(context.Background(), tt.input)

			if tt.wantErr != nil {
				if err == nil {
					t.Errorf("expected error %v, got nil", tt.wantErr)
				} else if err != tt.wantErr {
					t.Errorf("expected error %v, got %v", tt.wantErr, err)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestBucketService_ListBuckets(t *testing.T) {
	repo := NewMockBucketRepository()

	// Add some buckets
	repo.buckets["bucket-1"] = &domain.Bucket{ID: 1, OwnerID: 1, Name: "bucket-1", CreatedAt: time.Now()}
	repo.buckets["bucket-2"] = &domain.Bucket{ID: 2, OwnerID: 1, Name: "bucket-2", CreatedAt: time.Now()}
	repo.buckets["bucket-3"] = &domain.Bucket{ID: 3, OwnerID: 2, Name: "bucket-3", CreatedAt: time.Now()}

	logger := zerolog.Nop()
	svc := NewBucketService(repo, logger)

	// List buckets for user 1
	output, err := svc.ListBuckets(context.Background(), ListBucketsInput{OwnerID: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(output.Buckets) != 2 {
		t.Errorf("expected 2 buckets for user 1, got %d", len(output.Buckets))
	}

	// List buckets for user 2
	output, err = svc.ListBuckets(context.Background(), ListBucketsInput{OwnerID: 2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(output.Buckets) != 1 {
		t.Errorf("expected 1 bucket for user 2, got %d", len(output.Buckets))
	}
}

func TestBucketService_PutBucketVersioning(t *testing.T) {
	tests := []struct {
		name      string
		input     PutBucketVersioningInput
		wantErr   error
		setupRepo func(*MockBucketRepository)
	}{
		{
			name: "enable versioning",
			input: PutBucketVersioningInput{
				Name:    "my-bucket",
				OwnerID: 1,
				Status:  domain.VersioningEnabled,
			},
			wantErr: nil,
			setupRepo: func(m *MockBucketRepository) {
				m.buckets["my-bucket"] = &domain.Bucket{
					ID:         1,
					OwnerID:    1,
					Name:       "my-bucket",
					Versioning: domain.VersioningDisabled,
				}
			},
		},
		{
			name: "suspend versioning",
			input: PutBucketVersioningInput{
				Name:    "my-bucket",
				OwnerID: 1,
				Status:  domain.VersioningSuspended,
			},
			wantErr: nil,
			setupRepo: func(m *MockBucketRepository) {
				m.buckets["my-bucket"] = &domain.Bucket{
					ID:         1,
					OwnerID:    1,
					Name:       "my-bucket",
					Versioning: domain.VersioningEnabled,
				}
			},
		},
		{
			name: "invalid status - Disabled",
			input: PutBucketVersioningInput{
				Name:    "my-bucket",
				OwnerID: 1,
				Status:  domain.VersioningDisabled,
			},
			wantErr: ErrInvalidVersioningStatus,
			setupRepo: func(m *MockBucketRepository) {
				m.buckets["my-bucket"] = &domain.Bucket{
					ID:      1,
					OwnerID: 1,
					Name:    "my-bucket",
				}
			},
		},
		{
			name: "bucket not found",
			input: PutBucketVersioningInput{
				Name:    "non-existent",
				OwnerID: 1,
				Status:  domain.VersioningEnabled,
			},
			wantErr: domain.ErrBucketNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := NewMockBucketRepository()
			if tt.setupRepo != nil {
				tt.setupRepo(repo)
			}

			logger := zerolog.Nop()
			svc := NewBucketService(repo, logger)

			err := svc.PutBucketVersioning(context.Background(), tt.input)

			if tt.wantErr != nil {
				if err == nil {
					t.Errorf("expected error %v, got nil", tt.wantErr)
				} else if err != tt.wantErr {
					t.Errorf("expected error %v, got %v", tt.wantErr, err)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}
