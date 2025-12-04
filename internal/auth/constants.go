// Package auth provides AWS Signature Version 4 authentication for Alexander Storage.
// This implementation follows the AWS v4 signature specification for S3 compatibility.
package auth

import "time"

// =============================================================================
// Constants
// =============================================================================

const (
	// SignV4Algorithm is the algorithm identifier for AWS Signature Version 4.
	SignV4Algorithm = "AWS4-HMAC-SHA256"

	// ISO8601BasicFormat is the date format used in AWS v4 signatures.
	ISO8601BasicFormat = "20060102T150405Z"

	// YYYYMMDD is the short date format used in credential scope.
	YYYYMMDD = "20060102"

	// ServiceS3 is the service name for S3.
	ServiceS3 = "s3"

	// DefaultRegion is the default region if not specified.
	DefaultRegion = "us-east-1"

	// MaxSkewTime is the maximum allowed time skew for requests.
	MaxSkewTime = 15 * time.Minute

	// PresignedURLMaxExpiry is the maximum expiry time for presigned URLs (7 days).
	PresignedURLMaxExpiry = 7 * 24 * time.Hour

	// PresignedURLMinExpiry is the minimum expiry time for presigned URLs (1 second).
	PresignedURLMinExpiry = 1 * time.Second
)

// =============================================================================
// Authorization Header Constants
// =============================================================================

const (
	// AuthorizationHeader is the HTTP header for authorization.
	AuthorizationHeader = "Authorization"

	// XAmzDateHeader is the AWS date header.
	XAmzDateHeader = "X-Amz-Date"

	// XAmzContentSHA256Header is the content hash header.
	XAmzContentSHA256Header = "X-Amz-Content-Sha256"

	// XAmzSecurityTokenHeader is the session token header.
	XAmzSecurityTokenHeader = "X-Amz-Security-Token"

	// XAmzSignedHeadersHeader is the signed headers header.
	XAmzSignedHeadersHeader = "X-Amz-SignedHeaders"

	// XAmzAlgorithmHeader is the algorithm header (for presigned URLs).
	XAmzAlgorithmHeader = "X-Amz-Algorithm"

	// XAmzCredentialHeader is the credential header (for presigned URLs).
	XAmzCredentialHeader = "X-Amz-Credential"

	// XAmzExpiresHeader is the expiration header (for presigned URLs).
	XAmzExpiresHeader = "X-Amz-Expires"

	// XAmzSignatureHeader is the signature header (for presigned URLs).
	XAmzSignatureHeader = "X-Amz-Signature"
)

// =============================================================================
// Special Content Hash Values
// =============================================================================

const (
	// UnsignedPayload indicates the payload is not included in the signature.
	UnsignedPayload = "UNSIGNED-PAYLOAD"

	// StreamingPayload indicates chunked/streaming upload.
	StreamingPayload = "STREAMING-AWS4-HMAC-SHA256-PAYLOAD"

	// EmptyStringSHA256 is the SHA-256 hash of an empty string.
	EmptyStringSHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
)

// =============================================================================
// Ignored Headers
// =============================================================================

// V4IgnoredHeaders contains headers that should not be included in signature calculation.
var V4IgnoredHeaders = map[string]bool{
	"Authorization":   true,
	"User-Agent":      true,
	"Accept-Encoding": true,
	"Content-Length":  false, // Content-Length should be signed
}

// =============================================================================
// Request Scope Constants
// =============================================================================

const (
	// AWS4Request is the termination string for credential scope.
	AWS4Request = "aws4_request"
)
