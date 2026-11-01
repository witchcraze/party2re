package skill

import (
	"errors"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestDefenseGemProjectionBeforeLaterCallbacks(t *testing.T) {
	s, c := recoveryGemFixture(false)
	var d BattleDefinitions
	d.RegisterDefenseGems()
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"ally"}}
	ops, err := d.InvocationAction(&s, &f, []string{"gem_cover", "gem_awakening_sky", "gem_guard_2"}, noArithmeticDraws{t})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyInvocations(&f, d.Invocations, ops, noArithmeticDraws{t}); err != nil || s.Actors[0].Temporary != corebattle.TemporaryDefense || s.Actors[0].ExtendedTurns != 8 {
		t.Fatal(err, s)
	}
	// No uncertain preceding callback may bypass validation of a later entry.
	s, c = recoveryGemFixture(false)
	s.Actors[2].Temporary = corebattle.TemporaryAbsorbMagic
	f = corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"ally"}}
	before := s.Actors[2]
	if err := s.ApplyInvocations(&f, d.Invocations, []corebattle.CallbackInvocation{{Kind: "entry", EntryID: "gem_guard_2"}, {Kind: "entry", EntryID: "missing"}}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) || !reflect.DeepEqual(s.Actors[2], before) {
		t.Fatal(err, s)
	}
	// Absorption has random MP effects, so Project must return unknown without
	// writing even the deterministic incoming defense into the validation state.
	project := d.Invocations.Entries["gem_guard_2"].Project
	if known, err := project(&s, &f); err != nil || known || !reflect.DeepEqual(s.Actors[2], before) || !reflect.DeepEqual(f.Arguments, []string{"ally"}) {
		t.Fatal(known, err, s, f)
	}
}

func TestDefenseGemCustomFallbackAndPhysicalSeal(t *testing.T) {
	for _, tc := range []struct {
		target string
		custom bool
		draws  []float64
		ally   bool
	}{
		{"missing", true, nil, false},
		{"missing", false, []float64{.9}, true},
		{"ally", true, nil, true},
	} {
		s, c := recoveryGemFixture(false)
		c.Custom = tc.custom
		var d BattleDefinitions
		d.RegisterDefenseGems()
		f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{tc.target}}
		rng := &queuedRecoveryGemRNG{t: t, draws: tc.draws}
		if err := s.ApplyInvocations(&f, d.Invocations, []corebattle.CallbackInvocation{{Kind: "entry", EntryID: "gem_awakening_sky"}}, rng); err != nil || len(rng.draws) != 0 {
			t.Fatal(tc, err, rng.draws)
		}
		index := 0
		if tc.ally {
			index = 2
		}
		if s.Actors[index].ExtendedTurns != 8 {
			t.Fatal(tc, s)
		}
	}
	for _, draw := range []float64{.2499, .25} {
		s, c := recoveryGemFixture(false)
		s.Actors[0].Status = corebattle.StatusPhysicalSeal
		var d BattleDefinitions
		d.RegisterDefenseGems()
		f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"ally"}}
		rng := &queuedRecoveryGemRNG{t: t, draws: []float64{draw}}
		if err := s.ApplyInvocations(&f, d.Invocations, []corebattle.CallbackInvocation{{Kind: "entry", EntryID: "gem_cover"}}, rng); err != nil || len(rng.draws) != 0 {
			t.Fatal(err, rng.draws)
		}
		if (s.Actors[0].Temporary == corebattle.TemporaryCovering) != (draw < .25) || (s.Actors[2].Temporary == corebattle.TemporaryCover) != (draw < .25) {
			t.Fatal(draw, s)
		}
	}
}
