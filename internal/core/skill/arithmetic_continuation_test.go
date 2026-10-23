package skill

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

func TestArithmeticProjectsEarlierIntrinsicEligibility(t *testing.T) {
	for _, tc := range []struct {
		prior, later string
		hp, mp       int
	}{
		{"job-31-gravity", "job-31-death", 999, 20},
		{"job-31-reverse", "job-31-death", 100, 20},
		{"job-72-revive", "job-31-death", 0, 20},
		{"job-72-divisor-4", "job-72-damage", 100, 4},
	} {
		t.Run(tc.prior, func(t *testing.T) {
			s, c, d := arithmeticRegistryFixture()
			s.Actors[1].MP = 0
			s.Actors[2].HP, s.Actors[2].MP = tc.hp, tc.mp
			if tc.prior == "job-72-revive" {
				s.Actors[2].MaxHP = 999
			}
			c.SubmittedText = "＠倍数指定＠×反射 "
			frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy", "tail"}, ArithmeticDivisor: 4}
			if tc.prior == "job-72-divisor-4" {
				frame.ArithmeticDivisor = 0
			}
			before := append([]corebattle.LifetimeActor(nil), s.Actors...)
			args := frame.Arguments
			ops := []corebattle.CallbackInvocation{{Kind: "entry", EntryID: tc.prior}, {Kind: "entry", EntryID: tc.later}}
			if _, err := d.InvocationAction(&s, &frame, []string{tc.prior, tc.later}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
				t.Fatal(err)
			}
			if err := s.ApplyInvocations(&frame, d.Invocations, ops, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) || !reflect.DeepEqual(before, s.Actors) || args[0] != "enemy" || frame.PartyIDs[0] != "self" || c.Divisor != 0 {
				t.Fatal("late rejection changed caller facts", err, s, frame, c)
			}
			c.Defeat = func(string) error { return nil }
			if _, err := d.InvocationAction(&s, &frame, []string{tc.prior, tc.later}, noArithmeticDraws{t}); err != nil || !reflect.DeepEqual(before, s.Actors) {
				t.Fatal("supplied descendant rejected or admission mutated", err, s)
			}
			defeats := 0
			c.Defeat = func(string) error { defeats++; return nil }
			rng := &damageGemDraws{t: t}
			if tc.later == "job-72-damage" {
				rng.draws = []float64{0}
			}
			if err := s.ApplyInvocations(&frame, d.Invocations, ops, rng); err != nil || s.Actors[2].HP != 0 || defeats != 1 || rng.used != len(rng.draws) {
				t.Fatal("ordered execution disagrees with projected eligibility", err, s, defeats)
			}
		})
	}
}

func TestArithmeticInvocationUsesCurrentActorAndNakedArgumentFrame(t *testing.T) {
	s, c, d := arithmeticRegistryFixture()
	s.Actors[0].Status = corebattle.StatusMagicSeal
	s.Actors[1].HP = 100
	frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy", "tail"}}
	frame.ActorID = "ally"
	def := d.Invocations.Entries["job-31-reverse"]
	if err := def.Validate(&s, &frame); err != nil {
		t.Fatal(err)
	}
	if err := def.Execute(&s, &frame, noArithmeticDraws{t}); err != nil || s.Actors[2].HP != 21 || s.Actors[0].Status != corebattle.StatusMagicSeal || !reflect.DeepEqual(frame.Arguments, []string{"tail"}) || c.ActorID != "self" {
		t.Fatal(err, s, frame, c)
	}
}

func TestArithmeticUnknownEarlierEffectsRequireReachableDefeat(t *testing.T) {
	for _, id := range []string{"job-31-gravity", "job-31-death", "job-31-reverse", "job-72-damage", "job-72-death", "job-72-reflect", "job-72-heal", "job-72-revive", "job-72-divisor-4"} {
		s, c, d := arithmeticRegistryFixture()
		d.Invocations.Entries["prior"] = corebattle.CallbackDefinition{
			Validate: func(*corebattle.LifetimeState, *corebattle.CallbackContext) error { return nil },
			Execute: func(*corebattle.LifetimeState, *corebattle.CallbackContext, random.Generator) error {
				t.Fatal("admission ran external execution")
				return nil
			},
		}
		// Current members are safe; an unknown earlier effect may lower HP,
		// raise MP, revive a member, or supply a divisor before the later body.
		s.Actors[2].HP, s.Actors[2].MP = 1000, 0
		frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
		_, err := d.InvocationAction(&s, &frame, []string{"prior", id}, noArithmeticDraws{t})
		reject := id == "job-31-death" || id == "job-31-reverse" || id == "job-72-damage" || id == "job-72-death"
		if errors.Is(err, corebattle.ErrUnsupportedEffect) != reject || err != nil && !reject {
			t.Fatal(id, err)
		}
		c.Defeat = func(string) error { t.Fatal("admission ran defeat"); return nil }
		_, err = d.InvocationAction(&s, &frame, []string{"prior", id}, noArithmeticDraws{t})
		if (err != nil) != (id == "job-31-reverse") {
			t.Fatal(id, err)
		}
	}
}

