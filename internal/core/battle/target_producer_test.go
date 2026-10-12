package battle

import (
	"errors"
	"reflect"
	"testing"
)

func TestFiniteTemporaryGemProducers(t *testing.T) {
	for _, tc := range []struct {
		id     int
		effect TemporaryEffect
		scope  string
	}{
		{34, TemporaryDefense, TargetScopeSelf}, {35, TemporaryDefense, TargetScopeSingleAlly},
		{36, TemporaryDefense, TargetScopeAllAllies}, {37, TemporaryStrongDefense, TargetScopeSelf},
		{38, TemporaryReduceAttack, TargetScopeAllAllies}, {39, TemporaryReduceMagic, TargetScopeAllAllies},
		{40, TemporaryReduceBreath, TargetScopeAllAllies}, {41, TemporaryNullAttack, TargetScopeSingleAlly},
		{42, TemporaryNullMagic, TargetScopeSingleAlly}, {43, TemporaryNullMagic, TargetScopeAllAllies},
		{44, TemporaryReflectAttack, TargetScopeSelf}, {45, TemporaryReflectAttack, TargetScopeSingleAlly},
		{46, TemporaryReflectMagic, TargetScopeSelf}, {47, TemporaryReflectMagic, TargetScopeSingleAlly},
		{48, TemporaryReflectMagic, TargetScopeAllAllies}, {49, TemporaryReflectBreath, TargetScopeAllAllies},
		{50, TemporaryRegeneration, TargetScopeSingleAlly}, {51, TemporaryRevival, TargetScopeSingleAlly},
		{52, TemporaryCover, TargetScopeSingleAlly}, {53, TemporaryAbsorbMagic, TargetScopeSelf},
	} {
		t.Run(string(tc.effect)+tc.scope, func(t *testing.T) {
			s, c := targetFixture()
			s = reloadLifetime(t, s)
			before := reloadLifetime(t, s)
			rng := &targetRNG{t: t}
			applied, err := s.ApplyTemporaryGem(tc.id, c, "ally", rng)
			if err != nil || !applied.Applied {
				t.Fatal(tc, applied, err)
			}
			for i, a := range s.Actors {
				want := TemporaryEffect("")
				if (tc.scope == TargetScopeSelf && i == 0) || (tc.scope == TargetScopeSingleAlly && i == 2) || (tc.scope == TargetScopeAllAllies && (i == 0 || i == 2)) {
					want = tc.effect
				}
				if tc.id == 52 && i == 0 {
					want = TemporaryCovering
				}
				if a.Temporary != want {
					t.Fatal(tc, a, want)
				}
				a.Temporary = ""
				if !reflect.DeepEqual(a, before.Actors[i]) {
					t.Fatal("producer changed resources or incidental state", a)
				}
			}
			rng.done()
			reloadLifetime(t, s)
		})
	}
}

func TestTemporaryGemGuardsAndRejection(t *testing.T) {
	for id := 34; id <= 53; id++ {
		s, c := targetFixture()
		s.Actors[0].Status = StatusMagicSeal
		rng := &targetRNG{t: t}
		unconditional := id == 34 || id == 37 || id == 52
		unsupported := id == 41 || id == 42 || id == 45
		if !unconditional && !unsupported {
			rng.draws = []float64{0.25}
		}
		before := reloadLifetime(t, s)
		applied, err := s.ApplyTemporaryGem(id, c, "ally", rng)
		if applied.Applied != unconditional || (err != nil) != unsupported {
			t.Fatal(id, applied, err)
		}
		if !unconditional && !reflect.DeepEqual(s, before) {
			t.Fatal(id, "rejection mutated state")
		}
		rng.done()
	}
	// Revival TEMP is not the revival PAYLOAD exception for a fallen target.
	for _, id := range []int{35, 47, 50, 51} {
		s, c := targetFixture()
		applied, err := s.ApplyTemporaryGem(id, c, "fallen", &targetRNG{t: t})
		if err != nil || !applied.Applied || s.Actors[0].Temporary == "" || s.Actors[1].Temporary != "" {
			t.Fatal(id, applied, err, s)
		}
	}
	s, c := targetFixture()
	applied, err := s.ApplyTemporaryGem(52, c, "self", &targetRNG{t: t})
	if err != nil || applied.Applied || s.Actors[0].Temporary != "" {
		t.Fatal(applied, err, s)
	}
	before := reloadLifetime(t, s)
	_, err = s.ApplyTemporaryGem(54, c, "ally", &targetRNG{t: t})
	if !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(before, s) {
		t.Fatal(err, s)
	}
	// Reflection affects producer target once, with no positive/numeric coercion.
	s.Actors[2].Temporary, s.Actors[2].ItemDefinitionIDs = TemporaryReflectMagic, []string{"item-163"}
	applied, err = s.ApplyTemporaryGem(35, c, "ally", &targetRNG{t: t})
	if err != nil || !applied.Applied || s.Actors[0].Temporary != TemporaryDefense || s.Actors[2].Temporary != TemporaryReflectMagic {
		t.Fatal(applied, err, s)
	}
	s.Actors[2].Temporary = TemporaryAbsorbMagic
	rng := &targetRNG{t: t, draws: []float64{0.9}}
	applied, err = s.ApplyTemporaryGem(35, c, "ally", rng)
	if err != nil || !applied.Applied || applied.MPRecovery != 48 || applied.MPRecoveryTarget != "ally" || s.Actors[2].Temporary != TemporaryDefense {
		t.Fatal("producer lost incidental absorption", applied, err, s)
	}
	rng.done()
}
