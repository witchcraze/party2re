package skill

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestRangedRegistrationAndPureAdmission(t *testing.T) {
	s, _, d := rangedFixture()
	if len(d.Invocations.Entries) != 23 || len(d.Invocations.JobTables[11].Variants[0]) != 9 || len(d.Invocations.JobTables[42].Variants[0]) != 8 || len(d.Invocations.JobTables[54].Variants[0]) != 6 || corebattle.RangedEntries(1) != nil {
		t.Fatal(d)
	}
	for _, job := range []int{11, 42, 54} {
		for _, entry := range corebattle.RangedEntries(job) {
			t.Run(entry.ID, func(t *testing.T) {
				s, c, d := rangedFixture()
				c.Defeat = func(string) error { t.Fatal("admission executed damage hook"); return nil }
				c.Status.Defeat.Rewards.Apply = func(*corebattle.LifetimeState, corebattle.TargetContext, string) error {
					t.Fatal("admission executed defeat hook")
					return nil
				}
				before, err := json.Marshal(s)
				if err != nil {
					t.Fatal(err)
				}
				f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy", "tail"}}
				original, gear, facts, defeat := f, *s.Actors[1].Gear, c.EffectContext, *c.Status.Defeat
				if _, err := d.InvocationAction(&s, &f, []string{entry.ID}, noArithmeticDraws{t}); err != nil {
					t.Fatal(err)
				}
				after, err := json.Marshal(s)
				if err != nil {
					t.Fatal(err)
				}
				// Observe the referents independently of the shallow context copy.
				if string(before) != string(after) || !reflect.DeepEqual(original, f) || !reflect.DeepEqual(gear, *s.Actors[1].Gear) || !reflect.DeepEqual(facts, c.EffectContext) || c.Arguments != nil || c.Status.DeathCalls != 0 || !reflect.DeepEqual(defeat.TargetContext, c.Status.Defeat.TargetContext) {
					t.Fatal(s, c, f)
				}
				for _, empty := range []BattleDefinitions{{}, func() BattleDefinitions { d.RegisterRanged(nil); return d }()} {
					if _, err := empty.InvocationAction(&s, &f, []string{entry.ID}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
						t.Fatal(entry, err)
					}
				}
			})
		}
	}
	if err := s.ValidateRanged(nil, "job-11-entry-0", noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrInvalidRequest) {
		t.Fatal(err)
	}
}

func TestRangedWholeActionRejection(t *testing.T) {
	for _, bad := range []string{"missing", "later-hit", "status", "death-tail", "agility-tail", "tension", "opaque", "tool-110", "gem-101", "gem-102"} {
		t.Run(bad, func(t *testing.T) {
			s, c, d := rangedFixture()
			ids := []string{"job-11-entry-2", bad}
			switch bad {
			case "later-hit":
				ids, s.Actors[1].Temporary, s.Actors[1].Gear = []string{"job-54-entry-5"}, corebattle.TemporaryOneHitDefense, nil
			case "status":
				ids, s.Actors[1].Gear = []string{"job-42-entry-0"}, nil
			case "death-tail":
				ids, c.Status.Defeat = []string{"job-42-entry-6"}, nil
			case "agility-tail":
				ids, s.Actors[0].ItemDefinitionIDs = []string{"job-42-entry-2"}, []string{"item-201"}
			case "tension":
				ids, s.Actors[0].Tension = []string{"job-11-entry-8"}, 5
			case "opaque":
				ids = []string{"job-11-entry-8", "job-54-entry-1"}
			}
			before, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy", "tail"}}
			var ops []corebattle.CallbackInvocation
			for _, id := range ids {
				ops = append(ops, corebattle.CallbackInvocation{Kind: "entry", EntryID: id})
			}
			if _, err := d.InvocationAction(&s, &f, ids, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
				t.Fatal(ids, err)
			}
			if err := s.ApplyInvocations(&f, d.Invocations, ops, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
				t.Fatal(ids, err)
			}
			after, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(after) || !reflect.DeepEqual(f.Arguments, []string{"enemy", "tail"}) || c.Arguments != nil || c.DamageCalls != 0 || c.TensionCalls != 0 {
				t.Fatal(s, c)
			}
		})
	}
}

func TestRangedAcceptedNoEffectAndInlineGuards(t *testing.T) {
	for _, job := range []int{11, 42, 54} {
		for _, entry := range corebattle.RangedEntries(job) {
			s, c, d := rangedFixture()
			s.Actors[1].HP, s.Actors[1].Gear, c.Defeat, c.Status.Defeat = 0, nil, nil, nil
			draws := []float64{}
			if entry.ID == "job-11-entry-7" || entry.ID == "job-54-entry-5" {
				draws = []float64{0, 0, 0, 0} // Count plus three absent-target selections.
			}
			executeRanged(t, &s, c, d, entry.ID, []string{"enemy"}, draws...)
			if s.Actors[1].HP != 0 || s.Actors[0].HP != 500 || s.Actors[0].MP != 80 {
				t.Fatal(entry, s, c)
			}
		}
	}
	// Accuracy writes are unconditional: they lower hit>95 and work on fallen/sealed actors.
	for _, id := range []string{"job-11-entry-2", "job-54-entry-2"} {
		for _, hit := range []int{0, 95, 100} {
			s, c, d := rangedFixture()
			s.Actors[0].HP, s.Actors[0].Hit, s.Actors[0].Status = 0, hit, corebattle.StatusPhysicalSeal
			executeRanged(t, &s, c, d, id, nil)
			if s.Actors[0].Hit != 95 || s.Actors[0].Status != corebattle.StatusPhysicalSeal {
				t.Fatal(id, hit, s)
			}
		}
	}
	// The untyped self buff still observes the shared cap and binding guards.
	for _, guard := range []string{"cap", "binding"} {
		s, c, d := rangedFixture()
		want := 40
		if guard == "cap" {
			s.Actors[0].Agility, want = 100, 100
		} else {
			s.Actors[0].Status = corebattle.StatusSabaku
		}
		executeRanged(t, &s, c, d, "job-42-entry-2", nil)
		if s.Actors[0].Agility != want {
			t.Fatal(guard, s)
		}
	}
}

func TestRangedDelegatedCostsAndCompletePools(t *testing.T) {
	for _, delegated := range []bool{false, true} {
		s, c, d := rangedFixture()
		s.Actors[0].Jobs.JobID = "job-11"
		id, draws := "job-11-entry-4", []float64{0}
		if delegated {
			d.Invocations.Entries["tool"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "tool-84"}}
			id, draws = "tool", []float64{.5, 0}
		}
		executeRanged(t, &s, c, d, id, []string{"enemy"}, draws...)
		if s.Actors[0].MP != 94 || s.Actors[1].HP != 900 || s.Actors[1].MP != 85 {
			t.Fatal(delegated, s)
		}
	}
	s, c, d := rangedFixture()
	s.Actors[0].Jobs.JobID = "job-54"
	d.Invocations.Entries["tool"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "tool-84"}}
	d.Invocations.Entries["random"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "random"}}
	delete(d.Invocations.Entries, "job-54-entry-5")
	f := corebattle.CallbackContext{TargetContext: c.TargetContext}
	for _, id := range []string{"tool", "random"} {
		if _, err := d.InvocationAction(&s, &f, []string{id}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(id, err)
		}
	}
}
