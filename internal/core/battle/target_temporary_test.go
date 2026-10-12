package battle

import (
	"errors"
	"reflect"
	"testing"
)

func TestTemporaryNumericAndSymbolicPayloads(t *testing.T) {
	for _, tc := range []struct {
		tmp      TemporaryEffect
		category CallbackCategory
		want     float64
		changes  bool
	}{
		{TemporaryStrongDefense, CategoryMagic, 1.1, true},
		{TemporaryDefense, CategoryPhysical, 5.5, true},
		{TemporaryReduceAttack, CategoryPhysical, 2.75, true},
		{TemporaryReduceAttack, CategoryMagic, 11, false},
		{TemporaryReduceMagic, CategoryMagic, 2.75, true},
		{TemporaryReduceMagic, CategoryBreath, 11, false},
		{TemporaryReduceBreath, CategoryBreath, 2.75, true},
		{TemporaryReduceBreath, CategoryPhysical, 11, false},
		{TemporaryDouble, CategoryUntyped, 22, true},
	} {
		for _, symbol := range []string{"", "revival", "defense"} {
			s, c := targetFixture()
			s.Actors[3].Temporary = tc.tmp
			payload := CallbackPayload{Number: 11}
			if symbol != "" {
				payload = CallbackPayload{Symbol: symbol}
			}
			rng := &targetRNG{t: t}
			e := TargetEffect{TargetID: "enemy", Category: tc.category, Payload: payload}
			r, err := s.ResolveTemporary(c, e, rng)
			if err != nil {
				t.Fatal(err)
			}
			if (r.Payload != nil) != (tc.changes && symbol == "") {
				t.Fatal(tc, symbol, r)
			}
			if r.Payload != nil && r.Payload.Number != tc.want {
				t.Fatal(tc, r)
			}
			rng.done()
		}
	}
}

func TestTemporaryCoverAndReflectionAreOnePass(t *testing.T) {
	s, c := targetFixture()
	s.Actors[1].Temporary, s.Actors[2].Temporary = TemporaryCovering, TemporaryCovering
	s.Actors[0].Temporary = TemporaryCover
	rng := &targetRNG{t: t}
	r, err := s.ResolvePartyTarget(c, TargetEffect{TargetID: "self", Category: CategoryMagic, Payload: CallbackPayload{Number: 7}}, rng)
	if err != nil || r.TargetID != "fallen" || r.Payload.Number != 7 {
		t.Fatal(r, err)
	}
	// A protector's own effect is never recursively dispatched, even when fallen.
	s.Actors[1].Temporary = TemporaryReflectMagic
	r, err = s.ResolvePartyTarget(c, TargetEffect{TargetID: "self", Category: CategoryMagic}, rng)
	if err != nil || r.TargetID != "ally" {
		t.Fatal(r, err)
	}
	s.Actors[2].Temporary = ""
	r, err = s.ResolvePartyTarget(c, TargetEffect{TargetID: "self", Category: CategoryMagic}, rng)
	if err != nil || r.TargetID != "self" {
		t.Fatal(r, err)
	}
	rng.done()
	for _, tc := range []struct {
		tmp      TemporaryEffect
		category CallbackCategory
	}{
		{TemporaryReflectAttack, CategoryPhysical}, {TemporaryReflectMagic, CategoryMagic},
		{TemporaryReflectBreath, CategoryBreath}, {TemporaryReflectDance, CategoryDance},
	} {
		for _, item163 := range []bool{false, true} {
			for _, payload := range []CallbackPayload{{Number: 11}, {}, {Symbol: "revival"}} {
				s, c := targetFixture()
				s.Actors[0].Temporary, s.Actors[3].Temporary = tc.tmp, tc.tmp
				if item163 {
					s.Actors[3].ItemDefinitionIDs = []string{"item-163"}
				}
				rng := &targetRNG{t: t}
				r, err := s.ResolveEnemyTarget(c, TargetEffect{TargetID: "enemy", Category: tc.category, Payload: payload}, rng)
				want := payload
				if item163 {
					want.Number *= 2
				}
				if err != nil || !r.Resolved || r.TargetID != "self" || r.Payload != want || s.Actors[3].Temporary != tc.tmp {
					t.Fatal(r, err)
				}
				rng.done()
			}
		}
	}
}

