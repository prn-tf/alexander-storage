// Package auth provides AWS Signature Version 4 authentication for Alexander Storage.
package auth

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

// AccessKeyStore defines the interface for retrieving access keys.
type AccessKeyStore interface {
	// GetActiveAccessKey retrieves an active access key by its ID.
	// Returns the access key with the decrypted secret.
	GetActiveAccessKey(ctx context.Context, accessKeyID string) (*AccessKeyInfo, error)

	// UpdateLastUsed updates the last used timestamp for an access key.
	UpdateLastUsed(ctx context.Context, accessKeyID string) error
}

// BucketACLChecker defines the interface for checking bucket ACL permissions.
type BucketACLChecker interface {
	// GetBucketACL returns the ACL for a bucket by name.
	// Returns empty string if bucket not found.
	GetBucketACL(ctx context.Context, bucketName string) (string, error)
}

// AccessKeyInfo contains the information needed for signature verification.
type AccessKeyInfo struct {
	// AccessKeyID is the public identifier.
	AccessKeyID string

	// SecretKey is the decrypted secret key (plaintext).
	SecretKey string

	// UserID is the ID of the user who owns this key.
	UserID int64

	// Username is the username of the user who owns this key.
	Username string

	// IsActive indicates if the key is active.
	IsActive bool

	// ExpiresAt is the optional expiration time.
	ExpiresAt *time.Time
}

// Config contains configuration for the auth middleware.
type Config struct {
	// Region is the expected AWS region.
	Region string

	// Service is the expected AWS service (usually "s3").
	Service string

	// AllowAnonymous allows unauthenticated requests (for public buckets).
	AllowAnonymous bool

	// SkipPaths are paths that skip authentication.
	SkipPaths []string

	// BucketACLChecker checks bucket ACL for anonymous access (optional).
	BucketACLChecker BucketACLChecker
}

// DefaultConfig returns the default auth configuration.
func DefaultConfig() Config {
	return Config{
		Region:           DefaultRegion,
		Service:          ServiceS3,
		AllowAnonymous:   false,
		SkipPaths:        []string{"/health", "/metrics"},
		BucketACLChecker: nil,
	}
}

// extractBucketName extracts the bucket name from the URL path.
// S3-style path: /bucket-name/key or /bucket-name
func extractBucketName(path string) string {
	path = strings.TrimPrefix(path, "/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) > 0 && parts[0] != "" {
		return parts[0]
	}
	return ""
}

// isReadOperation checks if the HTTP method is a read operation.
func isReadOperation(method string) bool {
	return method == http.MethodGet || method == http.MethodHead
}

