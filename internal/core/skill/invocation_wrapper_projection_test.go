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

func TestInvocationWrapperNativeDismissAndMergeRetainDeadActor(t *testing.T) {
	for _, row := range []struct {
		id, gem string
		wantMP  int
	}{{"job-59-dismiss", "gem-88", 20}, {"job-59-merge", "gem-92", 60}} {
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
			ops, err := d.InvocationAction(&s, &f, []string{row.gem, "missing"}, noArithmeticDraws{t})
			if err != nil {
				t.Fatal("native dead-actor projection lost its actor record", err)
			}
			after, err := json.Marshal(s)
			if err != nil || string(before) != string(after) {
				t.Fatal("admission changed caller state", err)
			}
			// Check the actual registered projection as well as enclosing admission:
			// dismissal/merge mark the acting helper dead; they do not prune it.
			var projected corebattle.LifetimeState
			if err := json.Unmarshal(before, &projected); err != nil {
				t.Fatal(err)
			}
			frame := f
			known, err := d.Invocations.Entries[row.id].Project(&projected, &frame)
			if err != nil || !known || frame.ActorID != "self" || len(projected.Actors) != 2 || projected.Actors[0].ID != frame.ActorID || projected.Actors[0].HP != 0 || projected.Actors[0].MP != row.wantMP || !projected.Actors[0].RosterDetached {
				t.Fatal(known, err, projected, frame)
			}
			if err := s.ApplyInvocations(&f, d.Invocations, ops, noArithmeticDraws{t}); err != nil || !reflect.DeepEqual(s, projected) {
				t.Fatal("execution pruned a projected actor or reached the later missing slot", err, s)
			}
		})
	}
}

func TestInvocationWrapperNativeBranchedProjectionRetainsActor(t *testing.T) {
	s, c := recoveryGemFixture(false)
	s.Actors[0].Status = corebattle.StatusMagicSeal
	var d BattleDefinitions
	d.RegisterDefenseGems()
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"ally"}}
	branches, err := d.Invocations.Entries["gem_guard_sky"].ProjectBranches(&s, &f)
	if err != nil || len(branches) != 2 {
		t.Fatal(branches, err)
	}
	for _, branch := range branches {
		if branch.Context.ActorID != f.ActorID || len(branch.State.Actors) != len(s.Actors) || branch.State.Actors[0].ID != f.ActorID || branch.State.Actors[0].HP != s.Actors[0].HP {
			t.Fatal("native seal cure/failure removed the actor", branch)
		}
	}
}
