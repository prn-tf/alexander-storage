// Package service provides business logic services for Alexander Storage.
package service

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/prn-tf/alexander-storage/internal/domain"
	"github.com/prn-tf/alexander-storage/internal/lock"
	"github.com/prn-tf/alexander-storage/internal/repository"
)

// =============================================================================
// Mock Repository Types for ObjectService
// =============================================================================

type mockObjectRepository struct {
	mock.Mock
}

func (m *mockObjectRepository) Create(ctx context.Context, obj *domain.Object) error {
	args := m.Called(ctx, obj)
	return args.Error(0)
}

func (m *mockObjectRepository) GetByID(ctx context.Context, id int64) (*domain.Object, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Object), args.Error(1)
}

func (m *mockObjectRepository) GetByKey(ctx context.Context, bucketID int64, key string) (*domain.Object, error) {
	args := m.Called(ctx, bucketID, key)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Object), args.Error(1)
}

func (m *mockObjectRepository) GetByKeyAndVersion(ctx context.Context, bucketID int64, key string, versionID uuid.UUID) (*domain.Object, error) {
	args := m.Called(ctx, bucketID, key, versionID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Object), args.Error(1)
}

func (m *mockObjectRepository) List(ctx context.Context, bucketID int64, opts repository.ObjectListOptions) (*repository.ObjectListResult, error) {
	args := m.Called(ctx, bucketID, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*repository.ObjectListResult), args.Error(1)
}

func (m *mockObjectRepository) ListVersions(ctx context.Context, bucketID int64, opts repository.ObjectListOptions) (*repository.ObjectVersionListResult, error) {
	args := m.Called(ctx, bucketID, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*repository.ObjectVersionListResult), args.Error(1)
}

func (m *mockObjectRepository) Update(ctx context.Context, obj *domain.Object) error {
	args := m.Called(ctx, obj)
	return args.Error(0)
}

func (m *mockObjectRepository) MarkNotLatest(ctx context.Context, bucketID int64, key string) error {
	args := m.Called(ctx, bucketID, key)
	return args.Error(0)
}

func (m *mockObjectRepository) Delete(ctx context.Context, id int64) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *mockObjectRepository) DeleteAllVersions(ctx context.Context, bucketID int64, key string) error {
	args := m.Called(ctx, bucketID, key)
	return args.Error(0)
}

func (m *mockObjectRepository) CountByBucket(ctx context.Context, bucketID int64) (int64, error) {
	args := m.Called(ctx, bucketID)
	return args.Get(0).(int64), args.Error(1)
}

func (m *mockObjectRepository) GetContentHashForVersion(ctx context.Context, bucketID int64, key string, versionID uuid.UUID) (*string, error) {
	args := m.Called(ctx, bucketID, key, versionID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*string), args.Error(1)
}

func (m *mockObjectRepository) ListExpiredObjects(ctx context.Context, bucketID int64, prefix string, olderThan time.Time, limit int) ([]*domain.Object, error) {
	args := m.Called(ctx, bucketID, prefix, olderThan, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*domain.Object), args.Error(1)
}

type mockBlobRepository2 struct {
	mock.Mock
}

func (m *mockBlobRepository2) UpsertWithRefIncrement(ctx context.Context, contentHash string, size int64, storagePath string) (bool, error) {
	args := m.Called(ctx, contentHash, size, storagePath)
	return args.Bool(0), args.Error(1)
}

func (m *mockBlobRepository2) GetByHash(ctx context.Context, contentHash string) (*domain.Blob, error) {
	args := m.Called(ctx, contentHash)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Blob), args.Error(1)
}

func (m *mockBlobRepository2) IncrementRef(ctx context.Context, contentHash string) error {
	args := m.Called(ctx, contentHash)
	return args.Error(0)
}

func (m *mockBlobRepository2) DecrementRef(ctx context.Context, contentHash string) (int32, error) {
	args := m.Called(ctx, contentHash)
	return args.Get(0).(int32), args.Error(1)
}

func (m *mockBlobRepository2) GetRefCount(ctx context.Context, contentHash string) (int32, error) {
	args := m.Called(ctx, contentHash)
	return args.Get(0).(int32), args.Error(1)
}

func (m *mockBlobRepository2) Exists(ctx context.Context, contentHash string) (bool, error) {
	args := m.Called(ctx, contentHash)
	return args.Bool(0), args.Error(1)
}

func (m *mockBlobRepository2) Delete(ctx context.Context, contentHash string) error {
	args := m.Called(ctx, contentHash)
	return args.Error(0)
}

func (m *mockBlobRepository2) ListOrphans(ctx context.Context, gracePeriod time.Duration, limit int) ([]*domain.Blob, error) {
	args := m.Called(ctx, gracePeriod, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*domain.Blob), args.Error(1)
}

