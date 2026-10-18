package battle

import (
	"errors"
	"reflect"
	"testing"
)

func TestDamageGemCommitsTargetResourcesAtEveryHelper(t *testing.T) {
	for _, source := range []int{13, 26} {
		s, c := damageFixture()
		admitDamage(&s, &c)
		s.actor("enemy").HP = 1000
		s.actor("enemy").MP = 0
		s.actor("enemy").Temporary = TemporaryAbsorbMagic
		draws := []float64{0, 0}
		wantMP := 10
		if source == 26 {
			draws = []float64{0, 0, 0, 0}
			wantMP = 70
		}
		rng := &targetRNG{t: t, draws: draws}
		if _, err := s.ApplyDamageGem(source, &c, "enemy", rng); err != nil || s.actor("enemy").MP != wantMP {
			t.Fatal(source, s, err)
		}
		rng.done()
	}
	s, c := damageFixture()
	admitDamage(&s, &c)
	c.GuaranteedHit = true
	s.actor("self").Abilities = []string{"seal_kotowari"}
	s.actor("enemy").HP = 1000
	if _, err := s.ApplyDamageGem(1, &c, "enemy", &targetRNG{t: t, draws: []float64{0}}); err != nil || s.actor("self").MP != 0 {
		t.Fatal(s, err)
	}
}

func TestDrainCureThenActivationAdmissionIncludesNewTargets(t *testing.T) {
	s, c := damageFixture()
	admitDamage(&s, &c)
	s.actor("self").Status = StatusMagicSeal
	s.actor("enemy").HP = 1
	c.Item195Activated = true
	s.actor("ally").Gear = nil
	before := reloadLifetime(t, s)
	err := s.ValidateDamageGem(26, &c, "enemy", &targetRNG{t: t})
	if !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(before, s) {
		t.Fatal(err, s)
	}
	if _, err = s.ApplyDamageGem(26, &c, "enemy", &targetRNG{t: t}); !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(before, s) || c.DamageCalls != 0 {
		t.Fatal(err, s, c)
	}
}

func TestDamageGemMultiHitEndpointsPerHitPowerAndDeathStop(t *testing.T) {
	for _, source := range []int{6, 7, 8, 9} {
		for _, upper := range []bool{false, true} {
			s, c := damageFixture()
			admitDamage(&s, &c)
			c.GuaranteedHit = true
			s.actor("self").Attack = 100
			s.actor("enemy").HP, s.actor("enemy").Defense = 2000, 0
			hits := map[int]int{6: 3, 7: 7, 8: 2, 9: 2}[source]
			countDraw := 0.0
			if upper {
				countDraw = .999
				hits++
				if source == 9 {
					hits++
				}
			}
			draws := []float64{countDraw}
			for i := range hits {
				powerDraw := 0.0
				if i%2 == 1 {
					powerDraw = .999
				}
				draws = append(draws, powerDraw, 0, 0)
			}
			rng := &targetRNG{t: t, draws: draws}
			r, err := s.ApplyDamageGem(source, &c, "ignored", rng)
			if err != nil || len(r) != hits || c.DamageCalls != hits {
				t.Fatal(source, upper, r, err)
			}
			for i, hit := range r {
				want := 27
				if i%2 == 1 {
					want = 35
				}
				if source == 8 {
					want = 54
					if i%2 == 1 {
						want = 80
					}
				}
				if hit.Amount != want {
					t.Fatal(source, i, hit, want)
				}
			}
			rng.done()
		}
		s, c := damageFixture()
		admitDamage(&s, &c)
		c.GuaranteedHit = true
		s.actor("self").Attack, s.actor("self").Defense = 1000, 0
		s.actor("enemy").Temporary = TemporaryReflectAttack
		if source == 9 {
			s.actor("enemy").Temporary = TemporaryReflectDance
		}
		rng := &targetRNG{t: t, draws: []float64{.999, 0, 0, 0}}
		if r, err := s.ApplyDamageGem(source, &c, "ignored", rng); err != nil || len(r) != 1 || s.actor("self").HP != 0 || c.DamageCalls != 1 {
			t.Fatal(source, r, err, s)
		}
		rng.done()
	}
}

