package adventure_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/adventure"
	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/testutil"
)

// This bounded consumer verifies travel/SQL composition, not #1329's complete
// player/NPC action tail or #1330's transport discovery and input contract.
type travelCombat struct {
	f         *normalFixture
	skip      bool
	responses atomic.Int64
}

func travelFrame(r *adventure.NormalRun, actor string) corebattle.EquipmentResetContext {
	frame := corebattle.EquipmentResetContext{TargetContext: corebattle.TargetContext{ActorID: actor, EnemyIDs: slices.Clone(r.Enemies)}}
	for _, a := range r.Battle.Actors {
		if !slices.Contains(r.Enemies, a.ID) {
			frame.PartyIDs = append(frame.PartyIDs, a.ID)
		}
	}
	return frame
}

func (c *travelCombat) PrepareAdvance(_ context.Context, r *adventure.NormalRun, actor string) error {
	frame := travelFrame(r, actor)
	if err := battle.ResetAll(&r.Battle, &frame, c.f.rng); err != nil {
		return err
	}
	r.Battle.Fields.ResetFloor()
	return nil
}

func (c *travelCombat) PrepareBossField(_ context.Context, r *adventure.NormalRun, actor string) error {
	frame := travelFrame(r, actor)
	return r.Battle.ApplySpecialBossField(&frame, c.f.rng)
}

func (c *travelCombat) ExecuteRunAction(ctx context.Context, r *adventure.NormalRun, cmd adventure.RunCommand) (adventure.RunActionEffects, error) {
	if cmd.Action != "travel" {
		return c.f.combat.ExecuteRunAction(ctx, r, cmd)
	}
	fx := adventure.RunActionEffects{RoomActivity: true, ActorActivity: true, ReleaseStock: true}
	if c.f.clock.Now().Before(r.LastActions[cmd.ActorID].Add(time.Duration(r.Admission.Speed) * time.Second)) {
		return fx, adventure.ErrRunCooldown
	}
	input, err := adventure.ParseCombatTravelInput(cmd.Input)
	if err != nil {
		return fx, err
	}
	inv, err := c.f.invs.FindByCharacterID(ctx, cmd.ActorID)
	if err != nil {
		return fx, err
	}
	tool, ok := inv.Find(cmd.TargetID)
	if !ok || tool.Quantity < 1 {
		return fx, adventure.ErrRunConflict
	}
	frame := travelFrame(r, cmd.ActorID)
	if err := c.f.svc.ValidateCombatTravel(r, cmd.ActorID, tool.DefinitionID, input, &frame); err != nil {
		return fx, err
	}
	if c.skip {
		return fx, nil
	}
	transition, err := c.f.svc.ApplyCombatTravel(ctx, r, cmd.ActorID, tool.DefinitionID, input, &frame)
	if err != nil {
		return fx, err
	}
	if transition.Consume {
		fx.Consumed = map[string][]corebattle.ConsumedItem{cmd.ActorID: {{ID: tool.DefinitionID, InstanceID: tool.ID, Quantity: 1}}}
	}
	if npc := transition.BeginNPCResponse(r, c.f.clock.Now()); npc != "" {
		c.responses.Add(1)
		if err := r.RecordNPCResponse(npc, c.f.clock.Now()); err != nil {
			return fx, err
		}
	}
	return fx, nil
}

func installTravel(t *testing.T, f *normalFixture) *travelCombat {
	t.Helper()
	c := &travelCombat{f: f}
	f.cfg.Combat = c
	f.restart(t)
	return c
}

