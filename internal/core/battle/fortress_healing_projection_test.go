package battle

import (
	"errors"
	"reflect"
	"testing"
)

func fortressHealingSequence(t *testing.T, unsupportedLast bool) (LifetimeState, NamedFieldContext, *targetRNG) {
	t.Helper()
	s := LifetimeState{}
	for _, id := range []string{"self", "ally", "first", "second", "last"} {
		a := LifetimeActor{Participant: Participant{ID: id, TeamID: "enemy", HP: 1, MaxHP: 1000, BaseStats: BattleBaseStats{Attack: 100}, Gear: &BattleGearStats{}}, Hit: 95, Tension: 1, NPC: true}
		if id == "self" || id == "ally" {
			a.TeamID, a.NPC = "party", false
		}
		if id == "first" || id == "second" {
			a.Gear.AccessoryID, a.ItemDefinitionIDs = "item-212", []string{"item-212"}
		}
		if id == "second" {
			a.MaxHP = 1
		}
		s.Actors = append(s.Actors, a)
	}
	target := TargetContext{ActorID: "self", PartyIDs: []string{"self", "ally"}, EnemyIDs: []string{"first", "second", "last"}}
	level := 70
	c := NamedFieldContext{DamageContext: DamageContext{EffectContext: EffectContext{TargetContext: target, RequesterLevel: &level}}, Death: nativeDefeatContext(target)}
	c.Death.RequesterLevel, c.Death.FieldAtLoad = &level, "fortress"
	if unsupportedLast {
		c.Death.Rewards.Validate = func(_ *LifetimeState, _ TargetContext, id string) error {
			if id == "last" {
				return ErrUnsupportedEffect
			}
			return nil
		}
	}
	// First212 kills ally, whose fortress hook heals self from1 to181.
	// Second212 then deals67: self survives and reaches the last breath hit.
	rng := &targetRNG{t: t, draws: []float64{0, .75, 0, 0, 0, 0, 0, 0, 0, 0, 0}}
	c.Death.Fortress = FortressDefeat(rng)
	return s, c, rng
}

func TestFortressHealingSurvivalCannotHideLaterUnsupportedDefeat(t *testing.T) {
	s, c, rng := fortressHealingSequence(t, true)
	before := s.callbackCopy()
	admissionErr := s.ValidateNamedField(&c, "job-84-entry-4", rng)
	if rng.used != 0 || !reflect.DeepEqual(s, before) {
		t.Fatal("preflight changed state or consumed caller RNG")
	}
	executionErr := s.ApplyNamedField(&c, "job-84-entry-4", rng)
	if !errors.Is(admissionErr, ErrUnsupportedEffect) || !errors.Is(executionErr, ErrUnsupportedEffect) || !reflect.DeepEqual(s, before) || rng.used != 0 {
		t.Fatalf("admission=%v execution=%v draws=%d selfHP=%d lastHP=%d; unsupported last defeat must reject before the first effect", admissionErr, executionErr, rng.used, s.actor("self").HP, s.actor("last").HP)
	}
}

func TestFortressHealingSurvivalReachesSupportedLaterAttack(t *testing.T) {
	s, c, rng := fortressHealingSequence(t, false)
	if err := s.ValidateNamedField(&c, "job-84-entry-4", rng); err != nil || rng.used != 0 {
		t.Fatal(err, rng.used)
	}
	if err := s.ApplyNamedField(&c, "job-84-entry-4", rng); err != nil || s.actor("self").HP != 114 || s.actor("last").HP != 0 || c.DamageCalls != 3 {
		t.Fatal(err, s, c.DamageCalls)
	}
	rng.done()
}

func TestFortressHealingProjectionKeepsIndependentHPBounds(t *testing.T) {
	s, c := namedFieldFixture()
	s.actor("self").HP, s.actor("self").MaxHP = 810, 2000
	s.actor("ally").HP, s.actor("ally").MaxHP = 810, 2000
	s.actor("fallen").MaxHP = 1000
	before := s.callbackCopy()
	states, err := FortressDefeat(&targetRNG{t: t}).Project(&s, c.TargetContext, "fallen")
	if err != nil || len(states) != 16 || !reflect.DeepEqual(s, before) {
		t.Fatal(err, len(states), s)
	}
	pairs := map[[2]int]bool{}
	for _, state := range states {
		pairs[[2]int{state.actor("self").HP, state.actor("ally").HP}] = true
		if state.actor("fallen").HP != 0 {
			t.Fatal("projection revived fallen member")
		}
	}
	for _, self := range []int{990, 1050, 999, 1000} {
		for _, ally := range []int{990, 1050, 999, 1000} {
			if !pairs[[2]int{self, ally}] {
				t.Fatal("missing independent HP bounds", self, ally)
			}
		}
	}
}

func TestFortressHealingEmptyProjectionRetainsWholeEntryGate(t *testing.T) {
	for _, owner := range []string{"rewards", "fortress"} {
		s, c := finalStrikeFixture()
		c.FieldAtLoad = "fortress"
		c.Fortress = FortressDefeat(&targetRNG{t: t})
		empty := func(*LifetimeState, TargetContext, string) ([]LifetimeState, error) { return nil, nil }
		if owner == "rewards" {
			c.Rewards.Project = empty
		} else {
			c.Fortress.Project = empty
		}
		before := s.callbackCopy()
		if _, err := s.projectDefeatContinuation(c, "enemy", &targetRNG{t: t}, map[string]bool{}, true); !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(s, before) {
			t.Fatal(owner, err, s)
		}
	}
}
