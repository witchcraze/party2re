package skill

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestTreatmentMischiefFrozenReloadWithoutReplay(t *testing.T) {
	s, c, d := treatmentFixture()
	executeTreatment(t, &s, c, d, "job-78-entry-3", nil, 0)
	type checkpoint struct {
		State     corebattle.LifetimeState
		Target    corebattle.TargetContext
		Arguments []string
		EntryID   string
		HealCalls int
	}
	frozen := checkpoint{s, c.TargetContext, []string{"enemy", "tail"}, "job-57-entry-3", c.HealCalls}
	data, err := json.Marshal(frozen)
	if err != nil {
		t.Fatal(err)
	}
	var loaded checkpoint
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	_, resumed, _ := treatmentFixture()
	resumed.TargetContext, resumed.HealCalls = loaded.Target, loaded.HealCalls
	rebuilt := BattleDefinitions{}
	rebuilt.RegisterTreatmentMischief(resumed)
	frozen.Arguments[0] = "self"
	for _, run := range []struct {
		s *corebattle.LifetimeState
		c *corebattle.TreatmentMischiefContext
		d BattleDefinitions
	}{{&s, c, d}, {&loaded.State, resumed, rebuilt}} {
		executeTreatment(t, run.s, run.c, run.d, loaded.EntryID, loaded.Arguments, 0, 0)
	}
	if !reflect.DeepEqual(s, loaded.State) || s.Actors[0].HP != 680 || s.Actors[0].Defense != 64 || s.Actors[1].HP != 851 || s.Actors[1].Status != corebattle.StatusSleep || c.HealCalls != 1 || resumed.HealCalls != 1 || c.Status.StatusCalls != resumed.Status.StatusCalls || !reflect.DeepEqual(loaded.Arguments, []string{"enemy", "tail"}) {
		t.Fatal(s, loaded, c, resumed)
	}
}

func TestTreatmentMischiefDefaultEmptyAndOwnerRegistrationOrder(t *testing.T) {
	s, c, d := treatmentFixture()
	var own BattleDefinitions
	own.RegisterTreatmentMischief(c)
	if len(own.Invocations.Entries) != 29 {
		t.Fatal(len(own.Invocations.Entries))
	}
	var nilContext BattleDefinitions
	nilContext.RegisterTreatmentMischief(nil)
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"self"}}
	for _, empty := range []BattleDefinitions{{}, nilContext} {
		if _, err := empty.InvocationAction(&s, &f, []string{"job-50-entry-0"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(err)
		}
	}
	// Both directions preserve the resource/revival registrations.
	d.RegisterResources(&c.DamageContext)
	for range 2 {
		d.RegisterTreatmentMischief(c)
		if !d.RevivalSkills["job-57-entry-4"] {
			t.Fatal(d)
		}
		executeTreatment(t, &s, c, d, "job-50-entry-5", []string{"self"}, 0)
		executeTreatment(t, &s, c, d, "job-57-entry-4", []string{"fallen"}, .5)
		d.RegisterResources(&c.DamageContext)
		d.RegisterRevivals(&corebattle.RevivalContext{TargetContext: c.TargetContext})
	}
	if s.Actors[0].MP != 172 || s.Actors[2].HP != 0 {
		t.Fatal(s)
	}
	// Every original position remains in the table, including missing bodies.
	for job, count := range map[int]int{50: 8, 51: 8, 55: 8, 56: 6, 57: 7, 78: 7} {
		if len(d.Invocations.JobTables[job].Variants[0]) != count {
			t.Fatal(job)
		}
	}
	s.Actors[0].Jobs.JobID = "job-50"
	d.Invocations.Entries["tool"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "tool-84"}}
	if _, err := d.InvocationAction(&s, &f, []string{"tool"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
}

func TestTreatmentMischiefIndirectCostsAndCompleteDescendants(t *testing.T) {
	for _, kind := range []string{"tool-84", "random"} {
		s, c, d := treatmentFixture()
		s.Actors[0].Jobs.JobID, s.Actors[0].Jobs.SP, s.Actors[0].MP = "job-57", 0, 0
		d.Invocations.Entries["invoke"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: kind}}
		draws := []float64{0, 0}
		if kind == "random" {
			// Empty unrelated tables are test fixtures, not certified production.
			for job := 1; job <= 87; job++ {
				if job != 57 {
					d.Invocations.JobTables[job] = corebattle.CallbackTable{Variants: [][]corebattle.CallbackEntry{{}}}
				}
			}
			draws = append([]float64{56.1 / 87}, draws...)
		}
		executeTreatment(t, &s, c, d, "invoke", []string{"self"}, draws...)
		if s.Actors[0].HP != 527 || s.Actors[0].MP != 0 {
			t.Fatal(s.Actors[0])
		}
		// Losing an unselected successful descendant must reject before draw/cost.
		delete(d.Invocations.Entries, "job-57-entry-5")
		f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"self"}}
		if _, err := d.InvocationAction(&s, &f, []string{"invoke"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(err)
		}
	}
}

func TestTreatmentMischiefExactInlineProjectionBeforeLaterAction(t *testing.T) {
	s, c, d := treatmentFixture()
	s.Actors[2].Hit = 0
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"self"}}
	ops, err := d.InvocationAction(&s, &f, []string{"job-51-entry-1", "job-50-entry-0"}, noArithmeticDraws{t})
	if err != nil {
		t.Fatal(err)
	}
	if s.Actors[2].Hit != 0 {
		t.Fatal("projection mutated caller")
	}
	if err := s.ApplyInvocations(&f, d.Invocations, ops, &damageGemDraws{t: t, draws: []float64{0}}); err != nil {
		t.Fatal(err)
	}
	if s.Actors[2].Hit != 95 || s.Actors[0].HP != 572 {
		t.Fatal(s.Actors)
	}
}
