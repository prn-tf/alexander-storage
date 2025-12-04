// Package service provides business logic services for Alexander Storage.
package service

import (
	"bytes"
	"context"
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
// Mock Types for MultipartService
// =============================================================================

type mockMultipartRepository struct {
	mock.Mock
}

func (m *mockMultipartRepository) Create(ctx context.Context, upload *domain.MultipartUpload) error {
	args := m.Called(ctx, upload)
	return args.Error(0)
}

func (m *mockMultipartRepository) GetByID(ctx context.Context, uploadID uuid.UUID) (*domain.MultipartUpload, error) {
	args := m.Called(ctx, uploadID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.MultipartUpload), args.Error(1)
}

func (m *mockMultipartRepository) GetByBucketAndKey(ctx context.Context, bucketID int64, key string) ([]*domain.MultipartUpload, error) {
	args := m.Called(ctx, bucketID, key)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*domain.MultipartUpload), args.Error(1)
}

func (m *mockMultipartRepository) List(ctx context.Context, bucketID int64, opts repository.MultipartListOptions) (*repository.MultipartListResult, error) {
	args := m.Called(ctx, bucketID, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*repository.MultipartListResult), args.Error(1)
}

func (m *mockMultipartRepository) UpdateStatus(ctx context.Context, uploadID uuid.UUID, status domain.MultipartStatus) error {
	args := m.Called(ctx, uploadID, status)
	return args.Error(0)
}

func (m *mockMultipartRepository) Delete(ctx context.Context, uploadID uuid.UUID) error {
	args := m.Called(ctx, uploadID)
	return args.Error(0)
}

func (m *mockMultipartRepository) DeleteExpired(ctx context.Context) (int64, error) {
	args := m.Called(ctx)
	return args.Get(0).(int64), args.Error(1)
}

func (m *mockMultipartRepository) CreatePart(ctx context.Context, part *domain.UploadPart) error {
	args := m.Called(ctx, part)
	return args.Error(0)
}

func (m *mockMultipartRepository) GetPart(ctx context.Context, uploadID uuid.UUID, partNumber int) (*domain.UploadPart, error) {
	args := m.Called(ctx, uploadID, partNumber)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.UploadPart), args.Error(1)
}

func (m *mockMultipartRepository) ListParts(ctx context.Context, uploadID uuid.UUID, opts repository.PartListOptions) (*repository.PartListResult, error) {
	args := m.Called(ctx, uploadID, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*repository.PartListResult), args.Error(1)
}

func (m *mockMultipartRepository) DeleteParts(ctx context.Context, uploadID uuid.UUID) error {
	args := m.Called(ctx, uploadID)
	return args.Error(0)
}

func (m *mockMultipartRepository) GetPartsForCompletion(ctx context.Context, uploadID uuid.UUID, partNumbers []int) ([]*domain.UploadPart, error) {
	args := m.Called(ctx, uploadID, partNumbers)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*domain.UploadPart), args.Error(1)
}

// =============================================================================
// Helper Functions
// =============================================================================

func newTestMultipartService(t *testing.T) (*MultipartService, *mockMultipartRepository, *mockObjectRepository, *mockBlobRepository2, *mockBucketRepository, *mockStorageBackend2) {
	multipartRepo := new(mockMultipartRepository)
	objectRepo := new(mockObjectRepository)
	blobRepo := new(mockBlobRepository2)
	bucketRepo := new(mockBucketRepository)
	storage := new(mockStorageBackend2)
	locker := lock.NewNoOpLocker()

	logger := zerolog.Nop()
	svc := NewMultipartService(multipartRepo, objectRepo, blobRepo, bucketRepo, storage, locker, logger)

	return svc, multipartRepo, objectRepo, blobRepo, bucketRepo, storage
}

// =============================================================================
// InitiateMultipartUpload Tests
// =============================================================================

func TestMultipartService_InitiateMultipartUpload_Success(t *testing.T) {
	svc, multipartRepo, _, _, bucketRepo, _ := newTestMultipartService(t)

	bucketRepo.On("GetByName", mock.Anything, "test-bucket").Return(&domain.Bucket{
		ID:      1,
		Name:    "test-bucket",
		OwnerID: 1,
	}, nil)
	multipartRepo.On("Create", mock.Anything, mock.AnythingOfType("*domain.MultipartUpload")).Return(nil)

	out, err := svc.InitiateMultipartUpload(context.Background(), InitiateMultipartUploadInput{
		BucketName:  "test-bucket",
		Key:         "test-key.txt",
		ContentType: "text/plain",
		OwnerID:     1,
	})

	require.NoError(t, err)
	require.NotEmpty(t, out.UploadID)
	require.Equal(t, "test-bucket", out.Bucket)
	require.Equal(t, "test-key.txt", out.Key)
}

