package database_test

import (
	"context"
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

	// 1. Create character & fund
	char, err := database.CreateTestCharacter(ctx, db, "PrayingPriest")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = db.ExecContext(ctx, "UPDATE characters SET money = 10000 WHERE id = ?", char.ID)

	// 2. Initial state
	b, err := repo.GetBlessing(ctx, char.ID)
	if err != nil {
		t.Fatalf("GetBlessing failed: %v", err)
	}
	if b.ActiveBlessing != chapel.BlessingNone || b.DonationGoldTotal != 0 {
		t.Errorf("initial state = %+v", b)
	}

	// 3. Select Blessing
	b, err = repo.SelectBlessing(ctx, char.ID, chapel.BlessingGold)
	if err != nil {
		t.Fatalf("SelectBlessing failed: %v", err)
	}
	if b.ActiveBlessing != chapel.BlessingGold {
		t.Errorf("active blessing = %v, want GOLD", b.ActiveBlessing)
	}

	// 4. Donate gold
	b, err = repo.Donate(ctx, char.ID, 500)
	if err != nil {
		t.Fatalf("Donate failed: %v", err)
	}
	if b.DonationGoldTotal != 500 {
		t.Errorf("donation total = %d, want 500", b.DonationGoldTotal)
	}

	// 5. Donate more gold
	b, err = repo.Donate(ctx, char.ID, 300)
	if err != nil {
		t.Fatalf("Donate 2 failed: %v", err)
	}
	if b.DonationGoldTotal != 800 {
		t.Errorf("donation total = %d, want 800", b.DonationGoldTotal)
	}
}
