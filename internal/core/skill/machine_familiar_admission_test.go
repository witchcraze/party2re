package skill

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestMachineFamiliarAdmissionIsInertAndDefaultEmpty(t *testing.T) {
	_, _, catalog := machineFamiliarFixture()
	if len(catalog.Invocations.Entries) != 11 {
		t.Fatal(len(catalog.Invocations.Entries))
	}
	for id := range catalog.Invocations.Entries {
		for _, reject := range []bool{false, true} {
			s, c, d := machineFamiliarFixture()
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
			if string(before) != string(after) || !reflect.DeepEqual(original, f) || c.Arguments != nil || c.DamageCalls != 0 || c.HealCalls != 0 || c.StatDownCalls != 0 || c.StatUpCalls != 0 || c.TensionCalls != 0 || c.CureCalls != 0 || weapon != 7 || gear.WeaponAttack != &weapon || s.Actors[0].Gear != gear {
				t.Fatal(id, s, c, f)
			}
		}
	}
	s, c, _ := machineFamiliarFixture()
	f := corebattle.CallbackContext{TargetContext: c.TargetContext}
	var missing BattleDefinitions
	missing.RegisterMachineFamiliar(nil)
	for _, d := range []BattleDefinitions{{}, missing} {
		if _, err := d.InvocationAction(&s, &f, []string{"job-82-entry-5"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(err)
		}
	}
}

func TestMachineFamiliarRejectsCompleteClosureBeforeEffects(t *testing.T) {
	for _, bad := range []string{"damage", "reset", "song205", "stat201", "stat205", "later-target", "later-callback", "prior-effect"} {
		t.Run(bad, func(t *testing.T) {
			s, c, d := machineFamiliarFixture()
			ids := []string{"job-82-entry-5"}
			switch bad {
			case "damage":
				c.Defeat = nil
			case "reset":
				s.Actors[0].Extended = corebattle.ExtendedPurification
			case "song205":
				ids[0] = "job-81-entry-4"
				s.Actors[0].ItemDefinitionIDs = []string{"item-205"}
			case "stat201", "stat205":
				ids[0] = "job-82-entry-3"
				s.Actors[0].ItemDefinitionIDs = []string{"item-" + bad[4:]}
			case "later-target":
				ids[0] = "job-82-entry-4"
				s.Actors[1].ItemDefinitionIDs = []string{"item-195"}
				s.Actors[2].HP, s.Actors[2].Gear = 100, nil
			case "later-callback":
				ids = []string{"job-79-entry-0", "unknown"}
			case "prior-effect":
				ids = []string{"job-82-entry-3", "job-82-entry-5"}
			}
			before, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			ops := make([]corebattle.CallbackInvocation, len(ids))
			for i, id := range ids {
				ops[i] = corebattle.CallbackInvocation{Kind: "entry", EntryID: id}
			}
			f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
			if err := s.ApplyInvocations(&f, d.Invocations, ops, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
				t.Fatal(err)
			}
			after, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(after) || c.DamageCalls != 0 || c.StatUpCalls != 0 || c.Arguments != nil {
				t.Fatal(s, c)
			}
		})
	}
}

func TestMachineFamiliarFullTablesKeepOtherOwners(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		s, c, d := machineFamiliarFixture()
		forms := &corebattle.TransformationContext{EffectContext: c.EffectContext, RequesterGender: c.RequesterGender}
		if reverse {
			d = BattleDefinitions{}
			d.RegisterTransformations(forms)
			d.RegisterMachineFamiliar(c)
		} else {
			d.RegisterTransformations(forms)
		}
		for job, length := range map[int]int{79: 5, 81: 5, 82: 7} {
			for _, entries := range d.Invocations.JobTables[job].Variants {
				if len(entries) != length {
					t.Fatal(job, entries)
				}
			}
		}
		for _, id := range []string{"job-79-breath", "job-79-speed", "job-82-form", "job-82-entry-1"} {
			if d.Invocations.Entries[id].Execute == nil {
				t.Fatal(id)
			}
		}
		f := corebattle.CallbackContext{TargetContext: c.TargetContext}
		selectVariant := d.Invocations.JobTables[79].Select
		s.Actors[0].Icon = "assets/images/placeholders/battle/forms.svg#winged-f"
		if selectVariant(&s, &f) != 1 {
			t.Fatal("variant did not use requester gender")
		}
		s.Actors[0].Icon = "assets/images/placeholders/battle/forms.svg#winged-m"
		if selectVariant(&s, &f) != 0 {
			t.Fatal("variant used actor appearance gender as requester")
		}
		// No table may silently drop the time, inspection, wallet or generated
		// positions while their complete callback bindings remain unprovided.
		for _, id := range []string{"job-79-entry-2", "job-81-entry-1", "job-81-entry-2", "job-82-entry-0"} {
			if _, err := d.InvocationAction(&s, &f, []string{id}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
				t.Fatal(id, err)
			}
		}
		for _, job := range []string{"job-79", "job-81", "job-82"} {
			s.Actors[0].Jobs.JobID = job
			d.Invocations.Entries["tool"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "tool-84"}}
			if _, err := d.InvocationAction(&s, &f, []string{"tool"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
				t.Fatal(job, err)
			}
		}
	}
}

