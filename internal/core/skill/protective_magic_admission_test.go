package skill

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestProtectiveMagicAdmissionIsInert(t *testing.T) {
	_, _, catalog := protectiveMagicFixture()
	if len(catalog.Invocations.Entries) != 19 {
		t.Fatal(len(catalog.Invocations.Entries))
	}
	for id := range catalog.Invocations.Entries {
		for _, reject := range []bool{false, true} {
			s, c, d := protectiveMagicFixture()
			if reject {
				delete(d.Invocations.Entries, id)
			}
			weapon, level := 7, 99
			gear := s.Actors[0].Gear
			gear.WeaponAttack = &weapon
			c.RequesterLevel = &level
			c.Reset = func(string) error { t.Fatal("reset during admission"); return nil }
			c.Defeat = func(string) error { t.Fatal("defeat during admission"); return nil }
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
			if string(before) != string(after) || !reflect.DeepEqual(original, f) || c.DamageCalls != 0 || c.AreaDamageCalls != 0 || c.HealCalls != 0 || c.StatUpCalls != 0 || c.CureCalls != 0 || c.Arguments != nil || weapon != 7 || level != 99 || gear.WeaponAttack != &weapon || s.Actors[0].Gear != gear {
				t.Fatal(id, s, c, f)
			}
		}
	}
}

func TestProtectiveMagicRejectsCompleteAction(t *testing.T) {
	for _, bad := range []string{"damage", "group-second-helper", "symbol-conversion", "unknown-later", "unknown-transition"} {
		t.Run(bad, func(t *testing.T) {
			s, c, d := protectiveMagicFixture()
			ids := []string{"job-34-entry-6"}
			switch bad {
			case "damage":
				c.Defeat = nil
			case "group-second-helper":
				// The first helper redirects to a confused caster; the second
				// can hit that caster and requires its gear before ally debit.
				s.Actors[0].Status, s.Actors[0].Gear = corebattle.StatusConfusion, nil
			case "symbol-conversion":
				s.Actors[0].Extended, c.Command = corebattle.ExtendedMagicSword, "attack"
			case "unknown-later":
				ids = []string{"job-32-entry-2", "missing"}
			case "unknown-transition":
				ids = []string{"job-32-entry-0", "job-34-entry-6"}
			}
			before, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy", "tail"}}
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
			if string(before) != string(after) || c.Arguments != nil || c.DamageCalls != 0 {
				t.Fatal(s, c)
			}
		})
	}
}

func TestProtectiveMagicTablesKeepRevivalOwners(t *testing.T) {
	for _, protectiveFirst := range []bool{false, true} {
		s, c, _ := protectiveMagicFixture()
		d := BattleDefinitions{}
		if protectiveFirst {
			d.RegisterProtectiveMagic(c)
		}
		d.RegisterRevivals(&corebattle.RevivalContext{TargetContext: c.TargetContext})
		revive := d.Invocations.Entries["job-32-entry-4"]
		d.RegisterProtectiveMagic(c)
		if reflect.ValueOf(revive.Execute).Pointer() != reflect.ValueOf(d.Invocations.Entries["job-32-entry-4"].Execute).Pointer() {
			t.Fatal("revival binding replaced")
		}
		for job, count := range map[int]int{32: 6, 33: 8, 34: 7} {
			if len(d.Invocations.JobTables[job].Variants) != 1 || len(d.Invocations.JobTables[job].Variants[0]) != count {
				t.Fatal(job)
			}
		}
		s.Actors[2].MaxHP = 100
		executeProtectiveMagic(t, &s, c, d, "job-32-entry-4", nil)
		if s.Actors[2].HP != 15 {
			t.Fatal(s)
		}
		s.Actors[2].HP = 0
		executeProtectiveMagic(t, &s, c, d, "job-33-entry-6", []string{"fallen"})
		if s.Actors[2].HP != 100 {
			t.Fatal(s)
		}
	}
}

