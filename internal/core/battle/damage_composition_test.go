package battle

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
)

func TestAreaDamageInitialThresholdAndInvocationOrder(t *testing.T) {
	for _, tc := range []struct {
		power float64
		items []string
		want  []int
	}{
		{300, []string{"item-144"}, []int{270, 270}},
		{301, []string{"item-144"}, []int{270, 299}},
		{200, []string{"item-144", "item-143"}, []int{360, 306}},
		{200, nil, []int{180, 153}},
	} {
		s, c := damageFixture()
		admitDamage(&s, &c)
		s.actor("self").ItemDefinitionIDs = tc.items
		s.actor("enemy").HP = 1000
		s.actor("dead_enemy").HP = 1000
		rng := &targetRNG{t: t, draws: []float64{0, 0}}
		r, err := s.ApplyAreaDamage(&c, tc.power, CategoryMagic, true, rng)
		if err != nil || len(r) != 2 || r[0].Amount != tc.want[0] || r[1].Amount != tc.want[1] || c.AreaDamageCalls != 1 || c.DamageCalls != 2 {
			t.Fatal(tc, r, err, c)
		}
		rng.done()
	}
	// Initial modifiers evaluate even with no enemies, before the area count.
	s, c := damageFixture()
	s.Actors = s.Actors[:3]
	c.EnemyIDs = nil
	s.actor("self").ItemDefinitionIDs = []string{"weapon-37"}
	rng := &targetRNG{t: t, draws: []float64{0}}
	r, err := s.ApplyAreaDamage(&c, 0, CategoryMagic, true, rng)
	if err != nil || len(r) != 0 || c.AreaDamageCalls != 1 || c.DamageCalls != 0 {
		t.Fatal(r, err, c)
	}
	rng.done()
	// A reflected defeat stops the loop; the later target gets no invocation.
	s, c = damageFixture()
	admitDamage(&s, &c)
	s.actor("enemy").Temporary = TemporaryReflectMagic
	s.actor("dead_enemy").HP = 1000
	rng = &targetRNG{t: t, draws: []float64{0}}
	r, err = s.ApplyAreaDamage(&c, 200, CategoryMagic, true, rng)
	if err != nil || len(r) != 1 || r[0].TargetID != "self" || s.actor("self").HP != 0 || c.DamageCalls != 1 {
		t.Fatal(r, err, c)
	}
	rng.done()
}

func TestDamageReturnedTargetDefenseAndMinimumDraws(t *testing.T) {
	s, c := damageFixture()
	admitDamage(&s, &c)
	s.actor("enemy").Temporary = TemporaryCover
	s.actor("dead_enemy").Temporary = TemporaryCovering
	s.actor("dead_enemy").HP = 1000
	s.actor("dead_enemy").Defense = 200
	s.actor("dead_enemy").Gear.ArmorDefense = 50
	rng := &targetRNG{t: t, draws: []float64{0, .9}}
	r, err := s.ApplyDamage(&c, TargetEffect{TargetID: "enemy", Category: CategoryMagic, Payload: CallbackPayload{Number: 100}}, rng)
	if err != nil || r.TargetID != "dead_enemy" || r.Amount != 2 || s.actor("enemy").HP != 100 {
		t.Fatal(r, err, s)
	}
	rng.done()
	// Confusion can select fallen: damage redirects to actor after counting.
	s, c = damageFixture()
	admitDamage(&s, &c)
	s.actor("self").Status = StatusConfusion
	rng = &targetRNG{t: t, draws: []float64{.8, .2, 0}}
	r, err = s.ApplyDamage(&c, TargetEffect{TargetID: "enemy", Category: CategoryMagic, Payload: CallbackPayload{Number: 10}, Arguments: []string{"1"}}, rng)
	if err != nil || r.TargetID != "self" || r.Amount != 9 || c.DamageCalls != 1 {
		t.Fatal(r, err, c)
	}
	rng.done()
}