func TestDamageGemAreaCoefficientsCategoriesAndOrder(t *testing.T) {
	for _, tc := range []struct {
		source, first, second int
		category              CallbackCategory
	}{
		{10, 36, 30, CategoryPhysical}, {11, 45, 38, CategoryPhysical}, {12, 72, 61, CategoryPhysical},
		{16, 45, 38, CategoryMagic}, {17, 180, 153, CategoryMagic}, {18, 135, 114, CategoryBreath},
	} {
		s, c := damageFixture()
		admitDamage(&s, &c)
		c.GuaranteedHit = true
		s.actor("self").Attack = 100
		for _, id := range c.EnemyIDs {
			s.actor(id).HP, s.actor(id).Defense = 2000, 0
		}
		rng := &targetRNG{t: t, draws: []float64{0, 0}}
		r, err := s.ApplyDamageGem(tc.source, &c, "ignored", rng)
		if err != nil || len(r) != 2 || r[0].TargetID != "enemy" || r[1].TargetID != "dead_enemy" || r[0].Amount != tc.first || r[1].Amount != tc.second || r[0].Category != tc.category || r[1].Category != tc.category || c.AreaDamageCalls != 1 || c.DamageCalls != 2 {
			t.Fatal(tc, r, err, c)
		}
		rng.done()
	}
}

func TestDrainRequestedPowerCapAndReturnedTarget(t *testing.T) {
	for _, hp := range []int{1499, 1500, 4000} {
		s, c := damageFixture()
		admitDamage(&s, &c)
		s.actor("enemy").HP = hp
		s.actor("enemy").Defense = 10000
		s.actor("enemy").BaseStats.Defense = 1000
		rng := &targetRNG{t: t, draws: []float64{0, 0, 0}}
		r, err := s.ApplyDamageGem(26, &c, "enemy", rng)
		wantHeal := 270
		if hp == 1499 {
			wantHeal = 269
		}
		if err != nil || len(r) != 3 || r[1].Amount != 1 || r[2].Amount != wantHeal || s.actor("self").HP != 100+wantHeal {
			t.Fatal(hp, r, err, s)
		}
		rng.done()
	}
	s, c := damageFixture()
	admitDamage(&s, &c)
	s.actor("enemy").Temporary = TemporaryReflectMagic
	rng := &targetRNG{t: t, draws: []float64{0, 0, 0}}
	r, err := s.ApplyDamageGem(26, &c, "enemy", rng)
	if err != nil || len(r) != 3 || r[0].TargetID != "self" || r[1].TargetID != "self" || r[2].Amount != 18 || s.actor("self").HP != 100 {
		t.Fatal(r, err, s)
	}
	rng.done()
	for _, temporary := range []TemporaryEffect{TemporaryOneHitDefense, TemporaryNullMagic, TemporaryAbsorbMagic} {
		s, c := damageFixture()
		s.actor("enemy").Temporary = temporary
		var draws []float64
		if temporary == TemporaryAbsorbMagic {
			s.actor("enemy").HP = 1
			s.actor("enemy").MP = 0
			draws = []float64{0, 0}
		}
		rng := &targetRNG{t: t, draws: draws}
		r, err := s.ApplyDamageGem(26, &c, "enemy", rng)
		if err != nil || len(r) != 2 || c.HealCalls != 0 || s.actor("self").HP != 100 {
			t.Fatal(temporary, r, err, s)
		}
		if temporary == TemporaryAbsorbMagic && (s.actor("enemy").MP != 60 || c.DamageCalls != 0) {
			t.Fatal(s, c)
		}
		rng.done()
	}
}

func TestDamageGemRecoilSkipsReturnedZeroAndSacrificeStillRuns(t *testing.T) {
	s, c := damageFixture()
	s.actor("enemy").Temporary = TemporaryOneHitDefense
	if r, err := s.ApplyDamageGem(3, &c, "enemy", &targetRNG{t: t}); err != nil || len(r) != 1 || !r[0].Returned || r[0].Amount != 0 || c.End.ImmediateRiskCalls != 0 {
		t.Fatal(r, err, c)
	}
	s, c = damageFixture()
	s.actor("enemy").Temporary = TemporaryNullMagic
	defeats := 0
	c.Defeat = func(id string) error {
		defeats++
		if id != "self" {
			t.Fatal(id)
		}
		return nil
	}
	if r, err := s.ApplyDamageGem(72, &c, "enemy", &targetRNG{t: t}); err != nil || len(r) != 1 || r[0].Returned || defeats != 1 || c.DamageCalls != 0 {
		t.Fatal(r, err, defeats, c)
	}
}
