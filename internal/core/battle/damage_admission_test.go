package battle

import (
	"errors"
	"reflect"
	"testing"
)

func TestAreaAllOneHitDefensesAreAccepted(t *testing.T) {
	for _, field := range []string{"", "cosmic"} {
		s, c := damageFixture()
		s.Fields.Default = field
		for _, id := range c.EnemyIDs {
			s.actor(id).HP = 100
			s.actor(id).Temporary = TemporaryOneHitDefense
			s.actor(id).ItemDefinitionIDs = []string{"item-195"}
			s.actor(id).Gear = nil
		}
		before := reloadLifetime(t, s)
		e := TargetEffect{Category: CategoryMagic, Payload: CallbackPayload{Number: 50}, Arguments: []string{"1"}}
		rng := &targetRNG{t: t}
		if err := s.validateAreaDamage(&c, e, rng); err != nil {
			t.Fatal(field, err)
		}
		if !reflect.DeepEqual(s, before) || c.DamageCalls != 0 || c.AreaDamageCalls != 0 {
			t.Fatal("admission mutated state", s, c)
		}
		r, err := s.ApplyDamageGem(16, &c, "ignored", rng)
		if err != nil || len(r) != 2 || c.DamageCalls != 2 || c.AreaDamageCalls != 1 || c.Item195Activated {
			t.Fatal(field, r, err, c)
		}
		for i, id := range c.EnemyIDs {
			if !r[i].Returned || r[i].Applied || r[i].Amount != 0 || s.actor(id).HP != 100 || s.actor(id).Temporary != "" {
				t.Fatal(r, s)
			}
		}
		rng.done()
	}
}

func TestAreaShieldAdmissionDoesNotDrawInitialModifier(t *testing.T) {
	s, c := damageFixture()
	s.actor("self").ItemDefinitionIDs = []string{"item-144", "weapon-37"}
	for _, id := range c.EnemyIDs {
		s.actor(id).HP = 100
		s.actor(id).Temporary = TemporaryOneHitDefense
	}
	rng := &targetRNG{t: t, draws: []float64{0}}
	e := TargetEffect{Category: CategoryMagic, Payload: CallbackPayload{Number: 50}, Arguments: []string{"1"}}
	if err := s.validateAreaDamage(&c, e, rng); err != nil || rng.used != 0 {
		t.Fatal(err, rng.used)
	}
	r, err := s.ApplyAreaDamage(&c, 50, CategoryMagic, true, rng)
	if err != nil || len(r) != 2 || c.DamageCalls != 2 || c.AreaDamageCalls != 1 {
		t.Fatal(r, err, c)
	}
	rng.done()
}

func TestAreaThresholdCanPreservePowerAfterEarlierAreaCall(t *testing.T) {
	s, c := damageFixture()
	s.actor("self").ItemDefinitionIDs = []string{"item-144", "weapon-34"}
	c.AreaDamageCalls = 1
	e := TargetEffect{Category: CategoryMagic, Payload: CallbackPayload{Number: 250}, Arguments: []string{"1"}}
	before := reloadLifetime(t, s)
	if !s.canPreserveAreaPower(&c, e) || !reflect.DeepEqual(s, before) || c.AreaDamageCalls != 1 {
		t.Fatal(s, c)
	}
	for _, tc := range []struct{ draw, want float64 }{{.5, 325}, {0, 300}} {
		rng := &targetRNG{t: t, draws: []float64{tc.draw}}
		if v := s.modifyDamage(&c, nil, 250, CategoryMagic, true, false, rng.Float64); v != tc.want {
			t.Fatal(v, tc.want)
		}
		rng.done()
	}
}

