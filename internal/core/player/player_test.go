package player

import (
	"testing"
	"time"
)

func TestPlayerStoresOnlyPasswordHashAndAuthenticates(t *testing.T) {
	value, err := New(" alice ", "secret", time.Unix(100, 0))
	if err != nil {
		t.Fatal(err)
	}
	if value.Username != "alice" || value.PasswordHash == "secret" || !value.Authenticate("secret") || value.Authenticate("wrong") {
		t.Fatalf("player = %#v", value)
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
}
