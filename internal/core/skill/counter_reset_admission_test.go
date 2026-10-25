package skill

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestCounterResetAdmissionIsInert(t *testing.T) {
	_, _, catalog := counterResetFixture()
	if len(catalog.Invocations.Entries) != 18 {
		t.Fatal(len(catalog.Invocations.Entries))
	}
	for id := range catalog.Invocations.Entries {
		for _, reject := range []bool{false, true} {
			s, c, d := counterResetFixture()
			if reject {
				delete(d.Invocations.Entries, id)
			}
			weapon := 7
			gear := s.Actors[0].Gear
			gear.WeaponAttack = &weapon
			c.Reset = func(string) error { t.Fatal("external reset during admission"); return nil }
			c.Defeat = func(string) error { t.Fatal("external defeat during admission"); return nil }
			c.StatusDefeat.Rewards.Apply = func(*corebattle.LifetimeState, corebattle.TargetContext, string) error {
				t.Fatal("external rewards during admission")
				return nil
			}
			originalDefeat := *c.StatusDefeat
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
			if string(before) != string(after) || !reflect.DeepEqual(original, f) || !reflect.DeepEqual(originalDefeat.EquipmentResetContext, c.StatusDefeat.EquipmentResetContext) || c.DamageCalls != 0 || c.AreaDamageCalls != 0 || c.StatusCalls != 0 || c.DeathCalls != 0 || c.HealCalls != 0 || c.Arguments != nil || weapon != 7 || gear.WeaponAttack != &weapon || s.Actors[0].Gear != gear {
				t.Fatal(id, s, c, f)
			}
		}
	}
	// Exact deterministic projection also leaves the shared context untouched.
	s, c, d := counterResetFixture()
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
	if _, err := d.InvocationAction(&s, &f, []string{"job-36-entry-2", "job-35-entry-3"}, noArithmeticDraws{t}); err != nil {
		t.Fatal(err)
	}
	if s.Actors[0].Temporary != "" || c.Arguments != nil || c.DamageCalls != 0 {
		t.Fatal(s, c)
	}
}