func TestTemporaryNullHasNoPositiveFloor(t *testing.T) {
	for _, tc := range []struct {
		tmp      TemporaryEffect
		category CallbackCategory
	}{
		{TemporaryNullAttack, CategoryPhysical}, {TemporaryNullMagic, CategoryMagic},
	} {
		s, c := targetFixture()
		s.Actors[3].Temporary = tc.tmp
		r, err := s.ResolveEnemyTarget(c, TargetEffect{TargetID: "enemy", Category: tc.category, Payload: CallbackPayload{Number: 1}}, &targetRNG{t: t})
		if err != nil || !r.Resolved || r.Payload.Number != 0 || s.Actors[3].HP != 100 {
			t.Fatal(r, err)
		}
		r, err = s.ResolveEnemyTarget(c, TargetEffect{TargetID: "enemy", Category: CategorySong, Payload: CallbackPayload{Number: 1}}, &targetRNG{t: t})
		if err != nil || r.Payload.Number != 1 {
			t.Fatal(r, err)
		}
	}
}

func TestTemporaryOrderedRandomDraws(t *testing.T) {
	for _, tc := range []struct {
		name  string
		tmp   TemporaryEffect
		draws []float64
		want  string
	}{
		{"deflect party includes fallen", TemporaryDeflect, []float64{0.49, 0.4}, "fallen"},
		{"deflect enemy original still proceeds", TemporaryDeflect, []float64{0.5, 0}, "enemy"},
		{"deflect enemy fallen", TemporaryDeflect, []float64{0.9, 0.9}, "dead_enemy"},
		{"phantom draw then retain", TemporaryPhantom, []float64{0.4, 0.49}, "enemy"},
		{"phantom draw then replace", TemporaryPhantom, []float64{0.4, 0.5}, "fallen"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, c := targetFixture()
			s.Actors[3].Temporary = tc.tmp
			rng := &targetRNG{t: t, draws: tc.draws}
			r, err := s.ResolveEnemyTarget(c, TargetEffect{TargetID: "enemy", Category: CategoryPhysical}, rng)
			if err != nil || !r.Resolved || r.TargetID != tc.want {
				t.Fatal(r, err)
			}
			rng.done()
		})
	}
	s, c := targetFixture()
	s.Actors[0].Temporary, s.Actors[3].Temporary = TemporaryPhantom, TemporaryNullMagic
	r, err := s.ResolveEnemyTarget(c, TargetEffect{TargetID: "enemy", Category: CategoryMagic, Payload: CallbackPayload{Number: 10}}, &targetRNG{t: t})
	if err != nil || r.Payload.Number != 10 {
		t.Fatal(r, err)
	}
	// Named phantom redirection occurs after the single temporary dispatch, even
	// under anti-field (the source get_field_name does not consult anti-field).
	s.Fields = LifetimeFields{Default: "phantom", AntiTurns: 2}
	c.PhantomTargetID, c.PhantomEnemyIDs = "enemy", []string{"dead_enemy"}
	rng := &targetRNG{t: t, draws: []float64{0.9}}
	r, err = s.ResolveEnemyTarget(c, TargetEffect{TargetID: "enemy", Category: CategoryMagic}, rng)
	if err != nil || r.TargetID != "dead_enemy" {
		t.Fatal(r, err)
	}
	rng.done()
}

func TestMagicAbsorptionContinuesIncomingEffect(t *testing.T) {
	for _, tc := range []struct {
		value, draw float64
		den         bool
		want        int
	}{
		{49, 0, false, 30}, {49, 0.999, false, 49},
		{50, 0, false, 10}, {50, 0.999, false, 24},
		{200, 0, false, 40}, {200, 0.999, true, 3},
		{49, 0, true, 1},
	} {
		s, c := targetFixture()
		s.Actors[3].Temporary = TemporaryAbsorbMagic
		if tc.den {
			s.Fields.Default = "den"
		}
		rng := &targetRNG{t: t, draws: []float64{tc.draw}}
		r, err := s.ResolveEnemyTarget(c, TargetEffect{TargetID: "enemy", Category: CategoryMagic, Payload: CallbackPayload{Number: tc.value}}, rng)
		if err != nil || !r.Resolved || r.TargetID != "enemy" || r.Payload.Number != tc.value || r.MPRecovery != tc.want || r.MPRecoveryTarget != "enemy" {
			t.Fatal(tc, r, err)
		}
		// The resource owner caps MP, then the original attack can continue.
		target := s.actor(r.TargetID)
		target.MP = min(target.MaxMP, target.MP+r.MPRecovery)
		if target.MP > 100 || target.MP <= 90 || target.HP != 100 {
			t.Fatal(target)
		}
		rng.done()
	}
}