func addTravelTool(t *testing.T, f *normalFixture, actor int, definition string) coreitem.Instance {
	t.Helper()
	inv, err := f.invs.FindByCharacterID(context.Background(), f.actors[actor].ID)
	if err != nil {
		t.Fatal(err)
	}
	tool, err := coreitem.NewInstance(definition, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := inv.Add(tool); err != nil {
		t.Fatal(err)
	}
	if err := f.invs.Save(context.Background(), inv); err != nil {
		t.Fatal(err)
	}
	return tool
}

func travelCommand(t *testing.T, f *normalFixture, run string, actor int, tool coreitem.Instance, input string) adventure.RunCommand {
	t.Helper()
	r, err := f.store.FindRun(context.Background(), run)
	if err != nil || r == nil {
		t.Fatal(r, err)
	}
	f.clock.add(3 * time.Second)
	a := f.actors[actor]
	return adventure.RunCommand{RunID: run, ID: id.New(), PlayerID: a.PlayerID, ActorID: a.ID, Version: r.Version, Action: "travel", TargetID: tool.ID, Input: input}
}

func TestCombatTravelSQLReceiptRollbackAndTryConversion(t *testing.T) {
	f := newNormalFixture(t, 1)
	installTravel(t, f)
	tool := addTravelTool(t, f, 0, "item-107")
	entry := f.enter(t, "try")
	run := entry.Command.RunID
	f.send(t, run, 0, "earn", "")
	before, err := f.store.FindRun(context.Background(), run)
	if err != nil {
		t.Fatal(err)
	}
	cmd := travelCommand(t, f, run, 0, tool, `{}`)
	f.store.failWrite.Store(true)
	if _, err := f.svc.Command(context.Background(), cmd); err == nil {
		t.Fatal("SQL failure accepted")
	}
	after, err := f.store.FindRun(context.Background(), run)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("rollback changed run", err)
	}
	inv, err := f.invs.FindByCharacterID(context.Background(), cmd.ActorID)
	if _, ok := inv.Find(tool.ID); err != nil || !ok {
		t.Fatal("rollback consumed tool", err)
	}
	f.store.failWrite.Store(false)
	f.store.loseCommit.Store(true)
	first, err := f.svc.Command(context.Background(), cmd)
	if err != nil || first.Floor != 10 || first.StageID != before.Stage.ID {
		t.Fatal(first, err)
	}
	f.restart(t)
	calls := f.rng.calls.Load()
	again, err := f.svc.Command(context.Background(), cmd)
	if err != nil || !reflect.DeepEqual(first, again) || calls != f.rng.calls.Load() {
		t.Fatal("receipt replay changed facts", err)
	}
	inv, err = f.invs.FindByCharacterID(context.Background(), cmd.ActorID)
	if _, ok := inv.Find(tool.ID); err != nil || ok {
		t.Fatal("tool not consumed", err)
	}
	f.clock.add(3 * time.Second)
	advance := cmd
	advance.ID, advance.Version, advance.Action, advance.TargetID, advance.Input = "treasure", first.Version, "advance", "", ""
	created, err := f.svc.Command(context.Background(), advance)
	if err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	if retry, err := f.svc.Command(context.Background(), advance); err != nil || !reflect.DeepEqual(created, retry) {
		t.Fatal(retry, err)
	}
	r, err := f.store.FindRun(context.Background(), run)
	if err != nil || r.Floor != 11 || len(r.Enemies) != 0 || r.ClearPending || r.StageCleared || r.Mode != "normal" || r.Recipients[cmd.ActorID].Pending.Gold != 7 || r.Recipients[cmd.ActorID].Conditional.Gold != 0 {
		t.Fatal(r, err)
	}
	f.send(t, run, 0, "exit", "")
	c, err := f.chars.FindByID(context.Background(), cmd.ActorID)
	if err != nil || c.Money != f.actors[0].Money+7 || c.Tired != 34 {
		t.Fatal(c, err)
	}
}

