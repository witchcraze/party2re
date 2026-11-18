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
	"github.com/witchcraze/party2re/internal/core/random"
	"github.com/witchcraze/party2re/internal/core/skill"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/testutil"
)

func TestNormalRunPairedChestClaim(t *testing.T) {
	f := newNormalFixture(t, 2)
	_, lobby := f.waitingParty(t, "normal")
	a := f.actors[0]
	entry, err := f.svc.EnterParty(context.Background(), a.PlayerID, a.ID, lobby)
	if err != nil {
		t.Fatal(err)
	}
	runID := entry.Command.RunID
	f.send(t, runID, 0, "travel10", "")
	f.send(t, runID, 0, "advance", "")
	r, err := f.store.FindRun(context.Background(), runID)
	if err != nil || len(r.Boxes) == 0 {
		t.Fatal(r, err)
	}
	f.clock.add(3 * time.Second)
	stats := testutil.RunConcurrentStressTest(t, testutil.GetStressConfig(), func(worker, op int) error {
		c := f.actors[(worker+op)%2]
		_, err := f.svc.Command(context.Background(), adventure.RunCommand{PlayerID: c.PlayerID, ActorID: c.ID, RunID: runID, ID: "claim-" + c.ID, Version: r.Version, Action: "chest", TargetID: r.Boxes[0].ID})
		if errors.Is(err, adventure.ErrRunConflict) {
			return nil
		}
		return err
	})
	if stats.Failures != 0 {
		t.Fatal(stats)
	}
	total, inspections := 0, 0
	r, err = f.store.FindRun(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range f.actors {
		inv, err := f.invs.FindByCharacterID(context.Background(), c.ID)
		if err != nil {
			t.Fatal(err)
		}
		dep, err := f.depots.FindByCharacterID(context.Background(), c.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range append(inv.Items, dep.Items...) {
			total += item.Quantity
		}
		inspections += r.Recipients[c.ID].Inspected
	}
	if total != 1 || inspections != 1 || r.Boxes[0].OpenedBy == "" {
		t.Fatal("competing claims duplicated a chest", total, inspections)
	}
}

type treasureCombatPort struct {
	*normalCombat
	f         *normalFixture
	inspect   []corebattle.OrdinaryToolResult
	responses atomic.Int64
}

func (c *treasureCombatPort) ExecuteRunAction(ctx context.Context, r *adventure.NormalRun, cmd adventure.RunCommand) (adventure.RunActionEffects, error) {
	if cmd.Action != "attack" && cmd.Action != "job-78-entry-1" && cmd.Action != "job-16-entry-1" {
		return c.normalCombat.ExecuteRunAction(ctx, r, cmd)
	}
	fx := adventure.RunActionEffects{RoomActivity: true, ActorActivity: true, ReleaseStock: true}
	if c.f.clock.Now().Before(r.LastActions[cmd.ActorID].Add(time.Duration(r.Admission.Speed) * time.Second)) {
		return fx, adventure.ErrRunCooldown
	}
	state, frame, inspection, err := r.ProjectCombat(cmd.ActorID)
	if err != nil {
		return fx, err
	}
	ch, err := c.f.chars.FindByID(ctx, cmd.ActorID)
	if err != nil {
		return fx, err
	}
	level := ch.Level
	tools := &corebattle.OrdinaryToolContext{DamageContext: corebattle.DamageContext{EffectContext: corebattle.EffectContext{TargetContext: frame, RequesterLevel: &level}}}
	defeatCtx := corebattle.DefeatContext{
		EquipmentResetContext: corebattle.EquipmentResetContext{TargetContext: frame, RequesterLevel: &level},
		Rewards: &corebattle.DefeatRewards{
			Validate: func(*corebattle.LifetimeState, corebattle.TargetContext, string) error { return nil },
			Apply:    func(*corebattle.LifetimeState, corebattle.TargetContext, string) error { return nil },
		},
	}
	dmg := corebattle.DamageContext{
		EffectContext: corebattle.EffectContext{TargetContext: frame, RequesterLevel: &level},
		GuaranteedHit: true,
		Defeat:        func(id string) error { _, err := state.ApplyDefeat(&defeatCtx, id, c.f.rng); return err },
	}
	opCtx := adventure.NormalOperationContext{
		Callback: corebattle.CallbackContext{TargetContext: frame, Damage: &dmg, Defeat: &defeatCtx, Inspection: inspection},
	}
	opCtx.Definitions.RegisterPlayerCallbacks(skill.PlayerCallbackContexts{
		Tools: tools,
	}, len(r.Roster))
	parsed, err := r.ParseNormalOperation(cmd, opCtx)
	if err != nil {
		return fx, err
	}
	var action corebattle.PlayerAction
	if parsed.First.Action == "attack" {
		action = corebattle.PlayerAction{
			EntryID: "attack",
			Validate: func(s *corebattle.LifetimeState) error {
				return s.ValidateNormalAttack(&dmg, parsed.First.TargetID, c.f.rng)
			},
			Execute: func(s *corebattle.LifetimeState, rng random.Generator) (corebattle.PlayerActionFacts, error) {
				dmg.TargetContext = frame
				_, err := s.ApplyNormalAttack(&dmg, parsed.First.TargetID, rng)
				return corebattle.PlayerActionFacts{Target: dmg.TargetContext}, err
			},
		}
	} else {
		action = corebattle.PlayerAction{
			EntryID: parsed.First.EntryID,
			Cost:    parsed.First.Cost,
			Validate: func(s *corebattle.LifetimeState) error {
				f := opCtx.Callback
				f.Arguments = slices.Clone(parsed.First.Arguments)
				return s.ValidateInvocations(&f, opCtx.Definitions.Invocations, []corebattle.CallbackInvocation{parsed.First.Invocation}, c.f.rng)
			},
			Execute: func(s *corebattle.LifetimeState, rng random.Generator) (corebattle.PlayerActionFacts, error) {
				f := opCtx.Callback
				f.Arguments = slices.Clone(parsed.First.Arguments)
				err := s.ApplyInvocations(&f, opCtx.Definitions.Invocations, []corebattle.CallbackInvocation{parsed.First.Invocation}, rng)
				return corebattle.PlayerActionFacts{Target: f.TargetContext}, err
			},
		}
	}
	res, err := state.ApplyPlayerOperation(&corebattle.PlayerOperation{
		Tail:            corebattle.PlayerTailContext{DamageContext: dmg, DefeatContext: &defeatCtx},
		Field:           corebattle.NamedFieldContext{DamageContext: dmg, Death: &defeatCtx},
		First:           action,
		ResponseEnabled: r.Floor != adventure.TreasureRoomFloor && r.Floor != adventure.EXBossFloor+1,
	}, c.f.rng)
	if err != nil {
		return fx, err
	}
	if !res.Accepted {
		return fx, adventure.ErrRunConflict
	}
	r.ReconcileCombat(state, &frame)
	c.inspect = append(c.inspect, tools.Results...)
	if res.ResponseReached {
		if npc := r.NextNPC(c.f.clock.Now()); npc != "" {
			c.responses.Add(1)
		}
	}
	return fx, nil
}

func TestNormalRunTreasureCombatInspectionClaimAndReload(t *testing.T) {
	f := newNormalFixture(t, 2)
	port := &treasureCombatPort{normalCombat: f.combat, f: f}
	f.cfg.Combat = port
	f.restart(t)
	_, lobby := f.waitingParty(t, "normal")
	entry, err := f.svc.EnterParty(context.Background(), f.actors[0].PlayerID, f.actors[0].ID, lobby)
	if err != nil {
		t.Fatal(err)
	}
	runID := entry.Command.RunID
	f.send(t, runID, 0, "travel10", "")
	f.send(t, runID, 0, "advance", "")

	// Seed job-78/16 and high AT on actor 0 so partial then lethal hits and inspection can execute cleanly.
	err = f.store.WithRunForUpdate(context.Background(), runID, false, func(ctx context.Context) error {
		r, err := f.store.FindRun(ctx, runID)
		if err != nil {
			return err
		}
		r.Battle.Actors[0].Jobs = corebattle.BattleJobs{JobID: "job-78", SP: 20, OldJobID: "job-16", OldSP: 10}
		r.Battle.Actors[0].Attack = 1800
		r.Battle.Actors[0].MP = 100
		r.Version++
		return f.store.SaveRun(ctx, *r, r.Version-1)
	})
	if err != nil {
		t.Fatal(err)
	}
	f.restart(t)

	r, err := f.store.FindRun(context.Background(), runID)
	if err != nil || len(r.Boxes) < 2 {
		t.Fatalf("expected at least 2 treasure boxes, got %+v err=%v", r, err)
	}
	box1, box2 := r.Boxes[0], r.Boxes[1]

	// 1. Inspection operation succeeds with no loot and no NPC response.
	f.clock.add(3 * time.Second)
	a0, a1 := f.actors[0], f.actors[1]
	if _, err := f.svc.Command(context.Background(), adventure.RunCommand{PlayerID: a0.PlayerID, ActorID: a0.ID, RunID: runID, ID: id.New(), Version: r.Version, Action: "job-78-entry-1", TargetID: box1.ID}); err != nil {
		t.Fatal(err)
	}
	if len(port.inspect) != 1 || port.inspect[0].Facts[0].Kind != "valuation" || port.inspect[0].Facts[0].TargetID != box1.ID || port.responses.Load() != 0 {
		t.Fatalf("unexpected inspection result or NPC response: inspect=%+v responses=%d", port.inspect, port.responses.Load())
	}

	// 2. Non-lethal hostile hit reduces box-1 HP and survives reload.
	r, err = f.store.FindRun(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	f.clock.add(3 * time.Second)
	if _, err := f.svc.Command(context.Background(), adventure.RunCommand{PlayerID: a0.PlayerID, ActorID: a0.ID, RunID: runID, ID: id.New(), Version: r.Version, Action: "attack", TargetID: box1.ID}); err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	r, err = f.store.FindRun(context.Background(), runID)
	if err != nil || r.Boxes[0].Combat == nil || r.Boxes[0].Combat.HP >= 1000 || r.Boxes[0].Combat.HP <= 0 || len(r.Battle.Actors) != 2 {
		t.Fatalf("expected persisted partial combat damage on box-1: %+v err=%v", r, err)
	}

	// 3. Lethal hostile hit defeats box-1 without delivering its prize.
	err = f.store.WithRunForUpdate(context.Background(), runID, false, func(ctx context.Context) error {
		cur, err := f.store.FindRun(ctx, runID)
		if err != nil {
			return err
		}
		cur.Battle.Actors[0].Attack = 3000
		cur.Version++
		return f.store.SaveRun(ctx, *cur, cur.Version-1)
	})
	if err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	r, err = f.store.FindRun(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	f.clock.add(3 * time.Second)
	if _, err := f.svc.Command(context.Background(), adventure.RunCommand{PlayerID: a0.PlayerID, ActorID: a0.ID, RunID: runID, ID: id.New(), Version: r.Version, Action: "attack", TargetID: box1.ID}); err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	r, err = f.store.FindRun(context.Background(), runID)
	if err != nil || !r.Boxes[0].Defeated || r.Boxes[0].Combat != nil || r.Boxes[0].OpenedBy != "" || r.Boxes[0].DeliveredTo != "" {
		t.Fatalf("expected box-1 defeated without claim delivery: %+v err=%v", r.Boxes[0], err)
	}
	if _, frame, _, err := r.ProjectCombat(a0.ID); err != nil || slices.Contains(frame.EnemyIDs, box1.ID) || !slices.Contains(frame.EnemyIDs, box2.ID) {
		t.Fatalf("expected defeated box-1 removed from enemy projection: %+v err=%v", frame, err)
	}
	f.clock.add(3 * time.Second)
	if _, err := f.svc.Command(context.Background(), adventure.RunCommand{PlayerID: a0.PlayerID, ActorID: a0.ID, RunID: runID, ID: id.New(), Version: r.Version, Action: "chest", TargetID: box1.ID}); !errors.Is(err, adventure.ErrNoUnopenedTreasure) {
		t.Fatalf("expected defeated box-1 chest claim rejected, got %v", err)
	}

	// 4. Explicit claim opens box-2 once, removes it from enemy pool, and replay preserves single delivery.
	claimCmd := adventure.RunCommand{PlayerID: a1.PlayerID, ActorID: a1.ID, RunID: runID, ID: id.New(), Version: r.Version, Action: "chest", TargetID: box2.ID}
	firstReceipt, err := f.svc.Command(context.Background(), claimCmd)
	if err != nil || firstReceipt.Box == nil || firstReceipt.Box.OpenedBy != a1.ID || firstReceipt.Box.DeliveredTo == "" {
		t.Fatalf("explicit claim failed: receipt=%+v err=%v", firstReceipt, err)
	}
	f.restart(t)
	replayReceipt, err := f.svc.Command(context.Background(), claimCmd)
	if err != nil || !reflect.DeepEqual(firstReceipt, replayReceipt) {
		t.Fatalf("claim replay mismatch: first=%+v replay=%+v err=%v", firstReceipt, replayReceipt, err)
	}
	r, err = f.store.FindRun(context.Background(), runID)
	if err != nil || r.Boxes[1].OpenedBy != a1.ID {
		t.Fatalf("reloaded run lost box-2 claim: %+v err=%v", r, err)
	}
	if _, frame, _, err := r.ProjectCombat(a0.ID); err != nil || len(frame.EnemyIDs) != 0 {
		t.Fatalf("expected empty enemy pool after all boxes defeated/opened: %+v err=%v", frame, err)
	}
	totalItems := 0
	for _, actor := range f.actors {
		inv, err := f.invs.FindByCharacterID(context.Background(), actor.ID)
		if err != nil {
			t.Fatal(err)
		}
		dep, err := f.depots.FindByCharacterID(context.Background(), actor.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, it := range append(inv.Items, dep.Items...) {
			totalItems += it.Quantity
		}
	}
	if totalItems != 1 || port.responses.Load() != 0 {
		t.Fatalf("expected exactly 1 item delivered and 0 NPC responses, got items=%d responses=%d", totalItems, port.responses.Load())
	}
}