func TestAreaRevisitedDefenseRequiresTailAdmission(t *testing.T) {
	for _, confused := range []bool{false, true} {
		for _, supported := range []bool{false, true} {
			s, c := damageFixture()
			s.actor("dead_enemy").HP = 100
			s.actor("self").Temporary = TemporaryOneHitDefense
			for _, id := range c.EnemyIDs {
				s.actor(id).Temporary = TemporaryReflectMagic
			}
			if confused {
				s.actor("self").Status = StatusConfusion
				for i := range s.Actors {
					s.Actors[i].Temporary = TemporaryOneHitDefense
				}
			}
			if supported {
				admitDamage(&s, &c)
			}
			before := reloadLifetime(t, s)
			rng := &targetRNG{t: t}
			if supported {
				rng.draws = []float64{0}
				if confused {
					rng.draws = []float64{0, 0, 0}
				}
			}
			r, err := s.ApplyAreaDamage(&c, 100, CategoryMagic, true, rng)
			if !supported {
				if !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(s, before) || c.DamageCalls != 0 || c.AreaDamageCalls != 0 {
					t.Fatal(confused, r, err, s, c)
				}
			} else if err != nil || len(r) != 2 || !r[0].Returned || r[0].Applied || !r[1].Applied || r[1].TargetID != "self" || c.DamageCalls != 2 || s.actor("self").HP != 24 {
				t.Fatal(confused, r, err, s, c)
			}
			rng.done()
		}
	}
}

func TestAreaDefenseConsumptionUsesPerHitPower(t *testing.T) {
	for _, diamond := range []bool{false, true} {
		s, c := damageFixture()
		s.actor("self").Abilities = []string{"seal_kotowari"}
		s.actor("self").Temporary = TemporaryOneHitDefense
		if diamond {
			s.actor("self").ItemDefinitionIDs = []string{"item-144"}
		}
		s.actor("dead_enemy").HP = 100
		for _, id := range c.EnemyIDs {
			s.actor(id).Temporary = TemporaryReflectMagic
		}
		before := reloadLifetime(t, s)
		rng := &targetRNG{t: t}
		r, err := s.ApplyAreaDamage(&c, 1.4, CategoryPhysical, true, rng)
		if diamond {
			if !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(s, before) || c.AreaDamageCalls != 0 {
				t.Fatal(r, err, s, c)
			}
		} else if err != nil || len(r) != 2 || !r[0].Returned || r[1].Returned || r[1].Payload.Number != 0 || c.DamageCalls != 1 || c.AreaDamageCalls != 1 || s.actor("self").HP != 100 {
			t.Fatal(r, err, s, c)
		}
		rng.done()
	}
}

func TestAreaEnhancementCannotHideRevisitedDefense(t *testing.T) {
	s, c := damageFixture()
	s.actor("self").Extended = ExtendedAdditional
	s.actor("self").ItemDefinitionIDs = []string{"item-143"}
	s.actor("self").Temporary = TemporaryOneHitDefense
	s.actor("dead_enemy").HP = 100
	for _, id := range c.EnemyIDs {
		s.actor(id).Temporary = TemporaryReflectMagic
	}
	before := reloadLifetime(t, s)
	_, err := s.ApplyAreaDamage(&c, .4, CategoryMagic, true, &targetRNG{t: t})
	if !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(s, before) || c.AreaDamageCalls != 0 || c.DamageCalls != 0 {
		t.Fatal(err, s, c)
	}
}

func TestRandomMultiHitDefenseIsValidatedBeforeCountDraw(t *testing.T) {
	for _, source := range []int{6, 7, 8, 9} {
		s, c := damageFixture()
		for _, id := range c.EnemyIDs {
			s.actor(id).HP = 100
			s.actor(id).Temporary = TemporaryOneHitDefense
		}
		before := reloadLifetime(t, s)
		rng := &targetRNG{t: t}
		admission := s.ValidateDamageGem(source, &c, "ignored", rng)
		_, err := s.ApplyDamageGem(source, &c, "ignored", rng)
		if !errors.Is(admission, ErrUnsupportedEffect) || !errors.Is(err, admission) || !reflect.DeepEqual(s, before) || c.DamageCalls != 0 || c.AreaDamageCalls != 0 {
			t.Fatal(source, admission, err, s, c)
		}
		rng.done()
	}
	s, c := damageFixture()
	admitDamage(&s, &c)
	c.GuaranteedHit = true
	for _, id := range c.EnemyIDs {
		s.actor(id).HP = 100
		s.actor(id).Temporary = TemporaryOneHitDefense
	}
	rng := &targetRNG{t: t, draws: []float64{0, 0, 0, 0, 0, 0}}
	r, err := s.ApplyDamageGem(8, &c, "ignored", rng)
	if err != nil || len(r) != 2 || r[0].Applied || !r[1].Applied || r[1].Amount != 81 || c.DamageCalls != 2 || s.actor("enemy").HP != 19 || s.actor("dead_enemy").Temporary != TemporaryOneHitDefense {
		t.Fatal(r, err, s, c)
	}
	rng.done()
}