func TestDamageGemConcreteFormulasAndTails(t *testing.T) {
	for _, tc := range []struct{ id, want int }{
		{1, 54}, {2, 81}, {3, 128}, {4, 128}, {5, 189}, {10, 13}, {11, 27}, {12, 67},
		{13, 45}, {14, 180}, {15, 450}, {16, 45}, {17, 180}, {18, 135}, {72, 90}, {73, 610},
	} {
		t.Run(fmt.Sprint(tc.id), func(t *testing.T) {
			s, c := damageFixture()
			admitDamage(&s, &c)
			c.GuaranteedHit = true
			s.actor("enemy").HP = 2000
			rng := &targetRNG{t: t, draws: []float64{0}}
			r, err := s.ApplyDamageGem(tc.id, &c, "enemy", rng)
			if err != nil || len(r) == 0 || r[0].Amount != tc.want {
				t.Fatal(tc, r, err)
			}
			if tc.id == 3 && (len(r) != 2 || r[1].Amount != 21 || c.End.ImmediateRiskCalls != 1) {
				t.Fatal(r, c)
			}
			if tc.id == 72 && s.actor("self").HP != 0 {
				t.Fatal(s)
			}
			rng.done()
		})
	}
	for _, id := range []int{6, 7, 8, 9} {
		s, c := damageFixture()
		admitDamage(&s, &c)
		c.GuaranteedHit = true
		s.actor("enemy").HP = 2000
		hits := map[int]int{6: 3, 7: 7, 8: 2, 9: 2}[id]
		draws := []float64{0}
		for range hits {
			draws = append(draws, 0, 0, 0)
			if id != 8 {
				draws = append(draws, .9)
			}
		}
		rng := &targetRNG{t: t, draws: draws}
		r, err := s.ApplyDamageGem(id, &c, "ignored", rng)
		if err != nil || len(r) != hits || c.DamageCalls != hits || c.AreaDamageCalls != 0 {
			t.Fatal(id, r, err, c)
		}
		want := 2
		if id == 8 {
			want = 81
		}
		for _, hit := range r {
			if hit.Amount != want {
				t.Fatal(id, hit)
			}
		}
		rng.done()
	}
	// Drain's recovery uses the original capped power, not actual damage dealt.
	s, c := damageFixture()
	admitDamage(&s, &c)
	s.actor("enemy").HP = 2000
	s.actor("enemy").Temporary = TemporaryDouble
	rng := &targetRNG{t: t, draws: []float64{0, 0}}
	r, err := s.ApplyDamageGem(26, &c, "enemy", rng)
	if err != nil || len(r) != 3 || r[1].Amount != 540 || r[2].Amount != 270 || s.actor("self").HP != 370 {
		t.Fatal(r, err, s)
	}
	rng.done()
}

func TestDamageWholeProducerAdmissionAndNoEffect(t *testing.T) {
	for _, id := range []int{1, 6, 13, 26, 72, 73, 101, 102} {
		s, c := damageFixture()
		s.actor("self").Status = StatusMagicSeal
		before := reloadLifetime(t, s)
		rng := &targetRNG{t: t}
		admission := s.ValidateDamageGem(id, &c, "enemy", rng)
		_, err := s.ApplyDamageGem(id, &c, "enemy", rng)
		if !errors.Is(admission, ErrUnsupportedEffect) || !errors.Is(err, admission) || !reflect.DeepEqual(s, before) || c.DamageCalls != 0 {
			t.Fatal(id, admission, err, s)
		}
		rng.done()
	}
	// Repeated confusion can consume a block and later reach that target again;
	// admission precedes the first block and area pre-evaluation randomness.
	s, c := damageFixture()
	s.actor("self").Status = StatusConfusion
	for i := range s.Actors {
		s.Actors[i].Temporary = TemporaryOneHitDefense
	}
	before := reloadLifetime(t, s)
	_, err := s.ApplyAreaDamage(&c, 100, CategoryMagic, true, &targetRNG{t: t})
	if !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(s, before) || c.AreaDamageCalls != 0 {
		t.Fatal(err, s)
	}
	// No-effect target gates do not need skipped reset/death context.
	s, c = damageFixture()
	s.actor("self").Abilities = []string{"seal_kuu"}
	s.actor("enemy").Temporary = TemporaryNullMagic
	r, err := s.ApplyDamageGem(13, &c, "enemy", &targetRNG{t: t})
	if err != nil || len(r) != 1 || r[0].Returned || c.DamageCalls != 0 {
		t.Fatal(r, err, c)
	}
}

