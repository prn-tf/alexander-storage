// Package service provides business logic services for Alexander Storage.
package service

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"github.com/prn-tf/alexander-storage/internal/auth"
)

// PresignService handles presigned URL generation.
type PresignService struct {
	iamService    *IAMService
	region        string
	service       string
	defaultExpiry time.Duration
	endpoint      string // Base endpoint URL (e.g., "http://localhost:9000")
	logger        zerolog.Logger
}

// PresignConfig contains configuration for the presign service.
type PresignConfig struct {
	Region        string
	Service       string
	DefaultExpiry time.Duration
	Endpoint      string
}

// DefaultPresignConfig returns default presign configuration.
func DefaultPresignConfig() PresignConfig {
	return PresignConfig{
		Region:        auth.DefaultRegion,
		Service:       auth.ServiceS3,
		DefaultExpiry: 15 * time.Minute,
		Endpoint:      "http://localhost:9000",
	}
}

// NewPresignService creates a new PresignService.
func NewPresignService(iamService *IAMService, config PresignConfig, logger zerolog.Logger) *PresignService {
	return &PresignService{
		iamService:    iamService,
		region:        config.Region,
		service:       config.Service,
		defaultExpiry: config.DefaultExpiry,
		endpoint:      strings.TrimSuffix(config.Endpoint, "/"),
		logger:        logger.With().Str("service", "presign").Logger(),
	}
}

// PresignInput contains the data needed to generate a presigned URL.
type PresignInput struct {
	// AccessKeyID is the access key to sign with.
	AccessKeyID string

	// Method is the HTTP method (GET, PUT, DELETE, HEAD).
	Method string

	// Bucket is the bucket name.
	Bucket string

	// Key is the object key.
	Key string

	// Expiry is the URL expiration duration.
	// If zero, default expiry is used.
	Expiry time.Duration

	// ContentType is the expected content type (for PUT).
	ContentType string

	// ContentMD5 is the expected MD5 hash (for PUT).
	ContentMD5 string

	// Headers are additional headers to include in the signature.
	Headers map[string]string

	// QueryParams are additional query parameters.
	QueryParams map[string]string
}

// PresignOutput contains the result of generating a presigned URL.
type PresignOutput struct {
	// URL is the presigned URL.
	URL string

	// Method is the HTTP method for the request.
	Method string

	// Expiration is when the URL expires.
	Expiration time.Time

	// SignedHeaders are headers that must be included in the request.
	SignedHeaders map[string]string
}

// GeneratePresignedURL generates a presigned URL for S3 operations.
func (s *PresignService) GeneratePresignedURL(ctx context.Context, input PresignInput) (*PresignOutput, error) {
	// Validate input
	if err := s.validateInput(input); err != nil {
		return nil, err
	}

	// Get expiry duration
	expiry := input.Expiry
	if expiry == 0 {
		expiry = s.defaultExpiry
	}

	// Validate expiry range
	if expiry < auth.PresignedURLMinExpiry || expiry > auth.PresignedURLMaxExpiry {
		return nil, ErrInvalidExpiration
	}

	// Verify access key and get secret
	keyInfo, err := s.iamService.VerifyAccessKey(ctx, input.AccessKeyID)
	if err != nil {
		return nil, err
	}

	// Calculate times
	now := time.Now().UTC()
	expiresAt := now.Add(expiry)

	// Build the URL
	presignedURL, signedHeaders, err := s.buildPresignedURL(input, keyInfo, now, int64(expiry.Seconds()))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInternalError, err)
	}

	s.logger.Debug().
		Str("access_key_id", input.AccessKeyID).
		Str("method", input.Method).
		Str("bucket", input.Bucket).
		Str("key", input.Key).
		Time("expires_at", expiresAt).
		Msg("generated presigned URL")

	return &PresignOutput{
		URL:           presignedURL,
		Method:        input.Method,
		Expiration:    expiresAt,
		SignedHeaders: signedHeaders,
	}, nil
}

// GenerateGetObjectURL is a convenience method for generating a GET presigned URL.
func (s *PresignService) GenerateGetObjectURL(ctx context.Context, accessKeyID, bucket, key string, expiry time.Duration) (*PresignOutput, error) {
	return s.GeneratePresignedURL(ctx, PresignInput{
		AccessKeyID: accessKeyID,
		Method:      http.MethodGet,
		Bucket:      bucket,
		Key:         key,
		Expiry:      expiry,
	})
}

// GeneratePutObjectURL is a convenience method for generating a PUT presigned URL.
func (s *PresignService) GeneratePutObjectURL(ctx context.Context, accessKeyID, bucket, key, contentType string, expiry time.Duration) (*PresignOutput, error) {
	headers := make(map[string]string)
	if contentType != "" {
		headers["Content-Type"] = contentType
	}

	return s.GeneratePresignedURL(ctx, PresignInput{
		AccessKeyID: accessKeyID,
		Method:      http.MethodPut,
		Bucket:      bucket,
		Key:         key,
		Expiry:      expiry,
		ContentType: contentType,
		Headers:     headers,
	})
}

// GenerateDeleteObjectURL is a convenience method for generating a DELETE presigned URL.
func (s *PresignService) GenerateDeleteObjectURL(ctx context.Context, accessKeyID, bucket, key string, expiry time.Duration) (*PresignOutput, error) {
	return s.GeneratePresignedURL(ctx, PresignInput{
		AccessKeyID: accessKeyID,
		Method:      http.MethodDelete,
		Bucket:      bucket,
		Key:         key,
		Expiry:      expiry,
	})
}

