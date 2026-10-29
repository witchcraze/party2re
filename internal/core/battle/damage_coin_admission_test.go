package battle

import (
	"math"
	"reflect"
	"testing"
)

func coinProjectionFixture() (LifetimeState, TreatmentChanceContext) {
	s, damage := damageFixture()
	s.actor("enemy").HP, s.actor("enemy").MaxHP = 1100, 3600
	c := TreatmentChanceContext{DamageContext: damage}
	c.Status.Defeat = nativeDefeatContext(c.TargetContext)
	return s, c
}

func TestCoinNativeProjectionHPClassesAndRevival(t *testing.T) {
	for _, temporary := range []bool{false, true} {
		s, c := coinProjectionFixture()
		if temporary {
			s.actor("enemy").MaxHP, s.actor("enemy").Temporary = 6000, TemporaryRevival
		}
		before := s.callbackCopy()
		c.Status.Defeat.Rewards.Apply = func(*LifetimeState, TargetContext, string) error { t.Fatal("reward Apply in admission"); return nil }
		states, err := s.projectCoinDamage(&c, dualAgilityEffect("enemy", 1800, CategoryUntyped, true), true, &targetRNG{t: t})
		if err != nil || !reflect.DeepEqual(s, before) || c.DamageCalls != 0 {
			t.Fatal(err, s, states)
		}
		hp := map[int]bool{}
		for _, next := range states {
			hp[next.State.actor("enemy").HP] = true
			if next.Context.DamageCalls != 1 {
				t.Fatal(next)
			}
		}
		if !temporary && !hp[0] || temporary && (!hp[600] || !hp[1200]) {
			t.Fatal(temporary, hp)
		}
	}
	// A roll range crossing both death and999 keeps the living interior too.
	if got := coinSurvivorHPs(2000, 900, 2100); !reflect.DeepEqual(got, []int{1, 1100, 999, 1000}) {
		t.Fatal(got)
	}
}

func TestCoinNativeProjectionConsumesDefenseAndKeepsActivationAlternatives(t *testing.T) {
	s, c := coinProjectionFixture()
	s.actor("enemy").Temporary = TemporaryOneHitDefense
	s.actor("enemy").ItemDefinitionIDs = []string{"item-195"}
	states, err := s.projectCoinDamage(&c, dualAgilityEffect("enemy", 1800, CategoryUntyped, true), true, &targetRNG{t: t})
	if err != nil || len(states) != 1 || states[0].State.actor("enemy").HP != 1100 || states[0].State.actor("enemy").Temporary != "" || states[0].Context.Item195Activated {
		t.Fatal(err, states)
	}
	// A following hit can now activate195, then later resolution can confuse.
	child := c
	child.DamageContext = states[0].Context
	next, err := states[0].State.projectCoinDamage(&child, dualAgilityEffect("enemy", 100, CategoryUntyped, true), true, &targetRNG{t: t})
	if err != nil {
		t.Fatal(err)
	}
	for _, hit := range next {
		if !hit.Context.Item195Activated || hit.Context.DamageCalls != 2 {
			t.Fatal(hit)
		}
	}
}

func TestCoinPowerBoundsIncludeInterior144Peak(t *testing.T) {
	s, c := coinProjectionFixture()
	s.actor("self").ItemDefinitionIDs = []string{"item-144", "item-192"}
	s.actor("enemy").Temporary = TemporaryDouble
	c.AreaDamageCalls = 1
	e := dualAgilityEffect("enemy", 190, CategoryUntyped, true)
	low, high := s.coinPowerBounds(&c.DamageContext, s.actor("enemy"), e)
	if low != 300 || high != 390 {
		t.Fatal(low, high)
	}
	for _, first := range []float64{0, .2, .5, .9, math.Nextafter(1, 0)} {
		for _, second := range []float64{0, .9} {
			tape := []float64{first, second}
			i := 0
			draw := func() float64 { v := tape[i]; i++; return v }
			value := s.modifyDamage(&c.DamageContext, s.actor("enemy"), e.Payload.Number, CategoryUntyped, true, false, draw)
			if value < low || value > high {
				t.Fatal(first, second, value, low, high)
			}
		}
	}
}

func TestCoinResetProjectsIndependentStatsAndResourceOutcomes(t *testing.T) {
	for _, accessory := range []string{"item-111", "item-112", "item-159", "item-247"} {
		s, c := coinProjectionFixture()
		target := s.actor("enemy")
		target.Gear.AccessoryID = accessory
		target.BaseStats = BattleBaseStats{Attack: 100, Defense: 100, Agility: 100}
		target.HP, target.MP = 990, 0
		states, err := s.projectCoinReset(&c, "enemy", true, &targetRNG{t: t})
		if err != nil {
			t.Fatal(err)
		}
		found := map[int]bool{}
		mixed := false
		for _, state := range states {
			a := state.actor("enemy")
			switch accessory {
			case "item-111":
				found[a.MP] = true
			case "item-112":
				found[a.HP] = true
			case "item-159":
				mixed = mixed || a.Attack == 110 && a.Defense == 100 && a.Agility == 110
			case "item-247":
				found[a.ExtendedTurns] = true
			}
		}
		if accessory == "item-111" && len(found) != 9 || accessory == "item-112" && !found[999] || accessory == "item-159" && !mixed || accessory == "item-247" && len(found) != 3 {
			t.Fatal(accessory, found, mixed)
		}
	}
}

func TestCoinInterceptionIsNonlethalAndConsumesOneHitDefense(t *testing.T) {
	for _, defense := range []bool{false, true} {
		s, c := coinProjectionFixture()
		s.actor("self").HP = 1
		s.actor("enemy").Extended, s.actor("enemy").ExtendedTurns = ExtendedInterception, 1
		if defense {
			s.actor("self").Temporary = TemporaryOneHitDefense
		}
		states, err := s.projectCoinInterception(&c.DamageContext, "enemy", &targetRNG{t: t})
		if err != nil || len(states) != 1 || states[0].actor("self").HP != 1 || states[0].actor("self").Temporary != "" || states[0].actor("enemy").Extended != "" {
			t.Fatal(defense, err, states)
		}
	}
}
