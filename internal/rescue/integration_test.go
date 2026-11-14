package rescue_test

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/rescue"
	"github.com/witchcraze/party2re/internal/scheduling"
	vk "github.com/witchcraze/party2re/internal/valkey"
)

// Concurrent rescues of one stuck character share one committed record and add
// the penalty once to the existing sleep.
func TestRescueServiceIntegration_ConcurrentRescueAddsOnePenalty(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" || os.Getenv("PARTY2_VALKEY_ADDR") == "" {
		t.Skip("PARTY2_DB_DSN and PARTY2_VALKEY_ADDR are required")
	}

	ctx := context.Background()
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	valkeyClient, err := vk.NewClient()
	if err != nil {
		t.Fatal(err)
	}
	defer valkeyClient.Close()

	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	rescueRepo, err := database.NewRescueRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	char, err := database.CreateTestCharacter(ctx, db, "RescueHero")
	if err != nil {
		t.Fatal(err)
	}
	schedService := scheduling.NewService(scheduling.NewValkeyRepository(valkeyClient))
	timerSvc := timer.NewService(valkeyClient)
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM rescue_records WHERE character_id = ?`, char.ID)
		_ = timerSvc.ReleaseLock(context.Background(), timer.CategorySleep, char.ID)
		_, _ = schedService.CancelByActorID(context.Background(), char.ID)
	})

	if err := timerSvc.SetLock(ctx, timer.CategorySleep, char.ID, time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err := schedService.Schedule(ctx, "adventure:complete", char.ID, map[string]string{"key": "val"}, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	svc := rescue.NewService(rescueRepo, charRepo, schedService, timerSvc, database.NewTransactionProvider(db))
	now := time.Now().UTC()
	var wg sync.WaitGroup
	errs := make([]error, 4)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = svc.EmergencyRescue(ctx, char.ID, "stuck", now)
		}(i)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatalf("concurrent rescue failed: %v", err)
		}
	}

	var records, unapplied int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*), COUNT(*) - COUNT(applied_at) FROM rescue_records WHERE character_id = ?`, char.ID).Scan(&records, &unapplied); err != nil {
		t.Fatal(err)
	}
	if records != 1 || unapplied != 0 {
		t.Fatalf("records=%d unapplied=%d, want one confirmed record", records, unapplied)
	}
	rem, err := timerSvc.GetRemainingLock(ctx, timer.CategorySleep, char.ID)
	if err != nil || rem < time.Hour+590*time.Second || rem > time.Hour+600*time.Second {
		t.Fatalf("sleep remaining = %v (err %v), want existing hour + 600s once", rem, err)
	}
	if pending, err := schedService.FindPendingByActorID(ctx, char.ID); err != nil || len(pending) != 0 {
		t.Fatalf("scheduled work remains: %v (err %v)", pending, err)
	}
	if latest, err := rescueRepo.FindLatestByCharacterID(ctx, char.ID); err != nil || !latest.SleepUntil.After(now.Add(time.Hour)) {
		t.Fatalf("latest rescue record = %+v (err %v)", latest, err)
	}
}
