package skill

import (
	"errors"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

func TestCallingRegistrationPreservesCompleteTablesAndOtherOwners(t *testing.T) {
	_, c, d := rosterRegistryFixture()
	d.Invocations.Entries["job-64-grave"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "random"}}
	d.RegisterCallingJobs(&c)
	count := 0
	for _, job := range []int{59, 60, 61, 62, 63, 64, 65, 66, 67, 68, 69, 71, 80, 87} {
		entries := corebattle.RosterEntries(job)
		if !reflect.DeepEqual(d.Invocations.JobTables[job].Variants, [][]corebattle.CallbackEntry{entries}) {
			t.Fatal(job, "changed source order or metadata")
		}
		for _, e := range entries {
			registered := e.Name != "grave" && e.Name != "revive"
			if d.RosterSkills[e.ID] != registered {
				t.Fatal(e.ID, "incorrect body certification")
			}
			if registered {
				count++
				def := d.Invocations.Entries[e.ID]
				if def.Validate == nil || def.Execute == nil {
					t.Fatal(e.ID, "missing concrete callback")
				}
			}
		}
	}
	if count != 88 || d.Invocations.Entries["job-64-grave"].Invocation.Kind != "random" {
		t.Fatal(count, "overwrote another owner's definition")
	}
}

func TestCallingOrderedDismissThenAttackAndMissingProjection(t *testing.T) {
	s, c, d := rosterRegistryFixture()
	d.RegisterCallingJobs(&c)
	frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
	if err := d.Invocations.Entries["job-59-summon"].Execute(&s, &frame, random.NewDeterministic(1)); err != nil {
		t.Fatal(err)
	}
	ids := []string{"job-59-dismiss", "job-59-attack"}
	before := s.Actors[2]
	if _, err := d.InvocationAction(&s, &frame, ids, noArithmeticDraws{t}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(s.Actors[2], before) || len(c.PartyIDs) != 2 {
		t.Fatal("projection mutated live helper or context")
	}
	ops := []corebattle.CallbackInvocation{{Kind: "entry", EntryID: ids[0]}, {Kind: "entry", EntryID: ids[1]}}
	if err := s.ApplyInvocations(&frame, d.Invocations, ops, noArithmeticDraws{t}); err != nil || s.Actors[2].HP != 0 || s.Actors[1].HP != 1000 {
		t.Fatal(err, s)
	}
	// An arbitrary earlier body has no certified transition, even if its current
	// execution fixture happens to do nothing.
	d.Invocations.Entries["opaque"] = corebattle.CallbackDefinition{
		Validate: func(*corebattle.LifetimeState, *corebattle.CallbackContext) error { return nil },
		Execute: func(*corebattle.LifetimeState, *corebattle.CallbackContext, random.Generator) error {
			t.Fatal("executed during rejection")
			return nil
		},
	}
	if _, err := d.InvocationAction(&s, &frame, []string{"opaque", "job-59-attack"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
}

func TestCallingProjectedCopyDeathAndSpawnSelectCurrentEligibility(t *testing.T) {
	for _, change := range []string{"copy", "death", "spawn"} {
		t.Run(change, func(t *testing.T) {
			s, c, d := rosterRegistryFixture()
			d.RegisterCallingJobs(&c)
			frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
			transition := func(state *corebattle.LifetimeState, f *corebattle.CallbackContext) (bool, error) {
				switch change {
				case "copy":
					state.Actors[0].Jobs.OldJobID = "job-61"
				case "death":
					state.Actors[0].HP = 0
				case "spawn":
					a := state.Actors[0]
					a.ID, a.RosterIdentity = "frozen-helper", "calling-59"
					state.Actors = append(state.Actors, a)
					f.PartyIDs = append(f.PartyIDs, a.ID)
				}
				return true, nil
			}
			d.Invocations.Entries["prior"] = corebattle.CallbackDefinition{
				Validate: func(*corebattle.LifetimeState, *corebattle.CallbackContext) error { return nil },
				Project:  transition,
				Execute: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext, _ random.Generator) error {
					_, err := transition(s, f)
					return err
				},
			}
			kind := "gem-87"
			if change == "spawn" {
				kind = "gem-91"
			}
			d.Invocations.Entries["calling"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: kind}}
			ops, err := d.InvocationAction(&s, &frame, []string{"prior", "calling"}, noArithmeticDraws{t})
			if err != nil || len(s.Actors) != 2 || s.Actors[0].HP != 50 || s.Actors[0].Jobs.OldJobID != "job-59" {
				t.Fatal("projection changed caller", err, s)
			}
			if err := s.ApplyInvocations(&frame, d.Invocations, ops, random.NewDeterministic(1)); err != nil {
				t.Fatal(err)
			}
			switch change {
			case "copy":
				if s.Actors[2].RosterIdentity != "calling-61" {
					t.Fatal(s)
				}
			case "death":
				if len(s.Actors) != 2 {
					t.Fatal("continued after actor death")
				}
			case "spawn":
				if s.Actors[2].Temporary != corebattle.TemporaryDefense {
					t.Fatal(s)
				}
			}
		})
	}
}
