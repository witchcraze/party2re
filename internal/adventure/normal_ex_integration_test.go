package adventure_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/adventure"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/testutil"
)

func exCommand(t *testing.T, f *normalFixture, runID, key, action string) adventure.RunCommand {
	t.Helper()
	f.clock.add(3 * time.Second)
	r, err := f.store.FindRun(context.Background(), runID)
	if err != nil || r == nil {
		t.Fatal(r, err)
	}
	c := f.actors[0]
	return adventure.RunCommand{PlayerID: c.PlayerID, ActorID: c.ID, RunID: runID, ID: key, Version: r.Version, Action: action}
}

func TestNormalRunEXEntryRollbackCommitUnknownReplayAndDefeat(t *testing.T) {
	f := newNormalFixture(t, 1)
	runID := f.enter(t, "normal").Command.RunID
	f.send(t, runID, 0, "earn", "")
	f.send(t, runID, 0, "ex11", "stage-23")
	before, err := f.store.FindRun(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exCommand(t, f, runID, "ex-enter", "advance")
	draws := f.rng.calls.Load()
	f.store.failWrite.Store(true)
	if _, err := f.svc.Command(context.Background(), cmd); err == nil {
		t.Fatal("failed receipt write committed EX entry")
	}
	f.store.failWrite.Store(false)
	if after, err := f.store.FindRun(context.Background(), runID); err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("rolled-back EX entry persisted", err)
	}
	f.store.loseCommit.Store(true)
	result, err := f.svc.Command(context.Background(), cmd)
	if err != nil || result.Floor != adventure.EXBossFloor {
		t.Fatal("commit-unknown EX entry not resolved from receipt", result, err)
	}
	r, err := f.store.FindRun(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	actor := r.Battle.Actors[0]
	if r.Floor != adventure.EXBossFloor || r.EncounterFloor != adventure.EXBossFloor || len(r.Enemies) != 1 || actor.HP != actor.MaxHP || actor.MP != actor.MaxMP || r.Battle.Fields.Default != "snow" || len(r.Boxes) != 0 {
		t.Fatalf("EX encounter not persisted: %+v", r)
	}
	f.restart(t)
	drawn := f.rng.calls.Load()
	if again, err := f.svc.Command(context.Background(), cmd); err != nil || again != result {
		t.Fatal("receipt replay changed result", again, err)
	}
	if replayed, err := f.store.FindRun(context.Background(), runID); err != nil || !reflect.DeepEqual(r, replayed) || f.rng.calls.Load() != drawn || drawn == draws {
		t.Fatal("receipt replay rerolled or mutated", err)
	}

	f.send(t, runID, 0, "kill", "")
	if r, err = f.store.FindRun(context.Background(), runID); err != nil || r.FloorsCleared != adventure.EXBossFloor || len(r.Boxes) != 0 || r.StageCleared {
		t.Fatal("EX defeat recorded incorrectly", r, err)
	}
	gate := exCommand(t, f, runID, "ex-treasure", "advance")
	if _, err := f.svc.Command(context.Background(), gate); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal("EX treasure entry accepted before #1808", err)
	}
	if after, err := f.store.FindRun(context.Background(), runID); err != nil || !reflect.DeepEqual(r, after) {
		t.Fatal("gated EX treasure mutated", err)
	}
	f.send(t, runID, 0, "fall", "")
	if r, err = f.store.FindRun(context.Background(), runID); err != nil || r.Floor != adventure.EXBossFloor || len(r.Roster) != 1 || len(r.Boxes) != 0 {
		t.Fatal("defeat at 12 exited or created treasure", r, err)
	}
	f.send(t, runID, 0, "exit", "")
	c, err := f.chars.FindByID(context.Background(), f.actors[0].ID)
	if err != nil || c.Money-f.actors[0].Money != 10 {
		t.Fatal("earned assets lost after EX defeat", c.Money, err)
	}
}

