package adventure_test

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/adventure"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/runhost"
	"github.com/witchcraze/party2re/internal/testutil"
)

// installJobEncounter serves job78/79 room and order entries through the
// host executor with the zero RNG (room draws succeed).
func installJobEncounter(t *testing.T, f *normalFixture) {
	t.Helper()
	withAssetSession(t, f)
	f.restart(t)
}

func setJobEncounterRoom(t *testing.T, f *normalFixture, run, action string) {
	t.Helper()
	if err := f.store.WithRunForUpdate(context.Background(), run, false, func(ctx context.Context) error {
		r, err := f.store.FindRun(ctx, run)
		if err != nil {
			return err
		}
		r.Round = 8
		if action == "job-78-entry-6" {
			r.Stage, err = f.cfg.Stages.FindByID("stage-19")
			if err != nil {
				return err
			}
			r.Round = 11
			r.Boxes = []runhost.TreasureBox{{ID: "owned-box", ItemID: "item-001"}}
		}
		r.EncounterFloor, r.EncounterStageID = r.Round, r.Stage.ID
		if action == "job-78-entry-2" {
			// The room tool needs an existing encounter; this one cannot answer yet.
			foe := runEnemy("foe", 900)
			foe.EncounterIdentity = "old-foe"
			r.Battle.Actors, r.Enemies = append(r.Battle.Actors, foe), []string{"foe"}
			r.LastActions["foe"] = f.clock.Now().Add(time.Hour)
		}
		for i := range r.Battle.Actors {
			a := &r.Battle.Actors[i]
			a.Jobs = corebattle.BattleJobs{JobID: "job-78", SP: 250, OldJobID: "job-78", OldSP: 230}
			a.MP = 80
		}
		r.Battle.Fields = corebattle.LifetimeFields{Default: "snow", Temporary: "burn", Turns: 5}
		p := r.Recipients[f.actors[0].ID]
		p.Pending.Gold = 7
		r.Recipients[f.actors[0].ID] = p
		r.Version++
		return f.store.SaveRun(ctx, *r, r.Version-1)
	}); err != nil {
		t.Fatal(err)
	}
}

func TestJobEncounterSQLRollbackReceiptAndUnseal(t *testing.T) {
	for _, action := range []string{"job-78-entry-2", "job-78-entry-6"} {
		t.Run(action, func(t *testing.T) {
			f := newNormalFixture(t, 1)
			installJobEncounter(t, f)
			run := f.enter(t, "normal").Command.RunID
			setJobEncounterRoom(t, f, run, action)
			f.clock.add(3 * time.Second)
			target := "owned-box"
			if action == "job-78-entry-2" {
				target = ""
			}
			before, err := f.store.FindRun(context.Background(), run)
			if err != nil {
				t.Fatal(err)
			}
			a := f.actors[0]
			cmd := runhost.Command{RunID: run, PlayerID: a.PlayerID, ActorID: a.ID, ID: id.New(), Version: before.Version, Action: action, TargetID: target}
			f.store.failWrite.Store(true)
			if _, err := f.svc.Command(context.Background(), cmd); err == nil {
				t.Fatal("failed SQL receipt accepted")
			}
			after, err := f.store.FindRun(context.Background(), run)
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatal("rollback changed run", err)
			}
			char, err := f.chars.FindByID(context.Background(), a.ID)
			if err != nil || char.MaoCount != 0 || char.Money != a.Money {
				t.Fatal(char, err)
			}
			f.store.failWrite.Store(false)
			f.store.loseCommit.Store(true)
			first, err := f.svc.Command(context.Background(), cmd)
			if err != nil {
				t.Fatal(err)
			}
			f.restart(t)
			draws := f.rng.calls.Load()
			retry, err := f.svc.Command(context.Background(), cmd)
			if err != nil || !reflect.DeepEqual(first, retry) || draws != f.rng.calls.Load() {
				t.Fatal("receipt replayed effect", err)
			}
			after, err = f.store.FindRun(context.Background(), run)
			if err != nil || after.StageCleared || after.Recipients[a.ID].Pending.Gold != 0 || after.Battle.Fields.Default != "snow" {
				t.Fatal(after, err)
			}
			char, err = f.chars.FindByID(context.Background(), a.ID)
			wantCount := 0
			if action == "job-78-entry-6" {
				wantCount = 1
			}
			if err != nil || char.MaoCount != wantCount || char.Money != a.Money+7 {
				t.Fatal("wrong settlement", char, err)
			}
			if action == "job-78-entry-2" && (after.Round != 9 || len(after.Enemies) == 0 || after.Enemies[0] == "foe") {
				t.Fatal(after)
			}
			if action == "job-78-entry-6" && (after.Round != 11 || len(after.Boxes) == 0 || after.Boxes[0].ID == "owned-box") {
				t.Fatal(after)
			}
		})
	}
}

