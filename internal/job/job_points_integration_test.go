package job_test

import (
	"context"
	"errors"
	"os"
	"testing"

	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
	corejob "github.com/witchcraze/party2re/internal/core/job"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/economy"
	"github.com/witchcraze/party2re/internal/job"
)

type failAfterJobPointsSave struct {
	job.Repository
	err error
}

func (r failAfterJobPointsSave) Save(ctx context.Context, state corejob.CharacterJob) error {
	if err := r.Repository.Save(ctx, state); err != nil {
		return err
	}
	return r.err
}

func TestFutureMemoryUnmasteredSPDatabaseRoundTripAndRollback(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	for _, tc := range []struct{ recorded, fail bool }{{true, false}, {true, true}, {false, false}} {
		fail := tc.fail
		db, err := database.OpenFromEnvironment()
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		ctx := context.Background()
		char, err := database.CreateTestCharacter(ctx, db, "Job Points")
		if err != nil {
			t.Fatal(err)
		}
		char.JobID, char.OldJobID, char.SP, char.OldSP, char.Level = "job-01", "job-02", 17, 0, 50
		chars, err := database.NewCharacterRepository(db)
		if err != nil {
			t.Fatal(err)
		}
		if err := chars.Update(ctx, char); err != nil {
			t.Fatal(err)
		}
		jobs, err := database.NewCharacterJobRepository(db)
		if err != nil {
			t.Fatal(err)
		}
		state, err := corejob.NewCharacterJob(char.ID, char.JobID)
		if err != nil {
			t.Fatal(err)
		}
		state.RecordedJobSP = map[string]int{"job-01": 1, "job-02": 2}
		if tc.recorded {
			state.History = []corejob.Change{{FromJobID: "job-01", ToJobID: "job-02"}, {FromJobID: "job-02", ToJobID: "job-01"}}
		}
		if err := jobs.Save(ctx, state); err != nil {
			t.Fatal(err)
		}
		inventories, err := database.NewInventoryRepository(db)
		if err != nil {
			t.Fatal(err)
		}
		inv, err := coreinventory.New(char.ID)
		if err != nil {
			t.Fatal(err)
		}
		fragment, err := item.NewInstance("item-207", 1)
		if err != nil {
			t.Fatal(err)
		}
		if err := inv.Add(fragment); err != nil {
			t.Fatal(err)
		}
		if err := inventories.Save(ctx, inv); err != nil {
			t.Fatal(err)
		}
		memories, err := database.NewFutureMemoryRepository(db)
		if err != nil {
			t.Fatal(err)
		}
		runner, err := economy.NewService(chars, inventories, economy.WithTransactionProvider(database.NewTransactionProvider(db)))
		if err != nil {
			t.Fatal(err)
		}
		var repo job.Repository = jobs
		sentinel := errors.New("job points save failure")
		if fail {
			repo = failAfterJobPointsSave{Repository: jobs, err: sentinel}
		}
		svc, err := job.NewService(repo, job.WithCharacterRepository(chars), job.WithInventoryRepository(inventories), job.WithEconomy(runner), job.WithFutureMemoryRepository(memories))
		if err != nil {
			t.Fatal(err)
		}
		snapshot, saveErr := svc.SaveFutureMemory(ctx, char.ID)
		loaded, err := jobs.FindByCharacterID(ctx, char.ID)
		if err != nil {
			t.Fatal(err)
		}
		inv, err = inventories.FindByCharacterID(ctx, char.ID)
		if err != nil {
			t.Fatal(err)
		}
		if fail {
			stored, err := memories.FindByCharacterID(ctx, char.ID)
			if err != nil || !errors.Is(saveErr, sentinel) || len(stored) != 0 || inv.Quantity("item-207") != 1 || loaded.RecordedJobSP["job-01"] != 1 || loaded.RecordedJobSP["job-02"] != 2 {
				t.Fatal(stored, err, saveErr, inv, loaded)
			}
			continue
		}
		zero, known := loaded.SavedSP("job-02")
		wantSP, wantOldSP := 1, 2
		if tc.recorded {
			wantSP, wantOldSP = 17, 0
		}
		if saveErr != nil || loaded.RecordedJobSP["job-01"] != wantSP || !known || zero != wantOldSP || loaded.IsMastered("job-01") || loaded.IsMastered("job-02") || loaded.HasRecordedJob("job-01") != tc.recorded || loaded.HasRecordedJob("job-02") != tc.recorded || inv.Quantity("item-207") != 0 {
			t.Fatal(saveErr, loaded, inv)
		}
		char.JobID, char.OldJobID, char.SP, char.OldSP = "job-03", "job-04", 7, 9
		if err := chars.Update(ctx, char); err != nil {
			t.Fatal(err)
		}
		recalled, _, err := svc.RecallFutureMemory(ctx, char.ID, snapshot.ID)
		if !tc.recorded {
			wantSP, wantOldSP = 7, 9
		}
		if err != nil || recalled.SP != wantSP || recalled.OldSP != wantOldSP || recalled.JobID != "job-01" || recalled.OldJobID != "job-02" {
			t.Fatal(recalled, err)
		}
	}
}