func (m *mockBlobRepository2) DeleteOrphans(ctx context.Context, gracePeriod time.Duration, limit int) ([]*domain.Blob, error) {
	args := m.Called(ctx, gracePeriod, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*domain.Blob), args.Error(1)
}

func (m *mockBlobRepository2) UpdateLastAccessed(ctx context.Context, contentHash string) error {
	args := m.Called(ctx, contentHash)
	return args.Error(0)
}

func (m *mockBlobRepository2) GetEncryptionStatus(ctx context.Context, contentHash string) (bool, string, error) {
	args := m.Called(ctx, contentHash)
	return args.Bool(0), args.String(1), args.Error(2)
}

func (m *mockBlobRepository2) UpsertEncrypted(ctx context.Context, contentHash string, size int64, storagePath string, encryptionIV string) (bool, error) {
	args := m.Called(ctx, contentHash, size, storagePath, encryptionIV)
	return args.Bool(0), args.Error(1)
}

func (m *mockBlobRepository2) UpdateEncrypted(ctx context.Context, contentHash string, encryptionIV string) error {
	args := m.Called(ctx, contentHash, encryptionIV)
	return args.Error(0)
}

func (m *mockBlobRepository2) IsEncrypted(ctx context.Context, contentHash string) (bool, error) {
	args := m.Called(ctx, contentHash)
	return args.Bool(0), args.Error(1)
}

func (m *mockBlobRepository2) ListUnencrypted(ctx context.Context, limit int) ([]*domain.Blob, error) {
	args := m.Called(ctx, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*domain.Blob), args.Error(1)
}

type mockStorageBackend2 struct {
	mock.Mock
}

func (m *mockStorageBackend2) Store(ctx context.Context, reader io.Reader, size int64) (string, error) {
	args := m.Called(ctx, reader, size)
	return args.String(0), args.Error(1)
}

func (m *mockStorageBackend2) Retrieve(ctx context.Context, hash string) (io.ReadCloser, error) {
	args := m.Called(ctx, hash)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(io.ReadCloser), args.Error(1)
}

func (m *mockStorageBackend2) Delete(ctx context.Context, hash string) error {
	args := m.Called(ctx, hash)
	return args.Error(0)
}

func (m *mockStorageBackend2) Exists(ctx context.Context, hash string) (bool, error) {
	args := m.Called(ctx, hash)
	return args.Bool(0), args.Error(1)
}

func (m *mockStorageBackend2) GetSize(ctx context.Context, hash string) (int64, error) {
	args := m.Called(ctx, hash)
	return args.Get(0).(int64), args.Error(1)
}

func (m *mockStorageBackend2) GetPath(hash string) string {
	args := m.Called(hash)
	return args.String(0)
}

func (m *mockStorageBackend2) HealthCheck(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

// mockBucketRepository is a mock for bucket repository in object tests
type mockBucketRepository struct {
	mock.Mock
}

func (m *mockBucketRepository) GetByName(ctx context.Context, name string) (*domain.Bucket, error) {
	args := m.Called(ctx, name)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Bucket), args.Error(1)
}

func (m *mockBucketRepository) GetByID(ctx context.Context, id int64) (*domain.Bucket, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Bucket), args.Error(1)
}

func (m *mockBucketRepository) Create(ctx context.Context, bucket *domain.Bucket) error {
	args := m.Called(ctx, bucket)
	return args.Error(0)
}

func (m *mockBucketRepository) List(ctx context.Context, ownerID int64) ([]*domain.Bucket, error) {
	args := m.Called(ctx, ownerID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*domain.Bucket), args.Error(1)
}

func (m *mockBucketRepository) Update(ctx context.Context, bucket *domain.Bucket) error {
	args := m.Called(ctx, bucket)
	return args.Error(0)
}

func (m *mockBucketRepository) Delete(ctx context.Context, id int64) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *mockBucketRepository) Exists(ctx context.Context, name string) (bool, error) {
	args := m.Called(ctx, name)
	return args.Bool(0), args.Error(1)
}

func (m *mockBucketRepository) CountObjects(ctx context.Context, bucketID int64) (int64, error) {
	args := m.Called(ctx, bucketID)
	return args.Get(0).(int64), args.Error(1)
}

func (m *mockBucketRepository) DeleteByName(ctx context.Context, name string) error {
	args := m.Called(ctx, name)
	return args.Error(0)
}

func (m *mockBucketRepository) ExistsByName(ctx context.Context, name string) (bool, error) {
	args := m.Called(ctx, name)
	return args.Bool(0), args.Error(1)
}

func (m *mockBucketRepository) IsEmpty(ctx context.Context, id int64) (bool, error) {
	args := m.Called(ctx, id)
	return args.Bool(0), args.Error(1)
}