func TestArithmeticRegisteredDefeatChangesLaterMemberEligibility(t *testing.T) {
	s, c, d := arithmeticRegistryFixture()
	s.Actors[0].MP, s.Actors[0].HP = 5, 999
	s.Actors[2].HP = 1000
	var defeated []string
	c.Defeat = func(id string) error {
		defeated = append(defeated, id)
		if id == "self" {
			s.Actors[0].HP = 100
			s.Actors[1].HP = 100
			s.Actors[2].HP = 999
		}
		return nil
	}
	frame := corebattle.CallbackContext{TargetContext: c.TargetContext}
	ops, err := d.InvocationAction(&s, &frame, []string{"job-31-death"}, noArithmeticDraws{t})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyInvocations(&frame, d.Invocations, ops, noArithmeticDraws{t}); err != nil || !reflect.DeepEqual(defeated, []string{"self", "ally", "enemy"}) || s.Actors[0].HP != 100 || s.Actors[1].HP != 0 || s.Actors[2].HP != 0 {
		t.Fatal(err, defeated, s)
	}
}

func TestArithmeticFrozenContinuationReloadWithoutReplay(t *testing.T) {
	var final corebattle.LifetimeState
	for _, reload := range []bool{false, true} {
		s, c, _ := arithmeticRegistryFixture()
		s.Actors[2].HP, s.Actors[2].MaxHP = 900, 321
		s.Actors[1].Status = corebattle.StatusPoison
		input := struct {
			State      corebattle.LifetimeState
			Target     corebattle.TargetContext
			Discovered []corebattle.ArithmeticEntry
			Steps      []struct{ ID, Text string }
			Next, Draw int
		}{State: s, Target: c.TargetContext, Discovered: c.Discovered}
		input.Steps = []struct{ ID, Text string }{
			{"job-72-divisor-4", "＠倍数指定＠×魔力撃 "},
			{"job-72-divisor-4", "＠倍数指定＠×回復 "},
			{"job-31-death", ""},
			{"job-72-divisor-4", "＠倍数指定＠×蘇生 "},
			{"job-31-reverse", ""},
		}
		defeats := 0
		for input.Next < len(input.Steps) {
			if reload {
				data, err := json.Marshal(input)
				if err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(data, &input); err != nil {
					t.Fatal(err)
				}
			}
			step := input.Steps[input.Next]
			context := corebattle.ArithmeticContext{TargetContext: input.Target, SubmittedText: step.Text, Discovered: input.Discovered,
				Defeat: func(id string) error {
					defeats++
					if id != "enemy" {
						t.Fatal(id)
					}
					return nil
				}}
			d := BattleDefinitions{}
			d.RegisterArithmetic(&context)
			frame := corebattle.CallbackContext{TargetContext: input.Target, Arguments: []string{"enemy", "tail"}}
			ops, err := d.InvocationAction(&input.State, &frame, []string{step.ID}, noArithmeticDraws{t})
			if err != nil {
				t.Fatal(err)
			}
			rng := &damageGemDraws{t: t, draws: []float64{0, .99}, used: input.Draw}
			if err := input.State.ApplyInvocations(&frame, d.Invocations, ops, rng); err != nil {
				t.Fatal(err)
			}
			input.Draw, input.Next = rng.used, input.Next+1
		}
		if input.State.Actors[2].HP != 123 || input.State.Actors[1].HP != 500 || input.State.Actors[1].Status != corebattle.StatusPoison || defeats != 1 || input.Draw != 2 {
			t.Fatal(input, defeats)
		}
		if reload && !reflect.DeepEqual(input.State, final) {
			t.Fatal("reload changed continuation", input.State, final)
		}
		final = input.State
	}
}