func TestMultipartService_InitiateMultipartUpload_BucketNotFound(t *testing.T) {
	svc, _, _, _, bucketRepo, _ := newTestMultipartService(t)

	bucketRepo.On("GetByName", mock.Anything, "nonexistent-bucket").Return(nil, domain.ErrBucketNotFound)

	_, err := svc.InitiateMultipartUpload(context.Background(), InitiateMultipartUploadInput{
		BucketName: "nonexistent-bucket",
		Key:        "test-key.txt",
		OwnerID:    1,
	})

	require.ErrorIs(t, err, domain.ErrBucketNotFound)
}

func TestMultipartService_InitiateMultipartUpload_EmptyKey(t *testing.T) {
	svc, _, _, _, _, _ := newTestMultipartService(t)

	_, err := svc.InitiateMultipartUpload(context.Background(), InitiateMultipartUploadInput{
		BucketName: "test-bucket",
		Key:        "",
		OwnerID:    1,
	})

	require.ErrorIs(t, err, domain.ErrObjectKeyEmpty)
}

// =============================================================================
// UploadPart Tests
// =============================================================================

func TestMultipartService_UploadPart_Success(t *testing.T) {
	svc, multipartRepo, _, blobRepo, bucketRepo, storage := newTestMultipartService(t)
	uploadID := uuid.New()

	bucketRepo.On("GetByName", mock.Anything, "test-bucket").Return(&domain.Bucket{
		ID:      1,
		Name:    "test-bucket",
		OwnerID: 1,
	}, nil)
	multipartRepo.On("GetByID", mock.Anything, uploadID).Return(&domain.MultipartUpload{
		ID:          uploadID,
		BucketID:    1,
		Key:         "test-key.txt",
		Status:      domain.MultipartStatusInProgress,
		InitiatedAt: time.Now(),
		ExpiresAt:   time.Now().Add(24 * time.Hour),
	}, nil)
	multipartRepo.On("GetPart", mock.Anything, uploadID, 1).Return(nil, repository.ErrNotFound)
	storage.On("Store", mock.Anything, mock.Anything, int64(12)).Return("abc123hash", nil)
	storage.On("GetPath", "abc123hash").Return("/data/ab/c1/abc123hash")
	blobRepo.On("UpsertWithRefIncrement", mock.Anything, "abc123hash", int64(12), "/data/ab/c1/abc123hash").Return(true, nil)
	multipartRepo.On("CreatePart", mock.Anything, mock.AnythingOfType("*domain.UploadPart")).Return(nil)

	out, err := svc.UploadPart(context.Background(), UploadPartInput{
		BucketName: "test-bucket",
		Key:        "test-key.txt",
		UploadID:   uploadID.String(),
		PartNumber: 1,
		Body:       bytes.NewReader([]byte("test content")),
		Size:       12,
	})

	require.NoError(t, err)
	require.NotEmpty(t, out.ETag)
}

func TestMultipartService_UploadPart_InvalidPartNumber_Zero(t *testing.T) {
	svc, _, _, _, _, _ := newTestMultipartService(t)
	uploadID := uuid.New()

	_, err := svc.UploadPart(context.Background(), UploadPartInput{
		BucketName: "test-bucket",
		Key:        "test-key.txt",
		UploadID:   uploadID.String(),
		PartNumber: 0,
		Body:       bytes.NewReader([]byte("test content")),
	})

	require.ErrorIs(t, err, domain.ErrInvalidPartNumber)
}

func TestMultipartService_UploadPart_InvalidPartNumber_TooLarge(t *testing.T) {
	svc, _, _, _, _, _ := newTestMultipartService(t)
	uploadID := uuid.New()

	_, err := svc.UploadPart(context.Background(), UploadPartInput{
		BucketName: "test-bucket",
		Key:        "test-key.txt",
		UploadID:   uploadID.String(),
		PartNumber: 10001,
		Body:       bytes.NewReader([]byte("test content")),
	})

	require.ErrorIs(t, err, domain.ErrInvalidPartNumber)
}

