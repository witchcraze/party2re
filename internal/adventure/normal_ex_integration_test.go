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
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/runhost"
	"github.com/witchcraze/party2re/internal/testutil"
)

func exCommand(t *testing.T, f *normalFixture, runID, key, action string) runhost.Command {
	t.Helper()
	f.clock.add(3 * time.Second)
	r, err := f.store.FindRun(context.Background(), runID)
	if err != nil || r == nil {
		t.Fatal(r, err)
	}
	c := f.actors[0]
	return runhost.Command{PlayerID: c.PlayerID, ActorID: c.ID, RunID: runID, ID: key, Version: r.Version, Action: action}
}

func TestNormalRunEXEntryRollbackCommitUnknownReplayAndDefeat(t *testing.T) {
	f := newNormalFixture(t, 1)
	runID := f.enter(t, "normal").Command.RunID
	f.earn(t, runID, 0)
	f.stage(t, runID, ex11(t, f.actors[0].ID, "stage-23"))
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
	if err != nil || result.Round != adventure.EXBossFloor {
		t.Fatal("commit-unknown EX entry not resolved from receipt", result, err)
	}
	r, err := f.store.FindRun(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	actor := r.Battle.Actors[0]
	if r.Round != adventure.EXBossFloor || r.EncounterFloor != adventure.EXBossFloor || len(r.Enemies) != 1 || actor.HP != actor.MaxHP || actor.MP != actor.MaxMP || r.Battle.Fields.Default != "snow" || len(r.Boxes) != 0 {
		t.Fatalf("EX encounter not persisted: %+v", r)
	}
	f.restart(t)
	drawn := f.rng.calls.Load()
	if again, err := f.svc.Command(context.Background(), cmd); err != nil || !reflect.DeepEqual(again, result) {
		t.Fatal("receipt replay changed result", again, err)
	}
	if replayed, err := f.store.FindRun(context.Background(), runID); err != nil || !reflect.DeepEqual(r, replayed) || f.rng.calls.Load() != drawn || drawn == draws {
		t.Fatal("receipt replay rerolled or mutated", err)
	}

	f.kill(t, runID, 0)
	if r, err = f.store.FindRun(context.Background(), runID); err != nil || r.FloorsCleared != adventure.EXBossFloor || len(r.Boxes) != 0 || r.StageCleared {
		t.Fatal("EX defeat recorded incorrectly", r, err)
	}
	f.stage(t, runID, fall(f.actors[0].ID))
	if r, err = f.store.FindRun(context.Background(), runID); err != nil || r.Round != adventure.EXBossFloor || len(r.Roster) != 1 || len(r.Boxes) != 0 {
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
	stage, err := f.cfg.Stages.FindByID("stage-22")
	if err != nil {
		t.Fatal(err)
	}
	cmd := f.earnCommand(t, runID, 0)
	f.stage(t, runID, func(r *adventure.NormalRun) {
		r.Stage = stage
		runActorByID(r, cmd.TargetID).EncounterIdentity = "ordinary-22-monster-175B"
	})
	cmd.Version++
	draws := f.rng.calls.Load()
	if _, err := f.svc.Command(context.Background(), cmd); err != nil {
		t.Fatal(err)
	}
	r, err := f.store.FindRun(context.Background(), runID)
	drawn := f.rng.calls.Load()
	if err != nil || !r.EXUnlocked || drawn == draws {
		t.Fatal("royal bee draw missing", r, err)
	}
	if _, err := f.svc.Command(context.Background(), cmd); err != nil || f.rng.calls.Load() != drawn {
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
	if _, err := f.svc.EnterSolo(context.Background(), c.PlayerID, c.ID, "daily", "solo-quest", "stage-26", "normal", 3); err == nil {
		t.Fatal("failed admission succeeded")
	}
	f.store.failWrite.Store(false)
	draws := f.rng.calls.Load()
	entry, err := f.svc.EnterSolo(context.Background(), c.PlayerID, c.ID, "daily", "solo-quest", "stage-26", "normal", 3)
	if err != nil {
		t.Fatal(err)
	}
	r, err := f.store.FindRun(context.Background(), entry.Command.RunID)
	// The deterministic RNG selects the first season, then succeeds the EX draw.
	if err != nil || r.Stage.Season != "spring" || r.RecordSeason != "spring" || !r.EXUnlocked || f.rng.calls.Load() != draws+2 {
		t.Fatal("season/EX draw", r, err)
	}
	if _, err := f.svc.EnterSolo(context.Background(), c.PlayerID, c.ID, "daily", "solo-quest", "stage-26", "normal", 3); err != nil || f.rng.calls.Load() != draws+2 {
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
	f.earn(t, runID, 0)
	f.stage(t, runID, ex11(t, f.actors[0].ID, "stage-23"))
	enter := exCommand(t, f, runID, "race-ex", "advance")
	exit := runhost.Command{PlayerID: other.PlayerID, ActorID: other.ID, RunID: runID, ID: "race-exit", Version: enter.Version, Action: "exit"}
	stats := testutil.RunConcurrentStressTest(t, testutil.GetStressConfig(), func(worker, op int) error {
		request := enter
		if (worker+op)%2 == 0 {
			request = exit
		}
		_, err := f.svc.Command(context.Background(), request)
		if errors.Is(err, runhost.ErrConflict) || errors.Is(err, runhost.ErrForbidden) || errors.Is(err, runhost.ErrRecovery) {
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
	if (r.Round == adventure.EXBossFloor) == (len(r.Roster) == 1) {
		t.Fatalf("both or neither committed: floor=%d roster=%v", r.Round, r.Roster)
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

// exTreasureRun reaches a cleared stage26 spring EX encounter at floor 12.
func exTreasureRun(t *testing.T, f *normalFixture, runID string) {
	t.Helper()
	f.stage(t, runID, ex11(t, f.actors[0].ID, "stage-26"))
	f.send(t, runID, 0, "advance", "")
	f.kill(t, runID, 0)
}

func countItem(t *testing.T, f *normalFixture, characterID, itemID string) int {
	t.Helper()
	inv, err := f.invs.FindByCharacterID(context.Background(), characterID)
	if err != nil {
		t.Fatal(err)
	}
	dep, err := f.depots.FindByCharacterID(context.Background(), characterID)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, item := range append(inv.Items, dep.Items...) {
		if item.DefinitionID == itemID {
			n += item.Quantity
		}
	}
	return n
}

func TestNormalRunEXTreasureEntryAndSeparateClaims(t *testing.T) {
	f := newNormalFixture(t, 1)
	c := f.actors[0]
	runID := f.enter(t, "normal").Command.RunID
	exTreasureRun(t, f, runID)
	f.stage(t, runID, func(r *adventure.NormalRun) {
		p := r.Recipients[c.ID] // the ordinary allowance used earlier in the room
		p.Inspected = 1
		r.Recipients[c.ID] = p
	})
	before, err := f.store.FindRun(context.Background(), runID)
	if err != nil || before.Recipients[c.ID].Inspected != 1 {
		t.Fatal(before, err)
	}
	cmd := exCommand(t, f, runID, "ex-treasure", "advance")
	f.store.failWrite.Store(true)
	if _, err := f.svc.Command(context.Background(), cmd); err == nil {
		t.Fatal("failed receipt write committed EX treasure")
	}
	f.store.failWrite.Store(false)
	if after, err := f.store.FindRun(context.Background(), runID); err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("rolled-back EX treasure persisted", err)
	}
	f.store.loseCommit.Store(true)
	result, err := f.svc.Command(context.Background(), cmd)
	if err != nil || result.Round != adventure.EXTreasureFloor {
		t.Fatal("commit-unknown EX treasure not resolved from receipt", result, err)
	}
	r, err := f.store.FindRun(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	// Solo: one EX chest from the spring pool and one fixed chest; the allowance reset.
	want := []runhost.TreasureBox{{ID: "box-1", Name: "普通の宝箱A", ItemID: "item-043"}, {ID: "box-2", Name: "煤けた宝箱A", ItemID: "item-263", Fixed: true}}
	if !reflect.DeepEqual(r.Boxes, want) || r.Recipients[c.ID].Inspected != 0 || r.EncounterFloor != adventure.EXTreasureFloor {
		t.Fatalf("EX treasure: %+v %+v", r.Boxes, r.Recipients[c.ID])
	}
	f.restart(t)
	drawn := f.rng.calls.Load()
	if again, err := f.svc.Command(context.Background(), cmd); err != nil || !reflect.DeepEqual(again, result) || f.rng.calls.Load() != drawn {
		t.Fatal("receipt replay changed EX treasure", again, err)
	}
	if _, err := f.svc.Command(context.Background(), exCommand(t, f, runID, "past-13", "advance")); !errors.Is(err, adventure.ErrInvalidFloorAction) {
		t.Fatal("advanced past terminal 13", err)
	}

	inv, err := f.invs.FindByCharacterID(context.Background(), c.ID)
	if err != nil {
		t.Fatal(err)
	}
	key, err := coreitem.NewInstance(runhost.ItemTreasurePocket, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := inv.Add(key); err != nil {
		t.Fatal(err)
	}
	if err := f.invs.Save(context.Background(), inv); err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	for _, box := range []string{"box-2", "box-1"} {
		claim := exCommand(t, f, runID, "claim-"+box, "chest")
		claim.TargetID = box
		if _, err := f.svc.Command(context.Background(), claim); err != nil {
			t.Fatal(box, err)
		}
		if _, err := f.svc.Command(context.Background(), claim); err != nil {
			t.Fatal("claim replay", box, err)
		}
	}
	r, err = f.store.FindRun(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	if r.Boxes[0].OpenedBy != c.ID || r.Boxes[1].OpenedBy != c.ID || r.Recipients[c.ID].Inspected != 1 {
		t.Fatalf("claims: %+v %+v", r.Boxes, r.Recipients[c.ID])
	}
	if countItem(t, f, c.ID, "item-263") != 1 || countItem(t, f, c.ID, "item-043") != 1 || countItem(t, f, c.ID, runhost.ItemTreasurePocket) != 1 {
		t.Fatal("fixed and ordinary claims delivered or consumed incorrectly")
	}
}

func TestNormalRunEXFixedClaimVersusExitStress(t *testing.T) {
	f := newNormalFixture(t, 2)
	_, lobby := f.waitingParty(t, "normal")
	a, b := f.actors[0], f.actors[1]
	entry, err := f.svc.EnterParty(context.Background(), a.PlayerID, a.ID, lobby)
	if err != nil {
		t.Fatal(err)
	}
	runID := entry.Command.RunID
	exTreasureRun(t, f, runID)
	f.send(t, runID, 0, "advance", "")
	r, err := f.store.FindRun(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	fixed := ""
	for _, box := range r.Boxes {
		if box.Fixed {
			fixed = box.ID
		}
	}
	if r.Round != adventure.EXTreasureFloor || fixed == "" {
		t.Fatalf("EX treasure missing: %+v", r)
	}
	f.clock.add(3 * time.Second)
	commands := []runhost.Command{
		{PlayerID: a.PlayerID, ActorID: a.ID, RunID: runID, ID: "claim-a", Version: r.Version, Action: "chest", TargetID: fixed},
		{PlayerID: b.PlayerID, ActorID: b.ID, RunID: runID, ID: "claim-b", Version: r.Version, Action: "chest", TargetID: fixed},
		{PlayerID: b.PlayerID, ActorID: b.ID, RunID: runID, ID: "exit-b", Version: r.Version, Action: "exit"},
	}
	stats := testutil.RunConcurrentStressTest(t, testutil.GetStressConfig(), func(worker, op int) error {
		_, err := f.svc.Command(context.Background(), commands[(worker+op)%len(commands)])
		if errors.Is(err, runhost.ErrConflict) || errors.Is(err, runhost.ErrForbidden) || errors.Is(err, runhost.ErrRecovery) {
			return nil
		}
		return err
	})
	if stats.Failures != 0 {
		t.Fatal(stats)
	}
	r, err = f.store.FindRun(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	opened := 0
	for _, box := range r.Boxes {
		if box.ID == fixed && box.OpenedBy != "" {
			opened = 1
		}
	}
	delivered := countItem(t, f, a.ID, "item-263") + countItem(t, f, b.ID, "item-263")
	// Exactly one competing command committed against the shared version.
	if delivered != opened || opened+2-len(r.Roster) != 1 {
		t.Fatalf("opened=%d delivered=%d roster=%v", opened, delivered, r.Roster)
	}
}

func TestNormalRunSaveBoundaryDropsFallenNPCs(t *testing.T) {
	f := newNormalFixture(t, 1)
	c := f.actors[0]
	runID := f.enter(t, "normal").Command.RunID
	f.send(t, runID, 0, "advance", "")
	// A cleared stage23 floor 10 whose helper fell; the next command's save drops it.
	stage23, err := f.cfg.Stages.FindByID("stage-23")
	if err != nil {
		t.Fatal(err)
	}
	f.stage(t, runID, func(r *adventure.NormalRun) {
		r.Stage, r.Round = stage23, 10
		r.DiscardEncounter()
		helper := runActorByID(r, c.ID).Clone()
		helper.ID, helper.NPC, helper.RosterIdentity, helper.HP = "helper", true, "calling-64", 0
		r.Battle.Actors = append(r.Battle.Actors, helper)
	})
	f.earn(t, runID, 0)
	r, err := f.store.FindRun(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range r.Battle.Actors {
		if a.ID == "helper" || slices.Contains(r.Enemies, a.ID) {
			t.Fatalf("fallen NPC survived the save: %+v", a)
		}
	}
	// Legacy sees one ally at the next advance, so stage23 creates no marker.
	f.send(t, runID, 0, "advance", "")
	f.stage(t, runID, fall(c.ID))
	if r, err = f.store.FindRun(context.Background(), runID); err != nil || r.EXUnlocked || r.Round != adventure.TreasureRoomFloor {
		t.Fatal("fallen helper counted toward the stage23 marker", r, err)
	}
	if !slices.ContainsFunc(r.Battle.Actors, func(a corebattle.LifetimeActor) bool { return a.ID == c.ID && a.HP <= 0 }) {
		t.Fatal("fallen real player left the room")
	}
}
