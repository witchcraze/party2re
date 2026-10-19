package skill

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

func TestNonlethalFinalStrikeThroughRegisteredAndDelegatedInvocation(t *testing.T) {
	for _, id := range []string{"gem_instant_death_1", "delegated"} {
		t.Run(id, func(t *testing.T) {
			s, rc, d := rosterRegistryFixture()
			for i := range s.Actors {
				s.Actors[i].HP = 100
				s.Actors[i].Gear.AccessoryID = "item-212"
				s.Actors[i].ItemDefinitionIDs = []string{"item-212"}
			}
			s.Actors[1].BaseStats.Attack = 100
			c := corebattle.StatusContext{EffectContext: corebattle.EffectContext{TargetContext: rc.TargetContext}}
			c.Defeat = &corebattle.DefeatContext{EquipmentResetContext: corebattle.EquipmentResetContext{TargetContext: c.TargetContext}, Rewards: &corebattle.DefeatRewards{
				Validate: func(*corebattle.LifetimeState, corebattle.TargetContext, string) error { return nil },
				Apply:    func(*corebattle.LifetimeState, corebattle.TargetContext, string) error { return nil },
			}}
			d.RegisterStatuses(&c, nil)
			d.Invocations.Entries["delegated"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "tool-84"}}
			d.Invocations.JobTables[60] = corebattle.CallbackTable{Variants: [][]corebattle.CallbackEntry{{{ID: "gem_instant_death_1", Name: "death"}}}}
			frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
			before, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			ops, err := d.InvocationAction(&s, &frame, []string{id}, noArithmeticDraws{t})
			if err != nil {
				t.Fatal(err)
			}
			for _, ids := range [][]string{{"random"}, {"gem_cure_poison", id}} {
				d.Invocations.Entries["random"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "random"}}
				if _, err := d.InvocationAction(&s, &frame, ids, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
					t.Fatal(ids, err)
				}
			}
			after, err := json.Marshal(s)
			if err != nil || !reflect.DeepEqual(before, after) || c.DeathCalls != 0 {
				t.Fatal(s, err)
			}
			draws := []float64{0, 0, 0}
			if id == "delegated" {
				draws = append(draws, 0)
			}
			rng := &damageGemDraws{t: t, draws: draws}
			if err := s.ApplyInvocations(&frame, d.Invocations, ops, rng); err != nil || s.Actors[0].HP != 33 || s.Actors[1].HP != 0 || rng.used != len(draws) {
				t.Fatal(s, err, rng.used)
			}
		})
	}
}

func TestStatusRegistryConcreteBindingsAndWholeInvocationAdmission(t *testing.T) {
	s, rc, d := rosterRegistryFixture()
	s.Actors[1].HP = 100
	c := corebattle.StatusContext{EffectContext: corebattle.EffectContext{TargetContext: rc.TargetContext}}
	reset := corebattle.EquipmentResetContext{TargetContext: rc.TargetContext}
	if _, err := d.StatusAction(&s, &c, &reset, "job-35-entry-1", "", noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	d.RegisterStatuses(&c, &reset)
	if len(d.StatusSkills) != 18 || len(d.Invocations.JobTables) != 0 {
		t.Fatal(d)
	}
	for _, e := range corebattle.StatusEntries() {
		if d.Invocations.Entries[e.ID].Execute == nil || !d.StatusSkills[e.ID] {
			t.Fatal(e)
		}
	}
	e, err := d.StatusAction(&s, &c, &reset, "job-35-entry-1", "", noArithmeticDraws{t})
	if err != nil || e.RequiredSP != 30 || e.MPCost != 30 || e.Extended {
		t.Fatal(e, err)
	}
	d.Invocations.Entries["delegated"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "tool-84"}}
	d.Invocations.Entries["random"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "random"}}
	d.Invocations.JobTables[60] = corebattle.CallbackTable{Variants: [][]corebattle.CallbackEntry{{{ID: "gem_instant_death_1", Name: "death"}}}}
	frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy", "tail"}}
	before, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	for _, ids := range [][]string{{"gem_instant_death_1"}, {"delegated"}, {"random"}, {"gem_cure_poison", "gem_instant_death_1"}, {"tool-110"}} {
		if _, err := d.InvocationAction(&s, &frame, ids, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(ids, err)
		}
		var ops []corebattle.CallbackInvocation
		for _, id := range ids {
			ops = append(ops, corebattle.CallbackInvocation{Kind: "entry", EntryID: id})
		}
		if err := s.ApplyInvocations(&frame, d.Invocations, ops, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(ids, err)
		}
	}
	after, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) || c.DeathCalls != 0 || c.CureCalls != 0 {
		t.Fatal("rejected closure mutated state")
	}
	// A real registered body is used through delegation once its complete
	// descendant context is available, without ordinary MP/CMP charging.
	d.Invocations.JobTables[60] = corebattle.CallbackTable{Variants: [][]corebattle.CallbackEntry{{e.CallbackEntry}}}
	s.Actors[1].Status = corebattle.StatusSleep
	s.Actors[1].Tension = 5
	ops, err := d.InvocationAction(&s, &frame, []string{"delegated"}, noArithmeticDraws{t})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyInvocations(&frame, d.Invocations, ops, random.NewDeterministic(1)); err != nil || s.Actors[1].Status != "" || s.Actors[1].Tension != 5 {
		t.Fatal(s, err)
	}
}

func TestStatusRegistryReloadPreservesTargetArgumentConsumption(t *testing.T) {
	s, rc, d := rosterRegistryFixture()
	c := corebattle.StatusContext{EffectContext: corebattle.EffectContext{TargetContext: rc.TargetContext}}
	s.Actors[0].Status = corebattle.StatusSleep
	d.RegisterStatuses(&c, nil)
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var loaded corebattle.LifetimeState
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	var rebuilt BattleDefinitions
	rebuilt.RegisterStatuses(&c, nil)
	frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"self", "tail"}}
	ops, err := rebuilt.InvocationAction(&loaded, &frame, []string{"gem_cure_all_abnormal"}, noArithmeticDraws{t})
	if err != nil {
		t.Fatal(err)
	}
	if err := loaded.ApplyInvocations(&frame, rebuilt.Invocations, ops, noArithmeticDraws{t}); err != nil || loaded.Actors[0].Status != "" {
		t.Fatal(loaded, err)
	}
	// Inspect a naked nested frame directly: top-level custom slots each own
	// their argument frame, while nested explicit invocation consumes one.
	if err := rebuilt.Invocations.Entries["gem_cure_all_abnormal"].Execute(&loaded, &frame, noArithmeticDraws{t}); err != nil || len(frame.Arguments) != 1 || frame.Arguments[0] != "tail" {
		t.Fatal(frame, err)
	}
}
