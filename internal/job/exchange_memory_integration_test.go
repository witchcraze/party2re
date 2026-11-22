package job_test

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/item"
	corejob "github.com/witchcraze/party2re/internal/core/job"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/economy"
	"github.com/witchcraze/party2re/internal/job"
	"github.com/witchcraze/party2re/internal/testutil"
)

func TestExchangeJobTrajectoryDatabaseSnapshotsAndRollback(t *testing.T) {
	db := database.OpenTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	chars, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	jobs, err := database.NewCharacterJobRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	inventories, err := database.NewInventoryRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	memories, err := database.NewCharacterMemoryRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	runner, err := economy.NewService(chars, inventories, economy.WithTransactionProvider(database.NewTransactionProvider(db)))
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := corejob.NewCatalog([]corejob.Definition{
		{ID: "job-01", Name: "First", CMPTier: 1, MasterySP: 80},
		{ID: "job-02", Name: "Second", CMPTier: 1, MasterySP: 80},
		{ID: "job-03", Name: "Third", CMPTier: 1, MasterySP: 80},
		{ID: "job-04", Name: "Fourth", CMPTier: 1, MasterySP: 80},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, itemID := range []string{"item-168", "item-243"} {
		t.Run(itemID, func(t *testing.T) {
			char, err := database.CreateTestCharacter(ctx, db, "")
			if err != nil {
				t.Fatal(err)
			}
			char.JobID, char.SP, char.OldJobID, char.OldSP, char.Level = "job-01", 50, "job-02", 40, 20
			if err := chars.Update(ctx, char); err != nil {
				t.Fatal(err)
			}
			state, err := corejob.NewCharacterJob(char.ID, char.JobID)
			if err != nil {
				t.Fatal(err)
			}
			state.RecordMastery("job-03", 80, 80)
			state.RecordMastery("job-04", 100, 80)
			if err := jobs.Save(ctx, state); err != nil {
				t.Fatal(err)
			}
			token, err := item.NewInstance(itemID, 1)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := database.CreateTestInventoryWithItems(ctx, db, char.ID, []item.Instance{token}); err != nil {
				t.Fatal(err)
			}
			failRecord := false
			sentinel := errors.New("failure after trajectory insert")
			svc, err := job.NewService(jobs, job.WithCatalog(catalog), job.WithCharacterRepository(chars),
				job.WithInventoryRepository(inventories), job.WithEconomy(runner), job.WithMemoryRecorder(
					func(ctx context.Context, id string, kind corecharacter.MemoryKind, content string) error {
						if err := memories.AppendMemory(ctx, id, kind, content); err != nil {
							return err
						}
						if failRecord {
							return sentinel
						}
						return nil
					}))
			if err != nil {
				t.Fatal(err)
			}
			assertRollback := func(run func() error) {
				t.Helper()
				beforeChar, err := chars.FindByID(ctx, char.ID)
				if err != nil {
					t.Fatal(err)
				}
				beforeJobs, err := jobs.FindByCharacterID(ctx, char.ID)
				if err != nil {
					t.Fatal(err)
				}
				beforeInv, err := inventories.FindByCharacterID(ctx, char.ID)
				if err != nil {
					t.Fatal(err)
				}
				beforeLog, err := memories.ListMemories(ctx, char.ID, 999)
				if err != nil {
					t.Fatal(err)
				}
				failRecord = true
				err = run()
				failRecord = false
				if !errors.Is(err, sentinel) {
					t.Fatalf("expected injected failure, got %v", err)
				}
				afterChar, err := chars.FindByID(ctx, char.ID)
				if err != nil {
					t.Fatal(err)
				}
				afterJobs, err := jobs.FindByCharacterID(ctx, char.ID)
				if err != nil {
					t.Fatal(err)
				}
				afterInv, err := inventories.FindByCharacterID(ctx, char.ID)
				if err != nil {
					t.Fatal(err)
				}
				afterLog, err := memories.ListMemories(ctx, char.ID, 999)
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(beforeChar, afterChar) || !reflect.DeepEqual(beforeJobs, afterJobs) ||
					!reflect.DeepEqual(beforeInv, afterInv) || !reflect.DeepEqual(beforeLog, afterLog) {
					t.Fatal("failed recording did not roll back character, jobs, inventory and trajectory")
				}
			}
			assertSnapshot := func(kind corecharacter.MemoryKind, jobID string, sp, count int, content string) {
				t.Helper()
				stored, err := chars.FindByID(ctx, char.ID)
				if err != nil {
					t.Fatal(err)
				}
				entries, err := memories.ListMemories(ctx, char.ID, 999)
				if err != nil {
					t.Fatal(err)
				}
				if len(entries) != count {
					t.Fatalf("trajectory count = %d, want %d", len(entries), count)
				}
				if m := entries[0]; m.Kind != kind || m.JobID != jobID || m.SP != sp || m.Content != content ||
					stored.JobID != jobID || stored.SP != sp {
					t.Fatalf("snapshot mismatch: entry=%+v character=%+v", m, stored)
				}
			}
			assertRollback(func() error { _, _, err := svc.ExchangeJob(ctx, char.ID, "job-03", "job-04"); return err })
			if _, _, err := svc.ExchangeJob(ctx, char.ID, "job-03", "job-04"); err != nil {
				t.Fatal(err)
			}
			assertSnapshot(corecharacter.MemoryJobRecall, "job-03", 80, 1, "ThirdとFourthの記憶を思い出す")
			stored, err := chars.FindByID(ctx, char.ID)
			if err != nil {
				t.Fatal(err)
			}
			if stored.OldJobID != "job-04" || stored.OldSP != 100 || stored.JobMemory == nil ||
				stored.JobMemory.JobID != "job-01" || stored.JobMemory.SP != 50 {
				t.Fatalf("recall state = %+v", stored)
			}
			inv, err := inventories.FindByCharacterID(ctx, char.ID)
			if err != nil || inv.Quantity(itemID) != 0 {
				t.Fatalf("recall item not consumed: inventory=%+v err=%v", inv, err)
			}
			if _, _, err := svc.ExchangeJob(ctx, char.ID, "job-03", "job-04"); !errors.Is(err, corejob.ErrJobUnavailable) {
				t.Fatalf("overlapping recall: %v", err)
			}
			if itemID == "item-243" {
				if _, _, err := svc.RestoreJobMemory(ctx, char.ID); !errors.Is(err, corejob.ErrJobUnavailable) {
					t.Fatalf("temporary manual revert: %v", err)
				}
				assertSnapshot(corecharacter.MemoryJobRecall, "job-03", 80, 1, "ThirdとFourthの記憶を思い出す")
				return
			}
			assertRollback(func() error { _, _, err := svc.RestoreJobMemory(ctx, char.ID); return err })
			if _, _, err := svc.RestoreJobMemory(ctx, char.ID); err != nil {
				t.Fatal(err)
			}
			assertSnapshot(corecharacter.MemoryJobRevert, "job-01", 50, 2, "FirstとSecondに戻る")
			stored, err = chars.FindByID(ctx, char.ID)
			if err != nil || stored.JobMemory != nil || stored.OldJobID != "job-02" || stored.OldSP != 40 {
				t.Fatalf("revert state=%+v err=%v", stored, err)
			}
		})
	}
	t.Run("ordinary change and mastery keep departing snapshot", func(t *testing.T) {
		char, err := database.CreateTestCharacter(ctx, db, "")
		if err != nil {
			t.Fatal(err)
		}
		char.JobID, char.SP, char.Level = "job-01", 90, 20
		if err := chars.Update(ctx, char); err != nil {
			t.Fatal(err)
		}
		svc, err := job.NewService(jobs, job.WithCatalog(catalog), job.WithCharacterRepository(chars),
			job.WithInventoryRepository(inventories), job.WithEconomy(runner), job.WithMemoryRecorder(memories.AppendMemory))
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := svc.ChangeJob(ctx, char.ID, "job-02"); err != nil {
			t.Fatal(err)
		}
		entries, err := memories.ListMemories(ctx, char.ID, 999)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 2 || entries[0].Kind != corecharacter.MemoryJobMaster || entries[1].Kind != corecharacter.MemoryJobChange {
			t.Fatalf("departure entries = %+v", entries)
		}
		for _, entry := range entries {
			if entry.JobID != "job-01" || entry.SP != 90 || entry.Level != 20 {
				t.Fatalf("departure snapshot = %+v", entry)
			}
		}
	})
}

func TestRestoreJobMemory_Integration_RevalidatesOverLevelUnderLock(t *testing.T) {
	db := database.OpenTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	chars, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	jobs, err := database.NewCharacterJobRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	inventories, err := database.NewInventoryRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	memories, err := database.NewCharacterMemoryRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	char, err := database.CreateTestCharacter(ctx, db, "RestoreOverLevelLock")
	if err != nil {
		t.Fatal(err)
	}
	database.RegisterCharacterCleanup(t, db, char)

	char.Level = 99
	char.JobID = "job-03"
	char.SP = 80
	char.OldJobID = "job-04"
	char.OldSP = 100
	char.OverLevel = false
	char.JobMemory = &corecharacter.JobMemory{
		Kind:     corecharacter.JobMemoryKindPersistent,
		JobID:    "job-01",
		SP:       50,
		OldJobID: "job-02",
		OldSP:    40,
	}
	if err := chars.Update(ctx, char); err != nil {
		t.Fatal(err)
	}

	state, err := corejob.NewCharacterJob(char.ID, char.JobID)
	if err != nil {
		t.Fatal(err)
	}
	if err := jobs.Save(ctx, state); err != nil {
		t.Fatal(err)
	}

	txProvider := database.NewTransactionProvider(db)
	ecoSvc, err := economy.NewService(chars, inventories, economy.WithTransactionProvider(txProvider))
	if err != nil {
		t.Fatal(err)
	}

	preflightDone := make(chan struct{})
	overLevelCommitted := make(chan struct{})
	var once sync.Once
	interceptingChar := &interceptorCharRepo{
		inner: chars,
		onFindByID: func() {
			once.Do(func() {
				close(preflightDone)
				select {
				case <-overLevelCommitted:
				case <-ctx.Done():
				}
			})
		},
	}

	svc, err := job.NewService(
		jobs,
		job.WithCharacterRepository(interceptingChar),
		job.WithInventoryRepository(inventories),
		job.WithEconomy(ecoSvc),
		job.WithMemoryRecorder(memories.AppendMemory),
	)
	if err != nil {
		t.Fatal(err)
	}

	restoreErr, limitBreakErr := testutil.RunRace2(
		func() error {
			_, _, err := svc.RestoreJobMemory(ctx, char.ID)
			return err
		},
		func() error {
			defer close(overLevelCommitted)
			select {
			case <-preflightDone:
			case <-ctx.Done():
				return ctx.Err()
			}
			_, err := ecoSvc.ExecuteTransaction(ctx, economy.TransactionRequest{CharacterID: char.ID}, func(tc *economy.TxContext) error {
				tc.Character.OverLevel = true
				return nil
			})
			return err
		},
	)

	if limitBreakErr != nil {
		t.Fatalf("concurrent OverLevel update failed: %v", limitBreakErr)
	}
	if !errors.Is(restoreErr, corejob.ErrJobUnavailable) {
		t.Fatalf("expected RestoreJobMemory to fail with ErrJobUnavailable, got %v", restoreErr)
	}

	persisted, err := chars.FindByID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !persisted.OverLevel || persisted.JobID != "job-03" || persisted.SP != 80 ||
		persisted.OldJobID != "job-04" || persisted.OldSP != 100 {
		t.Fatalf("unexpected persisted character state: %+v", persisted)
	}
	if !reflect.DeepEqual(persisted.JobMemory, char.JobMemory) {
		t.Fatalf("JobMemory modified on rejection: got %+v, want %+v", persisted.JobMemory, char.JobMemory)
	}

	persistedJob, err := jobs.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persistedJob.CurrentJobID != "job-03" {
		t.Fatalf("CurrentJobID = %s, want job-03", persistedJob.CurrentJobID)
	}

	entries, err := memories.ListMemories(ctx, char.ID, 999)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no trajectory entries on rejection, got %+v", entries)
	}
}
