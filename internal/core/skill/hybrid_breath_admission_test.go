package skill

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestHybridBreathAdmissionIsInert(t *testing.T) {
	_, _, catalog := hybridBreathFixture()
	if len(catalog.Invocations.Entries) != 22 {
		t.Fatal(len(catalog.Invocations.Entries))
	}
	for id := range catalog.Invocations.Entries {
		for _, reject := range []bool{false, true} {
			s, c, d := hybridBreathFixture()
			if reject {
				delete(d.Invocations.Entries, id)
			}
			weapon := 7
			gear := s.Actors[0].Gear
			gear.WeaponAttack = &weapon
			c.Reset = func(string) error { t.Fatal("external reset during admission"); return nil }
			c.Defeat = func(string) error { t.Fatal("external defeat during admission"); return nil }
			f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy", "tail"}}
			original := f
			before, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			_, err = d.InvocationAction(&s, &f, []string{id}, noArithmeticDraws{t})
			if (err != nil) != reject {
				t.Fatal(id, reject, err)
			}
			after, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(after) || !reflect.DeepEqual(original, f) || c.DamageCalls != 0 || c.AreaDamageCalls != 0 || c.StatUpCalls != 0 || c.HealCalls != 0 || c.StatusCalls != 0 || c.End.ImmediateRiskCalls != 0 || c.Arguments != nil || weapon != 7 || gear.WeaponAttack != &weapon || s.Actors[0].Gear != gear {
				t.Fatal(id, s, c, f)
			}
		}
	}
}

func TestHybridBreathRejectsMissingDescendantsBeforeMutation(t *testing.T) {
	for _, bad := range []string{"damage", "purification", "buff", "later-status", "later-area", "unknown-later"} {
		t.Run(bad, func(t *testing.T) {
			s, c, d := hybridBreathFixture()
			ids := []string{"job-40-entry-2"}
			switch bad {
			case "damage":
				c.Defeat = nil
			case "purification":
				s.Actors[0].Extended = corebattle.ExtendedPurification
			case "buff":
				ids[0] = "job-39-entry-1"
				s.Actors[0].ItemDefinitionIDs = []string{"item-205"}
			case "later-status":
				ids[0] = "job-41-entry-3"
				other := s.Actors[1]
				other.ID, other.Gear = "other", nil
				s.Actors = append(s.Actors, other)
				c.EnemyIDs = append(c.EnemyIDs, "other")
			case "later-area":
				s.Actors[1].ItemDefinitionIDs = []string{"item-195"}
				s.Actors[2].HP, s.Actors[2].Gear = 100, nil
				other := s.Actors[1]
				other.ID, other.ItemDefinitionIDs = "other", nil
				s.Actors = append(s.Actors, other)
				c.EnemyIDs = append(c.EnemyIDs, "other")
			case "unknown-later":
				ids = append(ids, "unknown")
			}
			before, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
			ops := make([]corebattle.CallbackInvocation, len(ids))
			for i, id := range ids {
				ops[i] = corebattle.CallbackInvocation{Kind: "entry", EntryID: id}
			}
			if err := s.ApplyInvocations(&f, d.Invocations, ops, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
				t.Fatal(err)
			}
			after, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(after) || c.DamageCalls != 0 || c.StatusCalls != 0 || c.StatUpCalls != 0 {
				t.Fatal(s, c)
			}
		})
	}
}

func TestHybridBreathDefaultEmptyAndSeparateRevivalOwner(t *testing.T) {
	s, c, d := hybridBreathFixture()
	f := corebattle.CallbackContext{TargetContext: c.TargetContext}
	var missingContext BattleDefinitions
	missingContext.RegisterHybridBreath(nil)
	for _, empty := range []BattleDefinitions{{}, missingContext} {
		if _, err := empty.InvocationAction(&s, &f, []string{"job-40-entry-0"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(err)
		}
	}
	s.Actors[0].Jobs.JobID = "job-39"
	d.Invocations.Entries["tool"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "tool-84"}}
	if _, err := d.InvocationAction(&s, &f, []string{"tool"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	revival := &corebattle.RevivalContext{TargetContext: c.TargetContext}
	d.RegisterRevivals(revival)
	bound := d.Invocations.Entries["job-39-entry-5"]
	d.RegisterHybridBreath(c)
	if reflect.ValueOf(bound.Execute).Pointer() != reflect.ValueOf(d.Invocations.Entries["job-39-entry-5"].Execute).Pointer() {
		t.Fatal("revival binding replaced")
	}
	// Eight positions retain the revival slot, even though this owner binds22.
	executeHybridBreath(t, &s, c, d, "tool", []string{"enemy"}, 7.0/8, 0)
	if s.Actors[0].MP != 80 || s.Actors[1].HP != 836 {
		t.Fatal(s)
	}
}

func TestHybridBreathEveryPositionHasAcceptedNoEffectBoundary(t *testing.T) {
	_, _, catalog := hybridBreathFixture()
	for id := range catalog.Invocations.Entries {
		t.Run(id, func(t *testing.T) {
			s, c, d := hybridBreathFixture()
			args := []string{"enemy", "tail"}
			c.Defeat = nil
			s.Actors[1].Temporary = corebattle.TemporaryOneHitDefense
			s.Actors[1].Gear = nil
			switch id {
			case "job-39-entry-1", "job-44-entry-1":
				s.Actors[0].Defense = 200
			case "job-39-entry-2", "job-44-entry-4":
				args[0], s.Actors[0].HP = "self", 1000
			case "job-39-entry-3":
				s.Actors[1].Defense = 16
			case "job-39-entry-4", "job-41-entry-3":
				s.Actors[1].HP = 1000
			}
			beforeSelf, beforeEnemy := s.Actors[0], s.Actors[1]
			draws := []float64(nil)
			if id == "job-39-entry-2" || id == "job-44-entry-4" {
				draws = []float64{0}
			}
			executeHybridBreath(t, &s, c, d, id, args, draws...)
			if s.Actors[0].HP != beforeSelf.HP || s.Actors[1].HP != beforeEnemy.HP || s.Actors[0].Defense != beforeSelf.Defense || s.Actors[1].Defense != beforeEnemy.Defense || c.End.ImmediateRiskCalls != 0 {
				t.Fatal(id, s, c)
			}
		})
	}
}
