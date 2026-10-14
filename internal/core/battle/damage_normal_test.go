package battle

import (
	"testing"

	"github.com/witchcraze/party2re/internal/core/random"
)

func TestNormalAttackCriticalStillTestsPhysicalHit(t *testing.T) {
	for _, miss := range []bool{false, true} {
		s, c := damageFixture()
		admitDamage(&s, &c)
		s.actor("enemy").HP = 1000
		// Critical gate and two agility draws, then an independent hit check.
		draws := []float64{0, .9, 0, .95, .5, 0}
		if miss {
			draws = []float64{0, .9, 0, .99}
		}
		rng := &targetRNG{t: t, draws: draws}
		r, err := s.ApplyNormalAttack(&c, "enemy", rng)
		want := 101
		if miss {
			want = 0
		}
		if err != nil || len(r) != 1 || !r[0].Critical || r[0].Amount != want || c.DamageCalls != 1 {
			t.Fatal(r, err, c)
		}
		rng.done()
	}
}

func TestNormalAttackFallenTargetDoesNotRequireDamageTail(t *testing.T) {
	s, c := damageFixture()
	rng := &targetRNG{t: t, draws: []float64{.5}}
	r, err := s.ApplyNormalAttack(&c, "dead_enemy", rng)
	if err != nil || len(r) != 1 || r[0].Returned || c.DamageCalls != 0 {
		t.Fatal(r, err, c)
	}
	rng.done()
}

func TestNormalAttackSealAndItemAdditionalFacts(t *testing.T) {
	s, c := damageFixture()
	admitDamage(&s, &c)
	c.GuaranteedHit = true
	s.actor("enemy").HP = 1000
	s.actor("self").Abilities = []string{"seal_shinsoku"}
	s.actor("self").ItemDefinitionIDs = []string{"item-147"}
	rng := &targetRNG{t: t, draws: []float64{.5, 0, 0, 0, .5, 0, .9}}
	r, err := s.ApplyNormalAttack(&c, "enemy", rng)
	if err != nil || len(r) != 3 || c.DamageCalls != 3 || r[0].Critical || r[1].Critical || r[2].Critical {
		t.Fatal(r, err, c)
	}
	rng.done()
}

func TestStatDamageSharesOperationCounter(t *testing.T) {
	s, c := damageFixture()
	admitDamage(&s, &c)
	s.actor("enemy").HP = 1000
	s.actor("ally").ItemDefinitionIDs = []string{"item-201"}
	c.Damage = func(target TargetContext, e TargetEffect, rng random.Generator) error {
		_, err := s.ApplyDamage(&c, e, rng)
		return err
	}
	rng := &targetRNG{t: t, draws: []float64{0, 0}}
	r, err := s.ApplyStatUp(&c.EffectContext, TargetEffect{TargetID: "ally", Payload: CallbackPayload{Number: .25}}, StatAgility, rng)
	if err != nil || !r.Applied || c.DamageCalls != 1 || c.StatUpCalls != 1 || s.actor("enemy").HP != 922 {
		t.Fatal(r, err, c, s)
	}
	rng.done()
}
