// Package integration provides end-to-end tests for Alexander Storage S3 API.
package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/stretchr/testify/require"
)

// TestConfig holds the configuration for integration tests.
type TestConfig struct {
	Endpoint        string
	AccessKeyID     string
	SecretAccessKey string
	Region          string
}

// getTestConfig reads test configuration from environment variables.
func getTestConfig() TestConfig {
	return TestConfig{
		Endpoint:        getEnv("ALEXANDER_ENDPOINT", "http://localhost:8080"),
		AccessKeyID:     getEnv("ALEXANDER_ACCESS_KEY_ID", "test-access-key"),
		SecretAccessKey: getEnv("ALEXANDER_SECRET_ACCESS_KEY", "test-secret-key"),
		Region:          getEnv("ALEXANDER_REGION", "us-east-1"),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// newS3Client creates a new S3 client configured for Alexander Storage.
func newS3Client(t *testing.T, cfg TestConfig) *s3.Client {
	t.Helper()

	customResolver := aws.EndpointResolverWithOptionsFunc(
		func(service, region string, options ...interface{}) (aws.Endpoint, error) {
			return aws.Endpoint{
				URL:               cfg.Endpoint,
				HostnameImmutable: true,
				SigningRegion:     cfg.Region,
			}, nil
		},
	)

	awsCfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(cfg.Region),
		config.WithEndpointResolverWithOptions(customResolver),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.AccessKeyID,
			cfg.SecretAccessKey,
			"",
		)),
	)
	require.NoError(t, err)

	return s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.UsePathStyle = true
	})
}

// TestBucketOperations tests basic bucket CRUD operations.
func TestBucketOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cfg := getTestConfig()
	client := newS3Client(t, cfg)
	ctx := context.Background()

	bucketName := "test-bucket-" + time.Now().Format("20060102150405")

	t.Run("CreateBucket", func(t *testing.T) {
		_, err := client.CreateBucket(ctx, &s3.CreateBucketInput{
			Bucket: aws.String(bucketName),
		})
		require.NoError(t, err)
	})

	t.Run("HeadBucket", func(t *testing.T) {
		_, err := client.HeadBucket(ctx, &s3.HeadBucketInput{
			Bucket: aws.String(bucketName),
		})
		require.NoError(t, err)
	})

	t.Run("ListBuckets", func(t *testing.T) {
		result, err := client.ListBuckets(ctx, &s3.ListBucketsInput{})
		require.NoError(t, err)

		found := false
		for _, bucket := range result.Buckets {
			if *bucket.Name == bucketName {
				found = true
				break
			}
		}
		require.True(t, found, "created bucket should appear in list")
	})

	t.Run("DeleteBucket", func(t *testing.T) {
		_, err := client.DeleteBucket(ctx, &s3.DeleteBucketInput{
			Bucket: aws.String(bucketName),
		})
		require.NoError(t, err)
	})

	t.Run("HeadBucket_NotFound", func(t *testing.T) {
		_, err := client.HeadBucket(ctx, &s3.HeadBucketInput{
			Bucket: aws.String(bucketName),
		})
		require.Error(t, err)
	})
}

// TestBucketVersioning tests bucket versioning operations.
func TestBucketVersioning(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cfg := getTestConfig()
	client := newS3Client(t, cfg)
	ctx := context.Background()

	bucketName := "test-versioning-" + time.Now().Format("20060102150405")

	// Create bucket
	_, err := client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(bucketName),
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		// Clean up: delete bucket
		_, _ = client.DeleteBucket(ctx, &s3.DeleteBucketInput{
			Bucket: aws.String(bucketName),
		})
	})

	t.Run("GetBucketVersioning_Initial", func(t *testing.T) {
		result, err := client.GetBucketVersioning(ctx, &s3.GetBucketVersioningInput{
			Bucket: aws.String(bucketName),
		})
		require.NoError(t, err)
		// Initially versioning is not set
		require.Empty(t, result.Status)
	})

	t.Run("PutBucketVersioning_Enable", func(t *testing.T) {
		_, err := client.PutBucketVersioning(ctx, &s3.PutBucketVersioningInput{
			Bucket: aws.String(bucketName),
			VersioningConfiguration: &types.VersioningConfiguration{
				Status: types.BucketVersioningStatusEnabled,
			},
		})
		require.NoError(t, err)
	})

	t.Run("GetBucketVersioning_Enabled", func(t *testing.T) {
		result, err := client.GetBucketVersioning(ctx, &s3.GetBucketVersioningInput{
			Bucket: aws.String(bucketName),
		})
		require.NoError(t, err)
		require.Equal(t, types.BucketVersioningStatusEnabled, result.Status)
	})

	t.Run("PutBucketVersioning_Suspend", func(t *testing.T) {
		_, err := client.PutBucketVersioning(ctx, &s3.PutBucketVersioningInput{
			Bucket: aws.String(bucketName),
			VersioningConfiguration: &types.VersioningConfiguration{
				Status: types.BucketVersioningStatusSuspended,
			},
		})
		require.NoError(t, err)
	})

	t.Run("GetBucketVersioning_Suspended", func(t *testing.T) {
		result, err := client.GetBucketVersioning(ctx, &s3.GetBucketVersioningInput{
			Bucket: aws.String(bucketName),
		})
		require.NoError(t, err)
		require.Equal(t, types.BucketVersioningStatusSuspended, result.Status)
	})
}
