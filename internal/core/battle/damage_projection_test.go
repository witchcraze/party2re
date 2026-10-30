package battle

import (
	"math"
	"reflect"
	"testing"
)

func TestDirectMagicProjectionPreservesRevivalAndAbsorptionBounds(t *testing.T) {
	s, c := coinProjectionFixture()
	s.actor("enemy").MaxHP, s.actor("enemy").Temporary = 6000, TemporaryRevival
	before := s.callbackCopy()
	states, err := s.projectDirectDamage(&c.DamageContext, c.Status.Defeat, dualAgilityEffect("enemy", 1800, CategoryMagic, true), 1800, true, &targetRNG{t: t})
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
	states, err = s.projectDirectDamage(&c.DamageContext, c.Status.Defeat, dualAgilityEffect("enemy", 400, CategoryMagic, true), 600, true, &targetRNG{t: t})
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
	low, high := s.directPowerBounds(&c.DamageContext, s.actor("enemy"), e, 350)
	if low != 300 || high != 599 {
		t.Fatal(low, high)
	}
	s.actor("self").ItemDefinitionIDs = []string{"item-192", "weapon-34", "weapon-37"}
	s.actor("enemy").ItemDefinitionIDs = []string{"item-190"}
	s.actor("enemy").Temporary = TemporaryDouble
	low, high = s.directPowerBounds(&c.DamageContext, s.actor("enemy"), e, 350)
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
