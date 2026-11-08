package database_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/exile"
	"github.com/witchcraze/party2re/internal/testutil"
)

func TestExileBoard_Integration(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	ctx := context.Background()
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo, err := database.NewExileRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	// Test-owned characters; deleting them cascades to the petition and its votes.
	var cs []corecharacter.Character
	t.Cleanup(func() {
		for _, c := range cs {
			//lint:ignore error-swallow best-effort removal of test-owned rows
			_, _ = db.ExecContext(context.Background(), "DELETE FROM exile_votes WHERE voter_id = ?", c.ID)
			//lint:ignore error-swallow best-effort removal of test-owned rows
			_, _ = db.ExecContext(context.Background(), "DELETE FROM exile_petitions WHERE applicant_id = ?", c.ID)
			//lint:ignore error-swallow best-effort removal of test-owned rows
			_, _ = db.ExecContext(context.Background(), "DELETE FROM characters WHERE id = ?", c.ID)
		}
	})
	for i := range 12 {
		c, err := database.CreateTestCharacter(ctx, db, fmt.Sprintf("Ex%d", i))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, "UPDATE characters SET job_level = ? WHERE id = ?", exile.MinJobLevel, c.ID); err != nil {
			t.Fatal(err)
		}
		cs = append(cs, c)
	}
	sleeps := timer.NewService(nil)
	svc := exile.NewService(repo, charRepo, sleeps, database.NewTransactionProvider(db), nil)
	applicant, target, voters := cs[0], cs[1], cs[2:]
	now := time.Now().UTC().Truncate(time.Microsecond)

	p, err := svc.Petition(ctx, applicant.ID, target.Name, "暴言", now)
	if err != nil {
		t.Fatal(err)
	}
	board, err := svc.GetBoard(ctx, voters[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	var stored *exile.Petition
	for i := range board.Petitions {
		if board.Petitions[i].ID == p.ID {
			stored = &board.Petitions[i]
		}
	}
	if stored == nil || stored.TargetName != target.Name || stored.Reason != "暴言" || len(stored.Votes) != 1 || stored.Votes[0].VoterID != applicant.ID || !stored.Votes[0].Approve {
		t.Fatalf("petition not persisted: %+v", stored)
	}
	if updated, err := repo.LockBoard(ctx); err != nil || updated.Before(now) {
		t.Fatalf("board time not written: %v %v", updated, err)
	}
	if involved, err := repo.IsInvolved(ctx, target.ID); err != nil || !involved {
		t.Fatalf("target not involved: %v %v", involved, err)
	}

	// Ten concurrent approvals: nine are needed, so exactly one reaches the verdict.
	var verdicts, rejected atomic.Int32
	testutil.RunConcurrentStressTest(t, testutil.ConcurrencyStressConfig{Workers: len(voters), OpsPerWorker: 1}, func(worker, _ int) error {
		r, err := svc.Vote(ctx, voters[worker].ID, target.Name, true, now)
		switch {
		case errors.Is(err, exile.ErrPetitionNotFound):
			rejected.Add(1)
			return nil
		case err != nil:
			return err
		case r.Verdict == exile.VerdictGuilty:
			verdicts.Add(1)
		}
		return nil
	})
	if verdicts.Load() != 1 || rejected.Load() != 1 {
		t.Fatalf("verdicts=%d rejected=%d", verdicts.Load(), rejected.Load())
	}
	if remaining, err := sleeps.GetRemainingLock(ctx, timer.CategorySleep, target.ID); err != nil || remaining <= 59*time.Minute {
		t.Fatalf("guilty sleep: %v %v", remaining, err)
	}
	for _, c := range []corecharacter.Character{applicant, target, voters[0]} {
		if involved, err := repo.IsInvolved(ctx, c.ID); err != nil || involved {
			t.Fatalf("resolved petition still involves %s: %v", c.Name, err)
		}
	}
}
