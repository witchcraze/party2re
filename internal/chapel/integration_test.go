package chapel_test

import (
	"context"
	"os"
	"testing"

	"github.com/witchcraze/party2re/internal/chapel"
	"github.com/witchcraze/party2re/internal/database"
)

func TestChapelServiceDatabaseIntegration(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	chapelRepo, err := database.NewChapelRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	svc, err := chapel.NewService(chapelRepo)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// 1. Create character & fund
	char, err := database.CreateTestCharacter(ctx, db, "ChapelDevotee")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = db.ExecContext(ctx, "UPDATE characters SET money = 10000 WHERE id = ?", char.ID)

	// 2. Select Blessing
	b, err := svc.SelectBlessing(ctx, char.ID, chapel.BlessingDrop)
	if err != nil {
		t.Fatalf("SelectBlessing failed: %v", err)
	}
	if b.ActiveBlessing != chapel.BlessingDrop {
		t.Errorf("expected BlessingDrop, got %v", b.ActiveBlessing)
	}

	// 3. Donate
	b, err = svc.Donate(ctx, char.ID, 1000)
	if err != nil {
		t.Fatalf("Donate failed: %v", err)
	}
	if b.DonationGoldTotal != 1000 {
		t.Errorf("donation = %d, want 1000", b.DonationGoldTotal)
	}

	// 4. Retrieve
	b, err = svc.GetBlessing(ctx, char.ID)
	if err != nil {
		t.Fatalf("GetBlessing failed: %v", err)
	}
	if b.ActiveBlessing != chapel.BlessingDrop || b.DonationGoldTotal != 1000 {
		t.Errorf("got state %+v", b)
	}
}
