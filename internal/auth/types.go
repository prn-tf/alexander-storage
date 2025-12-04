// Package auth provides AWS Signature Version 4 authentication for Alexander Storage.
package auth

import (
	"time"
)

// =============================================================================
// Credential Types
// =============================================================================

// CredentialScope represents the scope of AWS credentials.
// Format: {date}/{region}/{service}/aws4_request
type CredentialScope struct {
	// Date is the date portion of the scope (YYYYMMDD).
	Date time.Time

	// Region is the AWS region (e.g., "us-east-1").
	Region string

	// Service is the AWS service (e.g., "s3").
	Service string
}

// String returns the credential scope as a string.
// Format: {date}/{region}/{service}/aws4_request
func (cs CredentialScope) String() string {
	return cs.Date.Format(YYYYMMDD) + "/" + cs.Region + "/" + cs.Service + "/" + AWS4Request
}

// CredentialHeader represents parsed AWS credentials from the Authorization header.
type CredentialHeader struct {
	// AccessKey is the access key ID.
	AccessKey string

	// Scope is the credential scope.
	Scope CredentialScope
}

// String returns the credential as a string.
// Format: {access_key}/{scope}
func (ch CredentialHeader) String() string {
	return ch.AccessKey + "/" + ch.Scope.String()
}

// =============================================================================
// Signature Types
// =============================================================================

// SignedValues represents the components of an AWS v4 signature.
// These are parsed from the Authorization header.
type SignedValues struct {
	// Credential contains the access key and scope.
	Credential CredentialHeader

	// SignedHeaders is the list of headers included in the signature.
	SignedHeaders []string

	// Signature is the calculated signature (hex-encoded).
	Signature string
}

// AuthType represents the type of authentication used in a request.
type AuthType int

const (
	// AuthTypeUnknown indicates an unrecognized auth type.
	AuthTypeUnknown AuthType = iota

	// AuthTypeAnonymous indicates no authentication (public access).
	AuthTypeAnonymous

	// AuthTypeSignedV4 indicates AWS Signature Version 4 in the Authorization header.
	AuthTypeSignedV4

	// AuthTypePresignedV4 indicates AWS Signature Version 4 in query parameters.
	AuthTypePresignedV4

	// AuthTypeStreamingSigned indicates chunked upload with streaming signature.
	AuthTypeStreamingSigned
)

// String returns the string representation of the auth type.
func (at AuthType) String() string {
	switch at {
	case AuthTypeAnonymous:
		return "Anonymous"
	case AuthTypeSignedV4:
		return "SignedV4"
	case AuthTypePresignedV4:
		return "PresignedV4"
	case AuthTypeStreamingSigned:
		return "StreamingSigned"
	default:
		return "Unknown"
	}
}

// =============================================================================
// Context Types
// =============================================================================

// AuthContext contains authentication information attached to a request.
// This is set by the auth middleware after successful authentication.
type AuthContext struct {
	// UserID is the authenticated user's ID.
	UserID int64

	// AccessKeyID is the access key used for authentication.
	AccessKeyID string

	// Credential contains the full credential information.
	Credential CredentialHeader

	// AuthType is the type of authentication used.
	AuthType AuthType

	// RequestTime is the time the request was signed.
	RequestTime time.Time

	// Region is the region from the credential scope.
	Region string
}

// authContextKey is the context key for AuthContext.
type authContextKey struct{}

// AuthContextKey is the key used to store AuthContext in request context.
var AuthContextKey = authContextKey{}

// =============================================================================
// Presigned URL Types
// =============================================================================

// PresignedRequest represents a presigned URL request.
type PresignedRequest struct {
	// Method is the HTTP method (GET, PUT, etc.).
	Method string

	// Bucket is the bucket name.
	Bucket string

	// Key is the object key.
	Key string

	// Expires is the URL expiration time.
	Expires time.Duration

	// Headers is additional headers to include.
	Headers map[string]string

	// QueryParams is additional query parameters.
	QueryParams map[string]string
}

// PresignedURL represents a generated presigned URL.
type PresignedURL struct {
	// URL is the presigned URL.
	URL string

	// Method is the HTTP method for the URL.
	Method string

	// Expiration is when the URL expires.
	Expiration time.Time

	// Headers are headers that must be included in the request.
	Headers map[string]string
}

// =============================================================================
// Signature Components
// =============================================================================

// CanonicalRequest represents the components of a canonical request.
// Used for debugging and testing signature calculation.
type CanonicalRequest struct {
	// Method is the HTTP method.
	Method string

	// URI is the canonical URI path.
	URI string

	// QueryString is the canonical query string.
	QueryString string

	// Headers is the canonical headers string.
	Headers string

	// SignedHeaders is the signed headers list.
	SignedHeaders string

	// PayloadHash is the hash of the request payload.
	PayloadHash string
}

// String returns the canonical request as a string for signing.
func (cr CanonicalRequest) String() string {
	return cr.Method + "\n" +
		cr.URI + "\n" +
		cr.QueryString + "\n" +
		cr.Headers + "\n" +
		cr.SignedHeaders + "\n" +
		cr.PayloadHash
}

// StringToSign represents the string to sign.
type StringToSign struct {
	// Algorithm is the signing algorithm.
	Algorithm string

	// RequestDateTime is the request timestamp.
	RequestDateTime string

	// CredentialScope is the credential scope string.
	CredentialScope string

	// CanonicalRequestHash is the hash of the canonical request.
	CanonicalRequestHash string
}

// String returns the string to sign.
func (sts StringToSign) String() string {
	return sts.Algorithm + "\n" +
		sts.RequestDateTime + "\n" +
		sts.CredentialScope + "\n" +
		sts.CanonicalRequestHash
}
