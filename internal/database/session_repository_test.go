package database

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	coreplayer "github.com/witchcraze/party2re/internal/core/player"
)

func TestNewSessionRepositoryNilDB(t *testing.T) {
	repo, err := NewSessionRepository(nil)
	if err == nil || repo != nil {
		t.Fatalf("NewSessionRepository(nil) = (%v, %v), want error", repo, err)
	}
}

func TestSessionRepositorySaveFindAndRevoke(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	playerRepo, err := NewPlayerRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	player, err := coreplayer.New("sessionplayer_"+now.Format("150405.000000"), "securepassword123", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := playerRepo.Save(context.Background(), player); err != nil {
		t.Fatal(err)
	}

	sessionRepo, err := NewSessionRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	session, err := coreplayer.NewSession(player.ID, now, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	if err := sessionRepo.Save(context.Background(), session); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, err := sessionRepo.FindByID(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	if got.ID != session.ID || got.PlayerID != session.PlayerID || !got.CreatedAt.Equal(session.CreatedAt) || !got.ExpiresAt.Equal(session.ExpiresAt) || got.RevokedAt != nil {
		t.Fatalf("FindByID() = %#v, want %#v", got, session)
	}

	// Revoke session
	revokedAt := now.Add(10 * time.Minute)
	if err := sessionRepo.Revoke(context.Background(), session.ID, revokedAt); err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}

	gotRevoked, err := sessionRepo.FindByID(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("FindByID() after revoke error = %v", err)
	}
	if gotRevoked.RevokedAt == nil || !gotRevoked.RevokedAt.Equal(revokedAt) {
		t.Fatalf("FindByID() RevokedAt = %v, want %v", gotRevoked.RevokedAt, revokedAt)
	}

	// Revoking already revoked session returns ErrInvalidSession
	if err := sessionRepo.Revoke(context.Background(), session.ID, revokedAt); !errors.Is(err, coreplayer.ErrInvalidSession) {
		t.Fatalf("Revoke(already revoked) error = %v, want %v", err, coreplayer.ErrInvalidSession)
	}

	// Nonexistent session FindByID and Revoke
	if _, err := sessionRepo.FindByID(context.Background(), "nonexistent_session"); !errors.Is(err, coreplayer.ErrInvalidSession) {
		t.Fatalf("FindByID(nonexistent) error = %v, want %v", err, coreplayer.ErrInvalidSession)
	}
	if err := sessionRepo.Revoke(context.Background(), "nonexistent_session", revokedAt); !errors.Is(err, coreplayer.ErrInvalidSession) {
		t.Fatalf("Revoke(nonexistent) error = %v, want %v", err, coreplayer.ErrInvalidSession)
	}
}
