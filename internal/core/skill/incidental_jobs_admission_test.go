package skill

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestIncidentalJobAdmissionIsInertForEveryBody(t *testing.T) {
	_, _, catalog := incidentalFixture()
	for id := range catalog.Invocations.Entries {
		t.Run(id, func(t *testing.T) {
			s, c, d := incidentalFixture()
			c.Status.Defeat.Rewards.Apply = func(*corebattle.LifetimeState, corebattle.TargetContext, string) error {
				t.Fatal("admission ran reward")
				return nil
			}
			c.Reset = func(string) error { t.Fatal("admission ran reset"); return nil }
			c.Defeat = func(string) error { t.Fatal("admission ran defeat"); return nil }
			f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"self", "tail"}}
			before, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			original := f
			if _, err := d.InvocationAction(&s, &f, []string{id}, noArithmeticDraws{t}); err != nil {
				t.Fatal(err)
			}
			ops := []corebattle.CallbackInvocation{{Kind: "entry", EntryID: id}, {Kind: "entry", EntryID: "missing-descendant"}}
			if err := s.ApplyInvocations(&f, d.Invocations, ops, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
				t.Fatal(err)
			}
			after, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(after) || !reflect.DeepEqual(f, original) || c.Arguments != nil || c.DamageCalls != 0 || c.HealCalls != 0 || c.StatUpCalls != 0 || len(c.JokeOutcomes) != 0 {
				t.Fatal(s, c, f)
			}
		})
	}
}

func TestIncidentalJobMissingDescendantsRejectBeforeReset(t *testing.T) {
	for _, bad := range []string{"lethal-reset", "damage", "later-status", "agility-tail", "tension"} {
		t.Run(bad, func(t *testing.T) {
			s, c, d := incidentalFixture()
			id, args := "job-9-entry-9", []string{"enemy"}
			s.Actors[1].Tension = 5
			switch bad {
			case "lethal-reset":
				s.Actors[1].HP, s.Actors[1].Gear = 1, nil
			case "damage":
				id = "job-7-entry-5"
				c.ToolCategory.Source = "unknown"
			case "later-status":
				id = "job-9-entry-7"
				s.Actors[0].Status = corebattle.StatusPhysicalSeal
				s.Actors[2].HP, s.Actors[2].Gear = 100, nil
				c.Item195Activated = true
			case "agility-tail":
				id, args[0] = "job-9-entry-2", "self"
				s.Actors[0].ItemDefinitionIDs = []string{"item-201"}
			case "tension":
				id = "job-8-entry-6"
				s.Actors[1].Tension = 5
			}
			before, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: args}
			ops := []corebattle.CallbackInvocation{{Kind: "entry", EntryID: "job-8-entry-8"}, {Kind: "entry", EntryID: id}}
			if bad == "tension" {
				ops = ops[1:]
			}
			if err := s.ApplyInvocations(&f, d.Invocations, ops, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
				t.Fatal(err)
			}
			after, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(after) || c.DamageCalls != 0 || c.StatUpCalls != 0 {
				t.Fatal(s, c)
			}
		})
	}
}

func TestIncidentalJobResetProjectsLaterEligibility(t *testing.T) {
	s, c, d := incidentalFixture()
	s.Actors[1].Tension = 5
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
	if _, err := d.InvocationAction(&s, &f, []string{"job-8-entry-6"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	ops, err := d.InvocationAction(&s, &f, []string{"job-8-entry-8", "job-8-entry-6"}, noArithmeticDraws{t})
	if err != nil {
		t.Fatal(err)
	}
	if s.Actors[1].Tension != 5 {
		t.Fatal(s)
	}
	if err := s.ApplyInvocations(&f, d.Invocations, ops, noArithmeticDraws{t}); err != nil {
		t.Fatal(err)
	}
	if s.Actors[1].Tension != 1.7 || c.TensionCalls != 1 {
		t.Fatal(s, c)
	}
	// Unknown earlier outcomes cannot be treated as deterministic admission facts.
	if _, err := d.InvocationAction(&s, &f, []string{"job-8-entry-2", "job-9-entry-7"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
}

func TestIncidentalJobDefaultEmptyAndCompleteTableBoundaries(t *testing.T) {
	s, c, d := incidentalFixture()
	if len(d.Invocations.Entries) != 21 {
		t.Fatal(len(d.Invocations.Entries))
	}
	var nilContext BattleDefinitions
	nilContext.RegisterIncidentalJobs(nil)
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"self"}}
	for _, empty := range []BattleDefinitions{{}, nilContext} {
		if _, err := empty.InvocationAction(&s, &f, []string{"job-8-entry-8"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(err)
		}
	}
	for job, count := range map[int]int{7: 9, 8: 9, 9: 10} {
		if len(d.Invocations.JobTables[job].Variants[0]) != count {
			t.Fatal(d)
		}
	}
	for _, id := range []string{"job-7-entry-0", "job-7-entry-2", "job-8-entry-0", "job-7-entry-4", "job-9-entry-4", "job-9-entry-6", "job-7-entry-8", "tool-110", "gem-101", "gem-102"} {
		if d.Invocations.Entries[id].Execute != nil {
			t.Fatal(id)
		}
	}
	// These real tables retain missing wallet descendants; tool/random cannot narrow them.
	for _, job := range []string{"job-7", "job-8"} {
		s.Actors[0].Jobs.JobID = job
		d.Invocations.Entries["tool"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "tool-84"}}
		if _, err := d.InvocationAction(&s, &f, []string{"tool"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(err)
		}
	}
	// Both registration orders retain the separately owned inspection and sacrifice.
	for _, reverse := range []bool{false, true} {
		var composed BattleDefinitions
		registerPeers := func() {
			composed.RegisterOrdinaryTools(&corebattle.OrdinaryToolContext{DamageContext: c.DamageContext})
			composed.RegisterRevivals(&corebattle.RevivalContext{TargetContext: c.TargetContext})
		}
		if reverse {
			registerPeers()
		}
		composed.RegisterIncidentalJobs(c)
		if !reverse {
			registerPeers()
		}
		for _, id := range []string{"job-7-entry-4", "job-9-entry-4", "job-9-entry-6", "job-7-entry-8"} {
			if composed.Invocations.Entries[id].Execute == nil {
				t.Fatal(id)
			}
		}
	}
}
