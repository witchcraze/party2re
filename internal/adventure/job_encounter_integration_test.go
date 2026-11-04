package adventure_test

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/adventure"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/skill"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/testutil"
)

// This consumer exercises the real intrinsic registrations and SQL receipt
// boundary. Full player/NPC tails and transport composition remain #1329/#1330.
type jobEncounterCombat struct {
	*travelCombat
	tails atomic.Int64
}

func (c *jobEncounterCombat) ExecuteRunAction(ctx context.Context, r *adventure.NormalRun, cmd adventure.RunCommand) (adventure.RunActionEffects, error) {
	if cmd.Action != "job-78-entry-2" && cmd.Action != "job-78-entry-6" && cmd.Action != "job-79-entry-2" {
		return c.travelCombat.ExecuteRunAction(ctx, r, cmd)
	}
	fx := adventure.RunActionEffects{RoomActivity: true, ActorActivity: true, ReleaseStock: true}
	reset := travelFrame(r, cmd.ActorID)
	op, transition, err := c.f.svc.PrepareJobEncounters(ctx, r, cmd.ActorID, &reset)
	if err != nil {
		return fx, err
	}
	var d skill.BattleDefinitions
	d.RegisterPlayerCallbacks(skill.PlayerCallbackContexts{JobEncounters: op}, len(r.Roster))
	frame := corebattle.CallbackContext{TargetContext: op.TargetContext, Arguments: []string{cmd.TargetID}}
	ops, err := d.InvocationAction(&r.Battle, &frame, []string{cmd.Action}, c.f.rng)
	if err != nil {
		return fx, err
	}
	if err := r.Battle.ApplyInvocations(&frame, d.Invocations, ops, c.f.rng); err != nil {
		return fx, err
	}
	r.Enemies = slices.Clone(frame.EnemyIDs)
	fx.MemberOnlyIDs = slices.Clone(frame.MemberOnlyIDs)
	for i := range r.Battle.Actors {
		if r.Battle.Actors[i].ID == cmd.ActorID {
			cost := map[string]int{"job-78-entry-2": 10, "job-79-entry-2": 30}[cmd.Action]
			r.Battle.Actors[i].MP = max(0, r.Battle.Actors[i].MP-cost)
		}
	}
	c.tails.Add(1)
	r.Battle.TickSkillBans()
	r.Battle.Fields.FinishOperation(len(r.Enemies) > 0, false)
	transition.ResponseSuppressed = transition.ResponseSuppressed || op.ResponseSuppressed
	if npc := transition.BeginNPCResponse(r, c.f.clock.Now()); npc != "" {
		c.responses.Add(1)
		if err := r.RecordNPCResponse(npc, c.f.clock.Now()); err != nil {
			return fx, err
		}
	}
	fx.UnsealDemonKing = transition.UnsealDemonKing
	return fx, nil
}

func installJobEncounter(t *testing.T, f *normalFixture) *jobEncounterCombat {
	t.Helper()
	c := &jobEncounterCombat{travelCombat: &travelCombat{f: f}}
	f.cfg.Combat = c
	f.restart(t)
	return c
}

func setJobEncounterRoom(t *testing.T, f *normalFixture, run, action string) {
	t.Helper()
	if err := f.store.WithRunForUpdate(context.Background(), run, false, func(ctx context.Context) error {
		r, err := f.store.FindRun(ctx, run)
		if err != nil {
			return err
		}
		r.Floor = 8
		if action == "job-78-entry-6" {
			r.Stage, err = f.cfg.Stages.FindByID("stage-19")
			if err != nil {
				return err
			}
			r.Floor = 11
			r.Boxes = []adventure.TreasureBox{{ID: "owned-box", ItemID: "item-001"}}
		}
		r.EncounterFloor, r.EncounterStageID = r.Floor, r.Stage.ID
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
			port := installJobEncounter(t, f)
			run := f.enter(t, "normal").Command.RunID
			setJobEncounterRoom(t, f, run, action)
			before, err := f.store.FindRun(context.Background(), run)
			if err != nil {
				t.Fatal(err)
			}
			a := f.actors[0]
			cmd := adventure.RunCommand{RunID: run, PlayerID: a.PlayerID, ActorID: a.ID, ID: id.New(), Version: before.Version, Action: action, TargetID: "owned-box"}
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
			draws, tails := f.rng.calls.Load(), port.tails.Load()
			retry, err := f.svc.Command(context.Background(), cmd)
			if err != nil || !reflect.DeepEqual(first, retry) || draws != f.rng.calls.Load() || tails != port.tails.Load() {
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
			if action == "job-78-entry-2" && (after.Floor != 9 || len(after.Enemies) == 0) {
				t.Fatal(after)
			}
			if action == "job-78-entry-6" && (after.Floor != 11 || len(after.Boxes) == 0 || after.Boxes[0].ID == "owned-box" || port.responses.Load() != 0) {
				t.Fatal(after)
			}
		})
	}
}

func TestJobEncounterSQLTimeRestoresHeadLastForNextRequest(t *testing.T) {
	f := newNormalFixture(t, 1)
	port := installJobEncounter(t, f)
	run := f.enter(t, "normal").Command.RunID
	f.send(t, run, 0, "advance", "")
	before, err := f.store.FindRun(context.Background(), run)
	if err != nil {
		t.Fatal(err)
	}
	f.send(t, run, 0, "job-79-entry-2", "")
	after, err := f.store.FindRun(context.Background(), run)
	want := append(slices.Clone(before.Enemies[1:]), before.Enemies[0])
	if err != nil || !slices.Equal(after.Enemies, want) || after.Battle.Actors[len(after.Battle.Actors)-1].ID != before.Enemies[0] || port.responses.Load() != 0 || after.Battle.SkillBans["job-79-entry-2"] != 3 {
		t.Fatal(after, err)
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
	commands := make([]adventure.RunCommand, 2)
	for i, a := range f.actors {
		commands[i] = adventure.RunCommand{RunID: run, PlayerID: a.PlayerID, ActorID: a.ID, ID: id.New(), Version: r.Version, Action: "job-78-entry-6", TargetID: "owned-box"}
	}
	stats := testutil.RunConcurrentStressTest(t, testutil.GetStressConfig(), func(worker, op int) error {
		_, err := f.svc.Command(context.Background(), commands[(worker+op)%2])
		if errors.Is(err, adventure.ErrRunConflict) {
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
