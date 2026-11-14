package job_test

import (
	"context"
	"errors"
	"os"
	"slices"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/item"
	corejob "github.com/witchcraze/party2re/internal/core/job"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/economy"
	"github.com/witchcraze/party2re/internal/job"
)

type failAfterGuildPointsAward struct {
	real job.GuildPointAwarder
	err  error
}

func (a failAfterGuildPointsAward) AddGuildPoints(ctx context.Context, characterID string, points int) error {
	if err := a.real.AddGuildPoints(ctx, characterID, points); err != nil {
		return err
	}
	return a.err
}

func TestChangeJob_RealDatabase_GuildPointsRollbackAndCommit(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

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
	guilds, err := database.NewGuildRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	txProvider := database.NewTransactionProvider(db)
	runner, err := economy.NewService(chars, inventories, economy.WithTransactionProvider(txProvider))
	if err != nil {
		t.Fatal(err)
	}

	const initialGP int64 = 120
	sentinel := errors.New("injected failure after guild point write")

	for _, tc := range []struct {
		name        string
		failAfterGP bool
	}{
		{name: "gp_failure_rolls_back_job_change_and_guild_points", failAfterGP: true},
		{name: "gp_success_commits_job_change_and_guild_points", failAfterGP: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, char, err := database.CreateTestGuildWithLeader(ctx, db, "", corecharacter.InitialMoney)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				cleanupCtx := context.Background()
				if _, err := db.ExecContext(cleanupCtx, "DELETE FROM guild_members WHERE guild_id = ?", g.ID); err != nil {
					t.Errorf("cleanup guild_members %s: %v", g.ID, err)
				}
				if _, err := db.ExecContext(cleanupCtx, "DELETE FROM guilds WHERE id = ?", g.ID); err != nil {
					t.Errorf("cleanup guild %s: %v", g.ID, err)
				}
				if err := chars.Delete(cleanupCtx, char.ID); err != nil {
					t.Errorf("cleanup character %s: %v", char.ID, err)
				}
				if _, err := db.ExecContext(cleanupCtx, "DELETE FROM players WHERE id = ?", char.PlayerID); err != nil {
					t.Errorf("cleanup player %s: %v", char.PlayerID, err)
				}
			})

			if err := guilds.AddPoints(ctx, g.ID, initialGP); err != nil {
				t.Fatal(err)
			}

			char.JobID, char.OldJobID, char.SP, char.OldSP, char.Level, char.Gender = "job-01", "", 90, 0, 20, "male"
			if err := chars.Update(ctx, char); err != nil {
				t.Fatal(err)
			}

			state, err := corejob.NewCharacterJob(char.ID, "job-01")
			if err != nil {
				t.Fatal(err)
			}
			if err := jobs.Save(ctx, state); err != nil {
				t.Fatal(err)
			}

			reqItem, err := item.NewInstance("item-027", 1)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := database.CreateTestInventoryWithItems(ctx, db, char.ID, []item.Instance{reqItem}); err != nil {
				t.Fatal(err)
			}

			var awarder job.GuildPointAwarder = guilds
			if tc.failAfterGP {
				awarder = failAfterGuildPointsAward{real: guilds, err: sentinel}
			}
			svc, err := job.NewService(
				jobs,
				job.WithCharacterRepository(chars),
				job.WithInventoryRepository(inventories),
				job.WithEconomy(runner),
				job.WithGuildPointAwarder(awarder),
			)
			if err != nil {
				t.Fatal(err)
			}

			updatedChar, updatedState, changeErr := svc.ChangeJob(ctx, char.ID, "job-33")

			storedChar, err := chars.FindByID(ctx, char.ID)
			if err != nil {
				t.Fatal(err)
			}
			loadedJob, err := jobs.FindByCharacterID(ctx, char.ID)
			if err != nil {
				t.Fatal(err)
			}
			loadedInv, err := inventories.FindByCharacterID(ctx, char.ID)
			if err != nil {
				t.Fatal(err)
			}
			loadedGuild, _, err := guilds.GetGuild(ctx, g.ID)
			if err != nil {
				t.Fatal(err)
			}

			if tc.failAfterGP {
				if !errors.Is(changeErr, sentinel) {
					t.Fatalf("expected error %v, got %v", sentinel, changeErr)
				}
				if storedChar.JobID != "job-01" || storedChar.OldJobID != "" || storedChar.SP != 90 || storedChar.OldSP != 0 {
					t.Fatalf("character state persisted after GP failure: %+v", storedChar)
				}
				if loadedJob.CurrentJobID != "job-01" || len(loadedJob.History) != 0 || loadedJob.HasRecordedJob("job-01") || loadedJob.IsMastered("job-01") {
					t.Fatalf("job state persisted after GP failure: %+v", loadedJob)
				}
				if _, ok := loadedJob.SavedSP("job-01"); ok {
					t.Fatalf("job SP persisted after GP failure: %+v", loadedJob.RecordedJobSP)
				}
				if loadedInv.Quantity("item-027") != 1 {
					t.Fatalf("expected item-027 quantity 1 after rollback, got %d", loadedInv.Quantity("item-027"))
				}
				if loadedGuild.Points != initialGP {
					t.Fatalf("expected guild points %d after rollback, got %d", initialGP, loadedGuild.Points)
				}
				return
			}

			if changeErr != nil {
				t.Fatalf("unexpected ChangeJob error: %v", changeErr)
			}
			if updatedChar.JobID != "job-33" || updatedState.CurrentJobID != "job-33" {
				t.Fatalf("unexpected returned job change result: char=%+v state=%+v", updatedChar, updatedState)
			}
			if storedChar.JobID != "job-33" || storedChar.OldJobID != "job-01" || storedChar.SP != 0 || storedChar.OldSP != 90 {
				t.Fatalf("unexpected persisted character state: %+v", storedChar)
			}
			if loadedJob.CurrentJobID != "job-33" || !slices.Equal(loadedJob.History, []corejob.Change{{FromJobID: "job-01", ToJobID: "job-33"}}) {
				t.Fatalf("unexpected persisted job state/history: %+v", loadedJob)
			}
			if sp, ok := loadedJob.SavedSP("job-01"); !ok || sp != 90 || !loadedJob.HasRecordedJob("job-01") {
				t.Fatalf("expected recorded job-01 SP 90, got sp=%d ok=%v state=%+v", sp, ok, loadedJob)
			}
			if !loadedJob.IsMastered("job-01") || loadedJob.MasteredJobSP["job-01"] != 90 {
				t.Fatalf("expected job-01 mastery with SP 90, got %+v", loadedJob)
			}
			if loadedInv.Quantity("item-027") != 0 {
				t.Fatalf("expected item-027 consumed, got quantity %d", loadedInv.Quantity("item-027"))
			}
			if loadedGuild.Points != initialGP+50 {
				t.Fatalf("expected guild points %d, got %d", initialGP+50, loadedGuild.Points)
			}
		})
	}
}
