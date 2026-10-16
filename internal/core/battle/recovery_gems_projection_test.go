package battle

import (
	"errors"
	"reflect"
	"testing"
)

func conditionalReductionFixture(stat BattleStat) (LifetimeState, EffectContext) {
	s, c := recoveryFixture()
	s.Actors = []LifetimeActor{*s.actor("self"), *s.actor("enemy")}
	c.PartyIDs, c.EnemyIDs = []string{"self"}, []string{"enemy"}
	s.actor("self").Status, s.actor("self").Tension = StatusConfusion, 3
	*s.actor("self").currentStat(stat) = 250
	s.actor("enemy").ItemDefinitionIDs = []string{"item-159"}
	return s, c
}

func TestRecoveryGemConditionalReductionRetainsCappedBranch(t *testing.T) {
	for _, tc := range []struct {
		down, up int
		stat     BattleStat
	}{{93, 29, StatAttack}, {94, 31, StatDefense}, {95, 33, StatAgility}} {
		s, c := conditionalReductionFixture(tc.stat)
		ids := []int{tc.down, tc.up, 67}
		before, contextBefore := reloadLifetime(t, s), c
		rng := &targetRNG{t: t}
		if err := s.ValidateRecoveryStatGems(ids, &c, "self", rng); !errors.Is(err, ErrUnsupportedEffect) {
			t.Fatal(tc, err)
		}
		if _, err := s.ApplyRecoveryStatGems(ids, &c, "self", rng); !errors.Is(err, ErrUnsupportedEffect) {
			t.Fatal(tc, err)
		}
		if !reflect.DeepEqual(s, before) || !reflect.DeepEqual(c, contextBefore) {
			t.Fatal("conditional reduction executed before rejection", tc, s, c)
		}
		rng.done()

		// Both enemy-helper draws choose the immune enemy, then ally resolution
		// chooses the unchanged capped actor. Only complete requester context admits67.
		level := 99
		c.RequesterLevel = &level
		rng = &targetRNG{t: t, draws: []float64{.9, .9, 0, 0}}
		r, err := s.ApplyRecoveryStatGems(ids, &c, "self", rng)
		if err != nil || r[0][0].Applied || r[1][0].Applied || !r[1][0].Returned || s.actor("self").Tension != 8 || *s.actor("self").currentStat(tc.stat) != 250 || *s.actor("enemy").currentStat(tc.stat) != 150 {
			t.Fatal(tc, r, err, s)
		}
		rng.done()
	}
}

func TestRecoveryGemGuaranteedReductionThenRaiseNeedsNoRequester(t *testing.T) {
	for _, tc := range []struct {
		down, up int
		stat     BattleStat
	}{{93, 29, StatAttack}, {94, 31, StatDefense}, {95, 33, StatAgility}} {
		s, c := conditionalReductionFixture(tc.stat)
		s.actor("self").Status = ""
		s.actor("enemy").ItemDefinitionIDs = nil
		s.actor("enemy").Temporary = TemporaryReflectMagic
		rng := &targetRNG{t: t}
		r, err := s.ApplyRecoveryStatGems([]int{tc.down, tc.up, 67}, &c, "self", rng)
		if err != nil || len(r) != 3 || !r[0][0].Applied || !r[1][0].Applied || *s.actor("self").currentStat(tc.stat) != 163 || s.actor("self").Tension != 3 {
			t.Fatal(tc, r, err, s)
		}
		rng.done()
	}
}

func TestRecoveryGemImpossibleReductionPreservesAcceptedCapNoEffect(t *testing.T) {
	s, c := conditionalReductionFixture(StatAgility)
	s.actor("self").Tension = 8
	s.actor("self").ItemDefinitionIDs = []string{"item-201"}
	s.actor("self").Temporary = TemporaryNullMagic
	s.actor("enemy").Agility = 250
	rng := &targetRNG{t: t, draws: []float64{.9, .9, 0}}
	r, err := s.ApplyRecoveryStatGems([]int{95, 33, 67}, &c, "self", rng)
	if err != nil || len(r) != 3 || r[0][0].Applied || r[1][0].Applied || s.actor("self").Agility != 250 || s.actor("self").Tension != 8 {
		t.Fatal(r, err, s, c)
	}
	rng.done()
}

func TestRecoveryGemPossibleReductionStillRequiresStatTail(t *testing.T) {
	s, c := conditionalReductionFixture(StatAgility)
	s.actor("self").ItemDefinitionIDs = []string{"item-201"}
	before, contextBefore := reloadLifetime(t, s), c
	rng := &targetRNG{t: t}
	if _, err := s.ApplyRecoveryStatGems([]int{95, 33}, &c, "self", rng); !errors.Is(err, ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(s, before) || !reflect.DeepEqual(c, contextBefore) {
		t.Fatal("possible agility tail ran before admission", s, c)
	}
	rng.done()
}
