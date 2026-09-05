package player

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrInvalidAPITokenName       = errors.New("api token name is invalid")
	ErrInvalidAPITokenExpiration = errors.New("api token expiration must be in the future")
)

const (
	APITokenPrefix     = "p2_sk_"
	MaxAPITokenNameLen = 64
)

// APIToken represents a persistent, hashed Personal Access Token (API Key).
type APIToken struct {
	ID         string     `json:"id"`
	PlayerID   string     `json:"player_id"`
	TokenHash  string     `json:"token_hash"`
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
}

// Active returns true if the token has not expired.
func (t APIToken) Active(now time.Time) bool {
	if t.ID == "" || t.PlayerID == "" || t.TokenHash == "" {
		return false
	}
	if t.ExpiresAt != nil && !now.Before(*t.ExpiresAt) {
		return false
	}
	return true
}

// NewAPIToken generates a new APIToken and its plaintext credential string.
// The plaintext token begins with "p2_sk_" followed by 32 cryptographically secure random bytes (64 hex characters).
// Only the SHA-256 hex digest is stored on the APIToken struct.
func NewAPIToken(playerID string, name string, expiresAt *time.Time, now time.Time) (APIToken, string, error) {
	playerID = strings.TrimSpace(playerID)
	if playerID == "" {
		return APIToken{}, "", ErrInvalidPlayer
	}

	name = strings.TrimSpace(name)
	if name == "" || len(name) > MaxAPITokenNameLen {
		return APIToken{}, "", ErrInvalidAPITokenName
	}

	now = now.UTC()
	if expiresAt != nil {
		expUTC := expiresAt.UTC()
		if !expUTC.After(now) {
			return APIToken{}, "", ErrInvalidAPITokenExpiration
		}
		expiresAt = &expUTC
	}

	tokenID, err := randomID()
	if err != nil {
		return APIToken{}, "", fmt.Errorf("generate api token id: %w", err)
	}

	// Generate 32 bytes of cryptographic randomness for the key secret
	secretBytes := make([]byte, 32)
	if _, err := rand.Read(secretBytes); err != nil {
		return APIToken{}, "", fmt.Errorf("generate api token secret: %w", err)
	}

	plaintext := APITokenPrefix + hex.EncodeToString(secretBytes)
	tokenHash := HashAPIToken(plaintext)

	token := APIToken{
		ID:        tokenID,
		PlayerID:  playerID,
		TokenHash: tokenHash,
		Name:      name,
		CreatedAt: now,
		ExpiresAt: expiresAt,
	}

	return token, plaintext, nil
}

// HashAPIToken computes the SHA-256 hex digest of the given plaintext API token.
func HashAPIToken(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}