func TestProtectiveMagicEmptyAndIncompleteRegistration(t *testing.T) {
	s, c, _ := protectiveMagicFixture()
	f := corebattle.CallbackContext{TargetContext: c.TargetContext}
	var missingContext BattleDefinitions
	missingContext.RegisterProtectiveMagic(nil)
	for _, d := range []BattleDefinitions{{}, missingContext} {
		if _, err := d.InvocationAction(&s, &f, []string{"job-32-entry-2"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(err)
		}
	}
	for _, job := range []int{32, 33, 34} {
		s, c, d := protectiveMagicFixture()
		s.Actors[0].Jobs.JobID = fmt.Sprintf("job-%d", job)
		d.Invocations.Entries["tool"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "tool-84"}}
		if job == 34 {
			delete(d.Invocations.Entries, "job-34-entry-6")
		} //32/33 must retain the missing revival in their original pools.
		f.TargetContext = c.TargetContext
		if _, err := d.InvocationAction(&s, &f, []string{"tool"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(job, err)
		}
	}
}

func TestProtectiveMagicIndirectCostsAndArguments(t *testing.T) {
	for _, kind := range []string{"tool-84", "random", "gem-86"} {
		s, c, d := protectiveMagicFixture()
		s.Actors[0].Jobs.JobID, s.Actors[0].Jobs.SP = "job-34", 0
		s.Actors[0].MP = 0
		d.Invocations.Entries["indirect"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: kind}}
		draws := []float64{6.0 / 7, 0}
		if kind == "random" {
			d.RegisterRevivals(&corebattle.RevivalContext{TargetContext: c.TargetContext})
			// Empty peer tables isolate these real contributions, without
			// claiming that the global production random catalog is complete.
			for job := 1; job <= 87; job++ {
				if job != 32 && job != 33 && job != 34 {
					d.Invocations.JobTables[job] = corebattle.CallbackTable{Variants: [][]corebattle.CallbackEntry{{}}}
				}
			}
			draws = []float64{33.0 / 87, 6.0 / 7, 0}
		}
		if kind == "gem-86" {
			s.Actors[0].Jobs.JobID, s.Actors[0].Jobs.SP, s.Actors[0].MP = "job-33", 160, 33
			draws = nil
		}
		executeProtectiveMagic(t, &s, c, d, "indirect", []string{"enemy", "tail"}, draws...)
		if s.Actors[0].MP != 0 || s.Actors[0].CMP != 90 {
			t.Fatal(kind, s)
		}
		if kind == "gem-86" {
			if s.Actors[0].ExtendedTurns != 5 || len(c.Arguments) != 1 || c.Arguments[0] != "enemy" {
				t.Fatal(s, c)
			}
		} else if s.Actors[2].MP != 0 || s.Actors[1].HP != 734 || len(c.Arguments) != 1 || c.Arguments[0] != "tail" {
			t.Fatal(kind, s, c)
		}
	}
}

func TestProtectiveMagicExtendedDelegationBoundaries(t *testing.T) {
	for _, row := range []struct {
		sp, mp  int
		applied bool
	}{
		{159, 33, false}, {160, 32, false}, {160, 33, true}, {160, 34, true},
	} {
		s, c, d := protectiveMagicFixture()
		s.Actors[0].Jobs.JobID, s.Actors[0].Jobs.SP, s.Actors[0].MP = "job-33", row.sp, row.mp
		d.Invocations.Entries["extended"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "gem-86"}}
		executeProtectiveMagic(t, &s, c, d, "extended", nil)
		wantMP := row.mp
		if row.applied {
			wantMP -= 33
		}
		if s.Actors[0].MP != wantMP || (s.Actors[0].ExtendedTurns == 5) != row.applied {
			t.Fatal(row, s)
		}
	}
	s, c, d := protectiveMagicFixture()
	s.Actors[0].Jobs.JobID, s.Actors[0].MP = "job-33", 0
	d.RegisterRevivals(&corebattle.RevivalContext{TargetContext: c.TargetContext})
	d.Invocations.Entries["tool"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "tool-84"}}
	// Tools include extended positions and ignore ordinary child SP/listed MP.
	executeProtectiveMagic(t, &s, c, d, "tool", nil, 7.0/8)
	if s.Actors[0].MP != 0 || s.Actors[0].ExtendedTurns != 5 {
		t.Fatal(s)
	}
}