func TestMultipartService_UploadPart_UploadNotFound(t *testing.T) {
	svc, multipartRepo, _, _, bucketRepo, _ := newTestMultipartService(t)
	uploadID := uuid.New()

	bucketRepo.On("GetByName", mock.Anything, "test-bucket").Return(&domain.Bucket{
		ID:      1,
		Name:    "test-bucket",
		OwnerID: 1,
	}, nil)
	multipartRepo.On("GetByID", mock.Anything, uploadID).Return(nil, domain.ErrMultipartUploadNotFound)

	_, err := svc.UploadPart(context.Background(), UploadPartInput{
		BucketName: "test-bucket",
		Key:        "test-key.txt",
		UploadID:   uploadID.String(),
		PartNumber: 1,
		Body:       bytes.NewReader([]byte("test content")),
	})

	require.ErrorIs(t, err, domain.ErrMultipartUploadNotFound)
}

// =============================================================================
// AbortMultipartUpload Tests
// =============================================================================

func TestMultipartService_AbortMultipartUpload_Success(t *testing.T) {
	svc, multipartRepo, _, blobRepo, bucketRepo, _ := newTestMultipartService(t)
	uploadID := uuid.New()

	bucketRepo.On("GetByName", mock.Anything, "test-bucket").Return(&domain.Bucket{
		ID:      1,
		Name:    "test-bucket",
		OwnerID: 1,
	}, nil)
	multipartRepo.On("GetByID", mock.Anything, uploadID).Return(&domain.MultipartUpload{
		ID:          uploadID,
		BucketID:    1,
		Key:         "test-key.txt",
		Status:      domain.MultipartStatusInProgress,
		InitiatedAt: time.Now(),
		ExpiresAt:   time.Now().Add(24 * time.Hour),
	}, nil)
	multipartRepo.On("ListParts", mock.Anything, uploadID, mock.AnythingOfType("repository.PartListOptions")).Return(&repository.PartListResult{
		Parts: []*domain.PartInfo{
			{PartNumber: 1},
		},
		IsTruncated: false,
	}, nil)
	multipartRepo.On("GetPart", mock.Anything, uploadID, 1).Return(&domain.UploadPart{
		UploadID:    uploadID,
		PartNumber:  1,
		ContentHash: "abc123hash",
	}, nil)
	blobRepo.On("DecrementRef", mock.Anything, "abc123hash").Return(int32(0), nil)
	multipartRepo.On("Delete", mock.Anything, uploadID).Return(nil)

	err := svc.AbortMultipartUpload(context.Background(), AbortMultipartUploadInput{
		BucketName: "test-bucket",
		Key:        "test-key.txt",
		UploadID:   uploadID.String(),
	})

	require.NoError(t, err)
}

func TestMultipartService_AbortMultipartUpload_UploadNotFound(t *testing.T) {
	svc, multipartRepo, _, _, bucketRepo, _ := newTestMultipartService(t)
	uploadID := uuid.New()

	bucketRepo.On("GetByName", mock.Anything, "test-bucket").Return(&domain.Bucket{
		ID:      1,
		Name:    "test-bucket",
		OwnerID: 1,
	}, nil)
	multipartRepo.On("GetByID", mock.Anything, uploadID).Return(nil, domain.ErrMultipartUploadNotFound)

	err := svc.AbortMultipartUpload(context.Background(), AbortMultipartUploadInput{
		BucketName: "test-bucket",
		Key:        "test-key.txt",
		UploadID:   uploadID.String(),
	})

	require.ErrorIs(t, err, domain.ErrMultipartUploadNotFound)
}

func TestMultipartService_AbortMultipartUpload_InvalidUploadID(t *testing.T) {
	svc, _, _, _, _, _ := newTestMultipartService(t)

	err := svc.AbortMultipartUpload(context.Background(), AbortMultipartUploadInput{
		BucketName: "test-bucket",
		Key:        "test-key.txt",
		UploadID:   "invalid-uuid",
	})

	require.ErrorIs(t, err, domain.ErrMultipartUploadNotFound)
}

// =============================================================================
// ListMultipartUploads Tests
// =============================================================================