func TestCounterResetRejectsMissingDescendantsBeforeMutation(t *testing.T) {
	for _, bad := range []string{"damage", "death", "death-reset", "later-status", "later-confused-status", "unknown-later"} {
		t.Run(bad, func(t *testing.T) {
			s, c, d := counterResetFixture()
			ids := []string{"job-35-entry-6"}
			switch bad {
			case "damage":
				c.Defeat = nil
			case "death":
				ids[0], c.StatusDefeat = "job-35-entry-2", nil
			case "death-reset":
				ids[0], s.Actors[1].Gear.AccessoryID = "job-35-entry-2", "item-110"
			case "later-status":
				s.Actors[1].Temporary, s.Actors[1].Gear = corebattle.TemporaryOneHitDefense, nil
			case "later-confused-status":
				s.Actors[1].ItemDefinitionIDs = []string{"item-195"}
				s.Actors[2].HP, s.Actors[2].Gear = 100, nil
			case "unknown-later":
				ids = []string{"job-36-entry-0", "unknown"}
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
			if string(before) != string(after) || c.DamageCalls != 0 || c.StatusCalls != 0 || c.DeathCalls != 0 || c.Arguments != nil {
				t.Fatal(s, c)
			}
		})
	}
	// Counted one-hit defense suppresses item239's unreachable status effect.
	s, c, d := counterResetFixture()
	s.Actors[0].ItemDefinitionIDs = []string{"item-239"}
	s.Actors[1].Temporary, s.Actors[1].Gear = corebattle.TemporaryOneHitDefense, nil
	c.Defeat = nil
	executeCounterReset(t, &s, c, d, "job-35-entry-6", nil, 0)
	if s.Actors[1].HP != 900 || s.Actors[1].Status != "" || c.DamageCalls != 1 || c.StatusCalls != 1 {
		t.Fatal(s, c)
	}
}

func TestCounterResetCompleteTablesKeepSeparateOwners(t *testing.T) {
	for _, counterFirst := range []bool{false, true} {
		s, c, d := counterResetFixture()
		d = BattleDefinitions{}
		if counterFirst {
			d.RegisterCounterReset(c)
		}
		status := &corebattle.StatusContext{EffectContext: c.EffectContext, Defeat: c.StatusDefeat}
		d.RegisterStatuses(status, &corebattle.EquipmentResetContext{})
		d.RegisterTransformations(&corebattle.TransformationContext{EffectContext: c.EffectContext})
		reset, copyBody := d.Invocations.Entries["job-35-entry-1"], d.Invocations.Entries["job-36-copy"]
		d.RegisterCounterReset(c)
		if reflect.ValueOf(reset.Execute).Pointer() != reflect.ValueOf(d.Invocations.Entries["job-35-entry-1"].Execute).Pointer() || reflect.ValueOf(copyBody.Execute).Pointer() != reflect.ValueOf(d.Invocations.Entries["job-36-copy"].Execute).Pointer() {
			t.Fatal("separate binding replaced")
		}
		for job, count := range map[int]int{35: 7, 36: 5, 37: 8} {
			if len(d.Invocations.JobTables[job].Variants[0]) != count {
				t.Fatal(job)
			}
		}
		s.Actors[0].Tension, s.Actors[1].Tension, s.Actors[2].Tension = 3, 5, 7
		executeCounterReset(t, &s, c, d, "job-35-entry-1", nil)
		if s.Actors[0].Tension != 3 || s.Actors[1].Tension != 5 || s.Actors[2].Tension != 7 || s.Actors[1].Attack != 50 || s.Actors[1].Defense != 40 {
			t.Fatal(s)
		}
	}
}

func TestCounterResetEmptyRegistrationAndIndirectCosts(t *testing.T) {
	s, c, d := counterResetFixture()
	f := corebattle.CallbackContext{TargetContext: c.TargetContext}
	var missingContext BattleDefinitions
	missingContext.RegisterCounterReset(nil)
	for _, empty := range []BattleDefinitions{{}, missingContext} {
		if _, err := empty.InvocationAction(&s, &f, []string{"job-35-entry-0"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(err)
		}
	}
	for _, job := range []int{35, 36, 37} {
		s, c, d = counterResetFixture()
		s.Actors[0].Jobs.JobID = fmt.Sprintf("job-%d", job)
		d.Invocations.Entries["tool"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "tool-84"}}
		//35.1/36.4 stay in their pools; a selected supported slot cannot hide
		// their missing bindings.37 rejects a missing final break similarly.
		delete(d.Invocations.Entries, "job-37-entry-7")
		if _, err := d.InvocationAction(&s, &f, []string{"tool"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(job, err)
		}
	}
	for _, kind := range []string{"tool-84", "random", "gem-86"} {
		s, c, d = counterResetFixture()
		s.Actors[0].Jobs.JobID, s.Actors[0].MP = "job-37", 0
		d.Invocations.Entries["indirect"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: kind}}
		draws := []float64{3.0 / 8}
		if kind == "random" {
			// Empty synthetic peer tables isolate this real job contribution;
			// they do not certify the unimplemented global random catalog.
			d.RegisterStatuses(&corebattle.StatusContext{EffectContext: c.EffectContext, Defeat: c.StatusDefeat}, &corebattle.EquipmentResetContext{})
			d.RegisterTransformations(&corebattle.TransformationContext{EffectContext: c.EffectContext})
			for job := 1; job <= 87; job++ {
				if job != 35 && job != 36 && job != 37 {
					d.Invocations.JobTables[job] = corebattle.CallbackTable{Variants: [][]corebattle.CallbackEntry{{}}}
				}
			}
			draws = []float64{36.0 / 87, 3.0 / 8}
		}
		if kind == "gem-86" {
			draws = nil
		}
		executeCounterReset(t, &s, c, d, "indirect", []string{"enemy", "tail"}, draws...)
		want := corebattle.TemporaryReduceMagic
		if kind == "gem-86" {
			want = ""
		} // No extension: handled without effect.
		if s.Actors[0].MP != 0 || s.Actors[0].Temporary != want {
			t.Fatal(kind, s)
		}
	}
}
