package battle

import (
	"errors"
	"math"
	"reflect"
	"testing"
)

func playerTailFixture() (LifetimeState, PlayerTailContext) {
	s, status := statusFixture()
	level := 37
	c := PlayerTailContext{DamageContext: DamageContext{EffectContext: status.EffectContext},
		Completion:    LifetimeCompletion{Accepted: true, ActorID: "self", EnemiesRemain: true},
		DefeatContext: nativeDefeatContext(status.TargetContext)}
	c.RequesterLevel = &level
	for i := range s.Actors {
		a := &s.Actors[i]
		a.HP, a.MaxHP, a.MP, a.MaxMP = 100, 1000, 100, 1000
		a.MaxCMP = 100
		a.Attack, a.Defense, a.Agility = 100, 100, 100
	}
	s.actor("self").CMPNeedCharge = 5
	return s, c
}

func TestPlayerTailItem210FrozenFallenAndResources(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		hp, mp, wantHP, wantMP int
	}{
		{"equal", 100, 100, 370, 100},
		{"zero mp", 100, 0, 370, 0},
		{"mp lower", 100, 99, 100, 369},
		{"still fallen", 0, 99, 0, 99},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, c := playerTailFixture()
			s.actor("self").ItemDefinitionIDs = []string{"item-210"}
			s.actor("ally").HP = 0
			var err error
			c.Item210Fallen, err = s.CaptureItem210(c.TargetContext)
			if err != nil || !reflect.DeepEqual(c.Item210Fallen, []string{"ally"}) {
				t.Fatal(c.Item210Fallen, err)
			}
			s.actor("ally").HP, s.actor("ally").MP = tc.hp, tc.mp
			s.actor("self").ItemDefinitionIDs = nil
			// Recovery is outside the eligible combat tail, including level-up.
			c.Completion.EnemiesRemain, c.Completion.LevelUp = false, true
			draws := []float64{}
			if tc.hp > 0 {
				draws = append(draws, 0)
			}
			rng := &targetRNG{t: t, draws: draws}
			if err := s.ApplyPlayerTail(&c, "item-210-revival-tail", rng); err != nil {
				t.Fatal(err)
			}
			if a := s.actor("ally"); a.HP != tc.wantHP || a.MP != tc.wantMP {
				t.Fatal(a)
			}
			if s.actor("fallen").HP != 100 {
				t.Fatal("snapshot included later member")
			}
			rng.done()
		})
	}
}

func TestPlayerTailItem189NormalGuardChanceAndCap(t *testing.T) {
	for _, tc := range []struct {
		name               string
		maxMP, mp, want    int
		normal, suppressed bool
		draws              []float64
	}{
		{"equality low", 1000, 300, 435, true, false, []float64{0, 0, 0}},
		{"equality high", 1000, 300, 598, true, false, []float64{0, .999, .999}},
		{"over guard", 1000, 301, 301, true, false, nil},
		{"chance equality", 1000, 300, 300, true, false, []float64{.2}},
		{"cap before tool roll", 10000, 3000, 4199, true, false, []float64{0, .999, .999}},
		{"incapacitated", 1000, 300, 300, false, false, nil},
		{"suppressed draw", 1000, 300, 300, true, true, []float64{0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, c := playerTailFixture()
			a := s.actor("self")
			a.ItemDefinitionIDs, a.MaxMP, a.MP = []string{"item-189"}, tc.maxMP, tc.mp
			c.NormalAction, c.Completion.Suppressed = tc.normal, tc.suppressed
			rng := &targetRNG{t: t, draws: tc.draws}
			if err := s.ApplyPlayerTail(&c, "item-189-normal-tail", rng); err != nil || a.MP != tc.want || c.MPHealCalls != 0 {
				t.Fatal(a, c, err)
			}
			rng.done()
		})
	}
}