func (m *mockBucketRepository) UpdateVersioning(ctx context.Context, id int64, status domain.VersioningStatus) error {
	args := m.Called(ctx, id, status)
	return args.Error(0)
}

func (m *mockBucketRepository) GetACLByName(ctx context.Context, name string) (domain.BucketACL, error) {
	args := m.Called(ctx, name)
	return args.Get(0).(domain.BucketACL), args.Error(1)
}

func (m *mockBucketRepository) UpdateACL(ctx context.Context, id int64, acl domain.BucketACL) error {
	args := m.Called(ctx, id, acl)
	return args.Error(0)
}

// =============================================================================
// Helper Functions
// =============================================================================

func newTestObjectService() (*ObjectService, *mockObjectRepository, *mockBlobRepository2, *mockBucketRepository, *mockStorageBackend2) {
	objectRepo := new(mockObjectRepository)
	blobRepo := new(mockBlobRepository2)
	bucketRepo := new(mockBucketRepository)
	storageBackend := new(mockStorageBackend2)
	locker := lock.NewNoOpLocker()
	logger := zerolog.Nop()

	svc := NewObjectService(objectRepo, blobRepo, bucketRepo, storageBackend, locker, logger)

	return svc, objectRepo, blobRepo, bucketRepo, storageBackend
}

// =============================================================================
// Test Cases
// =============================================================================

func TestObjectService_PutObject(t *testing.T) {
	tests := []struct {
		name    string
		input   PutObjectInput
		setup   func(*mockObjectRepository, *mockBlobRepository2, *mockBucketRepository, *mockStorageBackend2)
		wantErr error
	}{
		{
			name: "success - new object",
			input: PutObjectInput{
				BucketName:  "test-bucket",
				Key:         "test-key.txt",
				Body:        bytes.NewReader([]byte("hello world")),
				Size:        11,
				ContentType: "text/plain",
				Metadata:    map[string]string{"key1": "value1"},
				OwnerID:     1,
			},
			setup: func(objRepo *mockObjectRepository, blobRepo *mockBlobRepository2, bucketRepo *mockBucketRepository, storageBackend *mockStorageBackend2) {
				bucket := &domain.Bucket{
					ID:         1,
					Name:       "test-bucket",
					OwnerID:    1,
					Versioning: domain.VersioningDisabled,
				}
				bucketRepo.On("GetByName", mock.Anything, "test-bucket").Return(bucket, nil)

				// Store returns the hash
				storageBackend.On("Store", mock.Anything, mock.Anything, int64(11)).Return("abc123hash", nil)
				storageBackend.On("GetPath", "abc123hash").Return("/data/ab/c1/abc123hash")

				// Upsert blob (new)
				blobRepo.On("UpsertWithRefIncrement", mock.Anything, "abc123hash", int64(11), "/data/ab/c1/abc123hash").Return(true, nil)

				// Check for existing object - not found (for non-versioned bucket)
				objRepo.On("GetByKey", mock.Anything, int64(1), "test-key.txt").Return(nil, repository.ErrNotFound)

				// Mark any existing object as not latest (called for non-versioned buckets)
				objRepo.On("MarkNotLatest", mock.Anything, int64(1), "test-key.txt").Return(nil)

				// Create object
				objRepo.On("Create", mock.Anything, mock.AnythingOfType("*domain.Object")).Return(nil)
			},
			wantErr: nil,
		},
		{
			name: "bucket not found",
			input: PutObjectInput{
				BucketName: "nonexistent-bucket",
				Key:        "test-key.txt",
				Body:       bytes.NewReader([]byte("hello")),
				Size:       5,
				OwnerID:    1,
			},
			setup: func(objRepo *mockObjectRepository, blobRepo *mockBlobRepository2, bucketRepo *mockBucketRepository, storageBackend *mockStorageBackend2) {
				bucketRepo.On("GetByName", mock.Anything, "nonexistent-bucket").Return(nil, domain.ErrBucketNotFound)
			},
			wantErr: domain.ErrBucketNotFound,
		},
		{
			name: "empty key",
			input: PutObjectInput{
				BucketName: "test-bucket",
				Key:        "",
				Body:       bytes.NewReader([]byte("hello")),
				Size:       5,
				OwnerID:    1,
			},
			setup: func(objRepo *mockObjectRepository, blobRepo *mockBlobRepository2, bucketRepo *mockBucketRepository, storageBackend *mockStorageBackend2) {
				// No setup needed - validation fails first
			},
			wantErr: domain.ErrObjectKeyEmpty,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, objRepo, blobRepo, bucketRepo, storageBackend := newTestObjectService()
			tt.setup(objRepo, blobRepo, bucketRepo, storageBackend)

			output, err := svc.PutObject(context.Background(), tt.input)

			if tt.wantErr != nil {
				require.Error(t, err)
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
				require.NotEmpty(t, output.ETag)
			}

			mock.AssertExpectationsForObjects(t, objRepo, blobRepo, bucketRepo, storageBackend)
		})
	}
}

