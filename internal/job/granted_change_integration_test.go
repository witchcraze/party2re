package job_test

import (
	"context"
	"errors"
	"os"
	"slices"
	"testing"

	corejob "github.com/witchcraze/party2re/internal/core/job"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/economy"
	"github.com/witchcraze/party2re/internal/god"
	"github.com/witchcraze/party2re/internal/job"
)

type countingGuildAwarder struct{ calls int }

func (a *countingGuildAwarder) AddGuildPoints(context.Context, string, int) error {
	a.calls++
	return nil
}

func TestCelestialWishSynchronizesJobStateAcrossDatabaseReload(t *testing.T) {
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
	txProvider := database.NewTransactionProvider(db)
	runner, err := economy.NewService(chars, inventories, economy.WithTransactionProvider(txProvider))
	if err != nil {
		t.Fatal(err)
	}
	// job-70 is ineligible for a normal change: it requires itself.
	catalog, err := corejob.NewCatalog([]corejob.Definition{
		{ID: "job-01", Name: "First", CMPTier: 1, MasterySP: 80},
		{ID: "job-02", Name: "Second", CMPTier: 1, MasterySP: 80},
		{ID: "job-70", Name: "Celestial", CMPTier: 1, MasterySP: 80, RequiredJobIDs: []string{"job-70"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	sentinel := errors.New("job state save failure")

	for _, tc := range []struct {
		name         string
		sp, wantSP   int
		masterTarget bool
		wantMastered bool
		failJobSave  bool
	}{
		{name: "unmastered departure", sp: 30},
		{name: "mastery reached on departure", sp: 90, wantMastered: true},
		{name: "previously mastered target restores SP", sp: 10, masterTarget: true, wantSP: 150},
		{name: "job state failure rolls back character", sp: 30, failJobSave: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var repo job.Repository = jobs
			if tc.failJobSave {
				repo = failAfterJobPointsSave{Repository: jobs, err: sentinel}
			}
			guild := &countingGuildAwarder{}
			jobSvc, err := job.NewService(repo, job.WithCatalog(catalog), job.WithCharacterRepository(chars), job.WithInventoryRepository(inventories), job.WithEconomy(runner), job.WithGuildPointAwarder(guild))
			if err != nil {
				t.Fatal(err)
			}
			godSvc, err := god.NewService(chars, god.WithTransactionProvider(txProvider), god.WithJobChanger(jobSvc))
			if err != nil {
				t.Fatal(err)
			}

			char, err := database.CreateTestCharacter(ctx, db, "Celestial Sync")
			if err != nil {
				t.Fatal(err)
			}
			// Level 5 proves the normal Lv20 gate is not applied.
			char.JobID, char.OldJobID, char.SP, char.OldSP, char.Level = "job-01", "", tc.sp, 0, 5
			char.GodVisit = string(god.RealmHeaven)
			if err := chars.Update(ctx, char); err != nil {
				t.Fatal(err)
			}
			state, err := corejob.NewCharacterJob(char.ID, "job-01")
			if err != nil {
				t.Fatal(err)
			}
			if tc.masterTarget {
				state.RecordMastery("job-70", 150, 80)
				if err := state.RecordJobSP("job-70", 150); err != nil {
					t.Fatal(err)
				}
			}
			if err := jobs.Save(ctx, state); err != nil {
				t.Fatal(err)
			}

			_, wishErr := godSvc.GrantWish(ctx, char.ID, god.WishCelestialDragon, god.RealmHeaven)
			stored, err := chars.FindByID(ctx, char.ID)
			if err != nil {
				t.Fatal(err)
			}
			loaded, err := jobs.FindByCharacterID(ctx, char.ID)
			if err != nil {
				t.Fatal(err)
			}
			if tc.failJobSave {
				if !errors.Is(wishErr, sentinel) || stored.JobID != "job-01" || stored.SP != tc.sp || loaded.CurrentJobID != "job-01" || loaded.HasRecordedJob("job-01") {
					t.Fatalf("not rolled back: err=%v char=%+v state=%+v", wishErr, stored, loaded)
				}
				return
			}
			if wishErr != nil {
				t.Fatal(wishErr)
			}
			if stored.JobID != "job-70" || stored.OldJobID != "job-01" || stored.SP != tc.wantSP || stored.OldSP != tc.sp || loaded.CurrentJobID != "job-70" {
				t.Fatalf("job ids/SP out of sync: char=%+v state=%+v", stored, loaded)
			}
			if sp, ok := loaded.SavedSP("job-01"); !ok || sp != tc.sp || loaded.IsMastered("job-01") != tc.wantMastered {
				t.Fatalf("departure record: sp=%d ok=%v state=%+v", sp, ok, loaded)
			}
			if !slices.Contains(loaded.History, corejob.Change{FromJobID: "job-01", ToJobID: "job-70"}) || guild.calls != 0 {
				t.Fatalf("history=%v guildCalls=%d", loaded.History, guild.calls)
			}

			// A following normal change departs job-70, not the stale job-01.
			stored.Level, stored.SP = 20, 40
			if err := chars.Update(ctx, stored); err != nil {
				t.Fatal(err)
			}
			changed, _, err := jobSvc.ChangeJob(ctx, char.ID, "job-02")
			if err != nil {
				t.Fatal(err)
			}
			loaded, err = jobs.FindByCharacterID(ctx, char.ID)
			if err != nil {
				t.Fatal(err)
			}
			if sp, _ := loaded.SavedSP("job-70"); changed.OldJobID != "job-70" || sp != 40 || loaded.CurrentJobID != "job-02" || !slices.Contains(loaded.History, corejob.Change{FromJobID: "job-70", ToJobID: "job-02"}) {
				t.Fatalf("follow-up change: char=%+v state=%+v", changed, loaded)
			}
			if prior, _ := loaded.SavedSP("job-01"); prior != tc.sp {
				t.Fatalf("job-01 record overwritten: %d", prior)
			}
		})
	}
}