func TestDrainAdmissionIncludesSecondTargetResolution(t *testing.T) {
	s, c := damageFixture()
	admitDamage(&s, &c)
	s.actor("enemy").Temporary = TemporaryReflectMagic
	s.actor("dead_enemy").HP = 100
	s.actor("dead_enemy").Gear = nil
	before := reloadLifetime(t, s)
	admission := s.ValidateDamageGem(26, &c, "enemy", &targetRNG{t: t})
	_, err := s.ApplyDamageGem(26, &c, "enemy", &targetRNG{t: t})
	if !errors.Is(admission, ErrUnsupportedEffect) || !errors.Is(err, admission) || !reflect.DeepEqual(s, before) {
		t.Fatal(admission, err, s)
	}
}

func TestDamageProducerAdmissionUsesExactCoefficientBeforeConversion(t *testing.T) {
	for _, id := range []int{1, 2, 3, 4, 5, 12} {
		s, c := damageFixture()
		s.actor("self").Attack = 1
		s.actor("self").Abilities = []string{"seal_kotowari"}
		before := reloadLifetime(t, s)
		admission := s.ValidateDamageGem(id, &c, "enemy", &targetRNG{t: t})
		_, err := s.ApplyDamageGem(id, &c, "enemy", &targetRNG{t: t})
		if !errors.Is(admission, ErrUnsupportedEffect) || !errors.Is(err, admission) || !reflect.DeepEqual(s, before) {
			t.Fatal(id, admission, err, s)
		}
	}
}

func TestAreaNoEffectDoesNotActivateConfusionOrRemoveSingleBlock(t *testing.T) {
	s, c := damageFixture()
	s.actor("self").ItemDefinitionIDs = []string{"item-143"}
	s.actor("enemy").ItemDefinitionIDs = []string{"item-195"}
	s.actor("enemy").Temporary = TemporaryDefense
	s.actor("ally").Temporary = TemporaryDouble
	s.actor("ally").Gear = nil
	r, err := s.ApplyAreaDamage(&c, .4, CategoryMagic, true, &targetRNG{t: t})
	if err != nil || len(r) != 2 || c.Item195Activated || c.DamageCalls != 1 || c.AreaDamageCalls != 1 {
		t.Fatal(r, err, c)
	}
	s, c = damageFixture()
	s.Actors = s.Actors[:4]
	c.EnemyIDs = []string{"enemy"}
	s.actor("enemy").Temporary = TemporaryOneHitDefense
	r, err = s.ApplyAreaDamage(&c, 100, CategoryMagic, true, &targetRNG{t: t})
	if err != nil || len(r) != 1 || !r[0].Returned || r[0].Applied || c.DamageCalls != 1 || c.AreaDamageCalls != 1 {
		t.Fatal(r, err, c)
	}
}

func TestDamageRecoilAndInterceptionSkippedTails(t *testing.T) {
	for _, hp := range []int{999, 1000} {
		s, c := damageFixture()
		admitDamage(&s, &c)
		c.GuaranteedHit = true
		s.actor("self").HP = hp
		s.actor("enemy").HP = 1000
		rng := &targetRNG{t: t, draws: []float64{0}}
		r, err := s.ApplyDamageGem(3, &c, "enemy", rng)
		want := 200
		if hp == 1000 {
			want = 26
		}
		if err != nil || len(r) != 2 || r[1].Amount != want || s.actor("self").HP != hp-want {
			t.Fatal(hp, r, err, s)
		}
		rng.done()
	}
	s, c := damageFixture()
	admitDamage(&s, &c)
	rng := &targetRNG{t: t, draws: []float64{.99}}
	r, err := s.ApplyDamageGem(3, &c, "enemy", rng)
	if err != nil || len(r) != 1 || c.End.ImmediateRiskCalls != 0 {
		t.Fatal(r, err, c)
	}
	rng.done()
	s, c = damageFixture()
	admitDamage(&s, &c)
	c.GuaranteedHit = true
	s.actor("self").Temporary = TemporaryOneHitDefense
	s.actor("enemy").HP = 1000
	s.actor("enemy").Extended = ExtendedInterception
	s.actor("enemy").ExtendedTurns = 1
	rng = &targetRNG{t: t, draws: []float64{0}}
	_, err = s.ApplyDamage(&c, TargetEffect{TargetID: "enemy", Payload: CallbackPayload{Number: 50}, Arguments: []string{"1"}}, rng)
	if err != nil || s.actor("self").HP != 100 || s.actor("self").Temporary != "" || s.actor("enemy").Extended != "" || c.DamageCalls != 1 {
		t.Fatal(err, s, c)
	}
	rng.done()
}