func TestRepeatAdmissionUsesMaximumHitCountAfterItem195Activation(t *testing.T) {
	for _, source := range []int{8, 6} {
		s, c := damageFixture()
		admitDamage(&s, &c)
		c.GuaranteedHit = true
		s.actor("enemy").HP = 1000
		s.actor("enemy").Temporary = TemporaryOneHitDefense
		s.actor("enemy").ItemDefinitionIDs = []string{"item-195"}
		s.actor("self").Temporary = TemporaryOneHitDefense
		s.actor("ally").Temporary = TemporaryOneHitDefense
		s.actor("ally").Gear = nil
		before := reloadLifetime(t, s)
		rng := &targetRNG{t: t}
		admission := s.ValidateDamageGem(source, &c, "ignored", rng)
		if !reflect.DeepEqual(s, before) || rng.used != 0 || c.DamageCalls != 0 {
			t.Fatal("admission mutated state", source, s, c)
		}
		if source == 8 {
			rng.draws = []float64{.99, 0, 0, 0, 0, 0, 0, .1, .4, .4}
		}
		r, err := s.ApplyDamageGem(source, &c, "ignored", rng)
		if source == 6 {
			if !errors.Is(admission, ErrUnsupportedEffect) || !errors.Is(err, admission) || !reflect.DeepEqual(s, before) || c.DamageCalls != 0 {
				t.Fatal(source, admission, r, err, s, c)
			}
		} else if admission != nil || err != nil || len(r) != 3 || r[0].Applied || !r[1].Applied || r[2].Applied || c.DamageCalls != 3 || s.actor("ally").HP != 100 || s.actor("ally").Temporary != "" || s.actor("self").Status != StatusConfusion {
			t.Fatal(source, admission, r, err, s, c)
		}
		rng.done()
	}
}

func TestConfusedSelfRetainsItsTarget(t *testing.T) {
	s, c := damageFixture()
	s.actor("self").Status = StatusConfusion
	s.actor("self").Temporary = TemporaryOneHitDefense
	rng := &targetRNG{t: t}
	r, err := s.ApplyDamage(&c, TargetEffect{TargetID: "self", Category: CategoryMagic, Payload: CallbackPayload{Number: 100}}, rng)
	if err != nil || !r.Returned || r.Applied || c.DamageCalls != 1 || s.actor("self").HP != 100 {
		t.Fatal(r, err, c)
	}
	rng.done()
}

func TestPossibleConfusionOrBossReselectionStillRejectsBeforeEffects(t *testing.T) {
	for _, mode := range []string{"activated", "boss", "cure then activation"} {
		s, c := damageFixture()
		s.actor("self").Temporary = TemporaryOneHitDefense
		s.actor("enemy").Temporary = TemporaryOneHitDefense
		c.Item195Activated = mode != "boss"
		if mode == "activated" {
			s.actor("enemy").Temporary = ""
		}
		if mode == "boss" {
			s.actor("self").Status = StatusConfusion
			s.SpecialBossField = true
		}
		if mode == "cure then activation" {
			s.actor("self").Status = StatusPhysicalSeal
		}
		before := reloadLifetime(t, s)
		rng := &targetRNG{t: t}
		var err error
		if mode == "cure then activation" {
			_, err = s.ApplyDamageGem(8, &c, "ignored", rng)
		} else {
			_, err = s.ApplyDamage(&c, TargetEffect{TargetID: "self", Category: CategoryMagic, Payload: CallbackPayload{Number: 100}}, rng)
		}
		if !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(s, before) || c.DamageCalls != 0 || c.AreaDamageCalls != 0 {
			t.Fatal(mode, err, s, c)
		}
		rng.done()
	}
}

