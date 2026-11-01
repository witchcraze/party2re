package battle

import (
	"errors"
	"reflect"
	"testing"

	"github.com/witchcraze/party2re/internal/core/random"
)

func TestDefenseGemAdmissionAndOrderedTargets(t *testing.T) {
	sealed, sealedContext := targetFixture()
	sealed.actor("self").Status = StatusMagicSeal
	sealedBefore := reloadLifetime(t, sealed)
	if _, err := sealed.ApplyDefenseGems([]int{34, 41}, sealedContext, "ally", &targetRNG{t: t}); !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(sealedBefore, sealed) {
		t.Fatal("later unchecked helper rejected after a prefix", err, sealed)
	}
	for _, ids := range [][]int{{34, 101}, {84, 102}, {52, 110}, {34, 34, 34, 34}, {}} {
		s, c := targetFixture()
		before := reloadLifetime(t, s)
		if _, err := s.ApplyDefenseGems(ids, c, "ally", &targetRNG{t: t}); err == nil || !reflect.DeepEqual(before, s) {
			t.Fatal(ids, err, s)
		}
	}
	// The first area write removes deflection before the physical cover helper.
	s, c := targetFixture()
	s.Actors, c.EnemyIDs = s.Actors[:3], nil
	s.actor("ally").Temporary = TemporaryDeflect
	before := reloadLifetime(t, s)
	if err := s.ValidateDefenseGems([]int{36, 52, 85}, c, "ally", &targetRNG{t: t}); err != nil || !reflect.DeepEqual(before, s) {
		t.Fatal(err, s)
	}
	if _, err := s.ApplyDefenseGems([]int{36, 52, 85}, c, "ally", &targetRNG{t: t}); err != nil || s.actor("self").Temporary != TemporaryCovering || s.actor("ally").Temporary != TemporaryCover || s.actor("self").Extended != ExtendedAwakening || s.actor("self").ExtendedTurns != 8 {
		t.Fatal(err, s)
	}
	// A failed seal cure leaves the deflection reachable in a later slot.
	s, c = targetFixture()
	s.Actors, c.EnemyIDs = s.Actors[:3], nil
	s.actor("self").Status, s.actor("ally").Temporary = StatusMagicSeal, TemporaryDeflect
	before = reloadLifetime(t, s)
	if _, err := s.ApplyDefenseGems([]int{36, 52}, c, "ally", &targetRNG{t: t}); !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(before, s) {
		t.Fatal(err, s)
	}
}

func TestDefenseGemTargetAndLifetimeBoundaries(t *testing.T) {
	for _, tc := range []struct {
		id                   int
		target, status, want string
		tmp                  TemporaryEffect
		draws                []float64
	}{
		{35, "ally", "", "self", TemporaryReflectMagic, nil},
		{51, "fallen", "", "self", "", nil},
		{85, "fallen", "", "self", "", nil},
		{85, "ally", "", "ally", TemporaryReflectMagic, nil},
		{85, "ally", StatusConfusion, "dead_enemy", "", []float64{.9}},
		{52, "self", "", "self", "", nil},
		{52, "ally", "", "self", TemporaryReflectAttack, nil},
	} {
		s, c := targetFixture()
		s.actor("self").Status = tc.status
		s.actor("ally").Temporary = tc.tmp
		rng := &targetRNG{t: t, draws: tc.draws}
		r, err := s.ApplyDefenseGems([]int{tc.id}, c, tc.target, rng)
		if err != nil {
			t.Fatal(tc, err)
		}
		if tc.id == 85 {
			if s.actor(tc.want).Extended != ExtendedAwakening || s.actor(tc.want).ExtendedTurns != 8 {
				t.Fatal(tc, s)
			}
		} else if tc.id == 52 {
			if r[0].Applied || s.actor("self").Temporary != "" {
				t.Fatal(tc, r, s)
			}
		} else if !r[0].Applied || s.actor(tc.want).Temporary == "" {
			t.Fatal(tc, r, s)
		}
		rng.done()
	}
	s, c := targetFixture()
	if _, err := s.ApplyDefenseGems([]int{34, 84}, c, "", &targetRNG{t: t}); err != nil {
		t.Fatal(err)
	}
	s = reloadLifetime(t, s)
	a := s.actor("self")
	if err := a.BeginNormalAction(false, false, false, &targetRNG{t: t}); err != nil || a.Temporary != "" {
		t.Fatal(err, a)
	}
	for i := 0; i < 8; i++ {
		expired := a.FinishExtended(LifetimeTail{})
		if expired != (i == 7) || a.ExtendedTurns != 7-i {
			t.Fatal(i, a)
		}
	}
	if _, err := s.ApplyDefenseGems([]int{84}, c, "", &targetRNG{t: t}); err != nil {
		t.Fatal(err)
	}
	a = s.actor("self")
	a.ResetEffects(LifetimeReset{})
	if a.Extended != "" || a.ExtendedTurns != 0 || a.Temporary != "" {
		t.Fatal(a)
	}
}

func TestDefenseGemFiniteAdmissionExecutionAgreement(t *testing.T) {
	// Sample source triples against interacting seal/confusion and temporary
	// classes; every admitted sequence must execute on each remaining draw tape.
	entries := DefenseGemEntries()
	for i := 0; i < 66; i++ {
		s, c := targetFixture()
		c.Custom, c.Item195Activated = true, i%2 == 0
		s.actor("self").Status = []string{"", StatusMagicSeal, StatusPhysicalSeal, StatusConfusion}[i%4]
		s.actor("ally").Temporary = []TemporaryEffect{TemporaryCover, TemporaryReflectMagic, TemporaryAbsorbMagic, TemporaryDeflect, TemporaryNullMagic, TemporaryPhantom}[i%6]
		s.actor("self").Temporary = TemporaryCovering
		ids := []int{entries[i%22].SourceID, entries[(i*7+3)%22].SourceID, entries[(i*13+5)%22].SourceID}
		before := reloadLifetime(t, s)
		err := s.ValidateDefenseGems(ids, c, "ally", &targetRNG{t: t})
		if !reflect.DeepEqual(s, before) {
			t.Fatal("admission mutated input", i, ids)
		}
		if err != nil {
			continue
		}
		for seed := uint64(1); seed <= 3; seed++ {
			state := reloadLifetime(t, s)
			if _, err := state.ApplyDefenseGems(ids, c, "ally", random.NewDeterministic(seed)); err != nil {
				t.Fatal(i, ids, seed, err)
			}
		}
	}
}

func TestDefenseGemAwakeningAndAbsorption(t *testing.T) {
	for _, id := range []int{84, 85} {
		s, c := targetFixture()
		s.actor("self").Status = StatusMagicSeal
		if _, err := s.ApplyDefenseGems([]int{id}, c, "ally", &targetRNG{t: t}); err != nil {
			t.Fatal(err)
		}
		target := "self"
		if id == 85 {
			target = "ally"
		}
		if s.actor(target).Extended != ExtendedAwakening || s.actor(target).ExtendedTurns != 8 || s.actor("self").Status != StatusMagicSeal {
			t.Fatal(id, s)
		}
	}
	s, c := targetFixture()
	s.actor("ally").Temporary, s.actor("ally").MaxMP = TemporaryAbsorbMagic, 40
	rng := &targetRNG{t: t, draws: []float64{.9}}
	r, err := s.ApplyDefenseGems([]int{35}, c, "ally", rng)
	if err != nil || len(r) != 1 || r[0].MPRecovery != 48 || s.actor("ally").MP != 40 || s.actor("ally").Temporary != TemporaryDefense {
		t.Fatal(r, err, s)
	}
	rng.done()
}
