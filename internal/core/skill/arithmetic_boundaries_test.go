package skill

import (
	"errors"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestArithmeticRegisteredResidues(t *testing.T) {
	for _, divisor := range []struct {
		id string
		n  int
	}{{"job-72-divisor-3", 3}, {"job-72-divisor-4", 4}, {"job-72-divisor-5", 5}} {
		for mp := 0; mp <= 2*divisor.n; mp++ {
			for _, leaf := range corebattle.ArithmeticEntries("job-72")[3:] {
				s, c, d := arithmeticRegistryFixture()
				s.Actors[1].MP, s.Actors[2].MP = mp, mp
				c.SubmittedText = "＠倍数指定＠" + leaf.Name + " "
				c.Defeat = func(string) error { return nil }
				frame := corebattle.CallbackContext{TargetContext: c.TargetContext}
				ops, err := d.InvocationAction(&s, &frame, []string{divisor.id}, noArithmeticDraws{t})
				if err != nil {
					t.Fatal(divisor, leaf, err)
				}
				selected := mp != 0 && mp%divisor.n == 0
				rng := &damageGemDraws{t: t}
				if selected && (leaf.ID == "job-72-damage" || leaf.ID == "job-72-heal") {
					rng.draws = []float64{0}
				}
				if err := s.ApplyInvocations(&frame, d.Invocations, ops, rng); err != nil || rng.used != len(rng.draws) {
					t.Fatal(divisor, mp, leaf, err, rng)
				}
				wantHP := 120
				if selected {
					switch leaf.ID {
					case "job-72-damage", "job-72-death":
						wantHP = 0
					case "job-72-heal":
						wantHP = 570
					}
				}
				if s.Actors[2].HP != wantHP || (s.Actors[2].Temporary == corebattle.TemporaryReflectMagic) != (selected && leaf.ID == "job-72-reflect") || (s.Actors[1].HP == 500) != (selected && leaf.ID == "job-72-revive") || frame.ArithmeticDivisor != divisor.n {
					t.Fatal(divisor, mp, leaf.ID, s, frame)
				}
			}
		}
	}
}

func TestArithmeticRegisteredHPAndBaseDefenseBoundaries(t *testing.T) {
	for _, tc := range []struct {
		id                       string
		hp, defense, want, draws int
	}{
		{"job-31-gravity", 999, 1000, 249, 0}, {"job-31-gravity", 1000, 999, 1000, 0},
		{"job-31-death", 999, 1000, 0, 0}, {"job-31-death", 1000, 999, 1000, 0},
		{"job-72-death", 999, 1000, 0, 0}, {"job-72-death", 1000, 999, 1000, 0},
		{"job-72-damage", 1000, 999, 766, 1}, {"job-72-damage", 1000, 1000, 999, 2},
		{"job-31-reverse", 10, 0, 1, 0}, {"job-31-reverse", 100, 0, 1, 0},
		{"job-31-reverse", 999, 0, 999, 0}, {"job-31-reverse", 1000, 0, 1000, 0},
	} {
		s, c, d := arithmeticRegistryFixture()
		s.Actors[1].MP = 0
		s.Actors[2].HP, s.Actors[2].BaseStats.Defense = tc.hp, tc.defense
		c.Defeat = func(string) error { return nil }
		frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}, ArithmeticDivisor: 4}
		rng := &damageGemDraws{t: t, draws: make([]float64, tc.draws)}
		if err := s.ApplyInvocations(&frame, d.Invocations, []corebattle.CallbackInvocation{{Kind: "entry", EntryID: tc.id}}, rng); err != nil || s.Actors[2].HP != tc.want || rng.used != tc.draws {
			t.Fatal(tc, s, err, rng)
		}
	}
}

func TestArithmeticRegisteredMagicGuardAndArgumentConsumption(t *testing.T) {
	for _, job := range []string{"job-31", "job-72"} {
		for _, entry := range corebattle.ArithmeticEntries(job) {
			for _, cure := range []float64{.2499, .25} {
				s, c, d := arithmeticRegistryFixture()
				for i := range s.Actors {
					s.Actors[i].MP = 0
				}
				s.Actors[0].Status = corebattle.StatusMagicSeal
				c.SubmittedText = "＠倍数指定＠×反射 "
				frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy", "tail"}}
				rng := &damageGemDraws{t: t, draws: []float64{cure}}
				def := d.Invocations.Entries[entry.ID]
				if err := def.Validate(&s, &frame); err != nil {
					t.Fatal(entry, err)
				}
				if err := def.Execute(&s, &frame, rng); err != nil || rng.used != 1 || (s.Actors[0].Status == "") != (cure < .25) {
					t.Fatal(entry, cure, err, s)
				}
				wantArgs := []string{"enemy", "tail"}
				if entry.ID == "job-31-reverse" && cure < .25 {
					wantArgs = []string{"tail"}
				}
				if !reflect.DeepEqual(frame.Arguments, wantArgs) {
					t.Fatal(entry, cure, frame)
				}
			}
		}
	}
}

