// Package auth provides AWS Signature Version 4 authentication for Alexander Storage.
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// =============================================================================
// Signing Key Generation
// =============================================================================

// GetSigningKey derives the signing key for AWS v4 signatures.
// This implements the key derivation: HMAC(HMAC(HMAC(HMAC("AWS4"+secret, date), region), service), "aws4_request")
func GetSigningKey(secretKey string, date time.Time, region, service string) []byte {
	// Step 1: kDate = HMAC("AWS4" + secretKey, date)
	kDate := hmacSHA256([]byte("AWS4"+secretKey), []byte(date.Format(YYYYMMDD)))

	// Step 2: kRegion = HMAC(kDate, region)
	kRegion := hmacSHA256(kDate, []byte(region))

	// Step 3: kService = HMAC(kRegion, service)
	kService := hmacSHA256(kRegion, []byte(service))

	// Step 4: kSigning = HMAC(kService, "aws4_request")
	kSigning := hmacSHA256(kService, []byte(AWS4Request))

	return kSigning
}

// GetSignature calculates the signature using the signing key.
func GetSignature(signingKey []byte, stringToSign string) string {
	return hex.EncodeToString(hmacSHA256(signingKey, []byte(stringToSign)))
}

// hmacSHA256 computes HMAC-SHA256.
func hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

// =============================================================================
// Canonical Request Building
// =============================================================================

// GetCanonicalRequest builds the canonical request string for signing.
func GetCanonicalRequest(r *http.Request, signedHeaders []string, payloadHash string) string {
	return buildCanonicalRequest(
		r.Method,
		getCanonicalURI(r.URL.Path),
		getCanonicalQueryString(r.URL.Query()),
		getCanonicalHeaders(r.Header, signedHeaders),
		strings.Join(signedHeaders, ";"),
		payloadHash,
	)
}

// buildCanonicalRequest assembles the canonical request components.
func buildCanonicalRequest(method, uri, queryString, headers, signedHeaders, payloadHash string) string {
	return method + "\n" +
		uri + "\n" +
		queryString + "\n" +
		headers + "\n" +
		signedHeaders + "\n" +
		payloadHash
}

// getCanonicalURI returns the URI-encoded path.
// S3 requires the path to be URI-encoded with "/" not encoded.
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
	// Remove X-Amz-Signature from query (if present in presigned URL verification)
	delete(query, XAmzSignatureHeader)

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

// getCanonicalHeaders builds the canonical headers string.
func getCanonicalHeaders(headers http.Header, signedHeaders []string) string {
	var canonical strings.Builder

	for _, header := range signedHeaders {
		// Get header value (headers are case-insensitive)
		value := headers.Get(header)

		// Trim and collapse whitespace
		value = strings.TrimSpace(value)
		value = strings.Join(strings.Fields(value), " ")

		canonical.WriteString(strings.ToLower(header))
		canonical.WriteString(":")
		canonical.WriteString(value)
		canonical.WriteString("\n")
	}

	return canonical.String()
}

// =============================================================================
// String to Sign Building
// =============================================================================

// GetStringToSign builds the string to sign.
func GetStringToSign(canonicalRequest string, requestTime time.Time, scope CredentialScope) string {
	// Hash the canonical request
	hash := sha256.Sum256([]byte(canonicalRequest))
	canonicalRequestHash := hex.EncodeToString(hash[:])

	return SignV4Algorithm + "\n" +
		requestTime.Format(ISO8601BasicFormat) + "\n" +
		scope.String() + "\n" +
		canonicalRequestHash
}

// =============================================================================
// Signature Verification
// =============================================================================

// VerifySignature verifies an AWS v4 signature.
// Returns nil if the signature is valid, or an error describing the mismatch.
func VerifySignature(
	r *http.Request,
	secretKey string,
	signedValues SignedValues,
	payloadHash string,
) error {
	// Build canonical request
	canonicalRequest := GetCanonicalRequest(r, signedValues.SignedHeaders, payloadHash)

	// Build string to sign
	requestTime := signedValues.Credential.Scope.Date
	// Try to get more precise time from X-Amz-Date header
	if dateStr := r.Header.Get(XAmzDateHeader); dateStr != "" {
		if t, err := time.Parse(ISO8601BasicFormat, dateStr); err == nil {
			requestTime = t
		}
	}

	stringToSign := GetStringToSign(canonicalRequest, requestTime, signedValues.Credential.Scope)

	// Get signing key
	signingKey := GetSigningKey(
		secretKey,
		signedValues.Credential.Scope.Date,
		signedValues.Credential.Scope.Region,
		signedValues.Credential.Scope.Service,
	)

	// Calculate expected signature
	expectedSignature := GetSignature(signingKey, stringToSign)

	// Compare signatures (constant-time comparison)
	if !hmac.Equal([]byte(expectedSignature), []byte(signedValues.Signature)) {
		return ErrSignatureDoesNotMatch
	}

	return nil
}

// =============================================================================
// Content Hash Extraction
// =============================================================================

// GetPayloadHash extracts or computes the payload hash from a request.
func GetPayloadHash(r *http.Request) string {
	// Check X-Amz-Content-Sha256 header
	if hash := r.Header.Get(XAmzContentSHA256Header); hash != "" {
		return hash
	}

	// For GET, HEAD, DELETE requests, use empty string hash
	if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodDelete {
		return EmptyStringSHA256
	}

	// Default to unsigned payload
	return UnsignedPayload
}

// =============================================================================
// Time Validation
// =============================================================================

// ValidateRequestTime checks if the request time is within acceptable skew.
func ValidateRequestTime(requestTime time.Time) error {
	now := time.Now().UTC()
	skew := now.Sub(requestTime)

	if skew < 0 {
		skew = -skew
	}

	if skew > MaxSkewTime {
		return ErrRequestTimeTooSkewed
	}

	return nil
}
