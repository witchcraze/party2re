package database_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/witchcraze/party2re/internal/chapel"
	"github.com/witchcraze/party2re/internal/database"
)

func TestChapelRepository_Integration(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	repo, err := database.NewChapelRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// 1. Create character
	char, err := database.CreateTestCharacter(ctx, db, "PrayingPriest")
	if err != nil {
		t.Fatal(err)
	}

	// 2. Initial state
	b, err := repo.GetBlessing(ctx, char.ID)
	if err != nil {
		t.Fatalf("GetBlessing failed: %v", err)
	}
	if b.ActiveBlessing != chapel.BlessingNone {
		t.Errorf("initial state = %+v, want NONE", b)
	}

	// 3. Select Monster Blessing
	b, err = repo.SelectBlessing(ctx, char.ID, chapel.BlessingMonster)
	if err != nil {
		t.Fatalf("SelectBlessing failed: %v", err)
	}
	if b.ActiveBlessing != chapel.BlessingMonster {
		t.Errorf("active blessing = %v, want MONSTER", b.ActiveBlessing)
	}

	// 4. Enforce single active wish constraint: second prayer fails with ErrAlreadyPrayed
	_, err = repo.SelectBlessing(ctx, char.ID, chapel.BlessingExp)
	if !errors.Is(err, chapel.ErrAlreadyPrayed) {
		t.Fatalf("expected ErrAlreadyPrayed, got %v", err)
	}

	// 5. Clear blessing
	if err := repo.ClearBlessing(ctx, char.ID); err != nil {
		t.Fatalf("ClearBlessing failed: %v", err)
	}
	b, err = repo.GetBlessing(ctx, char.ID)
	if err != nil {
		t.Fatalf("GetBlessing failed: %v", err)
	}
	if b.ActiveBlessing != chapel.BlessingNone {
		t.Errorf("expected NONE after ClearBlessing, got %v", b.ActiveBlessing)
	}

	// 6. Select Casino Blessing after clear
	b, err = repo.SelectBlessing(ctx, char.ID, chapel.BlessingCasino)
	if err != nil {
		t.Fatalf("SelectBlessing after clear failed: %v", err)
	}
	if b.ActiveBlessing != chapel.BlessingCasino {
		t.Errorf("active blessing = %v, want CASINO", b.ActiveBlessing)
	}
}
