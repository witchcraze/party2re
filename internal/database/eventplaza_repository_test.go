package database_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/database"
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

	visitor, err := database.CreateTestCharacter(ctx, db, "VisitorTester")
	if err != nil {
		t.Fatal(err)
	}

	otherVisitor, err := database.CreateTestCharacter(ctx, db, "OtherVisitorTester")
	if err != nil {
		t.Fatal(err)
	}

	repo, err := database.NewEventPlazaRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	// 1. RecordPresence and CountActiveParticipants
	now := time.Now().UTC().Truncate(time.Second)
	if err := repo.RecordPresence(ctx, visitor.ID, now); err != nil {
		t.Fatalf("RecordPresence failed: %v", err)
	}
	if err := repo.RecordPresence(ctx, otherVisitor.ID, now); err != nil {
		t.Fatalf("RecordPresence failed: %v", err)
	}

	count, err := repo.CountActiveParticipants(ctx, now.Add(-5*time.Minute))
	if err != nil {
		t.Fatalf("CountActiveParticipants failed: %v", err)
	}
	if count < 2 {
		t.Errorf("expected at least 2 active characters, got %d", count)
	}

	// Cutoff in the future should return 0
	countFuture, err := repo.CountActiveParticipants(ctx, now.Add(5*time.Minute))
	if err != nil {
		t.Fatalf("CountActiveParticipants future cutoff failed: %v", err)
	}
	if countFuture != 0 {
		t.Errorf("expected 0 active characters for future cutoff, got %d", countFuture)
	}

}
