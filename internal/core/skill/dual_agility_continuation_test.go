package skill

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestDualAgilityReloadRetainsFrozenVariantWithoutReplay(t *testing.T) {
	s, c, d := dualAgilityFixture()
	s.Actors[0].Temporary = corebattle.TemporaryDefense
	frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
	table := d.Invocations.JobTables[77]
	frozenID := table.Variants[table.Select(&s, &frame)][4].ID
	executeDual(t, &s, c, d, "job-77-entry-3", []string{"enemy"}, 0, 0)
	// Later changes cannot switch the already selected callback on reload.
	s.Actors[0].Temporary = ""
	type checkpoint struct {
		State     corebattle.LifetimeState
		Target    corebattle.TargetContext
		Arguments []string
		EntryID   string
	}
	frozen := checkpoint{s, frame.TargetContext, frame.Arguments, frozenID}
	data, err := json.Marshal(frozen)
	if err != nil {
		t.Fatal(err)
	}
	var loaded checkpoint
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	_, resumed, _ := dualAgilityFixture()
	resumed.TargetContext = loaded.Target
	rebuilt := BattleDefinitions{}
	rebuilt.RegisterDualAgility(resumed)
	for _, run := range []struct {
		state       *corebattle.LifetimeState
		context     *corebattle.DualAgilityContext
		definitions BattleDefinitions
	}{{&s, c, d}, {&loaded.State, resumed, rebuilt}} {
		executeDual(t, run.state, run.context, run.definitions, loaded.EntryID, loaded.Arguments, make([]float64, 12)...)
	}
	if !reflect.DeepEqual(s, loaded.State) || s.Actors[1].HP != 616 || resumed.DamageCalls != 6 || c.DamageCalls != 8 {
		t.Fatal(s, loaded.State, c.DamageCalls, resumed.DamageCalls)
	}
}

func TestDualAgilityIndirectFullTablesAndCostWaiver(t *testing.T) {
	for _, job := range []int{77, 83} {
		s, c, d := dualAgilityFixture()
		// Unavailable ordinary MP/SP does not narrow an indirect draw pool.
		s.Actors[0].MP, s.Actors[0].Jobs.SP = 0, 0
		s.Actors[0].Jobs.JobID = "job-77"
		if job == 83 {
			s.Actors[0].Jobs.JobID = "job-83"
		}
		s.Actors[1].HP = 1001 // Instant-death immunity admits this branch without defeat.
		c.Status = &corebattle.StatusContext{}
		d.Invocations.Entries["tool"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "tool-84"}}
		frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
		draws, wantHP, missing := []float64{0, .5, 0, .5, 0}, 935, "job-77-entry-4-temporary"
		if job == 83 {
			draws, wantHP, missing = []float64{0, .5, 0}, 956, "job-83-entry-5"
		}
		executeDual(t, &s, c, d, "tool", frame.Arguments, draws...)
		if s.Actors[0].MP != 0 || s.Actors[0].CMP != 90 || s.Actors[1].HP != wantHP {
			t.Fatal(s)
		}
		delete(d.Invocations.Entries, missing)
		if _, err := d.InvocationAction(&s, &frame, []string{"tool"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(err)
		}
		d.Invocations.Entries["random"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "random"}}
		if _, err := d.InvocationAction(&s, &frame, []string{"random"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(err)
		}
	}
}