func TestObjectService_GetObject(t *testing.T) {
	tests := []struct {
		name    string
		input   GetObjectInput
		setup   func(*mockObjectRepository, *mockBlobRepository2, *mockBucketRepository, *mockStorageBackend2)
		wantErr error
	}{
		{
			name: "success",
			input: GetObjectInput{
				BucketName: "test-bucket",
				Key:        "test-key.txt",
				OwnerID:    1,
			},
			setup: func(objRepo *mockObjectRepository, blobRepo *mockBlobRepository2, bucketRepo *mockBucketRepository, storageBackend *mockStorageBackend2) {
				bucket := &domain.Bucket{
					ID:      1,
					Name:    "test-bucket",
					OwnerID: 1,
				}
				bucketRepo.On("GetByName", mock.Anything, "test-bucket").Return(bucket, nil)

				contentHash := "abc123hash"
				object := &domain.Object{
					ID:          1,
					BucketID:    1,
					Key:         "test-key.txt",
					Size:        11,
					ContentType: "text/plain",
					ETag:        "abc123",
					ContentHash: &contentHash,
					IsLatest:    true,
					Metadata:    map[string]string{},
				}
				objRepo.On("GetByKey", mock.Anything, int64(1), "test-key.txt").Return(object, nil)

				storageBackend.On("Retrieve", mock.Anything, "abc123hash").Return(io.NopCloser(bytes.NewReader([]byte("hello world"))), nil)
			},
			wantErr: nil,
		},
		{
			name: "object not found",
			input: GetObjectInput{
				BucketName: "test-bucket",
				Key:        "nonexistent-key.txt",
				OwnerID:    1,
			},
			setup: func(objRepo *mockObjectRepository, blobRepo *mockBlobRepository2, bucketRepo *mockBucketRepository, storageBackend *mockStorageBackend2) {
				bucket := &domain.Bucket{
					ID:      1,
					Name:    "test-bucket",
					OwnerID: 1,
				}
				bucketRepo.On("GetByName", mock.Anything, "test-bucket").Return(bucket, nil)
				objRepo.On("GetByKey", mock.Anything, int64(1), "nonexistent-key.txt").Return(nil, domain.ErrObjectNotFound)
			},
			wantErr: domain.ErrObjectNotFound,
		},
		{
			name: "bucket not found",
			input: GetObjectInput{
				BucketName: "nonexistent-bucket",
				Key:        "test-key.txt",
				OwnerID:    1,
			},
			setup: func(objRepo *mockObjectRepository, blobRepo *mockBlobRepository2, bucketRepo *mockBucketRepository, storageBackend *mockStorageBackend2) {
				bucketRepo.On("GetByName", mock.Anything, "nonexistent-bucket").Return(nil, domain.ErrBucketNotFound)
			},
			wantErr: domain.ErrBucketNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, objRepo, blobRepo, bucketRepo, storageBackend := newTestObjectService()
			tt.setup(objRepo, blobRepo, bucketRepo, storageBackend)

			output, err := svc.GetObject(context.Background(), tt.input)

			if tt.wantErr != nil {
				require.Error(t, err)
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
				require.NotNil(t, output.Body)
				output.Body.Close()
			}

			mock.AssertExpectationsForObjects(t, objRepo, blobRepo, bucketRepo, storageBackend)
		})
	}
}

func TestObjectService_HeadObject(t *testing.T) {
	tests := []struct {
		name    string
		input   HeadObjectInput
		setup   func(*mockObjectRepository, *mockBlobRepository2, *mockBucketRepository, *mockStorageBackend2)
		wantErr error
	}{
		{
			name: "success",
			input: HeadObjectInput{
				BucketName: "test-bucket",
				Key:        "test-key.txt",
				OwnerID:    1,
			},
			setup: func(objRepo *mockObjectRepository, blobRepo *mockBlobRepository2, bucketRepo *mockBucketRepository, storageBackend *mockStorageBackend2) {
				bucket := &domain.Bucket{
					ID:      1,
					Name:    "test-bucket",
					OwnerID: 1,
				}
				bucketRepo.On("GetByName", mock.Anything, "test-bucket").Return(bucket, nil)

				object := &domain.Object{
					ID:          1,
					BucketID:    1,
					Key:         "test-key.txt",
					Size:        11,
					ContentType: "text/plain",
					ETag:        "abc123",
					IsLatest:    true,
					Metadata:    map[string]string{"custom": "value"},
					CreatedAt:   time.Now(),
				}
				objRepo.On("GetByKey", mock.Anything, int64(1), "test-key.txt").Return(object, nil)
			},
			wantErr: nil,
		},
		{
			name: "object not found",
			input: HeadObjectInput{
				BucketName: "test-bucket",
				Key:        "nonexistent.txt",
				OwnerID:    1,
			},
			setup: func(objRepo *mockObjectRepository, blobRepo *mockBlobRepository2, bucketRepo *mockBucketRepository, storageBackend *mockStorageBackend2) {
				bucket := &domain.Bucket{
					ID:      1,
					Name:    "test-bucket",
					OwnerID: 1,
				}
				bucketRepo.On("GetByName", mock.Anything, "test-bucket").Return(bucket, nil)
				objRepo.On("GetByKey", mock.Anything, int64(1), "nonexistent.txt").Return(nil, domain.ErrObjectNotFound)
			},
			wantErr: domain.ErrObjectNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, objRepo, blobRepo, bucketRepo, storageBackend := newTestObjectService()
			tt.setup(objRepo, blobRepo, bucketRepo, storageBackend)

			output, err := svc.HeadObject(context.Background(), tt.input)

			if tt.wantErr != nil {
				require.Error(t, err)
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
				require.NotEmpty(t, output.ETag)
				require.Equal(t, int64(11), output.ContentLength)
			}

			mock.AssertExpectationsForObjects(t, objRepo, blobRepo, bucketRepo, storageBackend)
		})
	}
}

