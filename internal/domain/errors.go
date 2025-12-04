// Package domain contains the core business entities for Alexander Storage.
package domain

import (
	"errors"
	"fmt"
)

// Domain errors - these represent business rule violations.
// They are distinct from infrastructure errors (database, network, etc.).

var (
	// ===========================================
	// User Errors
	// ===========================================

	// ErrUserNotFound indicates the requested user does not exist.
	ErrUserNotFound = errors.New("user not found")

	// ErrUserAlreadyExists indicates a user with the same username/email exists.
	ErrUserAlreadyExists = errors.New("user already exists")

	// ErrUserInactive indicates the user account is disabled.
	ErrUserInactive = errors.New("user account is inactive")

	// ErrInvalidCredentials indicates authentication failed.
	ErrInvalidCredentials = errors.New("invalid credentials")

	// ===========================================
	// Access Key Errors
	// ===========================================

	// ErrAccessKeyNotFound indicates the requested access key does not exist.
	ErrAccessKeyNotFound = errors.New("access key not found")

	// ErrAccessKeyInactive indicates the access key is disabled.
	ErrAccessKeyInactive = errors.New("access key is inactive")

	// ErrAccessKeyExpired indicates the access key has expired.
	ErrAccessKeyExpired = errors.New("access key has expired")

	// ErrInvalidAccessKeyID indicates the access key ID format is invalid.
	ErrInvalidAccessKeyID = errors.New("invalid access key ID")

	// ErrInvalidSecretKey indicates the secret key format is invalid.
	ErrInvalidSecretKey = errors.New("invalid secret key")

	// ===========================================
	// Bucket Errors
	// ===========================================

	// ErrBucketNotFound indicates the requested bucket does not exist.
	ErrBucketNotFound = errors.New("bucket not found")

	// ErrBucketAlreadyExists indicates a bucket with the same name exists.
	ErrBucketAlreadyExists = errors.New("bucket already exists")

	// ErrBucketNotEmpty indicates the bucket contains objects and cannot be deleted.
	ErrBucketNotEmpty = errors.New("bucket is not empty")

	// ErrBucketNameLength indicates the bucket name length is invalid (3-63 chars).
	ErrBucketNameLength = errors.New("bucket name must be between 3 and 63 characters")

	// ErrBucketNameFormat indicates the bucket name format is invalid.
	ErrBucketNameFormat = errors.New("bucket name must contain only lowercase letters, numbers, hyphens, and periods")

	// ErrBucketNameIPFormat indicates the bucket name looks like an IP address.
	ErrBucketNameIPFormat = errors.New("bucket name cannot be formatted as an IP address")

	// ===========================================
	// Object Errors
	// ===========================================

	// ErrObjectNotFound indicates the requested object does not exist.
	ErrObjectNotFound = errors.New("object not found")

	// ErrObjectKeyTooLong indicates the object key exceeds maximum length.
	ErrObjectKeyTooLong = errors.New("object key exceeds maximum length of 1024 characters")

	// ErrObjectDeleted indicates the object has been deleted (is a delete marker).
	ErrObjectDeleted = errors.New("object has been deleted")

	// ErrVersionNotFound indicates the requested version does not exist.
	ErrVersionNotFound = errors.New("version not found")

	// ===========================================
	// Blob/Storage Errors
	// ===========================================

	// ErrBlobNotFound indicates the requested blob does not exist.
	ErrBlobNotFound = errors.New("blob not found")

	// ErrBlobCorrupted indicates the blob content does not match its hash.
	ErrBlobCorrupted = errors.New("blob content is corrupted")

	// ErrStorageFull indicates the storage backend has no space.
	ErrStorageFull = errors.New("storage is full")

	// ===========================================
	// Multipart Upload Errors
	// ===========================================

	// ErrMultipartUploadNotFound indicates the upload does not exist.
	ErrMultipartUploadNotFound = errors.New("multipart upload not found")

	// ErrMultipartUploadExpired indicates the upload has expired.
	ErrMultipartUploadExpired = errors.New("multipart upload has expired")

	// ErrMultipartUploadCompleted indicates the upload is already completed.
	ErrMultipartUploadCompleted = errors.New("multipart upload is already completed")

	// ErrMultipartUploadAborted indicates the upload has been aborted.
	ErrMultipartUploadAborted = errors.New("multipart upload has been aborted")

	// ErrInvalidPartNumber indicates the part number is outside valid range (1-10000).
	ErrInvalidPartNumber = errors.New("part number must be between 1 and 10000")

	// ErrPartTooSmall indicates the part size is below minimum (5MB, except last part).
	ErrPartTooSmall = errors.New("part size is below minimum (5MB)")

	// ErrPartTooLarge indicates the part size exceeds maximum (5GB).
	ErrPartTooLarge = errors.New("part size exceeds maximum (5GB)")

	// ErrPartNotFound indicates the specified part does not exist.
	ErrPartNotFound = errors.New("part not found")

	// ErrPartETagMismatch indicates the part ETag does not match.
	ErrPartETagMismatch = errors.New("part ETag mismatch")

	// ErrInvalidPartOrder indicates parts are not in ascending order.
	ErrInvalidPartOrder = errors.New("parts must be in ascending order")

	// ErrNoPartsProvided indicates no parts were provided for completion.
	ErrNoPartsProvided = errors.New("no parts provided for completion")

	// ===========================================
	// Authentication/Authorization Errors
	// ===========================================

	// ErrAccessDenied indicates the user does not have permission.
	ErrAccessDenied = errors.New("access denied")

	// ErrSignatureDoesNotMatch indicates the request signature is invalid.
	ErrSignatureDoesNotMatch = errors.New("signature does not match")

	// ErrRequestExpired indicates the request has expired.
	ErrRequestExpired = errors.New("request has expired")

	// ErrMissingSecurityHeader indicates a required security header is missing.
	ErrMissingSecurityHeader = errors.New("missing required security header")

	// ===========================================
	// Presigned URL Errors
	// ===========================================

	// ErrPresignedURLExpired indicates the presigned URL has expired.
	ErrPresignedURLExpired = errors.New("presigned URL has expired")

	// ErrInvalidPresignedURL indicates the presigned URL is malformed.
	ErrInvalidPresignedURL = errors.New("invalid presigned URL")
)

// DomainError wraps a domain error with additional context.
type DomainError struct {
	// Err is the underlying domain error.
	Err error

	// Message provides additional context.
	Message string

	// Resource identifies the affected resource (e.g., bucket name, object key).
	Resource string
}

// Error implements the error interface.
func (e *DomainError) Error() string {
	if e.Resource != "" {
		return fmt.Sprintf("%s: %s (%s)", e.Err.Error(), e.Message, e.Resource)
	}
	if e.Message != "" {
		return fmt.Sprintf("%s: %s", e.Err.Error(), e.Message)
	}
	return e.Err.Error()
}

// Unwrap returns the underlying error for errors.Is/errors.As.
func (e *DomainError) Unwrap() error {
	return e.Err
}

// NewDomainError creates a new DomainError with context.
func NewDomainError(err error, message, resource string) *DomainError {
	return &DomainError{
		Err:      err,
		Message:  message,
		Resource: resource,
	}
}

// WrapError wraps an error with domain context if it's not already a DomainError.
func WrapError(err error, message string) error {
	if err == nil {
		return nil
	}

	var domainErr *DomainError
	if errors.As(err, &domainErr) {
		return err
	}

	return &DomainError{
		Err:     err,
		Message: message,
	}
}
