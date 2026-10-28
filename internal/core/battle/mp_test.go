package battle

import (
	"reflect"
	"testing"
)

func TestMPHealingBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, item, field                string
		actorMP, mp, power, want, amount int
	}{
		{"ordinary", "", "", 50, 0, 50, 46, 46},
		{"exact max", "", "", 50, 54, 50, 100, 46},
		{"over max", "", "", 50, 80, 50, 100, 46},
		{"excluded189", "item-189", "", 50, 0, 50, 0, 0},
		{"half exactly", "item-198", "", 50, 0, 50, 46, 46},
		{"half above", "item-198", "", 51, 0, 50, 0, 0},
		{"excluded202", "item-202", "", 0, 0, 50, 0, 0},
		{"sanctuary", "", "sanctuary", 50, 80, 50, 65, 55},
		{"sanctuary floor", "", "sanctuary", 50, 80, 999, 1, 1079},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, c := recoveryFixture()
			a, target := s.actor("self"), s.actor("ally")
			a.MP, target.MP, target.MaxMP = tc.actorMP, tc.mp, 100
			target.ItemDefinitionIDs = []string{tc.item}
			target.Temporary = TemporaryRage
			s.Fields.Default = tc.field
			rng := &targetRNG{t: t, draws: []float64{0}}
			r, err := s.ApplyMPHealing(&c, controlEffect("ally", float64(tc.power), CategoryMagic), rng)
			if err != nil || !r.Returned || r.Amount != tc.amount || target.MP != tc.want || target.HP != 100 || target.Temporary != "" || a.Tension != 1 || c.MPHealCalls != 1 {
				t.Fatal(r, err, *target, c)
			}
			rng.done()
		})
	}
}

func TestMPHealingTargetAndModifierOrder(t *testing.T) {
	for _, tc := range []struct {
		name, target, field string
		draws               []float64
		want, amount        int
	}{
		{"fallen fallback", "fallen", "", []float64{0}, 49, 46},
		{"missing fallback", "missing", "", []float64{0, 0}, 49, 46},
		{"den after zero", "ally", "den", []float64{0}, 1, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, c := recoveryFixture()
			s.actor("ally").MaxMP = 100
			s.Fields.Default = tc.field
			if tc.field == "den" {
				s.actor("self").ItemDefinitionIDs = []string{"item-239"}
				c.DamageCalls = 1
			}
			rng := &targetRNG{t: t, draws: tc.draws}
			r, err := s.ApplyMPHealing(&c, controlEffect(tc.target, 50, CategoryMagic), rng)
			if err != nil || r.Amount != tc.amount || s.actor(r.TargetID).MP != tc.want {
				t.Fatal(r, err, s)
			}
			rng.done()
		})
	}
	s, c := recoveryFixture()
	s.actor("self").HP = 0
	r, err := s.ApplyMPHealing(&c, controlEffect("self", 50, CategoryMagic), &targetRNG{t: t})
	if err != nil || r.Returned || c.MPHealCalls != 0 {
		t.Fatal(r, err, c)
	}
}

func TestMPDrainImmunity(t *testing.T) {
	for _, kind := range []string{"defense", "item", "curse"} {
		s, c := recoveryFixture()
		target := s.actor("enemy")
		switch kind {
		case "defense":
			target.BaseStats.Defense = 1000
		case "item":
			target.ItemDefinitionIDs = []string{"item-159"}
		case "curse":
			target.Abilities = []string{"curse"}
		}
		s.actor("self").Tension = 3
		r, err := s.ApplyStatDown(&c, controlEffect("enemy", .2, CategoryMagic), StatMP, &targetRNG{t: t})
		if err != nil || r.Returned || target.MP != 90 || s.actor("self").Tension != 3 {
			t.Fatal(kind, r, err, s)
		}
	}
}

