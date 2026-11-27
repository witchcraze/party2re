package skill

import (
	"encoding/json"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
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
		}{s, c.TargetContext, []string{"ally", "tail"}, []string{"job-36-copy", "gem_secret_art_sky", "gem_guard_1"}}
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
	// Later slots check registration across every form table, which needs the
	// transformation context that selects job79's variant.
	d.RegisterPlayerCallbacks(PlayerCallbackContexts{Transformations: &c}, len(c.PartyIDs))
	d.RegisterNonJobInvocations()
	d.RegisterDefenseGems()
	f := corebattle.CallbackContext{TargetContext: c.TargetContext}
	ids := []string{"gem_secret_art_sky", "gem_secret_art_sky", "gem_secret_art_sky", "gem_guard_1"}
	ops, err := d.InvocationAction(&s, &f, ids, noArithmeticDraws{t})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyInvocations(&f, d.Invocations, ops, noArithmeticDraws{t}); err != nil || s.Actors[0].MP != 0 || s.Actors[0].Attack != 1000 || s.Actors[0].Temporary != corebattle.TemporaryDefense {
		t.Fatal(s, err)
	}
}

func TestInvocationWrapperNativeDismissAndMergeRetainDeadActor(t *testing.T) {
	for _, row := range []struct {
		id, gem string
		wantMP  int
	}{{"job-59-dismiss", "gem_order_flee", 20}, {"job-59-merge", "gem_order_special_sky", 60}} {
		t.Run(row.id, func(t *testing.T) {
			s, c, d := rosterRegistryFixture()
			s.Actors[0].RosterIdentity = "calling-59"
			s.Actors[0].Jobs = corebattle.BattleJobs{JobID: "job-59"}
			s.Actors[0].MP = 20
			d.RegisterPlayerCallbacks(PlayerCallbackContexts{Roster: &c}, len(c.PartyIDs))
			d.RegisterNonJobInvocations()
			f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
			before, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			d.Invocations.Entries["later"] = corebattle.CallbackDefinition{
				Validate: func(*corebattle.LifetimeState, *corebattle.CallbackContext) error { return nil },
				Execute: func(*corebattle.LifetimeState, *corebattle.CallbackContext, random.Generator) error {
					t.Fatal("later slot ran after the actor died")
					return nil
				},
			}
			ops, err := d.InvocationAction(&s, &f, []string{row.gem, "later"}, noArithmeticDraws{t})
			if err != nil {
				t.Fatal(err)
			}
			after, err := json.Marshal(s)
			if err != nil || string(before) != string(after) {
				t.Fatal("admission changed caller state", err)
			}
			// Dismissal/merge mark the acting helper dead; they do not prune it.
			if err := s.ApplyInvocations(&f, d.Invocations, ops, noArithmeticDraws{t}); err != nil || f.ActorID != "self" || len(s.Actors) != 2 || s.Actors[0].ID != f.ActorID || s.Actors[0].HP != 0 || s.Actors[0].MP != row.wantMP || !s.Actors[0].RosterDetached {
				t.Fatal("execution pruned the dead actor", err, s, f)
			}
		})
	}
}