func TestOptionalTemporaryReturnsForToolHooks(t *testing.T) {
	s, c := targetFixture()
	s.Actors[3].Temporary = TemporaryCover
	s.Actors[4].Temporary = TemporaryCovering
	r, err := s.ResolveTemporary(c, TargetEffect{TargetID: "enemy", Payload: CallbackPayload{Number: 40}}, &targetRNG{t: t})
	if err != nil || r.TargetID == nil || *r.TargetID != "dead_enemy" || r.Payload != nil {
		t.Fatal(r, err)
	}
	// A spell retains its incoming value; a tool's tuple assignment sees the
	// absent value. This distinction must not be erased in this shared operation.
	spell, err := s.ResolvePartyTarget(c, TargetEffect{TargetID: "ally", Payload: CallbackPayload{Number: 40}}, &targetRNG{t: t})
	if err != nil || spell.Payload.Number != 40 {
		t.Fatal(spell, err)
	}
	s.Actors[3].Temporary = TemporaryDefense
	r, err = s.ResolveTemporary(c, TargetEffect{TargetID: "enemy", Payload: CallbackPayload{Number: 40}}, &targetRNG{t: t})
	if err != nil || r.Payload == nil || r.Payload.Number != 20 {
		t.Fatal(r, err)
	}
}

func TestUnsupportedTargetContextsRejectBeforeDrawsAndMutation(t *testing.T) {
	for _, branch := range []string{"boss absent", "boss cure then activation", "deflection empty side", "symbol conversion"} {
		s, c := targetFixture()
		e := TargetEffect{TargetID: "missing", Category: CategoryPhysical}
		switch branch {
		case "boss absent":
			s.SpecialBossField, s.Actors[0].Status = true, StatusConfusion
		case "boss cure then activation":
			s.SpecialBossField, s.Actors[0].Status, c.Item195Activated = true, StatusMagicSeal, true
			e.Category = CategoryMagic
		case "deflection empty side":
			s.Actors, c.EnemyIDs = s.Actors[:3], nil
			s.Actors[1].Temporary = TemporaryDeflect
		case "symbol conversion":
			s.Actors[0].Extended, c.Command, c.Item195Activated = ExtendedMagicSword, "attack", true
			e.Payload.Symbol = "revival"
		}
		before := reloadLifetime(t, s)
		rng := &targetRNG{t: t}
		var err error
		if branch == "symbol conversion" {
			_, err = s.ResolveEnemyTarget(c, e, rng)
		} else {
			_, err = s.ResolvePartyTarget(c, e, rng)
		}
		if !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(s, before) {
			t.Fatal(branch, err, s)
		}
		rng.done()
	}
}

func TestTargetContinuationRoundTrip(t *testing.T) {
	run := func(reload bool) (LifetimeState, []TargetResolution, int) {
		s, c := targetFixture()
		s.Actors[0].Status, s.Actors[3].Temporary = StatusMagicSeal, TemporaryAbsorbMagic
		c.Item195Activated = true
		rng := &targetRNG{t: t, draws: []float64{0.24, 0.9, 0.19, 0.8, 0.2}}
		var results []TargetResolution
		for range 2 {
			if reload {
				s = reloadLifetime(t, s)
			}
			r, err := s.ResolveEnemyTarget(c, TargetEffect{TargetID: "enemy", Category: CategoryMagic, Payload: CallbackPayload{Number: 50}}, rng)
			if err != nil {
				t.Fatal(err)
			}
			results = append(results, r)
			if r.MPRecoveryTarget != "" {
				a := s.actor(r.MPRecoveryTarget)
				a.MP = min(a.MaxMP, a.MP+r.MPRecovery)
			}
			if reload {
				s = reloadLifetime(t, s)
			}
		}
		rng.done()
		return s, results, rng.used
	}
	a, resultsA, drawsA := run(false)
	b, resultsB, drawsB := run(true)
	if !reflect.DeepEqual(a, b) || !reflect.DeepEqual(resultsA, resultsB) || drawsA != drawsB {
		t.Fatal(a, b, resultsA, resultsB, drawsA, drawsB)
	}
}