func TestResourceBodiesAndReload(t *testing.T) {
	for _, tc := range []struct {
		id, target               string
		mp, cmp, enemyMP, amount int
		draws                    []float64
	}{
		{"gem_mp_heal_sky_1", "ally", 3, 90, 90, 46, []float64{0}},
		{"gem_mp_heal_sky_2", "enemy", 94, 90, 90, 91, []float64{0}},
		{"gem_drain_mp_sky", "enemy", 20, 90, 72, 18, []float64{0}},
		{"gem_debuff_mp", "enemy", 3, 90, 72, 18, nil},
		{"gem_mp_burst", "enemy", 0, 90, 90, 4, []float64{0}},
		{"gem_cmp_burst", "enemy", 3, 1, 90, 283, []float64{0}},
		{"job-50-entry-5", "ally", 3, 90, 90, 46, []float64{0}},
	} {
		t.Run(tc.id, func(t *testing.T) {
			s, c := damageFixture()
			admitDamage(&s, &c)
			c.GuaranteedHit = true
			s.actor("self").CMP = 90
			s.actor("ally").MaxMP = 100
			s.actor("enemy").HP = 1000
			beforeHP := s.actor("self").HP
			rng := &targetRNG{t: t, draws: tc.draws}
			r, err := s.ApplyResource(&c, tc.id, tc.target, rng)
			if err != nil || len(r) == 0 || r[0].Amount != tc.amount || s.actor("self").MP != tc.mp || s.actor("self").CMP != tc.cmp || s.actor("enemy").MP != tc.enemyMP || s.actor("self").HP != beforeHP {
				t.Fatal(r, err, s)
			}
			if tc.id == "gem_mp_heal_sky_1" || tc.id == "job-50-entry-5" {
				if s.actor("ally").MP != 46 {
					t.Fatal(s)
				}
			}
			if tc.id == "gem_mp_burst" || tc.id == "gem_cmp_burst" {
				if s.actor("enemy").HP != 1000-tc.amount {
					t.Fatal(s)
				}
			} else if s.actor("enemy").HP != 1000 {
				t.Fatal("MP operation changed HP")
			}
			rng.done()
			loaded := reloadLifetime(t, s)
			if !reflect.DeepEqual(loaded.Actors, s.Actors) {
				t.Fatal("reload changed resources")
			}
			// A fresh operation continues from stored MP, without replaying gains.
			op := EffectContext{TargetContext: c.TargetContext}
			_, err = loaded.ApplyMPHealing(&op, controlEffect("self", 1, CategoryTool), &targetRNG{t: t, draws: []float64{0}})
			if err != nil || loaded.actor("self").MP != min(100, tc.mp+1) {
				t.Fatal(err, loaded)
			}
		})
	}
}

func TestResourceStatusCMPBoundaries(t *testing.T) {
	for _, id := range []string{"gem_status_cmp_3", "gem_status_cmp_10"} {
		for _, status := range []string{"", StatusPoison} {
			for _, cmp := range []int{0, 2, 3, 9, 10} {
				s, c := damageFixture()
				a := s.actor("self")
				a.Status, a.CMP = status, cmp
				debit := 3
				if id == "gem_status_cmp_10" {
					debit = 10
				}
				want := cmp
				for range 2 {
					if status != "" {
						want = max(0, want-debit)
					}
					if _, err := s.ApplyResource(&c, id, "", &targetRNG{t: t}); err != nil {
						t.Fatal(err)
					}
					if a.CMP != want || a.Status != status {
						t.Fatal(id, cmp, status, *a)
					}
				}
			}
		}
	}
}

func TestMPDrainRequestedDebitAndCap(t *testing.T) {
	for _, tc := range []struct {
		mp, maxMP            int
		coefficient, tension float64
		draws                []float64
		amount, want         int
	}{
		{21, 100, .2, 1, nil, 4, 20},
		{20, 100, .2, 1, nil, 0, 20},
		{750, 1000, .2, 1, nil, 150, 600},
		{755, 1000, .2, 1, []float64{.98}, 149, 606},
		{1000, 1000, .2, 3, []float64{0}, 300, 700},
	} {
		s, c := recoveryFixture()
		target := s.actor("enemy")
		target.MP, target.MaxMP = tc.mp, tc.maxMP
		s.actor("self").Tension = tc.tension
		rng := &targetRNG{t: t, draws: tc.draws}
		r, err := s.ApplyStatDown(&c, controlEffect("enemy", tc.coefficient, CategoryMagic), StatMP, rng)
		if err != nil || r.Amount != tc.amount || target.MP != tc.want || target.HP != 100 {
			t.Fatal(r, err, *target)
		}
		rng.done()
	}
}

