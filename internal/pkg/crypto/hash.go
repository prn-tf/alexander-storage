// Package crypto provides cryptographic utilities for Alexander Storage.
package crypto

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
)

// HashReader wraps an io.Reader and computes hashes while reading.
// This allows computing SHA-256 and MD5 hashes in a single pass.
type HashReader struct {
	reader   io.Reader
	sha256   hash.Hash
	md5      hash.Hash
	size     int64
	finished bool
}

// NewHashReader creates a new HashReader that computes SHA-256 and MD5.
func NewHashReader(r io.Reader) *HashReader {
	return &HashReader{
		reader: r,
		sha256: sha256.New(),
		md5:    md5.New(),
	}
}

// Read implements io.Reader and updates hash computations.
func (h *HashReader) Read(p []byte) (n int, err error) {
	n, err = h.reader.Read(p)
	if n > 0 {
		h.sha256.Write(p[:n])
		h.md5.Write(p[:n])
		h.size += int64(n)
	}
	if err == io.EOF {
		h.finished = true
	}
	return n, err
}

// SHA256 returns the hex-encoded SHA-256 hash.
// Should only be called after reading is complete.
func (h *HashReader) SHA256() string {
	return hex.EncodeToString(h.sha256.Sum(nil))
}

// MD5 returns the hex-encoded MD5 hash.
// Should only be called after reading is complete.
func (h *HashReader) MD5() string {
	return hex.EncodeToString(h.md5.Sum(nil))
}

// ETag returns the ETag in S3 format (quoted MD5).
// Should only be called after reading is complete.
func (h *HashReader) ETag() string {
	return fmt.Sprintf("\"%s\"", h.MD5())
}

// Size returns the total number of bytes read.
func (h *HashReader) Size() int64 {
	return h.size
}

// IsFinished returns true if EOF was reached.
func (h *HashReader) IsFinished() bool {
	return h.finished
}

// ComputeSHA256 computes the SHA-256 hash of a byte slice.
func ComputeSHA256(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// ComputeMD5 computes the MD5 hash of a byte slice.
func ComputeMD5(data []byte) string {
	hash := md5.Sum(data)
	return hex.EncodeToString(hash[:])
}

// ComputeStreamSHA256 computes the SHA-256 hash of a reader's content.
func ComputeStreamSHA256(r io.Reader) (string, int64, error) {
	h := sha256.New()
	size, err := io.Copy(h, r)
	if err != nil {
		return "", 0, fmt.Errorf("failed to compute SHA-256: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), size, nil
}

// ComputeMultipartETag computes the ETag for a multipart upload.
// Format: "{md5_of_concatenated_part_md5s}-{part_count}"
func ComputeMultipartETag(partMD5s [][]byte) string {
	h := md5.New()
	for _, partMD5 := range partMD5s {
		h.Write(partMD5)
	}
	return fmt.Sprintf("\"%s-%d\"", hex.EncodeToString(h.Sum(nil)), len(partMD5s))
}

// ValidateSHA256 validates that a string is a valid SHA-256 hex hash.
func ValidateSHA256(hash string) bool {
	if len(hash) != 64 {
		return false
	}
	for _, c := range hash {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}
