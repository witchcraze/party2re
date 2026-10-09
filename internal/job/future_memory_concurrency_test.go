package job_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	corejob "github.com/witchcraze/party2re/internal/core/job"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/economy"
	"github.com/witchcraze/party2re/internal/job"
	"github.com/witchcraze/party2re/internal/testutil"
)

func TestFutureMemory_Integration_ConcurrentSavesEnforceCapacity(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	char, err := database.CreateTestCharacter(ctx, db, "ConcFutureSave")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		_, _ = db.ExecContext(cleanupCtx, "DELETE FROM character_future_memories WHERE character_id = ?", char.ID)
		_, _ = db.ExecContext(cleanupCtx, "DELETE FROM inventory_items WHERE character_id = ?", char.ID)
		_, _ = db.ExecContext(cleanupCtx, "DELETE FROM character_jobs WHERE character_id = ?", char.ID)
		_, _ = db.ExecContext(cleanupCtx, "DELETE FROM characters WHERE id = ?", char.ID)
		_, _ = db.ExecContext(cleanupCtx, "DELETE FROM players WHERE id = ?", char.PlayerID)
	})

	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	char.OverFuture = 0 // Allows at most 1 snapshot (0 <= 0)
	char.JobID = "job-05"
	char.OldJobID = "job-01"
	char.Level = 50
	char.SP = 100
	if err := charRepo.Update(ctx, char); err != nil {
		t.Fatal(err)
	}

	// Prepare inventory with 2 pieces of item-207
	invRepo, err := database.NewInventoryRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	inv, err := coreinventory.New(char.ID)
	if err != nil {
		t.Fatal(err)
	}
	frag1, err := coreitem.NewInstance("item-207", 1)
	if err != nil {
		t.Fatal(err)
	}
	frag2, err := coreitem.NewInstance("item-207", 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := inv.Add(frag1); err != nil {
		t.Fatal(err)
	}
	if err := inv.Add(frag2); err != nil {
		t.Fatal(err)
	}
	if err := invRepo.Save(ctx, inv); err != nil {
		t.Fatal(err)
	}

	jobRepo, err := database.NewCharacterJobRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	initialJob, err := corejob.NewCharacterJob(char.ID, "job-05")
	if err != nil {
		t.Fatal(err)
	}
	if err := jobRepo.Save(ctx, initialJob); err != nil {
		t.Fatal(err)
	}

	futureRepo, err := database.NewFutureMemoryRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	txProvider := database.NewTransactionProvider(db)
	ecoSvc, err := economy.NewService(charRepo, invRepo, economy.WithTransactionProvider(txProvider))
	if err != nil {
		t.Fatal(err)
	}

	svc, err := job.NewService(
		jobRepo,
		job.WithCharacterRepository(charRepo),
		job.WithInventoryRepository(invRepo),
		job.WithEconomy(ecoSvc),
		job.WithFutureMemoryRepository(futureRepo),
	)
	if err != nil {
		t.Fatal(err)
	}

	// Run 2 concurrent calls to SaveFutureMemory
	var successCount int32
	var slotLimitCount int32

	cfg := testutil.ConcurrencyStressConfig{
		Workers:      2,
		OpsPerWorker: 1,
	}

	res := testutil.RunConcurrentStressTest(t, cfg, func(_ int, _ int) error {
		_, err := svc.SaveFutureMemory(ctx, char.ID)
		if err == nil {
			atomic.AddInt32(&successCount, 1)
			return nil
		}
		if errors.Is(err, job.ErrFutureMemorySlotLimit) || err.Error() == "future memory slot limit reached" {
			atomic.AddInt32(&slotLimitCount, 1)
			return nil
		}
		return err
	})

	if res.Failures > 0 {
		t.Fatalf("unexpected failures in stress test: %#v", res)
	}

	if successCount != 1 {
		t.Fatalf("expected exactly 1 successful save, got %d", successCount)
	}
	if slotLimitCount != 1 {
		t.Fatalf("expected exactly 1 slot limit error, got %d", slotLimitCount)
	}

	// Verify database state: exactly 1 future memory row exists
	memories, err := futureRepo.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(memories) != 1 {
		t.Fatalf("expected exactly 1 memory in database, got %d", len(memories))
	}

	// Verify inventory state: exactly 1 item-207 consumed, 1 remaining
	updatedInv, err := invRepo.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updatedInv.Quantity("item-207") != 1 {
		t.Fatalf("expected 1 item-207 remaining in inventory, got %d", updatedInv.Quantity("item-207"))
	}
}