func TestMPHealingTensionAndExclusionOrder(t *testing.T) {
	for _, excluded := range []bool{false, true} {
		s, c := recoveryFixture()
		a, target := s.actor("self"), s.actor("ally")
		a.Tension = 3
		target.MaxMP, target.Temporary = 1000, TemporaryRage
		if excluded {
			target.ItemDefinitionIDs = []string{"item-189"}
		}
		rng := &targetRNG{t: t, draws: []float64{.5}}
		r, err := s.ApplyMPHealing(&c, controlEffect("ally", 50, CategoryTool), rng)
		want := 158
		if excluded {
			want = 0
		}
		if err != nil || r.Amount != want || target.MP != want || a.Tension != 1 || target.Temporary != "" {
			t.Fatal(r, err, *target)
		}
		rng.done()
	}
}

func TestResourceDrainUsesRequestedDebitAtFloor(t *testing.T) {
	s, c := damageFixture()
	s.actor("enemy").MP = 21
	r, err := s.ApplyResource(&c, "gem_drain_mp_sky", "enemy", &targetRNG{t: t, draws: []float64{0}})
	if err != nil || len(r) != 2 || r[0].Amount != 4 || r[0].Delta != -1 || r[1].Amount != 4 || s.actor("self").MP != 7 {
		t.Fatal(r, err, s)
	}
}

func TestResourceTargetResourcesAndNoEffectTails(t *testing.T) {
	for _, tmp := range []TemporaryEffect{TemporaryAbsorbMagic, TemporaryNullMagic} {
		s, c := damageFixture()
		target := s.actor("ally")
		target.MaxMP, target.Temporary = 100, tmp
		draws := []float64{0, 0}
		want := 56
		if tmp == TemporaryNullMagic {
			draws = nil
			want = 0
		}
		rng := &targetRNG{t: t, draws: draws}
		_, err := s.ApplyResource(&c, "gem_mp_heal_sky_1", "ally", rng)
		if err != nil || target.MP != want || target.HP != 100 {
			t.Fatal(tmp, err, *target)
		}
		rng.done()
	}
	for _, id := range []string{"gem_mp_burst", "gem_cmp_burst"} {
		s, c := damageFixture()
		c.EnemyIDs = nil
		// The empty roster must still cover all opposing actors for this context.
		s.Actors = s.Actors[:3]
		s.actor("self").MP, s.actor("self").CMP = 20, 20
		_, err := s.ApplyResource(&c, id, "", &targetRNG{t: t})
		if err != nil {
			t.Fatal(err)
		}
		if id == "gem_mp_burst" && s.actor("self").MP != 0 {
			t.Fatal(s)
		}
		if id == "gem_cmp_burst" {
			if s.actor("self").CMP != 1 {
				t.Fatal(s)
			}
			s.actor("self").CMP-- // Late enclosing cost, after the intrinsic assignment.
			loaded := reloadLifetime(t, s)
			if loaded.actor("self").CMP != 0 {
				t.Fatal(loaded)
			}
		}
	}
}

func TestResourceJob50RecoveryBoundaries(t *testing.T) {
	for _, tc := range []struct {
		target       string
		before, want int
		draws        []float64
	}{
		{"ally", 54, 100, []float64{0}},
		{"ally", 100, 100, []float64{0}},
		{"ally", 101, 100, []float64{0}},
		{"fallen", 0, 49, []float64{0}},
		{"missing", 0, 49, []float64{0, 0}},
	} {
		s, c := damageFixture()
		s.actor("ally").MP, s.actor("ally").MaxMP = tc.before, 100
		rng := &targetRNG{t: t, draws: tc.draws}
		r, err := s.ApplyResource(&c, "job-50-entry-5", tc.target, rng)
		if err != nil || len(r) != 1 || !r[0].Returned || r[0].Amount != 46 || s.actor(r[0].TargetID).MP != tc.want || c.MPHealCalls != 1 {
			t.Fatal(tc, r, err, s)
		}
		rng.done()
	}
}