func TestNormalRunRoyalBeeDrawCommitsOnce(t *testing.T) {
	f := newNormalFixture(t, 1)
	runID := f.enter(t, "normal").Command.RunID
	cmd := exCommand(t, f, runID, "bee", "bee")
	cmd.TargetID = "stage-22"
	draws := f.rng.calls.Load()
	if _, err := f.svc.Command(context.Background(), cmd); err != nil {
		t.Fatal(err)
	}
	r, err := f.store.FindRun(context.Background(), runID)
	if err != nil || !r.EXUnlocked || f.rng.calls.Load() != draws+1 {
		t.Fatal("royal bee draw missing or repeated", r, err)
	}
	if _, err := f.svc.Command(context.Background(), cmd); err != nil || f.rng.calls.Load() != draws+1 {
		t.Fatal("replay redrew", err)
	}
}

func TestNormalRunStage26EXDrawOnceAfterSeason(t *testing.T) {
	f := newNormalFixture(t, 1)
	c := f.actors[0]
	c.Level = 80
	if err := f.chars.Update(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	f.cfg.EntryLocks = timer.NewService(nil)
	f.restart(t)
	f.store.failWrite.Store(true)
	if _, err := f.svc.EnterSolo(context.Background(), c.PlayerID, c.ID, "daily", "stage-26", "normal", 3); err == nil {
		t.Fatal("failed admission succeeded")
	}
	f.store.failWrite.Store(false)
	draws := f.rng.calls.Load()
	entry, err := f.svc.EnterSolo(context.Background(), c.PlayerID, c.ID, "daily", "stage-26", "normal", 3)
	if err != nil {
		t.Fatal(err)
	}
	r, err := f.store.FindRun(context.Background(), entry.Command.RunID)
	// The deterministic RNG selects the first season, then succeeds the EX draw.
	if err != nil || r.Stage.Season != "spring" || r.RecordSeason != "spring" || !r.EXUnlocked || f.rng.calls.Load() != draws+2 {
		t.Fatal("season/EX draw", r, err)
	}
	if _, err := f.svc.EnterSolo(context.Background(), c.PlayerID, c.ID, "daily", "stage-26", "normal", 3); err != nil || f.rng.calls.Load() != draws+2 {
		t.Fatal("committed admission redrew", err)
	}
}

func TestNormalRunEXEntryVersusExitStress(t *testing.T) {
	f := newNormalFixture(t, 2)
	_, lobby := f.waitingParty(t, "normal")
	c, other := f.actors[0], f.actors[1]
	entry, err := f.svc.EnterParty(context.Background(), c.PlayerID, c.ID, lobby)
	if err != nil {
		t.Fatal(err)
	}
	runID := entry.Command.RunID
	f.send(t, runID, 0, "earn", "")
	f.send(t, runID, 0, "ex11", "stage-23")
	enter := exCommand(t, f, runID, "race-ex", "advance")
	exit := adventure.RunCommand{PlayerID: other.PlayerID, ActorID: other.ID, RunID: runID, ID: "race-exit", Version: enter.Version, Action: "exit"}
	stats := testutil.RunConcurrentStressTest(t, testutil.GetStressConfig(), func(worker, op int) error {
		request := enter
		if (worker+op)%2 == 0 {
			request = exit
		}
		_, err := f.svc.Command(context.Background(), request)
		if errors.Is(err, adventure.ErrRunConflict) || errors.Is(err, adventure.ErrRunForbidden) || errors.Is(err, adventure.ErrRunRecovery) {
			return nil
		}
		return err
	})
	if stats.Failures != 0 {
		t.Fatal(stats)
	}
	r, err := f.store.FindRun(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	// Exactly one of the competing commands committed against the shared version.
	if (r.Floor == adventure.EXBossFloor) == (len(r.Roster) == 1) {
		t.Fatalf("both or neither committed: floor=%d roster=%v", r.Floor, r.Roster)
	}
	for _, before := range f.actors {
		after, err := f.chars.FindByID(context.Background(), before.ID)
		if err != nil {
			t.Fatal(err)
		}
		recipient := r.Recipients[before.ID]
		if after.Money-before.Money+recipient.Pending.Gold+recipient.Conditional.Gold != 5 {
			t.Fatal("asset conservation failed", before.ID, after.Money, recipient)
		}
	}
}