func TestPlayerTailCombatEligibilityAndAgilityOvershoot(t *testing.T) {
	for _, reason := range []string{"eligible", "rejected", "npc", "actor npc", "suppressed", "no enemies", "comment", "level up", "fallen"} {
		t.Run(reason, func(t *testing.T) {
			s, c := playerTailFixture()
			a := s.actor("self")
			a.ItemDefinitionIDs, a.Agility = []string{"item-183"}, 299
			switch reason {
			case "rejected":
				c.Completion.Accepted = false
			case "npc":
				c.Completion.NPC = true
			case "actor npc":
				a.NPC = true
			case "suppressed":
				c.Completion.Suppressed = true
			case "no enemies":
				c.Completion.EnemiesRemain = false
			case "comment":
				c.Completion.NPCComment = true
			case "level up":
				c.Completion.LevelUp = true
			case "fallen":
				a.HP = 0
			}
			draws, want := []float64{}, 299
			if reason == "eligible" {
				draws, want = []float64{.999}, 348
			}
			rng := &targetRNG{t: t, draws: draws}
			if err := s.ApplyPlayerTail(&c, "item-183-combat-tail", rng); err != nil || a.Agility != want {
				t.Fatal(a, err)
			}
			rng.done()
		})
	}
}

func TestPlayerTailHalfThresholdsTemporaryAndZeroRecovery(t *testing.T) {
	for _, tc := range []struct {
		id, item             string
		hp, maxHP, mp, maxMP int
		tmp                  TemporaryEffect
		wantHP, wantMP       int
		draws                []float64
	}{
		{"item-202-combat-tail", "item-202", 50, 101, 10, 100, TemporaryDouble, 57, 10, []float64{0}},
		{"item-202-combat-tail", "item-202", 51, 101, 10, 100, "", 51, 10, nil},
		{"item-202-combat-tail", "item-202", 1, 3, 10, 100, "", 1, 10, []float64{0}},
		{"item-198-combat-tail", "item-198", 100, 101, 50, 101, TemporaryDefense, 100, 50, []float64{0}},
		{"item-198-combat-tail", "item-198", 100, 101, 50, 101, TemporaryDouble, 100, 53, []float64{0}},
		{"item-198-combat-tail", "item-198", 100, 101, 51, 101, "", 100, 51, nil},
	} {
		t.Run(tc.id+string(tc.tmp), func(t *testing.T) {
			s, c := playerTailFixture()
			a := s.actor("self")
			a.HP, a.MaxHP, a.MP, a.MaxMP, a.Temporary = tc.hp, tc.maxHP, tc.mp, tc.maxMP, tc.tmp
			a.ItemDefinitionIDs = []string{tc.item}
			rng := &targetRNG{t: t, draws: tc.draws}
			if err := s.ApplyPlayerTail(&c, tc.id, rng); err != nil || a.HP != tc.wantHP || a.MP != tc.wantMP || a.Tension != 1 {
				t.Fatal(a, err)
			}
			rng.done()
		})
	}
}

