package database_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/eventplaza"
	"github.com/witchcraze/party2re/internal/id"
)

func TestEventPlazaRepository_Database(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not set")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()

	slayer, err := database.CreateTestCharacter(ctx, db, "SlayerTester")
	if err != nil {
		t.Fatal(err)
	}

	toaster, err := database.CreateTestCharacter(ctx, db, "ToasterTester")
	if err != nil {
		t.Fatal(err)
	}

	repo, err := database.NewEventPlazaRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	// 1. CountActiveParticipants
	count, err := repo.CountActiveParticipants(ctx)
	if err != nil {
		t.Fatalf("CountActiveParticipants failed: %v", err)
	}
	if count < 2 {
		t.Errorf("expected at least 2 active characters, got %d", count)
	}

	// 2. SaveBanquet
	now := time.Now().UTC().Truncate(time.Second)
	banquet := eventplaza.CelebrationBanquet{
		ID:                  id.New(),
		BossID:              "boss-ancient-dragon",
		BossName:            "Ancient Dragon",
		SlayerCharacterID:   slayer.ID,
		SlayerCharacterName: slayer.Name,
		Tier:                2,
		ToastCount:          0,
		CelebratedAt:        now,
		ExpiresAt:           now.Add(24 * time.Hour),
	}

	err = repo.SaveBanquet(ctx, banquet)
	if err != nil {
		t.Fatalf("SaveBanquet failed: %v", err)
	}

	// 3. FindBanquetByID
	found, err := repo.FindBanquetByID(ctx, banquet.ID)
	if err != nil {
		t.Fatalf("FindBanquetByID failed: %v", err)
	}
	if found.BossName != "Ancient Dragon" || found.Tier != 2 {
		t.Errorf("unexpected banquet found: %+v", found)
	}

	// 4. ListActiveBanquets
	activeBanquets, err := repo.ListActiveBanquets(ctx, now.Add(-1*time.Minute))
	if err != nil {
		t.Fatalf("ListActiveBanquets failed: %v", err)
	}
	foundActive := false
	for _, b := range activeBanquets {
		if b.ID == banquet.ID {
			foundActive = true
			break
		}
	}
	if !foundActive {
		t.Errorf("expected banquet %s to be listed in active banquets", banquet.ID)
	}

	// 5. Toasting
	hasToasted, err := repo.HasToasted(ctx, banquet.ID, toaster.ID)
	if err != nil {
		t.Fatalf("HasToasted failed: %v", err)
	}
	if hasToasted {
		t.Error("expected hasToasted to be false before toasting")
	}

	err = repo.RecordToast(ctx, banquet.ID, toaster.ID, now)
	if err != nil {
		t.Fatalf("RecordToast failed: %v", err)
	}

	hasToastedAfter, err := repo.HasToasted(ctx, banquet.ID, toaster.ID)
	if err != nil {
		t.Fatalf("HasToasted after toast failed: %v", err)
	}
	if !hasToastedAfter {
		t.Error("expected hasToasted to be true after toasting")
	}

	// Verify updated toast count
	updatedBanquet, err := repo.FindBanquetByID(ctx, banquet.ID)
	if err != nil {
		t.Fatalf("FindBanquetByID failed: %v", err)
	}
	if updatedBanquet.ToastCount != 1 {
		t.Errorf("expected toast count 1, got %d", updatedBanquet.ToastCount)
	}
}
