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

func TestPlayerRepositoryListAndBan(t *testing.T) {
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

	ctx := context.Background()
	prefix := "listtest_" + time.Now().Format("150405_")
	now := time.Now().UTC().Truncate(time.Microsecond)

	p1, _ := coreplayer.New(prefix+"zeta", "pass", now)
	p1.LastIP = "10.0.0.2"
	p2, _ := coreplayer.New(prefix+"alpha", "pass", now.Add(time.Second))
	p2.LastIP = "10.0.0.1"

	if err := repository.Save(ctx, p1); err != nil {
		t.Fatal(err)
	}
	defer repository.Delete(ctx, p1.ID)
	if err := repository.Save(ctx, p2); err != nil {
		t.Fatal(err)
	}
	defer repository.Delete(ctx, p2.ID)

	// Test UpdateLastLogin
	newIP := "192.168.1.100"
	loginTime := now.Add(2 * time.Second)
	if err := repository.UpdateLastLogin(ctx, p1.ID, newIP, loginTime); err != nil {
		t.Fatalf("UpdateLastLogin() failed: %v", err)
	}
	gotP1, err := repository.FindByID(ctx, p1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gotP1.LastIP != newIP {
		t.Errorf("expected LastIP %q, got %q", newIP, gotP1.LastIP)
	}

	// Test UpdateBannedAt
	banTime := now.Add(3 * time.Second)
	if err := repository.UpdateBannedAt(ctx, p1.ID, &banTime); err != nil {
		t.Fatalf("UpdateBannedAt() failed: %v", err)
	}
	gotP1Banned, err := repository.FindByID(ctx, p1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !gotP1Banned.IsBanned() {
		t.Fatal("expected player to be banned")
	}

	// Non-existent player ban
	if err := repository.UpdateBannedAt(ctx, "nonexistent-id", &banTime); !errors.Is(err, ErrPlayerNotFound) {
		t.Fatalf("expected ErrPlayerNotFound for nonexistent player, got %v", err)
	}

	// Test List sorts
	for _, sortType := range []string{"name", "updated_at", "addr", "invalid"} {
		list, err := repository.List(ctx, sortType)
		if err != nil {
			t.Fatalf("List(%s) failed: %v", sortType, err)
		}
		if len(list) < 2 {
			t.Fatalf("expected at least 2 players in list, got %d", len(list))
		}
	}
}
