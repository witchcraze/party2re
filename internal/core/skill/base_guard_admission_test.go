package skill

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestBaseGuardAdmissionPurityEveryEntry(t *testing.T) {
	for job := 0; job <= 3; job++ {
		for _, entry := range corebattle.BaseGuardEntries(job) {
			if entry.ID == "job-3-entry-6" {
				continue
			}
			for _, reject := range []bool{false, true} {
				s, c, d := baseGuardFixture()
				if reject {
					delete(d.Invocations.Entries, entry.ID)
				}
				c.Reset = func(string) error { t.Fatal("validation ran reset"); return nil }
				c.Defeat = func(string) error { t.Fatal("validation ran defeat"); return nil }
				c.Arguments = []string{"frozen"}
				frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy", "tail"}}
				args, party, gear := frame.Arguments, frame.PartyIDs, s.Actors[0].Gear
				weapon := 7
				gear.WeaponAttack = &weapon
				before, err := json.Marshal(s)
				if err != nil {
					t.Fatal(err)
				}
				status := *c.Status
				_, err = d.InvocationAction(&s, &frame, []string{entry.ID}, noArithmeticDraws{t})
				if (err != nil) != reject {
					t.Fatal(entry.ID, reject, err)
				}
				after, err := json.Marshal(s)
				if err != nil {
					t.Fatal(err)
				}
				if string(before) != string(after) || weapon != 7 || gear.WeaponAttack != &weapon || s.Actors[0].Gear != gear || !reflect.DeepEqual(args, []string{"enemy", "tail"}) || !reflect.DeepEqual(party, []string{"self", "fallen"}) || !reflect.DeepEqual(c.Arguments, []string{"frozen"}) || !reflect.DeepEqual(status, *c.Status) || c.DamageCalls != 0 || c.StatUpCalls != 0 || c.StatDownCalls != 0 || c.TensionCalls != 0 || c.AreaDamageCalls != 0 {
					t.Fatal(entry.ID, s, c, frame)
				}
			}
		}
	}
}

func TestBaseGuardMissingDescendantsRejectWholeAction(t *testing.T) {
	for _, bad := range []string{"second-hit", "damage", "status", "party-stat-tail", "tension-level", "nil-registration", "empty-registration"} {
		t.Run(bad, func(t *testing.T) {
			s, c, d := baseGuardFixture()
			id := "job-2-entry-6"
			switch bad {
			case "second-hit":
				s.Actors[1].Temporary = corebattle.TemporaryOneHitDefense
				c.Defeat = nil
			case "damage":
				id = "job-1-entry-5"
				s.Fields.Default = "cosmic"
			case "status":
				id = "job-2-entry-2"
				c.Status = nil
			case "party-stat-tail":
				id = "job-3-entry-5"
				s.Actors[2].ItemDefinitionIDs = []string{"item-205"}
			case "tension-level":
				id = "job-0-entry-3"
				s.Actors[0].Tension = 5
			case "nil-registration":
				d = BattleDefinitions{}
				d.RegisterBaseGuard(nil)
			case "empty-registration":
				d = BattleDefinitions{}
			}
			before, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
			ops := []corebattle.CallbackInvocation{{Kind: "entry", EntryID: "job-0-entry-4"}, {Kind: "entry", EntryID: id}}
			if err := s.ApplyInvocations(&frame, d.Invocations, ops, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
				t.Fatal(err)
			}
			after, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(after) || c.DamageCalls != 0 || c.PartyStatUpCalls != 0 {
				t.Fatal(s, c)
			}
		})
	}
}

