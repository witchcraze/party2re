package job_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	corejob "github.com/witchcraze/party2re/internal/core/job"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/economy"
	"github.com/witchcraze/party2re/internal/job"
	"github.com/witchcraze/party2re/internal/testutil"
)

type interceptorCharRepo struct {
	inner      job.CharacterRepository
	onFindByID func()
}

func (r *interceptorCharRepo) FindByID(ctx context.Context, id string) (corecharacter.Character, error) {
	c, err := r.inner.FindByID(ctx, id)
	if err != nil {
		return c, err
	}
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
		cancelAt     string
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
		{
			name:     "canceled before preflight",
			itemID:   "item-168",
			cancelAt: "before preflight",
		},
		{
			name:     "canceled after preflight",
			itemID:   "item-168",
			cancelAt: "after preflight",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			db := database.OpenTestDB(t)

			charRepo, err := database.NewCharacterRepository(db)
			if err != nil {
				t.Fatal(err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
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
					if tc.cancelAt == "after preflight" {
						cancel()
					}
					close(preflightDone)
					select {
					case <-exchangeCommitted:
					case <-ctx.Done():
					}
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

			if tc.cancelAt == "before preflight" {
				cancel()
			}

			changeErr, exchangeErr := testutil.RunRace2(
				func() error {
					_, _, err := svcChange.ChangeJob(ctx, char.ID, "job-01")
					return err
				},
				func() error {
					defer close(exchangeCommitted)
					select {
					case <-preflightDone:
					case <-ctx.Done():
						return ctx.Err()
					}
					_, _, err := svcExchange.ExchangeJob(ctx, char.ID, "job-03", "job-04", tc.itemID)
					return err
				},
			)

			if tc.cancelAt != "" {
				if !errors.Is(changeErr, context.Canceled) || !errors.Is(exchangeErr, context.Canceled) {
					t.Fatalf("expected both workers canceled, got ChangeJob=%v ExchangeJob=%v", changeErr, exchangeErr)
				}
				return
			}

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

func TestChangeJob_Integration_RevalidatePrerequisitesUnderLock(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	setupFixture := func(t *testing.T, prefix string, jobID, oldJobID string, masteredJobID string, items []coreitem.Instance, equipArmor bool) (
		*database.CharacterRepository,
		*database.InventoryRepository,
		*database.EquipmentRepository,
		*database.CharacterJobRepository,
		*database.CharacterMemoryRepository,
		*economy.Service,
		corecharacter.Character,
	) {
		t.Helper()
		db := database.OpenTestDB(t)
		ctx := context.Background()

		charRepo, err := database.NewCharacterRepository(db)
		if err != nil {
			t.Fatal(err)
		}
		invRepo, err := database.NewInventoryRepository(db)
		if err != nil {
			t.Fatal(err)
		}
		equipRepo, err := database.NewEquipmentRepository(db)
		if err != nil {
			t.Fatal(err)
		}
		jobRepo, err := database.NewCharacterJobRepository(db)
		if err != nil {
			t.Fatal(err)
		}
		memRepo, err := database.NewCharacterMemoryRepository(db)
		if err != nil {
			t.Fatal(err)
		}
		txProvider := database.NewTransactionProvider(db)
		ecoSvc, err := economy.NewService(charRepo, invRepo, economy.WithTransactionProvider(txProvider))
		if err != nil {
			t.Fatal(err)
		}

		char, err := database.CreateTestCharacter(ctx, db, prefix)
		if err != nil {
			t.Fatal(err)
		}
		database.RegisterCharacterCleanup(t, db, char)

		char.JobID = jobID
		char.OldJobID = oldJobID
		char.Level = 50
		char.SP = 40
		char.OldSP = 20
		char.Gender = "male"
		char.CasinoWins = 10
		if err := charRepo.Update(ctx, char); err != nil {
			t.Fatal(err)
		}

		inv, err := coreinventory.New(char.ID)
		if err != nil {
			t.Fatal(err)
		}
		var armorInstID string
		for _, inst := range items {
			if err := inv.Add(inst); err != nil {
				t.Fatal(err)
			}
			if equipArmor && inst.DefinitionID == "armor-29" {
				armorInstID = inst.ID
			}
		}
		if err := invRepo.Save(ctx, inv); err != nil {
			t.Fatal(err)
		}
		if armorInstID != "" {
			equip, err := coreequipment.New(char.ID)
			if err != nil {
				t.Fatal(err)
			}
			equip.Slots[coreitem.SlotBody] = armorInstID
			if err := equipRepo.Save(ctx, equip); err != nil {
				t.Fatal(err)
			}
		}

		state, err := corejob.NewCharacterJob(char.ID, jobID)
		if err != nil {
			t.Fatal(err)
		}
		if masteredJobID != "" {
			state.RecordMastery(masteredJobID, 140, 140)
		}
		if err := jobRepo.Save(ctx, state); err != nil {
			t.Fatal(err)
		}

		return charRepo, invRepo, equipRepo, jobRepo, memRepo, ecoSvc, char
	}

	itemLossCases := []struct {
		name         string
		startJobID   string
		startOldJob  string
		masteredJob  string
		targetJobID  string
		requiredItem string
	}{
		{
			name:         "job08_to_job46_concurrent_item_loss_rejected",
			startJobID:   "job-08",
			startOldJob:  "job-01",
			targetJobID:  "job-46",
			requiredItem: "item-039",
		},
		{
			name:         "job01_to_job46_concurrent_item_loss_rejected",
			startJobID:   "job-01",
			startOldJob:  "job-02",
			targetJobID:  "job-46",
			requiredItem: "item-039",
		},
		{
			name:         "mastered_job46_without_current_or_old_still_requires_item_on_concurrent_loss",
			startJobID:   "job-01",
			startOldJob:  "job-02",
			masteredJob:  "job-46",
			targetJobID:  "job-46",
			requiredItem: "item-039",
		},
	}

	for _, tc := range itemLossCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			inst, err := coreitem.NewInstance(tc.requiredItem, 1)
			if err != nil {
				t.Fatal(err)
			}
			charRepo, invRepo, _, jobRepo, memRepo, ecoSvc, char := setupFixture(
				t, "PrereqItemLoss", tc.startJobID, tc.startOldJob, tc.masteredJob, []coreitem.Instance{inst}, false,
			)

			preflightDone := make(chan struct{})
			mutationDone := make(chan struct{})

			svcA, err := job.NewService(
				jobRepo,
				job.WithCharacterRepository(&interceptorCharRepo{
					inner: charRepo,
					onFindByID: func() {
						close(preflightDone)
						select {
						case <-mutationDone:
						case <-ctx.Done():
						}
					},
				}),
				job.WithInventoryRepository(invRepo),
				job.WithEconomy(ecoSvc),
				job.WithMemoryRecorder(memRepo.AppendMemory),
			)
			if err != nil {
				t.Fatal(err)
			}
			svcB, err := job.NewService(
				jobRepo,
				job.WithCharacterRepository(charRepo),
				job.WithInventoryRepository(invRepo),
				job.WithEconomy(ecoSvc),
				job.WithMemoryRecorder(memRepo.AppendMemory),
			)
			if err != nil {
				t.Fatal(err)
			}

			var controlErr error
			changeAErr, consumeErr := testutil.RunRace2(
				func() error {
					_, _, err := svcA.ChangeJob(ctx, char.ID, tc.targetJobID)
					return err
				},
				func() error {
					defer close(mutationDone)
					select {
					case <-preflightDone:
					case <-ctx.Done():
						return ctx.Err()
					}
					_, err := ecoSvc.ExecuteTransaction(ctx, economy.TransactionRequest{
						CharacterID: char.ID,
						Cost: economy.ResourceCost{
							ItemDefinitionID:  tc.requiredItem,
							ItemDefinitionQty: 1,
						},
					}, nil)
					if err != nil {
						return err
					}
					_, _, controlErr = svcB.ChangeJob(ctx, char.ID, tc.targetJobID)
					return nil
				},
			)

			if consumeErr != nil {
				t.Fatalf("concurrent item consume failed: %v", consumeErr)
			}
			if !errors.Is(controlErr, job.ErrRequiredItem) {
				t.Fatalf("expected fresh request control check to reject with ErrRequiredItem, got %v", controlErr)
			}
			if !errors.Is(changeAErr, job.ErrRequiredItem) {
				t.Fatalf("expected stalled ChangeJob A to reject with ErrRequiredItem, got %v", changeAErr)
			}

			finalChar, err := charRepo.FindByID(ctx, char.ID)
			if err != nil {
				t.Fatal(err)
			}
			if finalChar.JobID != tc.startJobID || finalChar.OldJobID != tc.startOldJob || finalChar.SP != 40 || finalChar.OldSP != 20 || finalChar.Level != 50 {
				t.Fatalf("expected character state untouched, got %+v", finalChar)
			}
			finalInv, err := invRepo.FindByCharacterID(ctx, char.ID)
			if err != nil {
				t.Fatal(err)
			}
			if finalInv.Quantity(tc.requiredItem) != 0 {
				t.Fatalf("expected %s quantity=0, got %d", tc.requiredItem, finalInv.Quantity(tc.requiredItem))
			}
			finalJob, err := jobRepo.FindByCharacterID(ctx, char.ID)
			if err != nil {
				t.Fatal(err)
			}
			if finalJob.CurrentJobID != tc.startJobID || len(finalJob.History) != 0 {
				t.Fatalf("expected job state untouched, got %+v", finalJob)
			}
			mems, err := memRepo.ListMemories(ctx, char.ID, 10)
			if err != nil {
				t.Fatal(err)
			}
			if len(mems) != 0 {
				t.Fatalf("expected 0 memories recorded on rejection, got %d", len(mems))
			}
		})
	}

	t.Run("job08_to_job46_concurrent_job_change_consumes_item", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		inst, err := coreitem.NewInstance("item-039", 1)
		if err != nil {
			t.Fatal(err)
		}
		charRepo, invRepo, _, jobRepo, memRepo, ecoSvc, char := setupFixture(
			t, "PrereqJobShift", "job-08", "job-01", "", []coreitem.Instance{inst}, false,
		)

		preflightDone := make(chan struct{})
		mutationDone := make(chan struct{})

		svcA, err := job.NewService(
			jobRepo,
			job.WithCharacterRepository(&interceptorCharRepo{
				inner: charRepo,
				onFindByID: func() {
					close(preflightDone)
					select {
					case <-mutationDone:
					case <-ctx.Done():
					}
				},
			}),
			job.WithInventoryRepository(invRepo),
			job.WithEconomy(ecoSvc),
			job.WithMemoryRecorder(memRepo.AppendMemory),
		)
		if err != nil {
			t.Fatal(err)
		}

		changeAErr, shiftErr := testutil.RunRace2(
			func() error {
				_, _, err := svcA.ChangeJob(ctx, char.ID, "job-46")
				return err
			},
			func() error {
				defer close(mutationDone)
				select {
				case <-preflightDone:
				case <-ctx.Done():
					return ctx.Err()
				}
				_, err := ecoSvc.ExecuteTransaction(ctx, economy.TransactionRequest{
					CharacterID: char.ID,
				}, func(tc *economy.TxContext) error {
					tc.Character.JobID = "job-01"
					tc.Character.OldJobID = "job-02"
					state, err := jobRepo.FindByCharacterID(tc.Context, char.ID)
					if err != nil {
						return err
					}
					state.RestoreCurrentJob("job-01")
					return jobRepo.Save(tc.Context, state)
				})
				return err
			},
		)

		if shiftErr != nil {
			t.Fatalf("concurrent job shift failed: %v", shiftErr)
		}
		if changeAErr != nil {
			t.Fatalf("expected ChangeJob A to succeed and consume item-039, got %v", changeAErr)
		}

		finalChar, err := charRepo.FindByID(ctx, char.ID)
		if err != nil {
			t.Fatal(err)
		}
		if finalChar.JobID != "job-46" || finalChar.OldJobID != "job-01" {
			t.Fatalf("expected JobID=job-46 OldJobID=job-01, got %+v", finalChar)
		}
		finalInv, err := invRepo.FindByCharacterID(ctx, char.ID)
		if err != nil {
			t.Fatal(err)
		}
		if finalInv.Quantity("item-039") != 0 {
			t.Fatalf("expected item-039 consumed when locked job shifted out of job-08, got %d", finalInv.Quantity("item-039"))
		}
		mems, err := memRepo.ListMemories(ctx, char.ID, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(mems) != 1 {
			t.Fatalf("expected 1 job-change memory, got %d", len(mems))
		}
	})

	t.Run("job01_to_job84_concurrent_armor_unequip_rejected", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		armor, err := coreitem.NewInstance("armor-29", 1)
		if err != nil {
			t.Fatal(err)
		}
		charRepo, invRepo, equipRepo, jobRepo, memRepo, ecoSvc, char := setupFixture(
			t, "PrereqArmorUnequip", "job-01", "job-04", "", []coreitem.Instance{armor}, true,
		)

		preflightDone := make(chan struct{})
		mutationDone := make(chan struct{})

		svcA, err := job.NewService(
			jobRepo,
			job.WithCharacterRepository(&interceptorCharRepo{
				inner: charRepo,
				onFindByID: func() {
					close(preflightDone)
					select {
					case <-mutationDone:
					case <-ctx.Done():
					}
				},
			}),
			job.WithInventoryRepository(invRepo),
			job.WithEquipmentRepository(equipRepo),
			job.WithEconomy(ecoSvc),
			job.WithMemoryRecorder(memRepo.AppendMemory),
		)
		if err != nil {
			t.Fatal(err)
		}
		svcB, err := job.NewService(
			jobRepo,
			job.WithCharacterRepository(charRepo),
			job.WithInventoryRepository(invRepo),
			job.WithEquipmentRepository(equipRepo),
			job.WithEconomy(ecoSvc),
			job.WithMemoryRecorder(memRepo.AppendMemory),
		)
		if err != nil {
			t.Fatal(err)
		}

		var controlErr error
		changeAErr, unequipErr := testutil.RunRace2(
			func() error {
				_, _, err := svcA.ChangeJob(ctx, char.ID, "job-84")
				return err
			},
			func() error {
				defer close(mutationDone)
				select {
				case <-preflightDone:
				case <-ctx.Done():
					return ctx.Err()
				}
				_, err := ecoSvc.ExecuteTransaction(ctx, economy.TransactionRequest{
					CharacterID: char.ID,
				}, func(tc *economy.TxContext) error {
					eq, err := equipRepo.FindByCharacterIDForUpdate(tc.Context, char.ID)
					if err != nil {
						return err
					}
					if _, err := eq.Unequip(coreitem.SlotBody); err != nil {
						return err
					}
					return equipRepo.Save(tc.Context, eq)
				})
				if err != nil {
					return err
				}
				_, _, controlErr = svcB.ChangeJob(ctx, char.ID, "job-84")
				return nil
			},
		)

		if unequipErr != nil {
			t.Fatalf("concurrent unequip failed: %v", unequipErr)
		}
		if !errors.Is(controlErr, job.ErrRequiredArmor) {
			t.Fatalf("expected fresh control ChangeJob to reject with ErrRequiredArmor, got %v", controlErr)
		}
		if !errors.Is(changeAErr, job.ErrRequiredArmor) {
			t.Fatalf("expected stalled ChangeJob A to reject with ErrRequiredArmor, got %v", changeAErr)
		}

		finalChar, err := charRepo.FindByID(ctx, char.ID)
		if err != nil {
			t.Fatal(err)
		}
		if finalChar.JobID != "job-01" || finalChar.OldJobID != "job-04" || finalChar.Level != 50 {
			t.Fatalf("expected character untouched, got %+v", finalChar)
		}
		finalInv, err := invRepo.FindByCharacterID(ctx, char.ID)
		if err != nil {
			t.Fatal(err)
		}
		if finalInv.Quantity("armor-29") != 1 {
			t.Fatalf("expected armor-29 unconsumed (qty=1), got %d", finalInv.Quantity("armor-29"))
		}
		finalJob, err := jobRepo.FindByCharacterID(ctx, char.ID)
		if err != nil {
			t.Fatal(err)
		}
		if finalJob.CurrentJobID != "job-01" || len(finalJob.History) != 0 {
			t.Fatalf("expected job state untouched, got %+v", finalJob)
		}
		mems, err := memRepo.ListMemories(ctx, char.ID, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(mems) != 0 {
			t.Fatalf("expected 0 memories, got %d", len(mems))
		}
	})

	t.Run("job01_to_job84_equipped_armor_consumed_in_real_sql", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		armor, err := coreitem.NewInstance("armor-29", 1)
		if err != nil {
			t.Fatal(err)
		}
		charRepo, invRepo, equipRepo, jobRepo, memRepo, ecoSvc, char := setupFixture(
			t, "ArmorConsumeOK", "job-01", "job-04", "", []coreitem.Instance{armor}, true,
		)
		svc, err := job.NewService(
			jobRepo,
			job.WithCharacterRepository(charRepo),
			job.WithInventoryRepository(invRepo),
			job.WithEquipmentRepository(equipRepo),
			job.WithEconomy(ecoSvc),
			job.WithMemoryRecorder(memRepo.AppendMemory),
		)
		if err != nil {
			t.Fatal(err)
		}

		updatedChar, updatedState, err := svc.ChangeJob(ctx, char.ID, "job-84")
		if err != nil {
			t.Fatalf("expected ChangeJob to job-84 to succeed, got %v", err)
		}
		if updatedChar.JobID != "job-84" || updatedState.CurrentJobID != "job-84" {
			t.Fatalf("expected job-84, got char=%s state=%s", updatedChar.JobID, updatedState.CurrentJobID)
		}
		finalEquip, err := equipRepo.FindByCharacterID(ctx, char.ID)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := finalEquip.Equipped(coreitem.SlotBody); ok {
			t.Fatal("expected body slot empty after job-84 change")
		}
		finalInv, err := invRepo.FindByCharacterID(ctx, char.ID)
		if err != nil {
			t.Fatal(err)
		}
		if finalInv.Quantity("armor-29") != 0 {
			t.Fatalf("expected armor-29 consumed, got %d", finalInv.Quantity("armor-29"))
		}
		mems, err := memRepo.ListMemories(ctx, char.ID, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(mems) != 1 {
			t.Fatalf("expected 1 job-change memory, got %d", len(mems))
		}
	})
}

func TestChangeJob_Integration_ConcurrentItemContentionStress(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db := database.OpenTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	invRepo, err := database.NewInventoryRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	jobRepo, err := database.NewCharacterJobRepository(db)
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
	)
	if err != nil {
		t.Fatal(err)
	}

	cfg := testutil.GetStressConfig()
	chars := make([]corecharacter.Character, cfg.Workers)
	for w := 0; w < cfg.Workers; w++ {
		char, err := database.CreateTestCharacter(ctx, db, fmt.Sprintf("JobStress_%d", w))
		if err != nil {
			t.Fatal(err)
		}
		database.RegisterCharacterCleanup(t, db, char)
		chars[w] = char
	}

	var totalChanged int64
	var totalRejected int64

	res := testutil.RunConcurrentStressTest(t, cfg, func(workerID, op int) error {
		char := chars[workerID]
		startJob := "job-08"
		if op%2 == 1 {
			startJob = "job-01"
		}

		// Reset worker's isolated character and inventory for this round.
		char.JobID = startJob
		char.OldJobID = "job-02"
		char.Level = 50
		char.SP = 30
		char.OldSP = 10
		char.Gender = "male"
		char.CasinoWins = 10
		if err := charRepo.Update(ctx, char); err != nil {
			return err
		}
		state, err := corejob.NewCharacterJob(char.ID, startJob)
		if err != nil {
			return err
		}
		if err := jobRepo.Save(ctx, state); err != nil {
			return err
		}
		inv, err := coreinventory.New(char.ID)
		if err != nil {
			return err
		}
		dice, err := coreitem.NewInstance("item-039", 1)
		if err != nil {
			return err
		}
		if err := inv.Add(dice); err != nil {
			return err
		}
		if err := invRepo.Save(ctx, inv); err != nil {
			return err
		}

		changeErr, consumeErr := testutil.RunRace2(
			func() error {
				_, _, err := svc.ChangeJob(ctx, char.ID, "job-46")
				return err
			},
			func() error {
				_, err := ecoSvc.ExecuteTransaction(ctx, economy.TransactionRequest{
					CharacterID: char.ID,
					Cost: economy.ResourceCost{
						ItemDefinitionID:  "item-039",
						ItemDefinitionQty: 1,
					},
				}, nil)
				return err
			},
		)

		if testutil.IsDeadlockError(changeErr) {
			return changeErr
		}
		if testutil.IsDeadlockError(consumeErr) {
			return consumeErr
		}
		if changeErr != nil && !errors.Is(changeErr, job.ErrRequiredItem) {
			return fmt.Errorf("unexpected ChangeJob error: %w", changeErr)
		}
		if consumeErr != nil && !errors.Is(consumeErr, economy.ErrInsufficientItemQuantity) {
			return fmt.Errorf("unexpected Consume error: %w", consumeErr)
		}

		if startJob == "job-01" && changeErr == nil && consumeErr == nil {
			return fmt.Errorf("both consuming ChangeJob(job-01->job-46) and item consume succeeded for single item-039")
		}
		if changeErr == nil {
			atomic.AddInt64(&totalChanged, 1)
		} else {
			atomic.AddInt64(&totalRejected, 1)
		}

		finalChar, err := charRepo.FindByID(ctx, char.ID)
		if err != nil {
			return err
		}
		finalInv, err := invRepo.FindByCharacterID(ctx, char.ID)
		if err != nil {
			return err
		}
		if finalInv.Quantity("item-039") != 0 {
			return fmt.Errorf("expected item-039 consumed (qty=0), got %d", finalInv.Quantity("item-039"))
		}
		if (finalChar.JobID == "job-46") != (changeErr == nil) {
			return fmt.Errorf("persisted JobID=%s inconsistent with changeErr=%v", finalChar.JobID, changeErr)
		}
		if changeErr != nil && finalChar.JobID != startJob {
			return fmt.Errorf("rejected ChangeJob mutated JobID to %s (want %s)", finalChar.JobID, startJob)
		}
		return nil
	})

	if res.Failures > 0 {
		t.Fatalf("stress test had %d unexpected failures", res.Failures)
	}
	t.Logf("stress completed: totalOps=%d changed=%d rejected=%d duration=%v",
		res.TotalOps, totalChanged, totalRejected, res.Duration)
}