func TestActivatedConfusionSelfChoiceCanStillBeFullyBlocked(t *testing.T) {
	for _, activate := range []bool{false, true} {
		s, c := damageFixture()
		c.Item195Activated = true
		s.actor("self").Temporary = TemporaryOneHitDefense
		s.actor("enemy").Temporary = TemporaryOneHitDefense
		s.actor("ally").Gear = nil
		e := TargetEffect{TargetID: "self", Category: CategoryMagic, Payload: CallbackPayload{Number: 100}}
		rng := &targetRNG{t: t, draws: []float64{.2, 0}}
		if activate {
			rng.draws = []float64{.1}
		}
		before := reloadLifetime(t, s)
		if err := s.ValidateDamage(&c, e, rng); err != nil || rng.used != 0 || !reflect.DeepEqual(s, before) {
			t.Fatal(activate, err, rng.used, s)
		}
		r, err := s.ApplyDamage(&c, e, rng)
		if err != nil || !r.Returned || r.Applied || c.DamageCalls != 1 || s.actor("self").HP != 100 || s.actor("enemy").HP != 100 {
			t.Fatal(activate, r, err, s, c)
		}
		rng.done()
	}
}

func TestNormalAttackDefenseRespectsRoleAndRepeatGates(t *testing.T) {
	for _, kind := range []string{"single", "npc147", "player147", "seal10"} {
		s, c := damageFixture()
		s.actor("enemy").Temporary = TemporaryOneHitDefense
		if kind == "npc147" || kind == "player147" {
			s.actor("self").ItemDefinitionIDs = []string{"item-147"}
		}
		c.ActorNPC = kind == "npc147"
		if kind == "seal10" {
			s.actor("self").Abilities = []string{"seal_shinsoku"}
		}
		before := reloadLifetime(t, s)
		rng := &targetRNG{t: t}
		accepted := kind == "single" || kind == "npc147"
		if accepted {
			rng.draws = []float64{.5}
			if kind == "npc147" {
				rng.draws = append(rng.draws, 0)
			}
		}
		r, err := s.ApplyNormalAttack(&c, "enemy", rng)
		if accepted {
			if err != nil || len(r) != 1 || r[0].Applied || c.DamageCalls != 1 {
				t.Fatal(kind, r, err, c)
			}
		} else if !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(s, before) || c.DamageCalls != 0 {
			t.Fatal(kind, r, err, s, c)
		}
		rng.done()
	}
}

func TestConfusedNormalAttackStillSelectsEnemyBeforeHelper(t *testing.T) {
	s, c := damageFixture()
	s.actor("self").Status = StatusConfusion
	s.actor("self").Temporary = TemporaryOneHitDefense
	before := reloadLifetime(t, s)
	_, err := s.ApplyNormalAttack(&c, "self", &targetRNG{t: t})
	if !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(s, before) || c.DamageCalls != 0 {
		t.Fatal(err, s, c)
	}
}

func TestNormal147FallenTargetCannotAddAnotherHit(t *testing.T) {
	for _, double := range []bool{false, true} {
		s, c := damageFixture()
		c.Item195Activated = true
		s.actor("self").ItemDefinitionIDs = []string{"item-147"}
		s.actor("self").Status = StatusPhysicalSeal
		rng := &targetRNG{t: t, draws: []float64{.5, 0, 0}}
		if double {
			s.actor("self").Abilities = []string{"seal_shinsoku"}
			for i := range s.Actors {
				s.Actors[i].Temporary = TemporaryOneHitDefense
			}
			rng.draws = append(rng.draws, .5, .1, 0, 0)
		}
		r, err := s.ApplyNormalAttack(&c, "dead_enemy", rng)
		wantHits, wantCalls := 1, 0
		if double {
			wantHits, wantCalls = 2, 1
		}
		if err != nil || len(r) != wantHits || c.DamageCalls != wantCalls || s.actor("self").HP != 100 {
			t.Fatal(double, r, err, s, c)
		}
		rng.done()
	}
}