func TestMultipartService_ListMultipartUploads_Success(t *testing.T) {
	svc, multipartRepo, _, _, bucketRepo, _ := newTestMultipartService(t)
	uploadID := uuid.New()

	bucketRepo.On("GetByName", mock.Anything, "test-bucket").Return(&domain.Bucket{
		ID:      1,
		Name:    "test-bucket",
		OwnerID: 1,
	}, nil)
	multipartRepo.On("List", mock.Anything, int64(1), mock.AnythingOfType("repository.MultipartListOptions")).Return(&repository.MultipartListResult{
		Uploads: []*domain.MultipartUploadInfo{
			{
				UploadID:  uploadID.String(),
				Key:       "test-key.txt",
				Initiated: time.Now(),
			},
		},
		IsTruncated: false,
	}, nil)

	out, err := svc.ListMultipartUploads(context.Background(), ListMultipartUploadsInput{
		BucketName: "test-bucket",
		MaxUploads: 100,
	})

	require.NoError(t, err)
	require.Len(t, out.Uploads, 1)
	require.Equal(t, "test-key.txt", out.Uploads[0].Key)
	require.False(t, out.IsTruncated)
}

func TestMultipartService_ListMultipartUploads_BucketNotFound(t *testing.T) {
	svc, _, _, _, bucketRepo, _ := newTestMultipartService(t)

	bucketRepo.On("GetByName", mock.Anything, "nonexistent-bucket").Return(nil, domain.ErrBucketNotFound)

	_, err := svc.ListMultipartUploads(context.Background(), ListMultipartUploadsInput{
		BucketName: "nonexistent-bucket",
	})

	require.ErrorIs(t, err, domain.ErrBucketNotFound)
}

// =============================================================================
// ListParts Tests
// =============================================================================

func TestMultipartService_ListParts_Success(t *testing.T) {
	svc, multipartRepo, _, _, bucketRepo, _ := newTestMultipartService(t)
	uploadID := uuid.New()

	bucketRepo.On("GetByName", mock.Anything, "test-bucket").Return(&domain.Bucket{
		ID:      1,
		Name:    "test-bucket",
		OwnerID: 1,
	}, nil)
	multipartRepo.On("GetByID", mock.Anything, uploadID).Return(&domain.MultipartUpload{
		ID:          uploadID,
		BucketID:    1,
		Key:         "test-key.txt",
		Status:      domain.MultipartStatusInProgress,
		InitiatedAt: time.Now(),
		ExpiresAt:   time.Now().Add(24 * time.Hour),
	}, nil)
	multipartRepo.On("ListParts", mock.Anything, uploadID, mock.AnythingOfType("repository.PartListOptions")).Return(&repository.PartListResult{
		Parts: []*domain.PartInfo{
			{
				PartNumber:   1,
				Size:         1024,
				ETag:         "\"abc123\"",
				LastModified: time.Now(),
			},
		},
		IsTruncated: false,
	}, nil)

	out, err := svc.ListParts(context.Background(), ListPartsInput{
		BucketName: "test-bucket",
		Key:        "test-key.txt",
		UploadID:   uploadID.String(),
		MaxParts:   100,
	})

	require.NoError(t, err)
	require.Len(t, out.Parts, 1)
	require.Equal(t, 1, out.Parts[0].PartNumber)
	require.Equal(t, int64(1024), out.Parts[0].Size)
	require.False(t, out.IsTruncated)
}

func TestMultipartService_ListParts_UploadNotFound(t *testing.T) {
	svc, multipartRepo, _, _, bucketRepo, _ := newTestMultipartService(t)
	uploadID := uuid.New()

	bucketRepo.On("GetByName", mock.Anything, "test-bucket").Return(&domain.Bucket{
		ID:      1,
		Name:    "test-bucket",
		OwnerID: 1,
	}, nil)
	multipartRepo.On("GetByID", mock.Anything, uploadID).Return(nil, domain.ErrMultipartUploadNotFound)

	_, err := svc.ListParts(context.Background(), ListPartsInput{
		BucketName: "test-bucket",
		Key:        "test-key.txt",
		UploadID:   uploadID.String(),
	})

	require.ErrorIs(t, err, domain.ErrMultipartUploadNotFound)
}

func TestMultipartService_ListParts_InvalidUploadID(t *testing.T) {
	svc, _, _, _, _, _ := newTestMultipartService(t)

	_, err := svc.ListParts(context.Background(), ListPartsInput{
		BucketName: "test-bucket",
		Key:        "test-key.txt",
		UploadID:   "invalid-uuid",
	})

	require.ErrorIs(t, err, domain.ErrMultipartUploadNotFound)
}
