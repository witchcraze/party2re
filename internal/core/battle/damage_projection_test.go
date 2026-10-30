package battle

import (
	"math"
	"reflect"
	"testing"

	"github.com/witchcraze/party2re/internal/core/random"
)

func TestPhysicalCallbackProjectionBoundsAndDefenseGrowth(t *testing.T) {
	for _, row := range []struct {
		direct, shield, flame bool
		baseDefense           int
		minimum, maximum      int
	}{
		{false, false, false, 50, 31, 42}, // (100*.5-50*.3)*[.9,1.2], including float rounding.
		{true, false, false, 999, 90, 120},
		{true, false, false, 1000, 31, 42},
		{true, true, false, 50, 24, 33},   //162 forces defense and grows it afterwards.
		{false, false, true, 50, 81, 108}, // Flame's .9 power becomes direct.
	} {
		s, c := coinProjectionFixture()
		a, target := s.actor("self"), s.actor("enemy")
		a.Attack, a.Tension = 100, 1
		target.HP, target.MaxHP, target.Defense, target.BaseStats.Defense = 900, 900, 50, row.baseDefense
		if row.shield {
			target.ItemDefinitionIDs = []string{"item-162"}
		}
		if row.flame {
			a.Extended = ExtendedFlame
		}
		c.GuaranteedHit = true
		c.Defeat = func(string) error { t.Fatal("nonlethal vector invoked defeat"); return nil }
		e := dualAgilityEffect("enemy", 100, CategoryPhysical, row.direct)
		states, err := s.projectCallbackDamage(&c.DamageContext, nil, e, 100, true, &targetRNG{t: t})
		if err != nil {
			t.Fatal(row, err)
		}
		hps := map[int]bool{}
		for _, next := range states {
			hps[next.State.actor("enemy").HP] = true
			wantDefense := 50
			if row.shield {
				wantDefense = 60
			}
			if next.State.actor("enemy").Defense != wantDefense {
				t.Fatal(row, next.State)
			}
		}
		if !hps[900-row.minimum] || !hps[900-row.maximum] {
			t.Fatal(row, hps)
		}
		// Execution is supplemental to the source-derived numerical bounds.
		for _, seed := range []uint64{0, 1459, 42, 54} {
			p, child := s.callbackCopy(), c.DamageContext
			r, err := p.ApplyDamage(&child, e, random.NewDeterministic(seed))
			if err != nil || r.Amount < row.minimum || r.Amount > row.maximum {
				t.Fatal(row, seed, r, err)
			}
		}
	}
}

func TestPhysicalProjectionRetainsConversionDebitAndMiss(t *testing.T) {
	for _, converted := range []bool{false, true} {
		s, c := coinProjectionFixture()
		s.actor("self").MP, s.actor("self").Tension = 80, 3
		if converted {
			s.actor("self").Abilities = []string{"seal_kotowari"}
		}
		c.GuaranteedHit = false
		e := dualAgilityEffect("enemy", 100, CategoryPhysical, false)
		states, err := s.projectCallbackDamage(&c.DamageContext, nil, e, 100, true, &targetRNG{t: t})
		if err != nil {
			t.Fatal(err)
		}
		miss := false
		for _, next := range states {
			wantMP := 80
			if converted {
				wantMP = 77
			}
			if next.State.actor("self").MP != wantMP || next.Context.DamageCalls != 1 {
				t.Fatal(converted, next)
			}
			if next.TargetID == "" {
				miss = true
				if next.State.actor("self").Tension != 1 || next.State.actor("enemy").HP != 1100 {
					t.Fatal(next)
				}
			}
		}
		if miss == converted || s.actor("self").MP != 80 || s.actor("self").Tension != 3 {
			t.Fatal(converted, miss, s)
		}
	}
}

func TestDirectMagicProjectionPreservesRevivalAndAbsorptionBounds(t *testing.T) {
	s, c := coinProjectionFixture()
	s.actor("enemy").MaxHP, s.actor("enemy").Temporary = 6000, TemporaryRevival
	before := s.callbackCopy()
	states, err := s.projectCallbackDamage(&c.DamageContext, c.Status.Defeat, dualAgilityEffect("enemy", 1800, CategoryMagic, true), 1800, true, &targetRNG{t: t})
	if err != nil || !reflect.DeepEqual(s, before) {
		t.Fatal(err, s)
	}
	hp := map[int]bool{}
	for _, next := range states {
		hp[next.State.actor("enemy").HP] = true
	}
	if !hp[600] || !hp[1200] {
		t.Fatal(hp)
	}
	s, c = coinProjectionFixture()
	s.actor("enemy").Temporary = TemporaryAbsorbMagic
	s.actor("enemy").MP, s.actor("enemy").MaxMP = 0, 500
	states, err = s.projectCallbackDamage(&c.DamageContext, c.Status.Defeat, dualAgilityEffect("enemy", 400, CategoryMagic, true), 600, true, &targetRNG{t: t})
	if err != nil {
		t.Fatal(err)
	}
	mp := map[int]bool{}
	for _, next := range states {
		mp[next.State.actor("enemy").MP] = true
	}
	if !mp[80] || !mp[299] {
		t.Fatal(mp)
	}
}

func TestDirectMagicPowerBoundsIncludeInterior143PeakAndFourDraws(t *testing.T) {
	s, c := coinProjectionFixture()
	s.actor("self").ItemDefinitionIDs = []string{"item-143"}
	e := dualAgilityEffect("enemy", 250, CategoryMagic, true)
	low, high := s.callbackPowerBounds(&c.DamageContext, s.actor("enemy"), e, 350)
	if low != 300 || high != 599 {
		t.Fatal(low, high)
	}
	s.actor("self").ItemDefinitionIDs = []string{"item-192", "weapon-34", "weapon-37"}
	s.actor("enemy").ItemDefinitionIDs = []string{"item-190"}
	s.actor("enemy").Temporary = TemporaryDouble
	low, high = s.callbackPowerBounds(&c.DamageContext, s.actor("enemy"), e, 350)
	for _, power := range []float64{250, 300, 350} {
		for mask := range 16 {
			bits := mask
			draw := func() float64 { v := float64(bits&1) * math.Nextafter(1, 0); bits >>= 1; return v }
			value := s.modifyDamage(&c.DamageContext, s.actor("enemy"), power, CategoryMagic, true, false, draw)
			if value < low || value > high {
				t.Fatal(power, mask, value, low, high)
			}
		}
	}
}
