package skill

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

func TestDuelCrownReloadContinuesWithoutReplay(t *testing.T) {
	s, c, d := duelFixture()
	executeDuel(t, &s, c, d, "job-74-entry-2", []string{"enemy"}, .5, 0, .5, 0, .5, 0, .5, 0, 0)
	if s.Actors[0].Extended != corebattle.ExtendedCrown || s.Actors[0].Attack != 120 {
		t.Fatal(s)
	}
	type checkpoint struct {
		State       corebattle.LifetimeState
		Target      corebattle.TargetContext
		Arguments   []string
		EntryID     string
		DamageCalls int
		End         corebattle.EndProcessing
	}
	frozen := checkpoint{s, c.TargetContext, []string{"enemy", "tail"}, "job-74-entry-5", c.DamageCalls, c.End}
	data, err := json.Marshal(frozen)
	if err != nil {
		t.Fatal(err)
	}
	var loaded checkpoint
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	resumed := &corebattle.DuelCrownContext{DamageContext: corebattle.DamageContext{EffectContext: corebattle.EffectContext{TargetContext: loaded.Target, DamageCalls: loaded.DamageCalls}, End: loaded.End, GuaranteedHit: true, Defeat: func(string) error { return nil }}}
	rebuilt := BattleDefinitions{}
	rebuilt.RegisterDuelCrown(resumed)
	for _, run := range []struct {
		state       *corebattle.LifetimeState
		context     *corebattle.DuelCrownContext
		definitions BattleDefinitions
	}{{&s, c, d}, {&loaded.State, resumed, rebuilt}} {
		// Captured target, fixed coefficient, no extension/recoil at these draws.
		var draws []float64
		for range 6 {
			draws = append(draws, 0, 0, .5, .5)
		}
		executeDuel(t, run.state, run.context, run.definitions, loaded.EntryID, loaded.Arguments, draws...)
	}
	if !reflect.DeepEqual(s, loaded.State) || s.Actors[1].HP != 614 || resumed.DamageCalls != 10 || c.DamageCalls != 10 || !reflect.DeepEqual(c.Arguments, resumed.Arguments) {
		t.Fatal(s, loaded.State, c, resumed)
	}
}

func TestDuelCrownIndirectCostsAndDefaultRejection(t *testing.T) {
	s, c, d := duelFixture()
	s.Actors[0].Jobs.JobID = "job-74"
	s.Actors[0].MP, s.Actors[0].Jobs.SP = 0, 0
	d.Invocations.Entries["tool"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "tool-84"}}
	// Select the second table entry despite unavailable ordinary SP/MP.
	executeDuel(t, &s, c, d, "tool", []string{"untouched"}, 1.0/6)
	if s.Actors[0].Temporary != corebattle.TemporaryReduceAttack || s.Actors[0].MP != 0 {
		t.Fatal(s)
	}
	frame := corebattle.CallbackContext{TargetContext: c.TargetContext}
	for _, definitions := range []BattleDefinitions{{}, d} {
		delete(definitions.Invocations.Entries, "job-74-entry-5")
		if _, err := definitions.InvocationAction(&s, &frame, []string{"tool"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(err)
		}
	}
	// Accepted no-effect is distinct from missing registration/descendants.
	s, c, d = duelFixture()
	s.Actors[1].HP = 0
	c.Defeat = nil
	executeDuel(t, &s, c, d, "job-74-entry-3", nil)
	executeDuel(t, &s, c, d, "job-74-entry-5", []string{"ignored"}, 0, 0, 0)
	if c.DamageCalls != 0 || c.AreaDamageCalls != 1 {
		t.Fatal(s, c)
	}
}

func TestDuelCrownAdmissionPurityAndEarlierEffects(t *testing.T) {
	for _, id := range []string{"job-74-entry-0", "job-74-entry-1", "job-74-entry-2", "job-74-entry-3", "job-74-entry-4", "job-74-entry-5"} {
		for _, reject := range []bool{false, true} {
			s, c, d := duelFixture()
			frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy", "tail"}}
			if reject {
				delete(d.Invocations.Entries, id)
			}
			before, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			original := frame
			_, err = d.InvocationAction(&s, &frame, []string{id}, noArithmeticDraws{t})
			if (err != nil) != reject {
				t.Fatal(id, err)
			}
			after, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(after) || !reflect.DeepEqual(original, frame) || c.DamageCalls != 0 || c.TensionCalls != 0 || len(c.Arguments) != 0 || c.End.ImmediateRiskCalls != 0 {
				t.Fatal(id, s, c, frame)
			}
		}
	}
	s, c, d := duelFixture()
	frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
	// An exact earlier stance can be projected; an unknown earlier verdict
	// cannot authorize later damage against stale target/actor eligibility.
	if _, err := d.InvocationAction(&s, &frame, []string{"job-74-entry-1", "job-74-entry-5"}, noArithmeticDraws{t}); err != nil {
		t.Fatal(err)
	}
	if _, err := d.InvocationAction(&s, &frame, []string{"job-74-entry-4", "job-74-entry-5"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	if s.Actors[0].Temporary != "" || s.Actors[0].Status != "" {
		t.Fatal(s)
	}
	// Earlier hits consume a one-hit defense and activate confusion, exposing
	// the other member's missing gear only on a later hit.
	s, c, d = duelFixture()
	s.Actors[1].Temporary = corebattle.TemporaryOneHitDefense
	s.Actors[1].ItemDefinitionIDs = []string{"item-195"}
	s.Actors[2].HP = 1000
	s.Actors[2].Gear = nil
	frame.TargetContext = c.TargetContext
	if _, err := d.InvocationAction(&s, &frame, []string{"job-74-entry-5"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
}

func TestDuelCrownSeededReplayVectors(t *testing.T) {
	// Keep concrete inputs and the seed in failures; draws span hit, miss,
	// reflection, extension and recoil without comparing two identical engines.
	for _, seed := range []uint64{1, 17, 74} {
		for _, crown := range []bool{false, true} {
			s, c, d := duelFixture()
			if crown {
				s.Actors[0].Extended = corebattle.ExtendedCrown
			}
			c.GuaranteedHit = false
			frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
			ops, err := d.InvocationAction(&s, &frame, []string{"job-74-entry-5"}, noArithmeticDraws{t})
			if err != nil {
				t.Fatal(seed, crown, err)
			}
			if err := s.ApplyInvocations(&frame, d.Invocations, ops, random.NewDeterministic(seed)); err != nil {
				t.Fatal(seed, crown, err)
			}
			if c.DamageCalls < 3 || c.DamageCalls > 6 || s.Actors[0].HP < 1 || s.Actors[0].HP > 1000 || s.Actors[1].HP >= 1000 || s.Actors[0].Attack != 100 {
				t.Fatal(seed, crown, s, c)
			}
		}
	}
}
