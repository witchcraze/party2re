package database

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/altar"
	"github.com/witchcraze/party2re/internal/id"
)

func TestNewAltarRepositoryNilDB(t *testing.T) {
	repo, err := NewAltarRepository(nil)
	if err == nil || repo != nil {
		t.Fatalf("NewAltarRepository(nil) = (%v, %v), want error", repo, err)
	}
}

func TestAltarRepositorySaveAndGetLatest(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	player, err := CreateTestPlayer(ctx, db)
	if err != nil {
		t.Fatal(err)
	}

	char, err := CreateTestCharacter(ctx, db, player.ID)
	if err != nil {
		t.Fatal(err)
	}

	altarRepo, err := NewAltarRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().Truncate(time.Second)
	awakening := altar.RamiaAwakening{
		ID:            id.New(),
		CharacterID:   char.ID,
		CharacterName: char.Name,
		AwakenedAt:    now,
		ExpiresAt:     now.Add(30 * time.Minute),
	}

	if err := altarRepo.SaveAwakening(ctx, awakening); err != nil {
		t.Fatalf("SaveAwakening error = %v", err)
	}

	latest, err := altarRepo.GetLatestAwakening(ctx)
	if err != nil {
		t.Fatalf("GetLatestAwakening error = %v", err)
	}
	if latest == nil {
		t.Fatal("expected latest awakening, got nil")
	}
	if latest.ID != awakening.ID || latest.CharacterID != awakening.CharacterID || latest.CharacterName != awakening.CharacterName {
		t.Fatalf("unexpected latest awakening: %+v, want %+v", latest, awakening)
	}
}