func TestArithmeticRegisteredLookupCapturedIdentityAndOperationDivisor(t *testing.T) {
	s, c, d := arithmeticRegistryFixture()
	c.Discovered = []corebattle.ArithmeticEntry{{ID: "unknown", Name: "×重複"}, {ID: "job-72-reflect", Name: "×重複"}}
	c.SubmittedText = "＠倍数指定＠×重複 "
	frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy", "tail"}}
	if err := s.ApplyInvocations(&frame, d.Invocations, []corebattle.CallbackInvocation{{Kind: "entry", EntryID: "job-72-divisor-4"}}, noArithmeticDraws{t}); err != nil || s.Actors[2].Temporary != corebattle.TemporaryReflectMagic || frame.ArithmeticDivisor != 4 || c.Divisor != 0 {
		t.Fatal(err, s, frame, c)
	}
	c.Discovered = append(c.Discovered, corebattle.ArithmeticEntry{ID: "unknown", Name: "×重複"})
	if _, err := d.InvocationAction(&s, &frame, []string{"job-72-divisor-4"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	// Captured positional leaf ignores the changed last-match discovery map.
	if err := d.Invocations.Entries["job-72-reflect"].Execute(&s, &frame, noArithmeticDraws{t}); err != nil || !reflect.DeepEqual(frame.Arguments, []string{"enemy", "tail"}) {
		t.Fatal(err, frame)
	}
	// A fresh operation has no divisor, even after a prior selector succeeded.
	frame = corebattle.CallbackContext{TargetContext: c.TargetContext}
	if err := s.ApplyInvocations(&frame, d.Invocations, []corebattle.CallbackInvocation{{Kind: "entry", EntryID: "job-72-damage"}}, noArithmeticDraws{t}); err != nil || s.Actors[2].HP != 120 {
		t.Fatal(err, s)
	}
	for _, text := range []string{"", "＠倍数指定＠×重複", "＠倍数指定＠×未発見 "} {
		c.SubmittedText = text
		if err := s.ApplyInvocations(&frame, d.Invocations, []corebattle.CallbackInvocation{{Kind: "entry", EntryID: "job-72-divisor-4"}}, noArithmeticDraws{t}); err != nil || frame.ArithmeticDivisor != 0 {
			t.Fatal(text, err, frame)
		}
	}
}

func TestArithmeticRegisteredDelegationCostsAndCompletePools(t *testing.T) {
	s, c, d := arithmeticRegistryFixture()
	s.Actors[0].Jobs.JobID, s.Actors[0].Jobs.SP, s.Actors[0].MP = "job-31", 155, 40
	frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy", "tail"}}
	if err := s.ApplyInvocations(&frame, d.Invocations, []corebattle.CallbackInvocation{{Kind: "gem-86"}}, noArithmeticDraws{t}); err != nil || s.Actors[2].HP != 21 || s.Actors[0].MP != 5 {
		t.Fatal(err, s)
	}
	for _, kind := range []string{"tool-84", "random"} {
		if err := s.ValidateInvocations(&frame, d.Invocations, []corebattle.CallbackInvocation{{Kind: kind}}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal("partial table was certified", kind, err)
		}
	}
	// The complete job72 table can select a naked leaf without charging its
	// listed MP; a missing divisor is an accepted failed cast.
	s.Actors[0].Jobs.JobID = "job-72"
	rng := &damageGemDraws{t: t, draws: []float64{.4}}
	if err := s.ApplyInvocations(&frame, d.Invocations, []corebattle.CallbackInvocation{{Kind: "tool-84"}}, rng); err != nil || s.Actors[0].MP != 5 || s.Actors[2].HP != 21 || rng.used != 1 {
		t.Fatal(err, s, rng)
	}
}
