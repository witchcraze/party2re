package player_test

import (
	"strings"
	"testing"
	"time"

	coreplayer "github.com/witchcraze/party2re/internal/core/player"
)

func TestNewAPIToken(t *testing.T) {
	now := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)

	t.Run("successful generation without expiration", func(t *testing.T) {
		token, plaintext, err := coreplayer.NewAPIToken("player1", "Agent Key", nil, now)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.HasPrefix(plaintext, "p2_sk_") {
			t.Errorf("expected plaintext to start with 'p2_sk_', got %q", plaintext)
		}
		if len(plaintext) != len("p2_sk_")+64 {
			t.Errorf("expected plaintext length %d, got %d", len("p2_sk_")+64, len(plaintext))
		}

		if token.PlayerID != "player1" {
			t.Errorf("expected playerID 'player1', got %q", token.PlayerID)
		}
		if token.Name != "Agent Key" {
			t.Errorf("expected name 'Agent Key', got %q", token.Name)
		}
		if token.ID == "" {
			t.Error("expected non-empty token ID")
		}
		if len(token.TokenHash) != 64 {
			t.Errorf("expected token hash length 64, got %d", len(token.TokenHash))
		}

		expectedHash := coreplayer.HashAPIToken(plaintext)
		if token.TokenHash != expectedHash {
			t.Errorf("expected token hash %q, got %q", expectedHash, token.TokenHash)
		}

		if !token.Active(now) {
			t.Error("expected token to be active")
		}
		if !token.Active(now.Add(365 * 24 * time.Hour)) {
			t.Error("expected token without expiration to remain active indefinitely")
		}
	})

	t.Run("successful generation with future expiration", func(t *testing.T) {
		expiresAt := now.Add(24 * time.Hour)
		token, _, err := coreplayer.NewAPIToken("player1", "Temporary Key", &expiresAt, now)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if token.ExpiresAt == nil || !token.ExpiresAt.Equal(expiresAt) {
			t.Errorf("expected expiresAt %v, got %v", expiresAt, token.ExpiresAt)
		}

		if !token.Active(now.Add(12 * time.Hour)) {
			t.Error("expected token to be active before expiration")
		}
		if token.Active(now.Add(25 * time.Hour)) {
			t.Error("expected token to be inactive after expiration")
		}
	})

	t.Run("validation errors", func(t *testing.T) {
		past := now.Add(-1 * time.Hour)

		cases := []struct {
			name      string
			playerID  string
			tokenName string
			expiresAt *time.Time
			wantErr   error
		}{
			{"empty playerID", "", "Key", nil, coreplayer.ErrInvalidPlayer},
			{"empty name", "player1", "", nil, coreplayer.ErrInvalidAPITokenName},
			{"whitespace name", "player1", "   ", nil, coreplayer.ErrInvalidAPITokenName},
			{"past expiration", "player1", "Key", &past, coreplayer.ErrInvalidAPITokenExpiration},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				_, _, err := coreplayer.NewAPIToken(tc.playerID, tc.tokenName, tc.expiresAt, now)
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tc.wantErr != nil && err != tc.wantErr {
					t.Errorf("expected error %v, got %v", tc.wantErr, err)
				}
			})
		}
	})
}
