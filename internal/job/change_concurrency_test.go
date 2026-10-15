package job_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	corejob "github.com/witchcraze/party2re/internal/core/job"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/economy"
	"github.com/witchcraze/party2re/internal/job"
)

type interceptorCharRepo struct {
	inner      job.CharacterRepository
	onFindByID func()
}

func (r *interceptorCharRepo) FindByID(ctx context.Context, id string) (corecharacter.Character, error) {
	c, err := r.inner.FindByID(ctx, id)
	if r.onFindByID != nil {
		r.onFindByID()
	}
	return c, err
}

func (r *interceptorCharRepo) FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error) {
	return r.inner.FindByIDForUpdate(ctx, id)
}

func (r *interceptorCharRepo) Update(ctx context.Context, char corecharacter.Character) error {
	return r.inner.Update(ctx, char)
}

func TestChangeJob_Integration_RevalidateJobMemoryUnderLock(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	cases := []struct {
		name         string
		itemID       string
		expectedKind corecharacter.JobMemoryKind
	}{
		{
			name:         "item-168 persistent memory conflict",
			itemID:       "item-168",
			expectedKind: corecharacter.JobMemoryKindPersistent,
		},
		{
			name:         "item-243 temporary memory conflict",
			itemID:       "item-243",
			expectedKind: corecharacter.JobMemoryKindTemporary,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			db, err := database.OpenFromEnvironment()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := db.Close(); err != nil {
					t.Errorf("failed to close database: %v", err)
				}
			})

			charRepo, err := database.NewCharacterRepository(db)
			if err != nil {
				t.Fatal(err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			char, err := database.CreateTestCharacter(ctx, db, "ChangeJobLock_"+tc.itemID)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				cleanupCtx := context.Background()
				if err := charRepo.Delete(cleanupCtx, char.ID); err != nil {
					t.Errorf("cleanup failed for character %s: %v", char.ID, err)
				}
				if _, err := db.ExecContext(cleanupCtx, "DELETE FROM players WHERE id = ?", char.PlayerID); err != nil {
					t.Errorf("cleanup failed for player %s: %v", char.PlayerID, err)
				}
			})
			char.JobID = "job-01"
			char.OldJobID = "job-02"
			char.Level = 50
			char.SP = 100
			char.OldSP = 50
			char.Gender = "male"
			if err := charRepo.Update(ctx, char); err != nil {
				t.Fatal(err)
			}

			// Prepare inventory with 1 piece of itemID (item-168 or item-243)
			invRepo, err := database.NewInventoryRepository(db)
			if err != nil {
				t.Fatal(err)
			}
			inv, err := coreinventory.New(char.ID)
			if err != nil {
				t.Fatal(err)
			}
			itemInst, err := coreitem.NewInstance(tc.itemID, 1)
			if err != nil {
				t.Fatal(err)
			}
			if err := inv.Add(itemInst); err != nil {
				t.Fatal(err)
			}
			if err := invRepo.Save(ctx, inv); err != nil {
				t.Fatal(err)
			}

			jobRepo, err := database.NewCharacterJobRepository(db)
			if err != nil {
				t.Fatal(err)
			}
			initialJob, err := corejob.NewCharacterJob(char.ID, "job-01")
			if err != nil {
				t.Fatal(err)
			}
			// Master job-03 and job-04 so ExchangeJob(job-03, job-04, itemID) succeeds
			initialJob.RecordMastery("job-03", 80, 80)
			initialJob.RecordMastery("job-04", 100, 100)
			if err := jobRepo.Save(ctx, initialJob); err != nil {
				t.Fatal(err)
			}

			txProvider := database.NewTransactionProvider(db)
			ecoSvcExchange, err := economy.NewService(charRepo, invRepo, economy.WithTransactionProvider(txProvider))
			if err != nil {
				t.Fatal(err)
			}

			svcExchange, err := job.NewService(
				jobRepo,
				job.WithCharacterRepository(charRepo),
				job.WithInventoryRepository(invRepo),
				job.WithEconomy(ecoSvcExchange),
			)
			if err != nil {
				t.Fatal(err)
			}

			// Wrap charRepo for ChangeJob so that right after preflight FindByID finishes,
			// ExchangeJob commits on svcExchange.
			preflightDone := make(chan struct{})
			exchangeCommitted := make(chan struct{})

			interceptingChar := &interceptorCharRepo{
				inner: charRepo,
				onFindByID: func() {
					close(preflightDone)
					<-exchangeCommitted
				},
			}

			ecoSvcChange, err := economy.NewService(charRepo, invRepo, economy.WithTransactionProvider(txProvider))
			if err != nil {
				t.Fatal(err)
			}

			svcChange, err := job.NewService(
				jobRepo,
				job.WithCharacterRepository(interceptingChar),
				job.WithInventoryRepository(invRepo),
				job.WithEconomy(ecoSvcChange),
			)
			if err != nil {
				t.Fatal(err)
			}

			var changeErr error
			var wg sync.WaitGroup
			wg.Add(2)

			// Goroutine 1: ChangeJob preflights, pauses, then resumes under lock
			go func() {
				defer wg.Done()
				_, _, changeErr = svcChange.ChangeJob(ctx, char.ID, "job-01")
			}()

			// Goroutine 2: ExchangeJob executes after ChangeJob preflight
			var exchangeErr error
			go func() {
				defer wg.Done()
				<-preflightDone
				_, _, exchangeErr = svcExchange.ExchangeJob(ctx, char.ID, "job-03", "job-04", tc.itemID)
				close(exchangeCommitted)
			}()

			wg.Wait()

			if exchangeErr != nil {
				t.Fatalf("ExchangeJob failed: %v", exchangeErr)
			}

			if !errors.Is(changeErr, corejob.ErrJobUnavailable) {
				t.Fatalf("expected ChangeJob to fail with ErrJobUnavailable, got %v", changeErr)
			}

			// Assert MariaDB state:
			// 1. Character state reflects ExchangeJob, NOT ChangeJob (Level 50 preserved, NOT reduced to 1)
			finalChar, err := charRepo.FindByID(ctx, char.ID)
			if err != nil {
				t.Fatal(err)
			}
			if finalChar.Level != 50 {
				t.Fatalf("expected Level=50 preserved, got %d", finalChar.Level)
			}
			if finalChar.JobID != "job-03" {
				t.Fatalf("expected JobID=job-03 from ExchangeJob, got %s", finalChar.JobID)
			}
			if finalChar.OldJobID != "job-04" {
				t.Fatalf("expected OldJobID=job-04 from ExchangeJob, got %s", finalChar.OldJobID)
			}
			if finalChar.JobMemory == nil {
				t.Fatal("expected JobMemory to be non-nil from ExchangeJob")
			}
			if finalChar.JobMemory.Kind != tc.expectedKind {
				t.Fatalf("expected JobMemory.Kind=%s, got %s", tc.expectedKind, finalChar.JobMemory.Kind)
			}
			if finalChar.JobMemory.JobID != "job-01" || finalChar.JobMemory.OldJobID != "job-02" {
				t.Fatalf("expected JobMemory remembered job-01/job-02, got %+v", finalChar.JobMemory)
			}

			// 2. Inventory: item consumed exactly once by ExchangeJob (quantity = 0)
			finalInv, err := invRepo.FindByCharacterID(ctx, char.ID)
			if err != nil {
				t.Fatal(err)
			}
			if finalInv.Quantity(tc.itemID) != 0 {
				t.Fatalf("expected item quantity=0, got %d", finalInv.Quantity(tc.itemID))
			}

			// 3. CharacterJobs: CurrentJobID must be job-03
			finalJob, err := jobRepo.FindByCharacterID(ctx, char.ID)
			if err != nil {
				t.Fatal(err)
			}
			if finalJob.CurrentJobID != "job-03" {
				t.Fatalf("expected character_jobs.CurrentJobID=job-03, got %s", finalJob.CurrentJobID)
			}
		})
	}
}