func TestPlayerTailWeaponSetAndInvocationFacts(t *testing.T) {
	for _, weapon := range []string{"weapon-69", "weapon-70"} {
		armor := "armor-54"
		if weapon == "weapon-70" {
			armor = "armor-55"
		}
		for _, tension := range []float64{1, 1.7, 2, 3} {
			s, c := playerTailFixture()
			a := s.actor("self")
			a.ItemDefinitionIDs, a.Tension = []string{weapon, armor}, tension
			rng := &targetRNG{t: t, draws: []float64{0}}
			if err := s.ApplyPlayerTail(&c, weapon+"-"+armor+"-tail", rng); err != nil {
				t.Fatal(err)
			}
			want := tension
			if tension == 1 {
				want = 1.7
			} else if tension == 1.7 {
				want = 3
			}
			if a.Tension != want || (c.TensionCalls == 1) != (tension < 2) {
				t.Fatal(a, c)
			}
			rng.done()
		}
	}
	for _, weapon := range []string{"weapon-30", "weapon-54"} {
		for _, heals := range []int{0, 1} {
			s, c := playerTailFixture()
			a := s.actor("self")
			a.ItemDefinitionIDs = []string{weapon}
			c.HealCalls, c.DamageCalls = heals, 1 // A missed/blocked helper still counts.
			draws, want := []float64{}, 100
			if heals == 0 {
				draws, want = []float64{.999, 0}, 135
			}
			rng := &targetRNG{t: t, draws: draws}
			if err := s.ApplyPlayerTail(&c, weapon+"-tail", rng); err != nil || a.HP != want {
				t.Fatal(a, err)
			}
			rng.done()
		}
	}
	for _, hp := range []int{300, 301} {
		s, c := playerTailFixture()
		a := s.actor("self")
		a.ItemDefinitionIDs, a.HP = []string{"weapon-35", "armor-37"}, hp
		draws, want := []float64{}, 301
		if hp == 300 {
			draws, want = []float64{0}, 390
		}
		rng := &targetRNG{t: t, draws: draws}
		if err := s.ApplyPlayerTail(&c, "weapon-35-armor-37-tail", rng); err != nil || a.HP != want {
			t.Fatal(a, err)
		}
		rng.done()
	}
}

func TestPlayerTailCMPRequesterBoundaryAndLifetimeCompletion(t *testing.T) {
	for _, tc := range []struct {
		charge            int
		item              bool
		draw              float64
		amount, threshold int
	}{
		{4, false, 0, 0, 0}, {5, false, 0, 1, 3}, {6, true, math.Nextafter(1, 0), 22, 9},
	} {
		s, c := playerTailFixture()
		a := s.actor("self")
		a.HP, a.CMP, a.MaxCMP, a.CMPCharge = 0, 9, 20, tc.charge
		if tc.item {
			a.ItemDefinitionIDs = []string{"item-238"}
		}
		draws := []float64{}
		if tc.charge >= 5 {
			draws = []float64{tc.draw, tc.draw}
		} else {
			c.RequesterLevel = nil
		}
		rng := &targetRNG{t: t, draws: draws}
		if err := s.ApplyPlayerTail(&c, "cmp-recharge-tail", rng); err != nil || c.Completion.CMPAmount != tc.amount || c.Completion.NextCMPThreshold != tc.threshold || a.CMP != 9 {
			t.Fatal(a, c, err)
		}
		if _, err := s.CompleteOperation(c.Completion, c.PartyIDs); err != nil {
			t.Fatal(err)
		}
		if a.CMP != min(20, 9+tc.amount) || (tc.amount == 0 && a.CMPCharge != 5) || (tc.amount > 0 && (a.CMPCharge != 0 || a.CMPNeedCharge != tc.threshold)) {
			t.Fatal(a)
		}
		rng.done()
	}
}

func TestPlayerTailUnknownAndUnsupportedAreInert(t *testing.T) {
	for _, id := range []string{"gem-101", "gem-102", "item-110", "item-194-combat-tail", "item-202-combat-tail", "frenzy-tail"} {
		s, c := playerTailFixture()
		a := s.actor("self")
		if id == "item-194-combat-tail" {
			a.ItemDefinitionIDs, a.Gear.AccessoryID = []string{"item-194"}, "item-110"
		} else if id == "item-202-combat-tail" {
			a.ItemDefinitionIDs, a.Temporary = []string{"item-202"}, TemporaryReflectMagic
		} else if id == "frenzy-tail" {
			a.Extended, a.Attack, a.BaseStats.Attack, c.DefeatContext = ExtendedFrenzy, 5000, 1000, nil
		}
		before, context := s.callbackCopy(), c
		rng := &targetRNG{t: t}
		if err := s.ApplyPlayerTail(&c, id, rng); !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(before, s) || !reflect.DeepEqual(context, c) {
			t.Fatal(id, err, s, c)
		}
		rng.done()
	}
}