func TestObjectService_DeleteObject(t *testing.T) {
	tests := []struct {
		name    string
		input   DeleteObjectInput
		setup   func(*mockObjectRepository, *mockBlobRepository2, *mockBucketRepository, *mockStorageBackend2)
		wantErr error
	}{
		{
			name: "success - non-versioned bucket",
			input: DeleteObjectInput{
				BucketName: "test-bucket",
				Key:        "test-key.txt",
				OwnerID:    1,
			},
			setup: func(objRepo *mockObjectRepository, blobRepo *mockBlobRepository2, bucketRepo *mockBucketRepository, storageBackend *mockStorageBackend2) {
				bucket := &domain.Bucket{
					ID:         1,
					Name:       "test-bucket",
					OwnerID:    1,
					Versioning: domain.VersioningDisabled,
				}
				bucketRepo.On("GetByName", mock.Anything, "test-bucket").Return(bucket, nil)

				contentHash := "abc123hash"
				object := &domain.Object{
					ID:          1,
					BucketID:    1,
					Key:         "test-key.txt",
					ContentHash: &contentHash,
				}
				objRepo.On("GetByKey", mock.Anything, int64(1), "test-key.txt").Return(object, nil)

				// Decrement blob ref count
				blobRepo.On("DecrementRef", mock.Anything, "abc123hash").Return(int32(0), nil)

				// Delete object record
				objRepo.On("Delete", mock.Anything, int64(1)).Return(nil)
			},
			wantErr: nil,
		},
		{
			name: "object not found - still returns success",
			input: DeleteObjectInput{
				BucketName: "test-bucket",
				Key:        "nonexistent.txt",
				OwnerID:    1,
			},
			setup: func(objRepo *mockObjectRepository, blobRepo *mockBlobRepository2, bucketRepo *mockBucketRepository, storageBackend *mockStorageBackend2) {
				bucket := &domain.Bucket{
					ID:         1,
					Name:       "test-bucket",
					OwnerID:    1,
					Versioning: domain.VersioningDisabled,
				}
				bucketRepo.On("GetByName", mock.Anything, "test-bucket").Return(bucket, nil)
				objRepo.On("GetByKey", mock.Anything, int64(1), "nonexistent.txt").Return(nil, domain.ErrObjectNotFound)
			},
			wantErr: nil, // S3 returns success for deleting non-existent objects
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, objRepo, blobRepo, bucketRepo, storageBackend := newTestObjectService()
			tt.setup(objRepo, blobRepo, bucketRepo, storageBackend)

			_, err := svc.DeleteObject(context.Background(), tt.input)

			if tt.wantErr != nil {
				require.Error(t, err)
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}

			mock.AssertExpectationsForObjects(t, objRepo, blobRepo, bucketRepo, storageBackend)
		})
	}
}