func TestFutureMemory_Integration_ConcurrentSaveAndJobMemoryConflict(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	char, err := database.CreateTestCharacter(ctx, db, "ConcJobMemConf")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		_, _ = db.ExecContext(cleanupCtx, "DELETE FROM character_future_memories WHERE character_id = ?", char.ID)
		_, _ = db.ExecContext(cleanupCtx, "DELETE FROM inventory_items WHERE character_id = ?", char.ID)
		_, _ = db.ExecContext(cleanupCtx, "DELETE FROM character_jobs WHERE character_id = ?", char.ID)
		_, _ = db.ExecContext(cleanupCtx, "DELETE FROM characters WHERE id = ?", char.ID)
		_, _ = db.ExecContext(cleanupCtx, "DELETE FROM players WHERE id = ?", char.PlayerID)
	})

	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	char.OverFuture = 0
	char.JobID = "job-05"
	char.OldJobID = "job-01"
	if err := charRepo.Update(ctx, char); err != nil {
		t.Fatal(err)
	}

	invRepo, err := database.NewInventoryRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	inv, err := coreinventory.New(char.ID)
	if err != nil {
		t.Fatal(err)
	}
	frag, err := coreitem.NewInstance("item-207", 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := inv.Add(frag); err != nil {
		t.Fatal(err)
	}
	if err := invRepo.Save(ctx, inv); err != nil {
		t.Fatal(err)
	}

	jobRepo, err := database.NewCharacterJobRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	initialJob, err := corejob.NewCharacterJob(char.ID, "job-05")
	if err != nil {
		t.Fatal(err)
	}
	if err := jobRepo.Save(ctx, initialJob); err != nil {
		t.Fatal(err)
	}

	futureRepo, err := database.NewFutureMemoryRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	txProvider := database.NewTransactionProvider(db)
	ecoSvc, err := economy.NewService(charRepo, invRepo, economy.WithTransactionProvider(txProvider))
	if err != nil {
		t.Fatal(err)
	}

	svc, err := job.NewService(
		jobRepo,
		job.WithCharacterRepository(charRepo),
		job.WithInventoryRepository(invRepo),
		job.WithEconomy(ecoSvc),
		job.WithFutureMemoryRepository(futureRepo),
	)
	if err != nil {
		t.Fatal(err)
	}

	// In this test, a goroutine holds the character lock and sets JobMemory != nil,
	// committing right before or while SaveFutureMemory waits for the lock.
	// SaveFutureMemory must see the active JobMemory upon acquiring the lock,
	// abort and return ErrJobUnavailable without consuming item-207.
	var wg sync.WaitGroup
	wg.Add(2)

	barrier := make(chan struct{})
	var saveErr error

	go func() {
		defer wg.Done()
		<-barrier
		// Introduce artificial contention by performing a transactional update that sets JobMemory
		err := database.RunInTx(ctx, db, func(txCtx context.Context) error {
			c, err := charRepo.FindByIDForUpdate(txCtx, char.ID)
			if err != nil {
				return err
			}
			c.JobMemory = &corecharacter.JobMemory{JobID: "job-02", SP: 50, OldJobID: "job-01", OldSP: 20}
			return charRepo.Update(txCtx, c)
		})
		if err != nil {
			t.Errorf("failed to set JobMemory in tx: %v", err)
		}
	}()

	go func() {
		defer wg.Done()
		<-barrier
		time.Sleep(5 * time.Millisecond) // Let JobMemory tx acquire lock first
		_, saveErr = svc.SaveFutureMemory(ctx, char.ID)
	}()

	close(barrier)
	wg.Wait()

	if !errors.Is(saveErr, corejob.ErrJobUnavailable) {
		t.Fatalf("expected ErrJobUnavailable when JobMemory became active, got %v", saveErr)
	}

	// Verify item-207 was preserved
	updatedInv, err := invRepo.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updatedInv.Quantity("item-207") != 1 {
		t.Fatalf("expected item-207 preserved in inventory, got %d", updatedInv.Quantity("item-207"))
	}

	// Verify no future memory saved
	memories, err := futureRepo.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(memories) != 0 {
		t.Fatalf("expected 0 future memories, got %d", len(memories))
	}
}