func TestExtension201CursorAfterDamage(t *testing.T) {
	for _, cursor := range []string{"self", "ally", "fallen"} {
		for _, mode := range []string{"damage", "block", "rejected", "redirected", "nested reset"} {
			s, c := damageFixture()
			admitDamage(&s, &c)
			c.Cursor = TargetCursor{Source: "item150", ActorID: cursor}
			s.actor("enemy").HP = 1000
			s.actor(cursor).Gear = &BattleGearStats{WeaponWeight: 20, ArmorWeight: 10}
			rng := &targetRNG{t: t, draws: []float64{0}}
			switch mode {
			case "redirected":
				s.actor("enemy").Temporary = TemporaryCover
				s.actor("dead_enemy").Temporary = TemporaryCovering
				s.actor("dead_enemy").HP = 1000
			case "block":
				s.actor("enemy").Temporary = TemporaryOneHitDefense
				rng.draws = nil
			case "rejected":
				s.actor("enemy").HP = 0
				rng.draws = nil
			case "nested reset":
				s.actor("self").Abilities = []string{"seal_kuu"}
				rng.draws = append(rng.draws, 0)
				c.Reset = func(id string) error { c.Cursor = TargetCursor{Source: "item159", ActorID: "enemy"}; return nil }
			}
			_, err := s.ApplyExtension201(&c, "enemy", rng)
			if err != nil || s.actor("self").Agility > s.actor("self").BaseStats.Agility {
				t.Fatal(cursor, mode, err, s)
			}
			want := 70
			if cursor == "self" {
				want = 70
			}
			if mode == "nested reset" {
				if s.actor("enemy").Agility != 100 {
					t.Fatal(s)
				}
			} else if s.actor(cursor).Agility != want {
				t.Fatal(cursor, mode, s.actor(cursor))
			}
			rng.done()
		}
	}
	for _, cursor := range []TargetCursor{{}, {Source: "submitted", ActorID: "enemy"}, {Source: "item150", ActorID: "missing"}} {
		s, c := damageFixture()
		admitDamage(&s, &c)
		c.Cursor = cursor
		before := reloadLifetime(t, s)
		_, err := s.ApplyExtension201(&c, "enemy", &targetRNG{t: t})
		if !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(s, before) {
			t.Fatal(cursor, err, s)
		}
	}
	s, c := damageFixture()
	s.actor("self").Agility = 100
	if _, err := s.ApplyExtension201(&c, "enemy", &targetRNG{t: t}); err != nil {
		t.Fatal(err)
	}
}

func TestDamageContinuationReloadMatches(t *testing.T) {
	run := func(reload bool) (LifetimeState, DamageContext, int) {
		s, c := damageFixture()
		admitDamage(&s, &c)
		s.Fields.Default = "darkness"
		s.actor("enemy").HP = 1000
		rng := &targetRNG{t: t, draws: []float64{0, .5}}
		_, err := s.ApplyDamageGem(13, &c, "enemy", rng)
		if err != nil {
			t.Fatal(err)
		}
		_, err = s.ApplyRecoil(&c.EffectContext, &c.End, 0, rng)
		if err != nil {
			t.Fatal(err)
		}
		c.End.Flush(s.actor("self"))
		if reload {
			s = reloadLifetime(t, s)
		}
		c.DamageCalls, c.End = 0, EndProcessing{}
		_, err = s.ApplyDamageGem(13, &c, "enemy", rng)
		if err != nil {
			t.Fatal(err)
		}
		c.End.Flush(s.actor("self"))
		rng.done()
		return s, c, rng.used
	}
	whole, wc, wd := run(false)
	resumed, rc, rd := run(true)
	if !reflect.DeepEqual(whole, resumed) || wc.DamageCalls != rc.DamageCalls || !reflect.DeepEqual(wc.End, rc.End) || wd != rd {
		t.Fatal(whole, resumed, wc, rc, wd, rd)
	}
}
