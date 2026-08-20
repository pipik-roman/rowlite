package core

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
)

// PreconditionFailedError indicates that an optimistic concurrency check failed.
type PreconditionFailedError struct {
	ExpectedETag string
	CurrentETag  string
}

func (e *PreconditionFailedError) Error() string {
	return fmt.Sprintf("precondition failed: table content has changed (expected ETag %q, current ETag %q)", e.ExpectedETag, e.CurrentETag)
}

// ComputeETag computes a 16-character hex checksum over the provided canonical byte payload using SHA-256.
func ComputeETag(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:8])
}

// ComputeFileETag reads the target file and computes its 16-character content ETag.
func ComputeFileETag(filePath string) (string, error) {
	bytes, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	return ComputeETag(bytes), nil
}

// VerifyETag validates that expectedETag matches currentETag (case-insensitive).
// If expectedETag is empty, no check is performed.
func VerifyETag(expectedETag, currentETag string) error {
	expectedETag = strings.TrimSpace(expectedETag)
	if expectedETag == "" {
		return nil
	}
	currentETag = strings.TrimSpace(currentETag)
	if !strings.EqualFold(expectedETag, currentETag) {
		return &PreconditionFailedError{
			ExpectedETag: expectedETag,
			CurrentETag:  currentETag,
		}
	}
	return nil
}