func TestChangeJobRecordsPresenceAcrossDatabaseReload(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
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
	runner, err := economy.NewService(chars, inventories, economy.WithTransactionProvider(database.NewTransactionProvider(db)))
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := corejob.NewCatalog([]corejob.Definition{{ID: "job-01", Name: "First", CMPTier: 1, MasterySP: 80}, {ID: "job-02", Name: "Second", CMPTier: 1, MasterySP: 80}})
	if err != nil {
		t.Fatal(err)
	}
	svc, err := job.NewService(jobs, job.WithCatalog(catalog), job.WithCharacterRepository(chars), job.WithInventoryRepository(inventories), job.WithEconomy(runner))
	if err != nil {
		t.Fatal(err)
	}
	for _, sameJob := range []bool{false, true} {
		char, err := database.CreateTestCharacter(ctx, db, "Record Presence")
		if err != nil {
			t.Fatal(err)
		}
		char.JobID, char.OldJobID, char.SP, char.Level = "job-01", "", 0, 20
		if err := chars.Update(ctx, char); err != nil {
			t.Fatal(err)
		}
		state, err := corejob.NewCharacterJob(char.ID, char.JobID)
		if err != nil {
			t.Fatal(err)
		}
		state.RecordedJobSP = map[string]int{"job-01": 0} // Migration-style preservation, not a record.
		if err := jobs.Save(ctx, state); err != nil {
			t.Fatal(err)
		}
		state, err = jobs.FindByCharacterID(ctx, char.ID)
		if err != nil || state.HasRecordedJob("job-01") {
			t.Fatal(state, err)
		}
		target := "job-02"
		if sameJob {
			target = "job-01"
		}
		changed, _, err := svc.ChangeJob(ctx, char.ID, target)
		if err != nil {
			t.Fatal(err)
		}
		state, err = jobs.FindByCharacterID(ctx, char.ID)
		if err != nil || !state.HasRecordedJob("job-01") || state.RecordedJobSP["job-01"] != 0 || state.IsMastered("job-01") {
			t.Fatal(state, err)
		}
		if sameJob {
			if len(state.History) != 0 {
				t.Fatal("invented a cross-job transition", state)
			}
			// A preservation-only partial save cannot erase a confirmed record.
			if err := jobs.Save(ctx, corejob.CharacterJob{CharacterID: char.ID, CurrentJobID: char.JobID, RecordedJobSP: map[string]int{"job-01": 0}}); err != nil {
				t.Fatal(err)
			}
		} else {
			changed.Level, changed.SP = 20, 7
			if err := chars.Update(ctx, changed); err != nil {
				t.Fatal(err)
			}
			if _, _, err := svc.ChangeJob(ctx, char.ID, "job-01"); err != nil {
				t.Fatal(err)
			}
		}
		state, err = jobs.FindByCharacterID(ctx, char.ID)
		if err != nil || state.CurrentJobID != "job-01" || !state.HasRecordedJob("job-01") || state.RecordedJobSP["job-01"] != 0 {
			t.Fatal(state, err)
		}
	}
}
