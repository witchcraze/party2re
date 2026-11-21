package adventure_test

import (
	"context"
	"errors"
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

type treasureClaimCombat struct {
	*normalCombat
	f         *normalFixture
	facts     []corebattle.InspectionFact
	responses atomic.Int64
}

func (c *treasureClaimCombat) ExecuteRunAction(ctx context.Context, r *adventure.NormalRun, cmd adventure.RunCommand) (adventure.RunActionEffects, error) {
	if cmd.Action != "inspect-valuation" && cmd.Action != "strike-nonlethal" && cmd.Action != "strike-lethal" {
		return c.normalCombat.ExecuteRunAction(ctx, r, cmd)
	}
	fx := adventure.RunActionEffects{RoomActivity: true, ActorActivity: true, ReleaseStock: true}
	state, frame, inspection, err := r.ProjectCombat(cmd.ActorID)
	if err != nil {
		return fx, err
	}
	requester, err := c.f.chars.FindByID(ctx, cmd.ActorID)
	if err != nil {
		return fx, err
	}
	level := requester.Level
	rewards := adventure.NewRunDefeatRewards(r, requester, "", false, c.f.rng, &fx)
	reset := corebattle.EquipmentResetContext{TargetContext: frame, RequesterLevel: &level}
	defeat := corebattle.DefeatContext{EquipmentResetContext: reset, Rewards: rewards}
	damage := corebattle.DamageContext{
		EffectContext: corebattle.EffectContext{TargetContext: frame, RequesterLevel: &level},
		GuaranteedHit: true,
		Defeat: func(id string) error {
			_, err := state.ApplyDefeat(&defeat, id, c.f.rng)
			return err
		},
		Reset: func(id string) error {
			return state.ApplyEquipmentReset(&reset, id, c.f.rng)
		},
	}
	tools := &corebattle.OrdinaryToolContext{DamageContext: damage, OrdinaryToolInput: corebattle.OrdinaryToolInput{Inspection: inspection}}
	var defs skill.BattleDefinitions
	defs.RegisterPlayerCallbacks(skill.PlayerCallbackContexts{Tools: tools}, len(r.Roster))

	var op corebattle.PlayerOperation
	op.Tail = corebattle.PlayerTailContext{DamageContext: damage, DefeatContext: &defeat}
	op.ResponseEnabled = r.Floor != adventure.TreasureRoomFloor && r.Floor != adventure.EXTreasureFloor

	switch cmd.Action {
	case "inspect-valuation":
		parsed, err := r.ParseNormalOperation(adventure.RunCommand{ActorID: cmd.ActorID, Input: "＠トレハンのかん>" + cmd.TargetID}, adventure.NormalOperationContext{Definitions: defs, Callback: corebattle.CallbackContext{Damage: &damage, Inspection: inspection}, Requester: &requester})
		if err != nil {
			return fx, err
		}
		op.First = parsed.First.PlayerAction
		op.First.Validate = func(s *corebattle.LifetimeState) error {
			f := corebattle.CallbackContext{TargetContext: frame, Arguments: parsed.First.Arguments, Inspection: inspection}
			return s.ValidateInvocations(&f, defs.Invocations, []corebattle.CallbackInvocation{parsed.First.Invocation}, c.f.rng)
		}
		op.First.Execute = func(s *corebattle.LifetimeState, rng random.Generator) (corebattle.PlayerActionFacts, error) {
			f := corebattle.CallbackContext{TargetContext: frame, Arguments: parsed.First.Arguments, Inspection: inspection}
			if err := s.ApplyInvocations(&f, defs.Invocations, []corebattle.CallbackInvocation{parsed.First.Invocation}, rng); err != nil {
				return corebattle.PlayerActionFacts{}, err
			}
			for _, res := range tools.Results {
				c.facts = append(c.facts, res.Facts...)
			}
			return corebattle.PlayerActionFacts{}, nil
		}
	case "strike-nonlethal", "strike-lethal":
		power := 1200.0
		cat := corebattle.CategoryPhysical
		if cmd.Action == "strike-lethal" {
			power = 5000.0
			cat = corebattle.CategoryBreath
		}
		op.First = corebattle.PlayerAction{
			EntryID: "attack",
			Validate: func(s *corebattle.LifetimeState) error {
				d := damage
				return s.ValidateDamage(&d, corebattle.TargetEffect{TargetID: cmd.TargetID, Category: cat, Payload: corebattle.CallbackPayload{Number: power}, Arguments: []string{"1"}}, c.f.rng)
			},
			Execute: func(s *corebattle.LifetimeState, rng random.Generator) (corebattle.PlayerActionFacts, error) {
				d := damage
				_, err := s.ApplyDamage(&d, corebattle.TargetEffect{TargetID: cmd.TargetID, Category: cat, Payload: corebattle.CallbackPayload{Number: power}, Arguments: []string{"1"}}, rng)
				return corebattle.PlayerActionFacts{DamageCalls: 1}, err
			},
		}
	}

	res, err := state.ApplyPlayerOperation(&op, c.f.rng)
	if err != nil {
		return fx, err
	}
	if err := r.ReconcileCombat(state, res.Target); err != nil {
		return fx, err
	}
	if res.ResponseReached {
		if npc := r.NextNPC(c.f.clock.Now()); npc != "" {
			c.responses.Add(1)
		}
	}
	return fx, nil
}

func TestNormalRunTreasureCombatInspectionClaimAndReload(t *testing.T) {
	for _, lethal := range []bool{false, true} {
		name := "nonlethal-then-claim"
		if lethal {
			name = "lethal-defeat-no-grant"
		}
		t.Run(name, func(t *testing.T) {
			f := newNormalFixture(t, 2)
			combat := &treasureClaimCombat{normalCombat: f.combat, f: f}
			f.cfg.Combat = combat
			f.restart(t)

			_, lobby := f.waitingParty(t, "normal")
			a := f.actors[0]
			entry, err := f.svc.EnterParty(context.Background(), a.PlayerID, a.ID, lobby)
			if err != nil {
				t.Fatal(err)
			}
			runID := entry.Command.RunID
			f.send(t, runID, 0, "travel10", "")
			f.send(t, runID, 0, "advance", "")

			var unopenedID, unopenedName, openedID string
			if err := f.store.WithRunForUpdate(context.Background(), runID, false, func(ctx context.Context) error {
				r, err := f.store.FindRun(ctx, runID)
				if err != nil || len(r.Boxes) < 2 {
					t.Fatal(r, err)
				}
				r.Boxes[0].ItemID = "item-001"
				r.Boxes[1].OpenedBy = f.actors[1].ID
				r.Boxes[1].ItemID = "item-002"
				for i := range r.Battle.Actors {
					if r.Battle.Actors[i].ID == a.ID {
						r.Battle.Actors[i].Jobs = corebattle.BattleJobs{JobID: "job-78", SP: 50}
					}
				}
				unopenedID, unopenedName, openedID = r.Boxes[0].ID, r.Boxes[0].Name, r.Boxes[1].ID
				r.Version++
				return f.store.SaveRun(ctx, *r, r.Version-1)
			}); err != nil {
				t.Fatal(err)
			}

			// Inspect unopened box by legacy name and parse opened/missing controls.
			r, err := f.store.FindRun(context.Background(), runID)
			if err != nil {
				t.Fatal(err)
			}
			if r.NextNPC(f.clock.Now().Add(time.Minute)) != "" {
				t.Fatal("treasure room allowed NPC response")
			}
			f.send(t, runID, 0, "inspect-valuation", unopenedName)
			if len(combat.facts) != 1 || combat.facts[0].Kind != "valuation" || combat.facts[0].TargetID != unopenedID {
				t.Fatalf("inspection failed: %+v", combat.facts)
			}

			if !lethal {
				f.send(t, runID, 0, "strike-nonlethal", unopenedID)
				f.restart(t)
				reloaded, err := f.store.FindRun(context.Background(), runID)
				if err != nil {
					t.Fatal(err)
				}
				state, frame, _, err := reloaded.ProjectCombat(a.ID)
				if err != nil || !slices.Equal(frame.EnemyIDs, []string{unopenedID}) || len(state.Actors) != 3 ||
					reloaded.Boxes[0].Combat == nil || reloaded.Boxes[0].Combat.HP >= 1000 {
					t.Fatalf("reloaded projected chest wrong: frame=%+v box=%+v err=%v", frame, reloaded.Boxes[0], err)
				}

				// Parse legacy shiraberu text and opened-box rejection before claimChest.
				if _, err := reloaded.ParseNormalOperation(adventure.RunCommand{ActorID: a.ID, Action: "chest", TargetID: openedID}, adventure.NormalOperationContext{}); !errors.Is(err, adventure.ErrNoUnopenedTreasure) {
					t.Fatalf("opened chest admitted by parse: %v", err)
				}
				parsed, err := reloaded.ParseNormalOperation(adventure.RunCommand{ActorID: a.ID, Input: "＠しらべる>" + unopenedName}, adventure.NormalOperationContext{})
				if err != nil || parsed.First.TargetID != unopenedID {
					t.Fatalf("legacy shiraberu parse failed: %+v err=%v", parsed, err)
				}

				f.clock.add(3 * time.Second)
				claimCmd := adventure.RunCommand{PlayerID: a.PlayerID, ActorID: a.ID, RunID: runID, ID: id.New(), Version: reloaded.Version, Action: "chest", TargetID: parsed.First.TargetID}
				receipt, err := f.svc.Command(context.Background(), claimCmd)
				if err != nil || receipt.Box == nil || receipt.Box.OpenedBy != a.ID {
					t.Fatalf("claim failed: %+v err=%v", receipt, err)
				}
				// Replay preserves single delivery.
				again, err := f.svc.Command(context.Background(), claimCmd)
				if err != nil || again.Box.ID != receipt.Box.ID {
					t.Fatalf("replay failed: %+v err=%v", again, err)
				}
				if got := countItem(t, f, a.ID, "item-001"); got != 1 {
					t.Fatalf("expected single delivery of item-001, got %d", got)
				}
				afterClaim, err := f.store.FindRun(context.Background(), runID)
				if err != nil {
					t.Fatal(err)
				}
				if _, nextFrame, _, err := afterClaim.ProjectCombat(a.ID); err != nil || len(nextFrame.EnemyIDs) != 0 {
					t.Fatalf("claimed box stayed in opposing pool: %+v err=%v", nextFrame, err)
				}
			} else {
				f.send(t, runID, 0, "strike-lethal", unopenedID)
				f.restart(t)
				reloaded, err := f.store.FindRun(context.Background(), runID)
				if err != nil {
					t.Fatal(err)
				}
				if !reloaded.Boxes[0].Defeated || reloaded.Boxes[0].OpenedBy != "" {
					t.Fatalf("defeated box state wrong: %+v", reloaded.Boxes[0])
				}
				_, frame, _, err := reloaded.ProjectCombat(a.ID)
				if err != nil || len(frame.EnemyIDs) != 0 {
					t.Fatalf("defeated box still in opposing pool: %+v err=%v", frame, err)
				}
				if got := countItem(t, f, a.ID, "item-001"); got != 0 {
					t.Fatalf("lethal defeat leaked treasure item: %d", got)
				}
				afterChar, err := f.chars.FindByID(context.Background(), a.ID)
				if err != nil || afterChar.Experience != a.Experience || afterChar.Money != a.Money {
					t.Fatalf("lethal defeat leaked EXP/money: before=%+v after=%+v err=%v", a, afterChar, err)
				}
				f.clock.add(3 * time.Second)
				if _, err := f.svc.Command(context.Background(), adventure.RunCommand{PlayerID: a.PlayerID, ActorID: a.ID, RunID: runID, ID: id.New(), Version: reloaded.Version, Action: "chest", TargetID: unopenedID}); !errors.Is(err, adventure.ErrNoUnopenedTreasure) {
					t.Fatalf("defeated box claimable: %v", err)
				}
			}
			if combat.responses.Load() != 0 {
				t.Fatalf("unexpected NPC responses in treasure room: %d", combat.responses.Load())
			}
		})
	}
}