func TestBaseGuardOrderedEligibilityAndConservativeUnknown(t *testing.T) {
	s, c, d := baseGuardFixture()
	s.Actors[0].Temporary = corebattle.TemporaryPhantom
	s.Actors[1].Temporary = corebattle.TemporaryNullAttack
	c.Defeat = nil
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
	// Replacing Phantom restores target nullification before the later hit.
	ops, err := d.InvocationAction(&s, &f, []string{"job-0-entry-4", "job-3-entry-3"}, noArithmeticDraws{t})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyInvocations(&f, d.Invocations, ops, noArithmeticDraws{t}); err != nil {
		t.Fatal(err)
	}
	if s.Actors[1].HP != 1000 || s.Actors[0].Temporary != corebattle.TemporaryDouble || c.DamageCalls != 0 {
		t.Fatal(s, c)
	}
	// Unknown damage/reset outcomes cannot authorize a later context-dependent body.
	s, c, d = baseGuardFixture()
	f = corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
	if _, err := d.InvocationAction(&s, &f, []string{"job-1-entry-1", "job-2-entry-8"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	// Capped first party buff leaves tension for the later item205 helper.
	s.Actors[0].Defense, s.Actors[0].Tension = 200, 3
	s.Actors[2].ItemDefinitionIDs = []string{"item-205"}
	if _, err := d.InvocationAction(&s, &f, []string{"job-3-entry-5"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	level := 99
	c.RequesterLevel = &level
	executeBaseGuard(t, &s, c, d, "job-3-entry-5", nil)
	if s.Actors[2].Defense != 140 || s.Actors[2].Attack != 60 || s.Actors[2].Temporary != corebattle.TemporaryDouble || s.Actors[0].Defense != 200 {
		t.Fatal(s, c)
	}
}

func TestBaseGuardIndirectTablesAndCosts(t *testing.T) {
	for _, job := range []int{0, 1, 2} {
		s, c, d := baseGuardFixture()
		s.Actors[0].Jobs.JobID = fmt.Sprintf("job-%d", job)
		s.Actors[0].MP = 0
		d.Invocations.Entries["tool"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "tool-84"}}
		draws := []float64{.999}
		if job == 1 {
			draws = append(draws, 0, 0)
		}
		if job == 2 {
			draws = append(draws, 0)
		}
		executeBaseGuard(t, &s, c, d, "tool", []string{"enemy", "tail"}, draws...)
		if s.Actors[0].MP != 0 || s.Actors[0].CMP != 90 {
			t.Fatal(s)
		}
		if job == 0 && s.Actors[0].Temporary != corebattle.TemporaryDefense || job == 1 && s.Actors[1].HP != 887 || job == 2 && s.Actors[1].HP != 883 {
			t.Fatal(job, s)
		}
		entries := d.Invocations.JobTables[job].Variants[0]
		delete(d.Invocations.Entries, entries[len(entries)-1].ID)
		f := corebattle.CallbackContext{TargetContext: c.TargetContext}
		if _, err := d.InvocationAction(&s, &f, []string{"tool"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(job, err)
		}
		// The original random pool stays incomplete, even with this complete subset.
		d.Invocations.Entries["random"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "random"}}
		if _, err := d.InvocationAction(&s, &f, []string{"random"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(err)
		}
	}
	for _, mp := range []int{48, 49} {
		s, c, d := baseGuardFixture()
		s.Actors[0].Jobs = corebattle.BattleJobs{JobID: "job-2", SP: 100}
		s.Actors[0].MP = mp
		d.Invocations.Entries["extended"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "gem-86"}}
		draws := []float64(nil)
		if mp == 49 {
			draws = []float64{0}
		}
		executeBaseGuard(t, &s, c, d, "extended", []string{"enemy"}, draws...)
		wantHP, wantMP := 1000, 48
		if mp == 49 {
			wantHP, wantMP = 883, 0
		}
		if s.Actors[1].HP != wantHP || s.Actors[0].MP != wantMP {
			t.Fatal(s)
		}
	}
}

func TestBaseGuardPreservesRevivalOwnerEitherOrder(t *testing.T) {
	for _, first := range []bool{false, true} {
		s, c, _ := baseGuardFixture()
		var d BattleDefinitions
		r := corebattle.RevivalContext{TargetContext: c.TargetContext}
		if first {
			d.RegisterRevivals(&r)
		}
		d.RegisterBaseGuard(c)
		if !first {
			d.RegisterRevivals(&r)
		}
		entry := d.Invocations.JobTables[3].Variants[0][6]
		if entry.ID != "job-3-entry-6" || entry.RequiredSP != 60 || entry.MPCost != 1 || entry.Extended || !d.RevivalSkills[entry.ID] || d.Invocations.Entries[entry.ID].Execute == nil {
			t.Fatal(entry, d)
		}
		// Sacrifice cannot be certified without its independently supplied defeat owner.
		s.Actors[0].Jobs.JobID = "job-3"
		d.Invocations.Entries["tool"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "tool-84"}}
		f := corebattle.CallbackContext{TargetContext: c.TargetContext}
		if _, err := d.InvocationAction(&s, &f, []string{"tool"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(err)
		}
	}
}