func TestJobEncounterSQLTimeRestoresHeadLastForNextRequest(t *testing.T) {
	f := newNormalFixture(t, 1)
	installJobEncounter(t, f)
	run := f.enter(t, "normal").Command.RunID
	f.send(t, run, 0, "advance", "")
	cmd := runhost.Command{RunID: run, Version: readStockRoom(t, f, run).Version}
	before := editStockRoom(t, f, &cmd, func(r *adventure.NormalRun) {
		a := runActorByID(r, f.actors[0].ID)
		a.Jobs, a.MP = corebattle.BattleJobs{JobID: "job-79", SP: 100}, 80
	})
	f.send(t, run, 0, "job-79-entry-2", "")
	after, err := f.store.FindRun(context.Background(), run)
	want := append(slices.Clone(before.Enemies[1:]), before.Enemies[0])
	responded := false
	for _, id := range before.Enemies {
		responded = responded || !after.LastActions[id].Equal(before.LastActions[id])
	}
	if err != nil || !slices.Equal(after.Enemies, want) || after.Battle.Actors[len(after.Battle.Actors)-1].ID != before.Enemies[0] || responded || after.Battle.SkillBans["job-79-entry-2"] != 4 {
		t.Fatalf("enemies=%v want=%v responded=%v bans=%v err=%v", after.Enemies, want, responded, after.Battle.SkillBans, err)
	}
	f.restart(t)
	f.clock.add(3 * time.Second)
	if after.NextNPC(f.clock.Now()) != want[0] {
		t.Fatal("wrong response after reload", after)
	}
}

func TestJobEncounterSQLPairedTreasureCommandsConserveCounterAndStock(t *testing.T) {
	f := newNormalFixture(t, 2)
	installJobEncounter(t, f)
	_, lobby := f.waitingParty(t, "normal")
	a := f.actors[0]
	entry, err := f.svc.EnterParty(context.Background(), a.PlayerID, a.ID, lobby)
	if err != nil {
		t.Fatal(err)
	}
	run := entry.Command.RunID
	setJobEncounterRoom(t, f, run, "job-78-entry-6")
	r, err := f.store.FindRun(context.Background(), run)
	if err != nil {
		t.Fatal(err)
	}
	commands := make([]runhost.Command, 2)
	for i, a := range f.actors {
		commands[i] = runhost.Command{RunID: run, PlayerID: a.PlayerID, ActorID: a.ID, ID: id.New(), Version: r.Version, Action: "job-78-entry-6", TargetID: "owned-box"}
	}
	stats := testutil.RunConcurrentStressTest(t, testutil.GetStressConfig(), func(worker, op int) error {
		_, err := f.svc.Command(context.Background(), commands[(worker+op)%2])
		if errors.Is(err, runhost.ErrConflict) {
			return nil
		}
		return err
	})
	if stats.Failures != 0 {
		t.Fatal(stats)
	}
	r, err = f.store.FindRun(context.Background(), run)
	if err != nil || r.Version != commands[0].Version+1 || r.StageCleared {
		t.Fatal(r, err)
	}
	total := r.Recipients[a.ID].Pending.Gold
	for _, a := range f.actors {
		c, err := f.chars.FindByID(context.Background(), a.ID)
		if err != nil || c.MaoCount != 1 {
			t.Fatal(c, err)
		}
		total += c.Money - a.Money
	}
	if total != 7 {
		t.Fatal("stock duplicated/lost", total)
	}
}
