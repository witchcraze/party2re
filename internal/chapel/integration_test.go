package chapel_test

import (
	"context"
	"errors"
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

	// 1. Create character
	char, err := database.CreateTestCharacter(ctx, db, "ChapelDevotee")
	if err != nil {
		t.Fatal(err)
	}

	// 2. Initial state
	status, err := svc.GetStatus(ctx, char.ID, char.Name)
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}
	if status.HasActiveBlessing {
		t.Errorf("expected no active blessing initially")
	}

	// 3. Select Blessing (Drop)
	b, err := svc.SelectBlessing(ctx, char.ID, chapel.BlessingDrop)
	if err != nil {
		t.Fatalf("SelectBlessing failed: %v", err)
	}
	if b.ActiveBlessing != chapel.BlessingDrop {
		t.Errorf("expected BlessingDrop, got %v", b.ActiveBlessing)
	}

	// 4. Single-active-wish constraint: second prayer fails with ErrAlreadyPrayed
	_, err = svc.SelectBlessing(ctx, char.ID, chapel.BlessingExp)
	if !errors.Is(err, chapel.ErrAlreadyPrayed) {
		t.Fatalf("expected ErrAlreadyPrayed, got %v", err)
	}

	// 5. Clear blessing and select Monster blessing
	if err := svc.ClearBlessing(ctx, char.ID); err != nil {
		t.Fatalf("ClearBlessing failed: %v", err)
	}
	b, err = svc.Pray(ctx, char.ID, chapel.BlessingMonster)
	if err != nil {
		t.Fatalf("Pray failed: %v", err)
	}
	if b.ActiveBlessing != chapel.BlessingMonster {
		t.Errorf("expected BlessingMonster, got %v", b.ActiveBlessing)
	}

	// 6. Retrieve
	b, err = svc.GetBlessing(ctx, char.ID)
	if err != nil {
		t.Fatalf("GetBlessing failed: %v", err)
	}
	if b.ActiveBlessing != chapel.BlessingMonster {
		t.Errorf("got blessing %+v, want MONSTER", b.ActiveBlessing)
	}
}
