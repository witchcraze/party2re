package database_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/rescue"
)

func TestRescueRepository_Integration(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

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

	// 2. FindRecentByCharacterID
	results, err := repo.FindRecentByCharacterID(ctx, "char-rescue-test", now.Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("FindRecentByCharacterID failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("expected at least 1 rescue record")
	}
	if results[0].Reason != "Client freeze rescue" {
		t.Errorf("unexpected reason: %s", results[0].Reason)
	}
}
