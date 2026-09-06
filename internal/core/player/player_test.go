package player

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestPlayerNew(t *testing.T) {
	now := time.Now()
	p, err := New("  bob  ", "correct-horse-battery-staple", now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.ID == "" {
		t.Error("expected non-empty player ID")
	}
	if p.Username != "bob" {
		t.Errorf("expected trimmed username 'bob', got %q", p.Username)
	}
	if !strings.HasPrefix(p.PasswordHash, "$2a$") && !strings.HasPrefix(p.PasswordHash, "$2b$") {
		t.Errorf("expected bcrypt hash prefix, got %q", p.PasswordHash)
	}
	if p.CreatedAt.Location() != time.UTC {
		t.Errorf("expected UTC location for CreatedAt, got %v", p.CreatedAt.Location())
	}

	// Validation errors
	if _, err := New("", "pass", now); !errors.Is(err, ErrInvalidPlayer) {
		t.Errorf("expected ErrInvalidPlayer, got %v", err)
	}
	if _, err := New("   ", "pass", now); !errors.Is(err, ErrInvalidPlayer) {
		t.Errorf("expected ErrInvalidPlayer for whitespace username, got %v", err)
	}
	if _, err := New("alice", "", now); !errors.Is(err, ErrInvalidPassword) {
		t.Errorf("expected ErrInvalidPassword, got %v", err)
	}
}

func TestPlayerStoresOnlyPasswordHashAndAuthenticates(t *testing.T) {
	value, err := New(" alice ", "secret", time.Unix(100, 0))
	if err != nil {
		t.Fatal(err)
	}
	if value.Username != "alice" || value.PasswordHash == "secret" || !value.Authenticate("secret") || value.Authenticate("wrong") {
		t.Fatalf("player = %#v", value)
	}
	if !strings.HasPrefix(value.PasswordHash, "$2a$") && !strings.HasPrefix(value.PasswordHash, "$2b$") {
		t.Fatalf("expected bcrypt hash prefix, got %q", value.PasswordHash)
	}
}

func TestPlayerAuthenticate(t *testing.T) {
	p, err := New("alice", "correct-password", time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !p.Authenticate("correct-password") {
		t.Error("expected Authenticate to succeed with correct password")
	}
	if p.Authenticate("wrong-password") {
		t.Error("expected Authenticate to fail with wrong password")
	}

	// Corrupted or invalid hash
	corruptPlayer := Player{ID: "p1", Username: "alice", PasswordHash: "invalid-hash"}
	if corruptPlayer.Authenticate("correct-password") {
		t.Error("expected Authenticate to fail with corrupted hash")
	}

	// Empty hash
	emptyHashPlayer := Player{ID: "p1", Username: "alice", PasswordHash: ""}
	if emptyHashPlayer.Authenticate("correct-password") {
		t.Error("expected Authenticate to fail with empty hash")
	}
}

func TestPlayerAuthenticateEmptyPassword(t *testing.T) {
	p, err := New("alice", "correct-password", time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if p.Authenticate("") {
		t.Error("expected Authenticate to return false for empty password without error")
	}
}

func TestSessionExpiresAndCanBeRevoked(t *testing.T) {
	now := time.Unix(100, 0)
	value, err := NewSession("player-1", now, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if !value.Active(now.Add(30*time.Minute)) || value.Active(now.Add(time.Hour)) {
		t.Fatalf("session activity = %#v", value)
	}
	revoked := now.Add(time.Minute)
	value.RevokedAt = &revoked
	if value.Active(now) {
		t.Fatal("revoked session is active")
	}

	// Edge cases
	if _, err := NewSession("", now, time.Hour); !errors.Is(err, ErrInvalidSession) {
		t.Errorf("expected ErrInvalidSession for empty playerID, got %v", err)
	}
	if _, err := NewSession("player-1", now, 0); !errors.Is(err, ErrInvalidSession) {
		t.Errorf("expected ErrInvalidSession for zero duration, got %v", err)
	}
	if _, err := NewSession("player-1", now, -time.Hour); !errors.Is(err, ErrInvalidSession) {
		t.Errorf("expected ErrInvalidSession for negative duration, got %v", err)
	}
}

func BenchmarkHashPassword(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, err := hashPassword("my-secret-password-12345")
		if err != nil {
			b.Fatalf("hashPassword failed: %v", err)
		}
	}
}