func TestObjectService_ListObjects(t *testing.T) {
	tests := []struct {
		name    string
		input   ListObjectsInput
		setup   func(*mockObjectRepository, *mockBlobRepository2, *mockBucketRepository, *mockStorageBackend2)
		wantErr error
	}{
		{
			name: "success - empty bucket",
			input: ListObjectsInput{
				BucketName: "test-bucket",
				Prefix:     "",
				MaxKeys:    1000,
				OwnerID:    1,
			},
			setup: func(objRepo *mockObjectRepository, blobRepo *mockBlobRepository2, bucketRepo *mockBucketRepository, storageBackend *mockStorageBackend2) {
				bucket := &domain.Bucket{
					ID:      1,
					Name:    "test-bucket",
					OwnerID: 1,
				}
				bucketRepo.On("GetByName", mock.Anything, "test-bucket").Return(bucket, nil)

				result := &repository.ObjectListResult{
					Objects:     []*domain.ObjectInfo{},
					IsTruncated: false,
				}
				objRepo.On("List", mock.Anything, int64(1), mock.AnythingOfType("repository.ObjectListOptions")).Return(result, nil)
			},
			wantErr: nil,
		},
		{
			name: "bucket not found",
			input: ListObjectsInput{
				BucketName: "nonexistent-bucket",
				MaxKeys:    1000,
				OwnerID:    1,
			},
			setup: func(objRepo *mockObjectRepository, blobRepo *mockBlobRepository2, bucketRepo *mockBucketRepository, storageBackend *mockStorageBackend2) {
				bucketRepo.On("GetByName", mock.Anything, "nonexistent-bucket").Return(nil, domain.ErrBucketNotFound)
			},
			wantErr: domain.ErrBucketNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, objRepo, blobRepo, bucketRepo, storageBackend := newTestObjectService()
			tt.setup(objRepo, blobRepo, bucketRepo, storageBackend)

			output, err := svc.ListObjects(context.Background(), tt.input)

			if tt.wantErr != nil {
				require.Error(t, err)
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
				require.NotNil(t, output)
			}

			mock.AssertExpectationsForObjects(t, objRepo, blobRepo, bucketRepo, storageBackend)
		})
	}
}

// =============================================================================
// Versioning Tests
// =============================================================================

func TestObjectService_PutObject_Versioned(t *testing.T) {
	tests := []struct {
		name    string
		input   PutObjectInput
		setup   func(*mockObjectRepository, *mockBlobRepository2, *mockBucketRepository, *mockStorageBackend2)
		wantErr error
	}{
		{
			name: "versioned bucket - creates new version",
			input: PutObjectInput{
				BucketName:  "versioned-bucket",
				Key:         "test-key.txt",
				Body:        bytes.NewReader([]byte("new content")),
				Size:        11,
				ContentType: "text/plain",
				OwnerID:     1,
			},
			setup: func(objRepo *mockObjectRepository, blobRepo *mockBlobRepository2, bucketRepo *mockBucketRepository, storageBackend *mockStorageBackend2) {
				bucket := &domain.Bucket{
					ID:         1,
					Name:       "versioned-bucket",
					OwnerID:    1,
					Versioning: domain.VersioningEnabled,
				}
				bucketRepo.On("GetByName", mock.Anything, "versioned-bucket").Return(bucket, nil)

				storageBackend.On("Store", mock.Anything, mock.Anything, int64(11)).Return("newhash123", nil)
				storageBackend.On("GetPath", "newhash123").Return("/data/ne/wh/newhash123")

				blobRepo.On("UpsertWithRefIncrement", mock.Anything, "newhash123", int64(11), "/data/ne/wh/newhash123").Return(true, nil)

				// For versioned bucket, should mark old as not latest but NOT decrement ref
				objRepo.On("MarkNotLatest", mock.Anything, int64(1), "test-key.txt").Return(nil)

				objRepo.On("Create", mock.Anything, mock.AnythingOfType("*domain.Object")).Return(nil)
			},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, objRepo, blobRepo, bucketRepo, storageBackend := newTestObjectService()
			tt.setup(objRepo, blobRepo, bucketRepo, storageBackend)

			output, err := svc.PutObject(context.Background(), tt.input)

			if tt.wantErr != nil {
				require.Error(t, err)
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
				require.NotEmpty(t, output.ETag)
				require.NotEmpty(t, output.VersionID)
				require.NotEqual(t, "null", output.VersionID)
			}

			mock.AssertExpectationsForObjects(t, objRepo, blobRepo, bucketRepo, storageBackend)
		})
	}
}

