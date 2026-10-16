package battle

import (
	"encoding/json"
	"errors"
	"reflect"
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

func TestNormalAttackDetachedTargetAdmission(t *testing.T) {
	setup := func(t *testing.T) (LifetimeState, RosterContext, string, string) {
		t.Helper()
		s, c := rosterFixture()
		admitRosterDamage(&s, &c)
		helper59 := addRosterHelper(t, &s, &c, 59)
		detached := addRosterHelper(t, &s, &c, 60)
		addRosterHelper(t, &s, &c, 61)
		if _, err := s.ApplyRoster(&c, "job-60-dismiss", "", &targetRNG{t: t}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.ApplyRoster(&c, "job-61-revive-party", "", &targetRNG{t: t}); err != nil {
			t.Fatal(err)
		}
		s.actor("enemy").Temporary = TemporaryOneHitDefense
		return s, c, helper59, detached
	}

	t.Run("unsupported field on detached target rejects before draws", func(t *testing.T) {
		s, c, _, detached := setup(t)
		s.Fields.Default = "cosmic"
		before, _ := json.Marshal(s)
		beforeParty := append([]string(nil), c.PartyIDs...)
		beforeEnemy := append([]string(nil), c.EnemyIDs...)
		rng := &targetRNG{t: t}
		err := s.ValidateRoster(&c, "job-59-attack", detached, rng)
		_, runErr := s.ApplyRoster(&c, "job-59-attack", detached, rng)
		after, _ := json.Marshal(s)
		if !errors.Is(err, ErrUnsupportedEffect) || !errors.Is(runErr, ErrUnsupportedEffect) || string(after) != string(before) || c.DamageCalls != 0 || c.ActorID != "player" || !reflect.DeepEqual(c.PartyIDs, beforeParty) || !reflect.DeepEqual(c.EnemyIDs, beforeEnemy) {
			t.Fatal(err, runErr, s, c)
		}
		rng.done()

		s.PruneRoster(&c.TargetContext)
		beforePruned := reloadLifetime(t, s)
		if err := s.ValidateRoster(&c, "job-59-attack", detached, rng); !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(s, beforePruned) {
			t.Fatal(err, s)
		}
		rng.done()
	})

	t.Run("one-hit defense on detached target admits single and rejects repeat strikes", func(t *testing.T) {
		for _, kind := range []string{"single", "seal10", "item147"} {
			s, c, helper59, detached := setup(t)
			s.Fields.Default = "cosmic"
			s.actor(detached).Temporary = TemporaryOneHitDefense
			switch kind {
			case "seal10":
				s.actor(helper59).Abilities = []string{"seal_shinsoku"}
			case "item147":
				s.actor(helper59).ItemDefinitionIDs = []string{"item-147"}
			}
			before, _ := json.Marshal(s)
			rng := &targetRNG{t: t}
			if kind == "single" {
				rng.draws = []float64{.5}
			}
			r, err := s.ApplyRoster(&c, "job-59-attack", detached, rng)
			after, _ := json.Marshal(s)
			if kind == "single" {
				if err != nil || !r.Applied || len(r.Hits) != 1 || r.Hits[0].Applied || c.DamageCalls != 1 || c.ActorID != "player" || s.actor(detached).Temporary != "" {
					t.Fatal(kind, r, err, c, s.actor(detached))
				}
			} else if !errors.Is(err, ErrUnsupportedEffect) || string(after) != string(before) || c.DamageCalls != 0 || c.ActorID != "player" {
				t.Fatal(kind, r, err, c, s)
			}
			rng.done()
		}
	})
}
