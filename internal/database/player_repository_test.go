package database

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	coreplayer "github.com/witchcraze/party2re/internal/core/player"
)

func TestNewPlayerRepositoryNilDB(t *testing.T) {
	repo, err := NewPlayerRepository(nil)
	if err == nil || repo != nil {
		t.Fatalf("NewPlayerRepository(nil) = (%v, %v), want error", repo, err)
	}
}

func TestPlayerRepositorySaveAndFind(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	repository, err := NewPlayerRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	player, err := coreplayer.New("testplayer_"+now.Format("150405.000000"), "securepassword123", now)
	if err != nil {
		t.Fatal(err)
	}

	if err := repository.Save(context.Background(), player); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// FindByID
	gotByID, err := repository.FindByID(context.Background(), player.ID)
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	if gotByID.ID != player.ID || gotByID.Username != player.Username || gotByID.PasswordHash != player.PasswordHash || !gotByID.CreatedAt.Equal(player.CreatedAt) {
		t.Fatalf("FindByID() = %#v, want %#v", gotByID, player)
	}

	// FindByUsername
	gotByName, err := repository.FindByUsername(context.Background(), player.Username)
	if err != nil {
		t.Fatalf("FindByUsername() error = %v", err)
	}
	if gotByName.ID != player.ID || gotByName.Username != player.Username || gotByName.PasswordHash != player.PasswordHash || !gotByName.CreatedAt.Equal(player.CreatedAt) {
		t.Fatalf("FindByUsername() = %#v, want %#v", gotByName, player)
	}

	// Duplicate Save on username should fail
	duplicatePlayer := player
	duplicatePlayer.ID = player.ID[:len(player.ID)-4] + "dupl"
	if err := repository.Save(context.Background(), duplicatePlayer); err == nil {
		t.Fatal("Save() duplicate username expected error, got nil")
	}

	// FindByID not found
	if _, err := repository.FindByID(context.Background(), "nonexistent_player_id"); !errors.Is(err, ErrPlayerNotFound) {
		t.Fatalf("FindByID(nonexistent) error = %v, want %v", err, ErrPlayerNotFound)
	}

	// FindByUsername not found
	if _, err := repository.FindByUsername(context.Background(), "nonexistent_username"); !errors.Is(err, ErrPlayerNotFound) {
		t.Fatalf("FindByUsername(nonexistent) error = %v, want %v", err, ErrPlayerNotFound)
	}

	// Delete player
	if err := repository.Delete(context.Background(), player.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// Ensure player is deleted
	if _, err := repository.FindByID(context.Background(), player.ID); !errors.Is(err, ErrPlayerNotFound) {
		t.Fatalf("FindByID() after Delete() = %v, want %v", err, ErrPlayerNotFound)
	}
}
