// Package auth provides AWS Signature Version 4 authentication for Alexander Storage.
package auth

import "errors"

// Authentication and signature errors.
var (
	// ErrInvalidAuthorizationHeader indicates the Authorization header is malformed.
	ErrInvalidAuthorizationHeader = errors.New("invalid authorization header")

	// ErrSignatureDoesNotMatch indicates the calculated signature doesn't match.
	ErrSignatureDoesNotMatch = errors.New("the request signature we calculated does not match the signature you provided")

	// ErrMissingSecurityHeader indicates a required security header is missing.
	ErrMissingSecurityHeader = errors.New("missing required security header")

	// ErrRequestTimeTooSkewed indicates the request time is too far from server time.
	ErrRequestTimeTooSkewed = errors.New("the difference between the request time and the server time is too large")

	// ErrAccessDenied indicates the request is not authorized.
	ErrAccessDenied = errors.New("access denied")

	// ErrInvalidAccessKeyID indicates the access key ID is not found or invalid.
	ErrInvalidAccessKeyID = errors.New("the access key ID you provided does not exist in our records")

	// ErrInvalidPresignedURL indicates the presigned URL is malformed or invalid.
	ErrInvalidPresignedURL = errors.New("invalid presigned URL")

	// ErrPresignedURLExpired indicates the presigned URL has expired.
	ErrPresignedURLExpired = errors.New("request has expired")

	// ErrPresignedURLNotYetValid indicates the presigned URL is not yet valid.
	ErrPresignedURLNotYetValid = errors.New("request is not yet valid")
)

// S3ErrorCode represents S3 error codes for proper API responses.
type S3ErrorCode string

const (
	// S3ErrorAccessDenied maps to HTTP 403
	S3ErrorAccessDenied S3ErrorCode = "AccessDenied"

	// S3ErrorSignatureDoesNotMatch maps to HTTP 403
	S3ErrorSignatureDoesNotMatch S3ErrorCode = "SignatureDoesNotMatch"

	// S3ErrorInvalidAccessKeyId maps to HTTP 403
	S3ErrorInvalidAccessKeyId S3ErrorCode = "InvalidAccessKeyId"

	// S3ErrorRequestTimeTooSkewed maps to HTTP 403
	S3ErrorRequestTimeTooSkewed S3ErrorCode = "RequestTimeTooSkewed"

	// S3ErrorExpiredToken maps to HTTP 400
	S3ErrorExpiredToken S3ErrorCode = "ExpiredToken"

	// S3ErrorMissingSecurityHeader maps to HTTP 400
	S3ErrorMissingSecurityHeader S3ErrorCode = "MissingSecurityHeader"

	// S3ErrorAuthorizationHeaderMalformed maps to HTTP 400
	S3ErrorAuthorizationHeaderMalformed S3ErrorCode = "AuthorizationHeaderMalformed"
)

// AuthError represents an authentication error with S3-compatible error code.
type AuthError struct {
	// Code is the S3 error code.
	Code S3ErrorCode

	// Message is the error message.
	Message string

	// HTTPStatus is the HTTP status code.
	HTTPStatus int

	// Resource is the affected resource (optional).
	Resource string

	// RequestID is the request ID (optional).
	RequestID string
}

func (e *AuthError) Error() string {
	return string(e.Code) + ": " + e.Message
}

// NewAuthError creates a new AuthError from a standard error.
func NewAuthError(err error) *AuthError {
	switch {
	case errors.Is(err, ErrSignatureDoesNotMatch):
		return &AuthError{
			Code:       S3ErrorSignatureDoesNotMatch,
			Message:    err.Error(),
			HTTPStatus: 403,
		}

	case errors.Is(err, ErrInvalidAccessKeyID):
		return &AuthError{
			Code:       S3ErrorInvalidAccessKeyId,
			Message:    err.Error(),
			HTTPStatus: 403,
		}

	case errors.Is(err, ErrRequestTimeTooSkewed):
		return &AuthError{
			Code:       S3ErrorRequestTimeTooSkewed,
			Message:    err.Error(),
			HTTPStatus: 403,
		}

	case errors.Is(err, ErrMissingSecurityHeader):
		return &AuthError{
			Code:       S3ErrorMissingSecurityHeader,
			Message:    err.Error(),
			HTTPStatus: 400,
		}

	case errors.Is(err, ErrInvalidAuthorizationHeader):
		return &AuthError{
			Code:       S3ErrorAuthorizationHeaderMalformed,
			Message:    err.Error(),
			HTTPStatus: 400,
		}

	case errors.Is(err, ErrPresignedURLExpired):
		return &AuthError{
			Code:       S3ErrorExpiredToken,
			Message:    err.Error(),
			HTTPStatus: 400,
		}

	default:
		return &AuthError{
			Code:       S3ErrorAccessDenied,
			Message:    err.Error(),
			HTTPStatus: 403,
		}
	}
}
