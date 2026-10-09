package database_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/chapel"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/testutil"
)

func TestChapelRepository_LifecycleAndQuota(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	char, err := database.CreateTestCharacter(ctx, db, "PrayingDevotee")
	if err != nil {
		t.Fatal(err)
	}

	// 1. Initial unprayed state
	currentTime := time.Date(2026, 10, 9, 10, 0, 0, 0, chapel.JST)
	repo, err := database.NewChapelRepository(db, database.WithChapelNowFunc(func() time.Time {
		return currentTime
	}))
	if err != nil {
		t.Fatal(err)
	}

	b, err := repo.GetBlessing(ctx, char.ID)
	if err != nil {
		t.Fatalf("GetBlessing failed: %v", err)
	}
	if b.ActiveBlessing != chapel.BlessingNone {
		t.Errorf("initial active blessing = %v, want NONE", b.ActiveBlessing)
	}
	if !b.PrayedAt.IsZero() {
		t.Errorf("initial PrayedAt = %v, want zero time", b.PrayedAt)
	}

	// 2. Select blessing at 10:00 JST Day 1
	b, err = repo.SelectBlessing(ctx, char.ID, chapel.BlessingMonster)
	if err != nil {
		t.Fatalf("SelectBlessing failed: %v", err)
	}
	if b.ActiveBlessing != chapel.BlessingMonster {
		t.Errorf("active blessing = %v, want MONSTER", b.ActiveBlessing)
	}
	if b.PrayedAt.IsZero() {
		t.Errorf("expected non-zero PrayedAt after prayer")
	}

	// 3. Double-prayer rejection while active
	currentTime = time.Date(2026, 10, 9, 10, 30, 0, 0, chapel.JST)
	_, err = repo.SelectBlessing(ctx, char.ID, chapel.BlessingExp)
	if !errors.Is(err, chapel.ErrAlreadyPrayed) {
		t.Fatalf("expected ErrAlreadyPrayed, got %v", err)
	}

	// 4. Same-day sleep at 14:00 JST: active blessing cleared, prayer record kept
	currentTime = time.Date(2026, 10, 9, 14, 0, 0, 0, chapel.JST)
	if err := repo.ClearBlessing(ctx, char.ID); err != nil {
		t.Fatalf("ClearBlessing failed: %v", err)
	}
	b, err = repo.GetBlessing(ctx, char.ID)
	if err != nil {
		t.Fatalf("GetBlessing failed: %v", err)
	}
	if b.ActiveBlessing != chapel.BlessingNone {
		t.Errorf("expected active blessing NONE after same-day sleep, got %v", b.ActiveBlessing)
	}
	if b.PrayedAt.IsZero() {
		t.Errorf("expected PrayedAt record to be retained after same-day sleep")
	}

	// 5. Attempt re-prayer on same day at 15:00 JST -> must be REJECTED
	currentTime = time.Date(2026, 10, 9, 15, 0, 0, 0, chapel.JST)
	_, err = repo.SelectBlessing(ctx, char.ID, chapel.BlessingGold)
	if !errors.Is(err, chapel.ErrAlreadyPrayed) {
		t.Fatalf("expected ErrAlreadyPrayed on same-day re-prayer after sleep, got %v", err)
	}

	// 6. Midnight rollover to Day 2 08:00 JST without sleep: prayer still rejected
	currentTime = time.Date(2026, 10, 10, 8, 0, 0, 0, chapel.JST)
	_, err = repo.SelectBlessing(ctx, char.ID, chapel.BlessingGold)
	if !errors.Is(err, chapel.ErrAlreadyPrayed) {
		t.Fatalf("expected ErrAlreadyPrayed on Day 2 before sleep, got %v", err)
	}

	// 7. Sleep at Day 2 08:30 JST: clears prayer record from yesterday
	currentTime = time.Date(2026, 10, 10, 8, 30, 0, 0, chapel.JST)
	if err := repo.ClearBlessing(ctx, char.ID); err != nil {
		t.Fatalf("ClearBlessing on Day 2 failed: %v", err)
	}
	b, err = repo.GetBlessing(ctx, char.ID)
	if err != nil {
		t.Fatalf("GetBlessing failed: %v", err)
	}
	if b.ActiveBlessing != chapel.BlessingNone {
		t.Errorf("expected active blessing NONE, got %v", b.ActiveBlessing)
	}
	if !b.PrayedAt.IsZero() {
		t.Errorf("expected PrayedAt to be cleared after next-day sleep, got %v", b.PrayedAt)
	}

	// 8. Re-prayer at Day 2 09:00 JST succeeds
	currentTime = time.Date(2026, 10, 10, 9, 0, 0, 0, chapel.JST)
	b, err = repo.SelectBlessing(ctx, char.ID, chapel.BlessingCasino)
	if err != nil {
		t.Fatalf("SelectBlessing on Day 2 after sleep failed: %v", err)
	}
	if b.ActiveBlessing != chapel.BlessingCasino {
		t.Errorf("active blessing = %v, want CASINO", b.ActiveBlessing)
	}
}