// Middleware creates an authentication middleware.
func Middleware(store AccessKeyStore, config Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check if path should skip authentication
			for _, path := range config.SkipPaths {
				if r.URL.Path == path {
					next.ServeHTTP(w, r)
					return
				}
			}

			// Determine auth type
			authType := GetAuthType(r)

			switch authType {
			case AuthTypeAnonymous:
				// Check if anonymous access is allowed
				if config.AllowAnonymous {
					next.ServeHTTP(w, r)
					return
				}

				// Check bucket ACL for anonymous access
				if config.BucketACLChecker != nil {
					bucketName := extractBucketName(r.URL.Path)
					if bucketName != "" {
						acl, err := config.BucketACLChecker.GetBucketACL(r.Context(), bucketName)
						if err == nil && acl != "" {
							// Check if ACL allows anonymous access for this operation
							if isReadOperation(r.Method) && (acl == "public-read" || acl == "public-read-write") {
								// Allow read operations on public-read buckets
								next.ServeHTTP(w, r)
								return
							}
							if acl == "public-read-write" {
								// Allow all operations on public-read-write buckets
								next.ServeHTTP(w, r)
								return
							}
						}
					}
				}

				writeAuthError(w, ErrAccessDenied)
				return

			case AuthTypeSignedV4:
				authCtx, err := handleSignedV4(r, store, config)
				if err != nil {
					log.Debug().Err(err).Str("path", r.URL.Path).Msg("SignedV4 authentication failed")
					writeAuthError(w, err)
					return
				}
				r = r.WithContext(context.WithValue(r.Context(), AuthContextKey, authCtx))

			case AuthTypePresignedV4:
				authCtx, err := handlePresignedV4(r, store, config)
				if err != nil {
					log.Debug().Err(err).Str("path", r.URL.Path).Msg("PresignedV4 authentication failed")
					writeAuthError(w, err)
					return
				}
				r = r.WithContext(context.WithValue(r.Context(), AuthContextKey, authCtx))

			default:
				writeAuthError(w, ErrInvalidAuthorizationHeader)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// handleSignedV4 handles AWS Signature V4 authentication.
func handleSignedV4(r *http.Request, store AccessKeyStore, config Config) (*AuthContext, error) {
	// Parse authorization header
	authHeader := r.Header.Get(AuthorizationHeader)
	signedValues, err := ParseSignV4(authHeader)
	if err != nil {
		return nil, err
	}

	// Validate request time
	requestTime, err := GetRequestTime(r)
	if err != nil {
		return nil, ErrMissingSecurityHeader
	}

	if err := ValidateRequestTime(requestTime); err != nil {
		return nil, err
	}

	// Lookup access key
	keyInfo, err := store.GetActiveAccessKey(r.Context(), signedValues.Credential.AccessKey)
	if err != nil {
		return nil, ErrInvalidAccessKeyID
	}

	// Check if key is expired
	if keyInfo.ExpiresAt != nil && time.Now().UTC().After(*keyInfo.ExpiresAt) {
		return nil, ErrPresignedURLExpired
	}

	// Get payload hash
	payloadHash := GetPayloadHash(r)

	// Verify signature
	if err := VerifySignature(r, keyInfo.SecretKey, *signedValues, payloadHash); err != nil {
		return nil, err
	}

	// Update last used timestamp (async, don't block request)
	go func() {
		_ = store.UpdateLastUsed(context.Background(), keyInfo.AccessKeyID)
	}()

	return &AuthContext{
		UserID:      keyInfo.UserID,
		Username:    keyInfo.Username,
		AccessKeyID: keyInfo.AccessKeyID,
		Credential:  signedValues.Credential,
		AuthType:    AuthTypeSignedV4,
		RequestTime: requestTime,
		Region:      signedValues.Credential.Scope.Region,
	}, nil
}

// handlePresignedV4 handles presigned URL authentication.
func handlePresignedV4(r *http.Request, store AccessKeyStore, config Config) (*AuthContext, error) {
	// Parse presigned URL parameters
	signedValues, expires, err := ParsePresignedV4(r)
	if err != nil {
		return nil, err
	}

	// Get request time
	requestTime, err := GetRequestTime(r)
	if err != nil {
		return nil, ErrMissingSecurityHeader
	}

	// Check if URL has expired
	expirationTime := requestTime.Add(time.Duration(expires) * time.Second)
	if time.Now().UTC().After(expirationTime) {
		return nil, ErrPresignedURLExpired
	}

	// Lookup access key
	keyInfo, err := store.GetActiveAccessKey(r.Context(), signedValues.Credential.AccessKey)
	if err != nil {
		return nil, ErrInvalidAccessKeyID
	}

	// For presigned URLs, we need to reconstruct the canonical request
	// The signed headers are in query params, and we verify against those
	payloadHash := GetPayloadHash(r)

	// Build canonical request for presigned URL
	// Note: For presigned URLs, the query string includes auth params which need special handling
	if err := VerifySignature(r, keyInfo.SecretKey, *signedValues, payloadHash); err != nil {
		return nil, err
	}

	return &AuthContext{
		UserID:      keyInfo.UserID,
		Username:    keyInfo.Username,
		AccessKeyID: keyInfo.AccessKeyID,
		Credential:  signedValues.Credential,
		AuthType:    AuthTypePresignedV4,
		RequestTime: requestTime,
		Region:      signedValues.Credential.Scope.Region,
	}, nil
}

// writeAuthError writes an S3-compatible error response.
func writeAuthError(w http.ResponseWriter, err error) {
	authErr := NewAuthError(err)

	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(authErr.HTTPStatus)

	// Write S3-style XML error response
	xmlResponse := `<?xml version="1.0" encoding="UTF-8"?>
<Error>
    <Code>` + string(authErr.Code) + `</Code>
    <Message>` + authErr.Message + `</Message>
</Error>`

	_, _ = w.Write([]byte(xmlResponse))
}

// GetAuthContext retrieves the AuthContext from a request context.
func GetAuthContext(ctx context.Context) *AuthContext {
	if authCtx, ok := ctx.Value(AuthContextKey).(*AuthContext); ok {
		return authCtx
	}
	return nil
}

// GetUserContext retrieves user information from the auth context.
// This is a convenience wrapper that returns UserID and Username.
func GetUserContext(ctx context.Context) (*AuthContext, bool) {
	authCtx := GetAuthContext(ctx)
	if authCtx == nil {
		return nil, false
	}
	return authCtx, true
}

// RequireAuth is a helper to get auth context or return error.
func RequireAuth(ctx context.Context) (*AuthContext, error) {
	authCtx := GetAuthContext(ctx)
	if authCtx == nil {
		return nil, ErrAccessDenied
	}
	return authCtx, nil
}
