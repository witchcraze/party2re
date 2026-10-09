package chapel_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

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

	ctx := context.Background()

	// 1. Create character
	char, err := database.CreateTestCharacter(ctx, db, "ChapelDevotee")
	if err != nil {
		t.Fatal(err)
	}

	currentTime := time.Date(2026, 10, 9, 10, 0, 0, 0, chapel.JST)
	nowFunc := func() time.Time { return currentTime }

	chapelRepo, err := database.NewChapelRepository(db, database.WithChapelNowFunc(nowFunc))
	if err != nil {
		t.Fatal(err)
	}

	svc, err := chapel.NewService(chapelRepo, chapel.WithNowFunc(nowFunc))
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
	if status.PrayedAt != nil {
		t.Errorf("expected nil PrayedAt initially, got %v", status.PrayedAt)
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

	// 5. Sleep on same day: clears active blessing effect, retains prayer quota record
	currentTime = time.Date(2026, 10, 9, 14, 0, 0, 0, chapel.JST)
	if err := svc.ClearBlessing(ctx, char.ID); err != nil {
		t.Fatalf("ClearBlessing failed: %v", err)
	}

	b, err = svc.GetBlessing(ctx, char.ID)
	if err != nil {
		t.Fatalf("GetBlessing failed: %v", err)
	}
	if b.ActiveBlessing != chapel.BlessingNone {
		t.Errorf("expected BlessingNone after sleep, got %v", b.ActiveBlessing)
	}
	if b.PrayedAt.IsZero() {
		t.Errorf("expected non-zero PrayedAt retained after same-day sleep")
	}

	// Re-prayer on same day after sleep must be REJECTED
	currentTime = time.Date(2026, 10, 9, 15, 0, 0, 0, chapel.JST)
	_, err = svc.Pray(ctx, char.ID, chapel.BlessingMonster)
	if !errors.Is(err, chapel.ErrAlreadyPrayed) {
		t.Fatalf("expected ErrAlreadyPrayed on same day after sleep, got %v", err)
	}

	// 6. Midnight passes to next day without sleep: prayer still rejected
	currentTime = time.Date(2026, 10, 10, 8, 0, 0, 0, chapel.JST)
	_, err = svc.Pray(ctx, char.ID, chapel.BlessingMonster)
	if !errors.Is(err, chapel.ErrAlreadyPrayed) {
		t.Fatalf("expected ErrAlreadyPrayed on Day 2 before sleep, got %v", err)
	}

	// 7. Sleep on Day 2: clears yesterday's prayer record
	currentTime = time.Date(2026, 10, 10, 8, 30, 0, 0, chapel.JST)
	if err := svc.ClearBlessing(ctx, char.ID); err != nil {
		t.Fatalf("ClearBlessing on Day 2 failed: %v", err)
	}

	// 8. Re-prayer on Day 2 after sleep succeeds!
	currentTime = time.Date(2026, 10, 10, 9, 0, 0, 0, chapel.JST)
	b, err = svc.Pray(ctx, char.ID, chapel.BlessingMonster)
	if err != nil {
		t.Fatalf("Pray on Day 2 failed: %v", err)
	}
	if b.ActiveBlessing != chapel.BlessingMonster {
		t.Errorf("expected BlessingMonster on Day 2, got %v", b.ActiveBlessing)
	}
}
