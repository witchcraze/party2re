package skill

import (
	"encoding/json"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

// Complete native registration, with the actual copy and defense bodies.
// This certifies these paths only, not the general random descendant closure.
func TestInvocationWrapperNativeCopyExtendedAndReload(t *testing.T) {
	var uninterrupted corebattle.LifetimeState
	for _, reload := range []bool{false, true} {
		s, c, _ := transformationRegistryFixture()
		s.Actors[1].Jobs = corebattle.BattleJobs{JobID: "job-73"}
		input := struct {
			State     corebattle.LifetimeState
			Target    corebattle.TargetContext
			Arguments []string
			IDs       []string
		}{s, c.TargetContext, []string{"ally", "tail"}, []string{"job-36-copy", "gem-86", "gem_guard_1"}}
		data, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		if reload {
			if err := json.Unmarshal(data, &input); err != nil {
				t.Fatal(err)
			}
		}
		frame := corebattle.CallbackContext{TargetContext: input.Target, Arguments: input.Arguments}
		c.TargetContext = input.Target
		var d BattleDefinitions
		d.RegisterPlayerCallbacks(PlayerCallbackContexts{Transformations: &c}, len(frame.PartyIDs))
		d.RegisterNonJobInvocations()
		d.RegisterDefenseGems()
		ops, err := d.InvocationAction(&input.State, &frame, input.IDs, noArithmeticDraws{t})
		if err != nil {
			t.Fatal(err)
		}
		before, err := json.Marshal(input)
		if err != nil || string(before) != string(data) {
			t.Fatal("discovery mutated input", err)
		}
		if err := input.State.ApplyInvocations(&frame, d.Invocations, ops, noArithmeticDraws{t}); err != nil {
			t.Fatal(err)
		}
		a := input.State.Actors[0]
		// Copy floors the target's MP3 to50. Gem86's special job73 branch
		// then adds party2 and subtracts exactly one MP, including above MaxMP.
		if a.Jobs.JobID != "job-73" || a.Attack != 82 || a.MP != 49 || a.MaxMP != 20 || a.Temporary != corebattle.TemporaryDefense || !reflect.DeepEqual(frame.Arguments, []string{"ally", "tail"}) {
			t.Fatal(a, frame)
		}
		if reload {
			if !reflect.DeepEqual(input.State, uninterrupted) {
				t.Fatal("rebuilt registration changed results")
			}
		} else {
			uninterrupted = input.State
		}
		facts, err := json.Marshal(input.State)
		if err != nil {
			t.Fatal(err)
		}
		var loaded corebattle.LifetimeState
		if err := json.Unmarshal(facts, &loaded); err != nil || !reflect.DeepEqual(input.State, loaded) {
			t.Fatal("result facts did not survive reload", err)
		}
	}
}

func TestInvocationWrapperNativeRepeatedExtendedHandledWithoutPorts(t *testing.T) {
	s, c, _ := transformationRegistryFixture()
	s.Actors[0].Jobs = corebattle.BattleJobs{JobID: "job-73", OldJobID: "job-86", OldSP: 999}
	s.Actors[0].MP, s.Actors[0].Attack = 2, 998
	var d BattleDefinitions
	d.RegisterPlayerCallbacks(PlayerCallbackContexts{}, len(c.PartyIDs))
	d.RegisterNonJobInvocations()
	d.RegisterDefenseGems()
	f := corebattle.CallbackContext{TargetContext: c.TargetContext}
	ids := []string{"gem-86", "gem-86", "gem-86", "gem_guard_1"}
	ops, err := d.InvocationAction(&s, &f, ids, noArithmeticDraws{t})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyInvocations(&f, d.Invocations, ops, noArithmeticDraws{t}); err != nil || s.Actors[0].MP != 0 || s.Actors[0].Attack != 1000 || s.Actors[0].Temporary != corebattle.TemporaryDefense {
		t.Fatal(s, err)
	}
}
