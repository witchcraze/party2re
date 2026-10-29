package skill

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestMachineFamiliarTensionAdmissionReachability(t *testing.T) {
	for _, mode := range []string{
		"nonlethal", "supplied-level", "maximum-survives", "maximum-lethal",
		"lethal", "reflected-living", "reflected-lethal", "cover-lethal", "random-lethal",
		"retained", "consumed", "one-hit", "miss", "double-lethal", "shield-survives", "shield-lethal",
		"shield-pierced", "burn-lethal", "prior-damage-lethal", "reset", "random-modifier",
	} {
		t.Run(mode, func(t *testing.T) {
			s, c, d := machineFamiliarFixture()
			s.Actors[0].ItemDefinitionIDs = []string{"item-145"}
			s.Actors[1].HP, s.Actors[1].MaxHP = 10000, 10000
			c.RequesterLevel = nil
			reject, wantHP, wantTension, wantCalls := false, 9802, 3.0, 2
			// The recursive145 call does not roll for another recursion.
			draws := []float64{0, 0}
			args := []string{"enemy", "tail"}
			switch mode {
			case "supplied-level":
				level := 99
				c.RequesterLevel = &level
			case "maximum-survives":
				s.Actors[1].HP, wantHP = 265, 67
			case "maximum-lethal":
				s.Actors[1].HP, reject = 264, true
			case "lethal":
				s.Actors[1].HP, reject = 1, true
			case "reflected-living":
				s.Actors[1].Temporary, wantHP = corebattle.TemporaryReflectAttack, 10000
			case "reflected-lethal":
				s.Actors[1].Temporary = corebattle.TemporaryReflectAttack
				s.Actors[0].HP, reject = 1, true
			case "cover-lethal":
				other := s.Actors[1]
				other.ID, other.HP, other.Temporary = "coverer", 1, corebattle.TemporaryCovering
				s.Actors = append(s.Actors, other)
				c.EnemyIDs = append(c.EnemyIDs, other.ID)
				s.Actors[1].Temporary, reject = corebattle.TemporaryCover, true
			case "random-lethal":
				other := s.Actors[1]
				other.ID, other.HP = "other", 1
				s.Actors = append(s.Actors, other)
				c.EnemyIDs = append(c.EnemyIDs, other.ID)
				args[0], reject = "", true
			case "retained", "consumed":
				s.Actors[0].Tension = 3
				if mode == "retained" {
					s.Actors[0].ItemDefinitionIDs = append(s.Actors[0].ItemDefinitionIDs, "item-149")
					reject = true
				} else {
					wantHP = 9406
				}
			case "one-hit":
				s.Actors[1].Temporary = corebattle.TemporaryOneHitDefense
				draws, wantHP, wantTension, wantCalls = nil, 10000, 1, 0
			case "miss":
				c.GuaranteedHit, s.Actors[0].Hit = false, 0
				draws, wantHP, wantTension, wantCalls = []float64{.9}, 10000, 1, 0
			case "double-lethal":
				s.Actors[1].Temporary, s.Actors[1].HP, reject = corebattle.TemporaryDouble, 300, true
			case "shield-survives", "shield-lethal", "shield-pierced":
				s.Actors[1].ItemDefinitionIDs = []string{"item-162"}
				s.Actors[1].Defense, s.Actors[1].HP = 1000, 3
				draws, wantHP = []float64{0, 0, 0}, 2 // Shield forces indirect damage and the1–2 floor.
				if mode == "shield-lethal" {
					s.Actors[1].HP, reject = 2, true
				} else if mode == "shield-pierced" {
					s.Actors[0].ItemDefinitionIDs = append(s.Actors[0].ItemDefinitionIDs, "item-181")
					s.Actors[1].Defense, s.Actors[1].HP, reject = 100, 100, true
				}
			case "burn-lethal":
				s.Fields.Default, s.Actors[1].HP, reject = "burn", 300, true
			case "prior-damage-lethal":
				s.Actors[0].ItemDefinitionIDs = append(s.Actors[0].ItemDefinitionIDs, "item-239")
				c.DamageCalls, s.Actors[1].HP, reject = 3, 400, true
			case "reset":
				s.Actors[0].Extended, reject = corebattle.ExtendedPurification, true
			case "random-modifier":
				s.Actors[0].ItemDefinitionIDs = append(s.Actors[0].ItemDefinitionIDs, "weapon-35")
				s.Actors[1].HP, reject = 300, true // Optional×1.2 makes the hit lethal.
			}
			c.Reset = func(string) error { t.Fatal("reset during admission"); return nil }
			c.Defeat = func(string) error { t.Fatal("defeat during admission"); return nil }
			frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: args}
			before, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			contextBefore, frameBefore := *c, frame
			contextBefore.Reset, contextBefore.Defeat = nil, nil
			argsBefore := append([]string(nil), args...)
			levelBefore := 0
			if c.RequesterLevel != nil {
				levelBefore = *c.RequesterLevel
			}
			partyBefore, enemiesBefore := append([]string(nil), c.PartyIDs...), append([]string(nil), c.EnemyIDs...)
			ops, err := d.InvocationAction(&s, &frame, []string{"job-79-entry-4"}, noArithmeticDraws{t})
			if (err != nil) != reject || reject && !errors.Is(err, corebattle.ErrUnsupportedEffect) {
				t.Fatalf("discovery: reject=%v, err=%v", reject, err)
			}
			if reject {
				ops = []corebattle.CallbackInvocation{{Kind: "entry", EntryID: "job-79-entry-4"}}
				if err := s.ApplyInvocations(&frame, d.Invocations, ops, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
					t.Fatalf("submission: %v", err)
				}
			}
			after, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			// Inspect shared slices independently of the shallow context snapshots.
			contextAfter := *c
			contextAfter.Reset, contextAfter.Defeat = nil, nil
			if string(before) != string(after) || !reflect.DeepEqual(frameBefore, frame) ||
				!reflect.DeepEqual(partyBefore, c.PartyIDs) || !reflect.DeepEqual(enemiesBefore, c.EnemyIDs) ||
				!reflect.DeepEqual(args, argsBefore) ||
				!reflect.DeepEqual(contextBefore, contextAfter) || c.RequesterLevel != nil && *c.RequesterLevel != levelBefore {
				t.Fatal("admission changed state or context")
			}
			if reject {
				return
			}
			rng := &damageGemDraws{t: t, draws: draws}
			if err := s.ApplyInvocations(&frame, d.Invocations, ops, rng); err != nil {
				t.Fatal(err)
			}
			if s.Actors[1].HP != wantHP || s.Actors[0].Tension != wantTension || c.TensionCalls != wantCalls ||
				c.DamageCalls != 1 || rng.used != len(draws) || s.Actors[0].MP != 80 || s.Actors[0].CMP != 90 ||
				!reflect.DeepEqual(c.Arguments, []string{"tail"}) {
				t.Fatal(s, c, frame, rng.used)
			}
			if mode == "reflected-living" && s.Actors[0].HP != 302 {
				t.Fatal("damage did not use the returned target", s)
			}
		})
	}
}