func TestChapelRepository_ConcurrentSelectBlessing(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	char, err := database.CreateTestCharacter(ctx, db, "RaceDevotee")
	if err != nil {
		t.Fatal(err)
	}

	repo, err := database.NewChapelRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	var successes int32
	var alreadyPrayedErrors int32

	cfg := testutil.ConcurrencyStressConfig{
		Workers:      10,
		OpsPerWorker: 1,
	}

	res := testutil.RunConcurrentStressTest(t, cfg, func(workerID int, op int) error {
		blessings := []chapel.BlessingType{
			chapel.BlessingGold,
			chapel.BlessingExp,
			chapel.BlessingMonster,
			chapel.BlessingDrop,
			chapel.BlessingCasino,
		}
		targetBlessing := blessings[workerID%len(blessings)]

		_, err := repo.SelectBlessing(ctx, char.ID, targetBlessing)
		if err == nil {
			atomic.AddInt32(&successes, 1)
			return nil
		}
		if errors.Is(err, chapel.ErrAlreadyPrayed) {
			atomic.AddInt32(&alreadyPrayedErrors, 1)
			return nil
		}
		return err
	})

	if res.Deadlocks > 0 {
		t.Fatalf("detected %d deadlocks during concurrent SelectBlessing", res.Deadlocks)
	}

	if successes != 1 {
		t.Errorf("expected exactly 1 success among concurrent prayers, got %d", successes)
	}
	if alreadyPrayedErrors != 9 {
		t.Errorf("expected 9 ErrAlreadyPrayed rejections, got %d", alreadyPrayedErrors)
	}

	// Verify state in DB
	b, err := repo.GetBlessing(ctx, char.ID)
	if err != nil {
		t.Fatalf("GetBlessing failed: %v", err)
	}
	if b.ActiveBlessing == chapel.BlessingNone || b.ActiveBlessing == "" {
		t.Errorf("expected character to hold an active blessing, got NONE")
	}
}

func TestChapelRepository_ConcurrentSelectAndClear(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	char, err := database.CreateTestCharacter(ctx, db, "SleepRaceDevotee")
	if err != nil {
		t.Fatal(err)
	}

	today := time.Date(2026, 10, 9, 12, 0, 0, 0, chapel.JST)
	repo, err := database.NewChapelRepository(db, database.WithChapelNowFunc(func() time.Time {
		return today
	}))
	if err != nil {
		t.Fatal(err)
	}

	var start sync.WaitGroup
	var done sync.WaitGroup
	start.Add(1)
	done.Add(2)

	var selectErr error
	var clearErr error

	go func() {
		defer done.Done()
		start.Wait()
		_, selectErr = repo.SelectBlessing(ctx, char.ID, chapel.BlessingExp)
	}()

	go func() {
		defer done.Done()
		start.Wait()
		clearErr = repo.ClearBlessing(ctx, char.ID)
	}()

	start.Done()
	done.Wait()

	if clearErr != nil {
		t.Fatalf("ClearBlessing failed: %v", clearErr)
	}
	if selectErr != nil {
		t.Fatalf("SelectBlessing failed: %v", selectErr)
	}

	// In either execution order on the same day:
	// A re-prayer attempt on the same day MUST be rejected because prayed_at is today!
	_, err = repo.SelectBlessing(ctx, char.ID, chapel.BlessingGold)
	if !errors.Is(err, chapel.ErrAlreadyPrayed) {
		t.Errorf("expected ErrAlreadyPrayed after concurrent select and sleep on same day, got %v", err)
	}
}