func TestCombatTravelSQL150GeneratedAndOldResponse(t *testing.T) {
	for _, ready := range []bool{false, true} {
		t.Run(map[bool]string{false: "generated", true: "old-response"}[ready], func(t *testing.T) {
			f := newNormalFixture(t, 1)
			port := installTravel(t, f)
			tool := addTravelTool(t, f, 0, "item-150")
			run := f.enter(t, "normal").Command.RunID
			f.send(t, run, 0, "advance", "")
			before, err := f.store.FindRun(context.Background(), run)
			if err != nil {
				t.Fatal(err)
			}
			if !ready {
				if err := f.store.WithRunForUpdate(context.Background(), run, false, func(ctx context.Context) error {
					r, err := f.store.FindRun(ctx, run)
					if err != nil {
						return err
					}
					for _, enemy := range r.Enemies {
						r.LastActions[enemy] = f.clock.Now().Add(time.Hour)
					}
					r.Version++
					return f.store.SaveRun(ctx, *r, r.Version-1)
				}); err != nil {
					t.Fatal(err)
				}
			}
			cmd := travelCommand(t, f, run, 0, tool, `{"floor":10}`)
			f.store.failWrite.Store(true)
			if _, err := f.svc.Command(context.Background(), cmd); err == nil {
				t.Fatal("failed SQL accepted")
			}
			f.store.failWrite.Store(false)
			f.restart(t)
			if _, err := f.svc.Command(context.Background(), cmd); err != nil {
				t.Fatal(err)
			}
			r, err := f.store.FindRun(context.Background(), run)
			if err != nil {
				t.Fatal(err)
			}
			if (r.Enemies[0] == before.Enemies[0]) != ready || !r.ClearPending || r.Floor != 10 || len(r.EncounterPool) == 0 {
				t.Fatal("wrong persisted roster/generation flags", r)
			}
			data, _ := json.Marshal(r)
			calls := f.rng.calls.Load()
			responses := port.responses.Load()
			f.restart(t)
			if _, err := f.svc.Command(context.Background(), cmd); err != nil {
				t.Fatal(err)
			}
			loaded, err := f.store.FindRun(context.Background(), run)
			if err != nil {
				t.Fatal(err)
			}
			again, _ := json.Marshal(loaded)
			if string(data) != string(again) || calls != f.rng.calls.Load() || responses != port.responses.Load() {
				t.Fatal("replay repeated generation/response")
			}
		})
	}
}

func TestCombatTravelSQLNoEffectSkipAndRejection(t *testing.T) {
	for _, scenario := range []string{"no-effect", "skip", "invalid"} {
		t.Run(scenario, func(t *testing.T) {
			f := newNormalFixture(t, 1)
			port := installTravel(t, f)
			definition := "item-083"
			if scenario == "invalid" {
				definition = "item-150"
			}
			tool := addTravelTool(t, f, 0, definition)
			run := f.enter(t, "normal").Command.RunID
			f.send(t, run, 0, "travel10", "")
			port.skip = scenario == "skip"
			before, err := f.store.FindRun(context.Background(), run)
			if err != nil {
				t.Fatal(err)
			}
			cmd := travelCommand(t, f, run, 0, tool, `{}`)
			_, err = f.svc.Command(context.Background(), cmd)
			if (err != nil) != (scenario == "invalid") {
				t.Fatal(err)
			}
			after, readErr := f.store.FindRun(context.Background(), run)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if after.Floor != 10 || scenario == "invalid" && !reflect.DeepEqual(before, after) {
				t.Fatal("wrong no-effect/rejection", after)
			}
			inv, err := f.invs.FindByCharacterID(context.Background(), cmd.ActorID)
			if err != nil {
				t.Fatal(err)
			}
			_, exists := inv.Find(tool.ID)
			if exists != (scenario != "no-effect") {
				t.Fatal("wrong consumption", scenario)
			}
		})
	}
}

