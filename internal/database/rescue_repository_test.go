package database_test

import (
	"context"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/rescue"
)

func TestRescueRepository_Integration(t *testing.T) {
	db := database.OpenTestDB(t)

	repo, err := database.NewRescueRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	rec := rescue.RescueRecord{
		ID:             "test-rec-" + now.Format("150405"),
		CharacterID:    "char-rescue-test",
		Reason:         "Client freeze rescue",
		PenaltySeconds: 600,
		CreatedAt:      now,
	}

	// 1. Save
	if err := repo.Save(ctx, rec); err != nil {
		t.Fatalf("Save rescue record failed: %v", err)
	}

	// 2. FindLatestByCharacterID
	latest, err := repo.FindLatestByCharacterID(ctx, "char-rescue-test")
	if err != nil {
		t.Fatalf("FindLatestByCharacterID failed: %v", err)
	}
	if latest.ID != rec.ID {
		t.Errorf("expected ID %s, got %s", rec.ID, latest.ID)
	}
	if latest.Reason != "Client freeze rescue" {
		t.Errorf("unexpected reason: %s", latest.Reason)
	}
	if latest.PenaltySeconds != 600 {
		t.Errorf("expected penalty 600, got %d", latest.PenaltySeconds)
	}
}