func TestObjectService_DeleteObject_Versioned(t *testing.T) {
	tests := []struct {
		name          string
		input         DeleteObjectInput
		setup         func(*mockObjectRepository, *mockBlobRepository2, *mockBucketRepository, *mockStorageBackend2)
		wantErr       error
		wantMarker    bool
		wantVersionID bool
	}{
		{
			name: "versioned bucket without versionId - creates delete marker",
			input: DeleteObjectInput{
				BucketName: "versioned-bucket",
				Key:        "test-key.txt",
				OwnerID:    1,
			},
			setup: func(objRepo *mockObjectRepository, blobRepo *mockBlobRepository2, bucketRepo *mockBucketRepository, storageBackend *mockStorageBackend2) {
				bucket := &domain.Bucket{
					ID:         1,
					Name:       "versioned-bucket",
					OwnerID:    1,
					Versioning: domain.VersioningEnabled,
				}
				bucketRepo.On("GetByName", mock.Anything, "versioned-bucket").Return(bucket, nil)

				// Should mark existing as not latest
				objRepo.On("MarkNotLatest", mock.Anything, int64(1), "test-key.txt").Return(nil)

				// Should create delete marker
				objRepo.On("Create", mock.Anything, mock.MatchedBy(func(obj *domain.Object) bool {
					return obj.IsDeleteMarker && obj.Key == "test-key.txt"
				})).Return(nil)
			},
			wantErr:       nil,
			wantMarker:    true,
			wantVersionID: true,
		},
		{
			name: "versioned bucket with versionId - hard deletes specific version",
			input: DeleteObjectInput{
				BucketName: "versioned-bucket",
				Key:        "test-key.txt",
				VersionID:  "550e8400-e29b-41d4-a716-446655440000",
				OwnerID:    1,
			},
			setup: func(objRepo *mockObjectRepository, blobRepo *mockBlobRepository2, bucketRepo *mockBucketRepository, storageBackend *mockStorageBackend2) {
				bucket := &domain.Bucket{
					ID:         1,
					Name:       "versioned-bucket",
					OwnerID:    1,
					Versioning: domain.VersioningEnabled,
				}
				bucketRepo.On("GetByName", mock.Anything, "versioned-bucket").Return(bucket, nil)

				versionUUID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
				contentHash := "abc123hash"
				obj := &domain.Object{
					ID:             1,
					BucketID:       1,
					Key:            "test-key.txt",
					VersionID:      versionUUID,
					ContentHash:    &contentHash,
					IsDeleteMarker: false,
				}
				objRepo.On("GetByKeyAndVersion", mock.Anything, int64(1), "test-key.txt", versionUUID).Return(obj, nil)

				// Should decrement ref count
				blobRepo.On("DecrementRef", mock.Anything, "abc123hash").Return(int32(0), nil)

				// Should hard delete
				objRepo.On("Delete", mock.Anything, int64(1)).Return(nil)
			},
			wantErr:       nil,
			wantMarker:    false,
			wantVersionID: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, objRepo, blobRepo, bucketRepo, storageBackend := newTestObjectService()
			tt.setup(objRepo, blobRepo, bucketRepo, storageBackend)

			output, err := svc.DeleteObject(context.Background(), tt.input)

			if tt.wantErr != nil {
				require.Error(t, err)
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.wantMarker, output.DeleteMarker)
				if tt.wantVersionID {
					require.NotEmpty(t, output.VersionID)
				}
			}

			mock.AssertExpectationsForObjects(t, objRepo, blobRepo, bucketRepo, storageBackend)
		})
	}
}

func TestObjectService_GetObject_Versioned(t *testing.T) {
	tests := []struct {
		name    string
		input   GetObjectInput
		setup   func(*mockObjectRepository, *mockBlobRepository2, *mockBucketRepository, *mockStorageBackend2)
		wantErr error
	}{
		{
			name: "get specific version",
			input: GetObjectInput{
				BucketName: "versioned-bucket",
				Key:        "test-key.txt",
				VersionID:  "550e8400-e29b-41d4-a716-446655440000",
				OwnerID:    1,
			},
			setup: func(objRepo *mockObjectRepository, blobRepo *mockBlobRepository2, bucketRepo *mockBucketRepository, storageBackend *mockStorageBackend2) {
				bucket := &domain.Bucket{
					ID:         1,
					Name:       "versioned-bucket",
					OwnerID:    1,
					Versioning: domain.VersioningEnabled,
				}
				bucketRepo.On("GetByName", mock.Anything, "versioned-bucket").Return(bucket, nil)

				versionUUID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
				contentHash := "abc123hash"
				obj := &domain.Object{
					ID:             1,
					BucketID:       1,
					Key:            "test-key.txt",
					VersionID:      versionUUID,
					ContentHash:    &contentHash,
					Size:           100,
					ContentType:    "text/plain",
					ETag:           "\"abc123\"",
					IsDeleteMarker: false,
					CreatedAt:      time.Now(),
				}
				objRepo.On("GetByKeyAndVersion", mock.Anything, int64(1), "test-key.txt", versionUUID).Return(obj, nil)

				// Return content
				content := io.NopCloser(bytes.NewReader([]byte("test content")))
				storageBackend.On("Retrieve", mock.Anything, "abc123hash").Return(content, nil)
			},
			wantErr: nil,
		},
		{
			name: "get delete marker returns error",
			input: GetObjectInput{
				BucketName: "versioned-bucket",
				Key:        "deleted-key.txt",
				OwnerID:    1,
			},
			setup: func(objRepo *mockObjectRepository, blobRepo *mockBlobRepository2, bucketRepo *mockBucketRepository, storageBackend *mockStorageBackend2) {
				bucket := &domain.Bucket{
					ID:         1,
					Name:       "versioned-bucket",
					OwnerID:    1,
					Versioning: domain.VersioningEnabled,
				}
				bucketRepo.On("GetByName", mock.Anything, "versioned-bucket").Return(bucket, nil)

				// Latest version is a delete marker
				obj := &domain.Object{
					ID:             1,
					BucketID:       1,
					Key:            "deleted-key.txt",
					VersionID:      uuid.New(),
					IsDeleteMarker: true,
					IsLatest:       true,
				}
				objRepo.On("GetByKey", mock.Anything, int64(1), "deleted-key.txt").Return(obj, nil)
			},
			wantErr: domain.ErrObjectDeleted,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, objRepo, blobRepo, bucketRepo, storageBackend := newTestObjectService()
			tt.setup(objRepo, blobRepo, bucketRepo, storageBackend)

			output, err := svc.GetObject(context.Background(), tt.input)

			if tt.wantErr != nil {
				require.Error(t, err)
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
				require.NotNil(t, output)
				require.NotEmpty(t, output.VersionID)
				output.Body.Close()
			}

			mock.AssertExpectationsForObjects(t, objRepo, blobRepo, bucketRepo, storageBackend)
		})
	}
}