func TestCombatTravelSQLPairedTravelAndExit(t *testing.T) {
	f := newNormalFixture(t, 2)
	installTravel(t, f)
	tools := []coreitem.Instance{addTravelTool(t, f, 0, "item-107"), addTravelTool(t, f, 1, "item-107")}
	_, lobby := f.waitingParty(t, "normal")
	leader := f.actors[0]
	entry, err := f.svc.EnterParty(context.Background(), leader.PlayerID, leader.ID, lobby)
	if err != nil {
		t.Fatal(err)
	}
	run := entry.Command.RunID
	commands := []adventure.RunCommand{travelCommand(t, f, run, 0, tools[0], `{}`), travelCommand(t, f, run, 1, tools[1], `{}`)}
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
	remaining := 0
	for _, actor := range f.actors {
		inv, err := f.invs.FindByCharacterID(context.Background(), actor.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range inv.Items {
			remaining += item.Quantity
		}
	}
	if remaining != 1 {
		t.Fatal("competing commands consumed wrong quantity", remaining)
	}
	// Next version races the surviving tool command against the other actor's exit.
	r, err := f.store.FindRun(context.Background(), run)
	if err != nil {
		t.Fatal(err)
	}
	inv, err := f.invs.FindByCharacterID(context.Background(), leader.ID)
	if err != nil {
		t.Fatal(err)
	}
	owner := 0
	if _, ok := inv.Find(tools[0].ID); !ok {
		owner = 1
	}
	travel := travelCommand(t, f, run, owner, tools[owner], `{}`)
	other := f.actors[1-owner]
	exit := adventure.RunCommand{RunID: run, ID: "racing-exit", PlayerID: other.PlayerID, ActorID: other.ID, Version: r.Version, Action: "exit"}
	stats = testutil.RunConcurrentStressTest(t, testutil.GetStressConfig(), func(worker, op int) error {
		cmd := travel
		if (worker+op)%2 == 1 {
			cmd = exit
		}
		_, err := f.svc.Command(context.Background(), cmd)
		if errors.Is(err, adventure.ErrRunConflict) {
			return nil
		}
		return err
	})
	if stats.Failures != 0 {
		t.Fatal(stats)
	}
	r, err = f.store.FindRun(context.Background(), run)
	if err != nil || r == nil {
		t.Fatal(r, err)
	}
	if r.Version != travel.Version+1 || !slices.Contains(r.Roster, f.actors[owner].ID) {
		t.Fatal("race lost surviving roster", r)
	}
	inv, err = f.invs.FindByCharacterID(context.Background(), f.actors[owner].ID)
	if err != nil {
		t.Fatal(err)
	}
	_, exists := inv.Find(tools[owner].ID)
	if exists != (len(r.Roster) == 1) {
		t.Fatal("exit race duplicated/lost a tool", r)
	}
}

func TestCombatTravelSQLTreasureEncounterSurvivesCoordinates(t *testing.T) {
	f := newNormalFixture(t, 1)
	port := installTravel(t, f)
	key := addTravelTool(t, f, 0, "item-107")
	book := addTravelTool(t, f, 0, "item-150")
	run := f.enter(t, "normal").Command.RunID
	f.send(t, run, 0, "travel10", "")
	f.send(t, run, 0, "advance", "")
	before, err := f.store.FindRun(context.Background(), run)
	if err != nil || len(before.Boxes) == 0 {
		t.Fatal(before, err)
	}
	cmd := travelCommand(t, f, run, 0, key, `{}`)
	if _, err := f.svc.Command(context.Background(), cmd); err != nil {
		t.Fatal(err)
	}
	after, err := f.store.FindRun(context.Background(), run)
	if err != nil || after.Floor != 10 || !reflect.DeepEqual(before.Boxes, after.Boxes) || port.responses.Load() != 0 {
		t.Fatal("treasure encounter lost", after, err)
	}
	f.clock.add(3 * time.Second)
	advance := cmd
	advance.Action, advance.TargetID, advance.Input, advance.ID, advance.Version = "advance", "", "", "blocked-advance", after.Version
	if _, err := f.svc.Command(context.Background(), advance); !errors.Is(err, adventure.ErrInvalidFloorAction) {
		t.Fatal("unopened treasure was treated as no enemies", err)
	}
	if _, err := f.svc.Command(context.Background(), travelCommand(t, f, run, 0, book, `{"floor":1}`)); err != nil {
		t.Fatal(err)
	}
	after, err = f.store.FindRun(context.Background(), run)
	if err != nil || len(after.Boxes) != 0 || len(after.Enemies) == 0 {
		t.Fatal("tool150 retained replaced chests", after, err)
	}
}