func TestMachineFamiliarSongEarlier205TailChangesLaterEligibility(t *testing.T) {
	s, c, d := machineFamiliarFixture()
	level := 99
	c.RequesterLevel = &level
	s.Actors[0].ItemDefinitionIDs = []string{"item-205"}
	s.Actors[0].Temporary = corebattle.TemporaryCovering
	s.Actors[2].HP, s.Actors[2].MaxHP = 100, 1000
	s.Actors[2].Temporary = corebattle.TemporaryCover
	s.Actors[2].BaseStats = corebattle.BattleBaseStats{Attack: 50, Defense: 40}
	s.Actors[2].Attack, s.Actors[2].Defense = 50, 40
	executeMachineFamiliar(t, &s, c, d, "job-81-entry-4", nil, .25, 0, 0)
	// Self DF triggers205, replacing covering with double. Later cover has no
	// covering member left and buffs/heals the ally instead of redirecting.
	if s.Actors[0].Temporary != corebattle.TemporaryDouble || s.Actors[0].Attack != 136 || s.Actors[2].Attack != 67 || s.Actors[2].Defense != 48 || s.Actors[2].HP != 235 || c.TensionCalls != 1 {
		t.Fatal(s, c)
	}
}

func TestMachineFamiliarEveryPositionHasAcceptedNoEffect(t *testing.T) {
	_, _, catalog := machineFamiliarFixture()
	for id := range catalog.Invocations.Entries {
		t.Run(id, func(t *testing.T) {
			s, c, d := machineFamiliarFixture()
			c.Defeat = nil
			s.Actors[1].Temporary, s.Actors[1].Gear = corebattle.TemporaryOneHitDefense, nil
			args, draws := []string{"enemy"}, []float64(nil)
			switch id {
			case "job-79-entry-0":
				s.Actors[1].MP = 20
			case "job-81-entry-3":
				draws = []float64{0}
			case "job-81-entry-4":
				s.Actors[0].HP = 0
			case "job-82-entry-1":
				draws = []float64{.5}
			case "job-82-entry-2":
				args[0], s.Actors[0].HP = "self", 0
			case "job-82-entry-3":
				s.Actors[0].Attack, s.Actors[0].Defense, s.Actors[0].Agility = 250, 200, 100
				draws = []float64{0}
			case "job-82-entry-4":
				draws = []float64{0, 0}
			}
			beforeHP, beforeMP := s.Actors[0].HP, s.Actors[1].MP
			executeMachineFamiliar(t, &s, c, d, id, args, draws...)
			if s.Actors[0].HP != beforeHP || s.Actors[0].MP != 80 || s.Actors[1].HP != 900 || s.Actors[1].MP != beforeMP || c.MPHealCalls != 0 || c.TensionCalls != 0 || c.HealCalls != 0 || c.CureCalls != 0 {
				t.Fatal(id, s, c)
			}
		})
	}
}