// validateInput validates the presign input.
func (s *PresignService) validateInput(input PresignInput) error {
	if input.AccessKeyID == "" {
		return fmt.Errorf("%w: access_key_id is required", ErrMissingRequiredParams)
	}

	if input.Method == "" {
		return fmt.Errorf("%w: method is required", ErrMissingRequiredParams)
	}

	if input.Bucket == "" {
		return fmt.Errorf("%w: bucket is required", ErrMissingRequiredParams)
	}

	// Validate method
	switch input.Method {
	case http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodHead:
		// Valid methods
	default:
		return fmt.Errorf("%w: invalid method '%s'", ErrInvalidPresignedURL, input.Method)
	}

	return nil
}

// buildPresignedURL builds the presigned URL with signature.
func (s *PresignService) buildPresignedURL(
	input PresignInput,
	keyInfo *auth.AccessKeyInfo,
	requestTime time.Time,
	expirySeconds int64,
) (string, map[string]string, error) {
	// Build the path
	path := "/" + input.Bucket
	if input.Key != "" {
		path += "/" + input.Key
	}

	// Build the host
	parsedEndpoint, err := url.Parse(s.endpoint)
	if err != nil {
		return "", nil, err
	}
	host := parsedEndpoint.Host

	// Build credential scope
	scope := auth.CredentialScope{
		Date:    requestTime,
		Region:  s.region,
		Service: s.service,
	}

	credential := keyInfo.AccessKeyID + "/" + scope.String()

	// Determine signed headers
	signedHeadersList := []string{"host"}
	headers := map[string]string{
		"host": host,
	}

	// Add custom headers
	for k, v := range input.Headers {
		lk := strings.ToLower(k)
		if lk != "host" {
			signedHeadersList = append(signedHeadersList, lk)
			headers[lk] = v
		}
	}

	// Sort signed headers
	sort.Strings(signedHeadersList)
	signedHeaders := strings.Join(signedHeadersList, ";")

	// Build query parameters for presigned URL
	queryParams := url.Values{}
	queryParams.Set(auth.XAmzAlgorithmHeader, auth.SignV4Algorithm)
	queryParams.Set(auth.XAmzCredentialHeader, credential)
	queryParams.Set(auth.XAmzDateHeader, requestTime.Format(auth.ISO8601BasicFormat))
	queryParams.Set(auth.XAmzExpiresHeader, fmt.Sprintf("%d", expirySeconds))
	queryParams.Set(auth.XAmzSignedHeadersHeader, signedHeaders)

	// Add custom query parameters
	for k, v := range input.QueryParams {
		queryParams.Set(k, v)
	}

	// Build canonical request
	canonicalURI := getCanonicalURI(path)
	canonicalQueryString := getCanonicalQueryString(queryParams)
	canonicalHeaders := buildCanonicalHeaders(headers, signedHeadersList)

	canonicalRequest := input.Method + "\n" +
		canonicalURI + "\n" +
		canonicalQueryString + "\n" +
		canonicalHeaders + "\n" +
		signedHeaders + "\n" +
		auth.UnsignedPayload // Presigned URLs use UNSIGNED-PAYLOAD

	// Build string to sign
	stringToSign := auth.GetStringToSign(canonicalRequest, requestTime, scope)

	// Get signing key
	signingKey := auth.GetSigningKey(keyInfo.SecretKey, requestTime, s.region, s.service)

	// Calculate signature
	signature := auth.GetSignature(signingKey, stringToSign)

	// Add signature to query params
	queryParams.Set(auth.XAmzSignatureHeader, signature)

	// Build final URL
	finalURL := s.endpoint + canonicalURI + "?" + queryParams.Encode()

	return finalURL, headers, nil
}

// getCanonicalURI returns the URI-encoded path.
func getCanonicalURI(path string) string {
	if path == "" {
		return "/"
	}

	// URI encode each path segment
	segments := strings.Split(path, "/")
	for i, segment := range segments {
		segments[i] = url.PathEscape(segment)
	}

	return strings.Join(segments, "/")
}

// getCanonicalQueryString returns the sorted, URI-encoded query string.
func getCanonicalQueryString(query url.Values) string {
	// Remove X-Amz-Signature from query (not included in canonical query)
	delete(query, auth.XAmzSignatureHeader)

	if len(query) == 0 {
		return ""
	}

	// Get sorted keys
	keys := make([]string, 0, len(query))
	for key := range query {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	// Build canonical query string
	var pairs []string
	for _, key := range keys {
		values := query[key]
		sort.Strings(values)
		for _, value := range values {
			pairs = append(pairs, url.QueryEscape(key)+"="+url.QueryEscape(value))
		}
	}

	return strings.Join(pairs, "&")
}

// buildCanonicalHeaders builds the canonical headers string.
func buildCanonicalHeaders(headers map[string]string, signedHeaders []string) string {
	var builder strings.Builder

	for _, header := range signedHeaders {
		value := headers[header]
		value = strings.TrimSpace(value)
		value = strings.Join(strings.Fields(value), " ")

		builder.WriteString(strings.ToLower(header))
		builder.WriteString(":")
		builder.WriteString(value)
		builder.WriteString("\n")
	}

	return builder.String()
}
