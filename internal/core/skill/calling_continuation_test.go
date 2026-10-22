package skill

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

func TestCallingRevivalProjectsNewlyReachableUnsupportedTail(t *testing.T) {
	s, c, d := callingManifestFixture()
	d.RegisterCallingJobs(&c)
	addCallingManifestHelper(&s, &c, 61)
	addCallingManifestHelper(&s, &c, 87)
	s.Actors[4].ID = "snake"
	c.PartyIDs[len(c.PartyIDs)-1] = "snake"
	s.Actors[4].HP = 0
	s.Actors[1].MaxHP = 999
	frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
	// With a fallen helper this concrete body is accepted no-effect. Revival
	// makes its missing status port reachable and must reject the entire pair.
	if _, err := d.InvocationAction(&s, &frame, []string{"job-87-venom"}, noArithmeticDraws{t}); err != nil {
		t.Fatal(err)
	}
	before, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	ops := []corebattle.CallbackInvocation{{Kind: "entry", EntryID: "job-61-revive-party"}, {Kind: "entry", EntryID: "job-87-venom"}}
	if err := s.ApplyInvocations(&frame, d.Invocations, ops, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	after, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) || c.DamageCalls != 0 || s.Actors[4].HP != 0 {
		t.Fatal("partial revival/damage")
	}
}

func TestCallingReloadRebuildsClosuresWithoutRepeatingIncarnation(t *testing.T) {
	for _, row := range callingManifest {
		t.Run(fmt.Sprint(row.job), func(t *testing.T) {
			s, c, d := rosterRegistryFixture()
			s.Actors[0].Jobs.OldJobID = fmt.Sprintf("job-%d", row.job)
			d.RegisterCallingJobs(&c)
			frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
			if err := s.ApplyInvocations(&frame, d.Invocations, []corebattle.CallbackInvocation{{Kind: "gem-87"}}, random.NewDeterministic(9)); err != nil {
				t.Fatal(err)
			}
			c.TargetContext = frame.TargetContext
			// Only facts and frozen next input survive; callbacks are rebuilt.
			frozen := struct {
				State     corebattle.LifetimeState
				Target    corebattle.TargetContext
				Arguments []string
				Seed      uint64
			}{s, frame.TargetContext, frame.Arguments, 17}
			data, err := json.Marshal(frozen)
			if err != nil {
				t.Fatal(err)
			}
			loaded := frozen
			if err := json.Unmarshal(data, &loaded); err != nil {
				t.Fatal(err)
			}
			context := c
			context.TargetContext = loaded.Target
			rebuilt := BattleDefinitions{}
			rebuilt.RegisterCallingJobs(&context)
			resumed := corebattle.CallbackContext{TargetContext: loaded.Target, Arguments: loaded.Arguments}
			next := []corebattle.CallbackInvocation{{Kind: "gem-87"}, {Kind: "gem-91"}}
			if err := s.ApplyInvocations(&frame, d.Invocations, next, random.NewDeterministic(frozen.Seed)); err != nil {
				t.Fatal(err)
			}
			if err := loaded.State.ApplyInvocations(&resumed, rebuilt.Invocations, next, random.NewDeterministic(loaded.Seed)); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(s, loaded.State) || !reflect.DeepEqual(frame.TargetContext, resumed.TargetContext) || len(s.Actors) != 3 || s.RosterSequence != 1 {
				t.Fatal("reload diverged or repeated summon", s, loaded.State)
			}
		})
	}
}

func TestCallingDelegationUsesCompleteOrderedNativeContribution(t *testing.T) {
	for _, row := range callingManifest {
		for offset, command := range []string{"summon", "dismiss", "attack", "special", "defend", row.last} {
			t.Run(fmt.Sprintf("%d/gem-%d", row.job, 87+offset), func(t *testing.T) {
				s, c, d := callingManifestFixture()
				d.RegisterCallingJobs(&c)
				s.Actors[0].Jobs.OldJobID = fmt.Sprintf("job-%d", row.job)
				if command != "summon" {
					addCallingManifestHelper(&s, &c, row.job)
				}
				frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy", "outer-trailing"}}
				ops := []corebattle.CallbackInvocation{{Kind: fmt.Sprintf("gem-%d", 87+offset)}}
				id := fmt.Sprintf("job-%d-%s", row.job, command)
				if !d.RosterSkills[id] {
					if err := s.ApplyInvocations(&frame, d.Invocations, ops, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
						t.Fatal(err)
					}
					return
				}
				if err := s.ValidateInvocations(&frame, d.Invocations, ops, noArithmeticDraws{t}); err != nil {
					t.Fatal(err)
				}
				if err := s.ApplyInvocations(&frame, d.Invocations, ops, callingZeroDraw{}); err != nil {
					t.Fatal(err)
				}
				checkCallingIntrinsic(t, &s, &c, command, row.job, row.stats)
				if !reflect.DeepEqual(frame.Arguments, []string{"enemy", "outer-trailing"}) || frame.ActorID != "self" {
					t.Fatal(frame)
				}
			})
		}
	}
}

func TestCallingContributionPreservesRevivalOwnerInEitherRegistrationOrder(t *testing.T) {
	for _, callingFirst := range []bool{false, true} {
		s, c, d := rosterRegistryFixture()
		revival := corebattle.RevivalContext{TargetContext: c.TargetContext}
		if callingFirst {
			d.RegisterCallingJobs(&c)
		}
		d.RegisterRevivals(&revival)
		d.RegisterCallingJobs(&c)
		s.Actors[0].Jobs.OldJobID, s.Actors[0].Jobs.OldSP = "job-66", 150
		frame := corebattle.CallbackContext{TargetContext: c.TargetContext}
		if err := s.ApplyInvocations(&frame, d.Invocations, []corebattle.CallbackInvocation{{Kind: "gem-86"}}, noArithmeticDraws{t}); err != nil {
			t.Fatal(err)
		}
	}
}