func TestObjectService_ListObjectVersions(t *testing.T) {
	tests := []struct {
		name    string
		input   ListObjectVersionsInput
		setup   func(*mockObjectRepository, *mockBlobRepository2, *mockBucketRepository, *mockStorageBackend2)
		wantErr error
		check   func(*testing.T, *ListObjectVersionsOutput)
	}{
		{
			name: "success - list versions with delete markers",
			input: ListObjectVersionsInput{
				BucketName: "versioned-bucket",
				MaxKeys:    1000,
				OwnerID:    1,
			},
			setup: func(objRepo *mockObjectRepository, blobRepo *mockBlobRepository2, bucketRepo *mockBucketRepository, storageBackend *mockStorageBackend2) {
				bucket := &domain.Bucket{
					ID:         1,
					Name:       "versioned-bucket",
					OwnerID:    1,
					Versioning: domain.VersioningEnabled,
				}
				bucketRepo.On("GetByName", mock.Anything, "versioned-bucket").Return(bucket, nil)

				result := &repository.ObjectVersionListResult{
					Versions: []*domain.ObjectVersion{
						{
							Key:          "file1.txt",
							VersionID:    "version-1",
							IsLatest:     true,
							Size:         100,
							ETag:         "\"etag1\"",
							LastModified: time.Now(),
							StorageClass: domain.StorageClassStandard,
						},
						{
							Key:          "file1.txt",
							VersionID:    "version-0",
							IsLatest:     false,
							Size:         50,
							ETag:         "\"etag0\"",
							LastModified: time.Now().Add(-time.Hour),
							StorageClass: domain.StorageClassStandard,
						},
					},
					DeleteMarkers: []*domain.ObjectVersion{
						{
							Key:          "deleted-file.txt",
							VersionID:    "dm-version-1",
							IsLatest:     true,
							LastModified: time.Now(),
						},
					},
					IsTruncated: false,
				}
				objRepo.On("ListVersions", mock.Anything, int64(1), mock.AnythingOfType("repository.ObjectListOptions")).Return(result, nil)
			},
			wantErr: nil,
			check: func(t *testing.T, output *ListObjectVersionsOutput) {
				require.Equal(t, 2, len(output.Versions))
				require.Equal(t, 1, len(output.DeleteMarkers))
				require.Equal(t, "file1.txt", output.Versions[0].Key)
				require.True(t, output.Versions[0].IsLatest)
				require.Equal(t, "deleted-file.txt", output.DeleteMarkers[0].Key)
			},
		},
		{
			name: "bucket not found",
			input: ListObjectVersionsInput{
				BucketName: "nonexistent-bucket",
				MaxKeys:    1000,
				OwnerID:    1,
			},
			setup: func(objRepo *mockObjectRepository, blobRepo *mockBlobRepository2, bucketRepo *mockBucketRepository, storageBackend *mockStorageBackend2) {
				bucketRepo.On("GetByName", mock.Anything, "nonexistent-bucket").Return(nil, domain.ErrBucketNotFound)
			},
			wantErr: domain.ErrBucketNotFound,
			check:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, objRepo, blobRepo, bucketRepo, storageBackend := newTestObjectService()
			tt.setup(objRepo, blobRepo, bucketRepo, storageBackend)

			output, err := svc.ListObjectVersions(context.Background(), tt.input)

			if tt.wantErr != nil {
				require.Error(t, err)
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
				require.NotNil(t, output)
				if tt.check != nil {
					tt.check(t, output)
				}
			}

			mock.AssertExpectationsForObjects(t, objRepo, blobRepo, bucketRepo, storageBackend)
		})
	}
}
