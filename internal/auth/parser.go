// Package auth provides AWS Signature Version 4 authentication for Alexander Storage.
package auth

import (
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"
)

// =============================================================================
// Authorization Header Parsing
// =============================================================================

// Regular expressions for parsing AWS v4 authorization header
var (
	// credentialRegex matches Credential=accessKey/date/region/service/aws4_request
	credentialRegex = regexp.MustCompile(`Credential=([^/]+)/(\d{8})/([^/]+)/([^/]+)/aws4_request`)

	// signedHeadersRegex matches SignedHeaders=header1;header2;header3
	signedHeadersRegex = regexp.MustCompile(`SignedHeaders=([^,\s]+)`)

	// signatureRegex matches Signature=hexstring
	signatureRegex = regexp.MustCompile(`Signature=([a-f0-9]{64})`)
)

// GetAuthType determines the authentication type from a request.
func GetAuthType(r *http.Request) AuthType {
	// Check Authorization header
	authHeader := r.Header.Get(AuthorizationHeader)

	if authHeader != "" {
		if strings.HasPrefix(authHeader, SignV4Algorithm) {
			return AuthTypeSignedV4
		}
		return AuthTypeUnknown
	}

	// Check for presigned URL
	query := r.URL.Query()
	if query.Get(XAmzAlgorithmHeader) == SignV4Algorithm {
		return AuthTypePresignedV4
	}

	return AuthTypeAnonymous
}

// ParseSignV4 parses an AWS v4 Authorization header.
// Format: AWS4-HMAC-SHA256 Credential=access_key/date/region/service/aws4_request, SignedHeaders=..., Signature=...
func ParseSignV4(authHeader string) (*SignedValues, error) {
	// Validate algorithm prefix
	if !strings.HasPrefix(authHeader, SignV4Algorithm) {
		return nil, ErrInvalidAuthorizationHeader
	}

	// Extract credential
	credentialMatch := credentialRegex.FindStringSubmatch(authHeader)
	if credentialMatch == nil || len(credentialMatch) < 5 {
		return nil, fmt.Errorf("%w: invalid credential format", ErrInvalidAuthorizationHeader)
	}

	// Parse date
	date, err := time.Parse(YYYYMMDD, credentialMatch[2])
	if err != nil {
		return nil, fmt.Errorf("%w: invalid date in credential", ErrInvalidAuthorizationHeader)
	}

	// Extract signed headers
	signedHeadersMatch := signedHeadersRegex.FindStringSubmatch(authHeader)
	if signedHeadersMatch == nil || len(signedHeadersMatch) < 2 {
		return nil, fmt.Errorf("%w: missing signed headers", ErrInvalidAuthorizationHeader)
	}
	signedHeaders := strings.Split(signedHeadersMatch[1], ";")

	// Validate signed headers are sorted
	sortedHeaders := make([]string, len(signedHeaders))
	copy(sortedHeaders, signedHeaders)
	sort.Strings(sortedHeaders)
	for i, h := range signedHeaders {
		if h != sortedHeaders[i] {
			return nil, fmt.Errorf("%w: signed headers not sorted", ErrInvalidAuthorizationHeader)
		}
	}

	// Extract signature
	signatureMatch := signatureRegex.FindStringSubmatch(authHeader)
	if signatureMatch == nil || len(signatureMatch) < 2 {
		return nil, fmt.Errorf("%w: missing or invalid signature", ErrInvalidAuthorizationHeader)
	}

	return &SignedValues{
		Credential: CredentialHeader{
			AccessKey: credentialMatch[1],
			Scope: CredentialScope{
				Date:    date,
				Region:  credentialMatch[3],
				Service: credentialMatch[4],
			},
		},
		SignedHeaders: signedHeaders,
		Signature:     signatureMatch[1],
	}, nil
}

// ParsePresignedV4 parses presigned URL query parameters.
func ParsePresignedV4(r *http.Request) (*SignedValues, int64, error) {
	query := r.URL.Query()

	// Validate algorithm
	algorithm := query.Get(XAmzAlgorithmHeader)
	if algorithm != SignV4Algorithm {
		return nil, 0, ErrInvalidPresignedURL
	}

	// Parse credential
	credential := query.Get(XAmzCredentialHeader)
	if credential == "" {
		return nil, 0, fmt.Errorf("%w: missing credential", ErrInvalidPresignedURL)
	}

	parts := strings.Split(credential, "/")
	if len(parts) != 5 || parts[4] != AWS4Request {
		return nil, 0, fmt.Errorf("%w: invalid credential format", ErrInvalidPresignedURL)
	}

	date, err := time.Parse(YYYYMMDD, parts[1])
	if err != nil {
		return nil, 0, fmt.Errorf("%w: invalid date in credential", ErrInvalidPresignedURL)
	}

	// Parse signed headers
	signedHeadersStr := query.Get(XAmzSignedHeadersHeader)
	var signedHeaders []string
	if signedHeadersStr != "" {
		signedHeaders = strings.Split(signedHeadersStr, ";")
	}

	// Parse signature
	signature := query.Get(XAmzSignatureHeader)
	if signature == "" || len(signature) != 64 {
		return nil, 0, fmt.Errorf("%w: missing or invalid signature", ErrInvalidPresignedURL)
	}

	// Parse expires
	expiresStr := query.Get(XAmzExpiresHeader)
	if expiresStr == "" {
		return nil, 0, fmt.Errorf("%w: missing expires", ErrInvalidPresignedURL)
	}
	var expires int64
	if _, err := fmt.Sscanf(expiresStr, "%d", &expires); err != nil {
		return nil, 0, fmt.Errorf("%w: invalid expires value", ErrInvalidPresignedURL)
	}

	return &SignedValues{
		Credential: CredentialHeader{
			AccessKey: parts[0],
			Scope: CredentialScope{
				Date:    date,
				Region:  parts[2],
				Service: parts[3],
			},
		},
		SignedHeaders: signedHeaders,
		Signature:     signature,
	}, expires, nil
}

// ExtractSignedHeaders extracts header values for the signed headers.
func ExtractSignedHeaders(r *http.Request, signedHeaders []string) (http.Header, error) {
	extracted := make(http.Header)

	for _, header := range signedHeaders {
		headerLower := strings.ToLower(header)

		// Special case for host header
		if headerLower == "host" {
			extracted.Set("host", r.Host)
			continue
		}

		// Get header value
		value := r.Header.Get(header)
		if value == "" {
			// Check if it's a required header
			if headerLower == "host" || headerLower == "x-amz-date" || headerLower == "x-amz-content-sha256" {
				return nil, fmt.Errorf("%w: missing required header %s", ErrMissingSecurityHeader, header)
			}
		}

		extracted.Set(header, value)
	}

	return extracted, nil
}

// GetRequestTime extracts the request time from headers or query parameters.
func GetRequestTime(r *http.Request) (time.Time, error) {
	// Try X-Amz-Date header first
	if dateStr := r.Header.Get(XAmzDateHeader); dateStr != "" {
		return time.Parse(ISO8601BasicFormat, dateStr)
	}

	// Try X-Amz-Date query parameter (for presigned URLs)
	if dateStr := r.URL.Query().Get(XAmzDateHeader); dateStr != "" {
		return time.Parse(ISO8601BasicFormat, dateStr)
	}

	// Try Date header
	if dateStr := r.Header.Get("Date"); dateStr != "" {
		return time.Parse(time.RFC1123, dateStr)
	}

	return time.Time{}, ErrMissingSecurityHeader
}
