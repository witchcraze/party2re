package id

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// DefaultByteLength is the default number of cryptographically secure random bytes (16 bytes = 32 hex characters).
const DefaultByteLength = 16

// New generates a cryptographically secure 16-byte random hex string (32 characters).
// It panics if the system CSPRNG fails, which indicates a fatal runtime environment failure.
func New() string {
	s, err := NewLength(DefaultByteLength)
	if err != nil {
		panic(fmt.Sprintf("id: crypto/rand read failed: %v", err))
	}
	return s
}

// Generate generates a cryptographically secure 16-byte random hex string and returns an error if CSPRNG fails.
func Generate() (string, error) {
	return NewLength(DefaultByteLength)
}

// NewLength generates a cryptographically secure random hex string of n bytes (2*n characters).
func NewLength(byteLength int) (string, error) {
	if byteLength <= 0 {
		return "", fmt.Errorf("id: byte length must be positive, got %d", byteLength)
	}
	b := make([]byte, byteLength)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("id: crypto/rand read: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// Sort2 returns two identifiers in deterministic ascending alphanumeric order.
// This is used for two-party transactions to maintain a consistent row-locking order
// and prevent database deadlocks.
func Sort2(a, b string) (first, second string) {
	if a <= b {
		return a, b
	}
	return b, a
}

// Sort returns a slice of identifiers sorted in deterministic ascending alphanumeric order.
func Sort(ids ...string) []string {
	res := make([]string, len(ids))
	copy(res, ids)
	for i := 0; i < len(res)-1; i++ {
		for j := i + 1; j < len(res); j++ {
			if res[i] > res[j] {
				res[i], res[j] = res[j], res[i]
			}
		}
	}
	return res
}
