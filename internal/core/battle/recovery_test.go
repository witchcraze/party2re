package battle

import (
	"errors"
	"math"
	"reflect"
	"testing"
)

func recoveryFixture() (LifetimeState, EffectContext) {
	s, c := targetFixture()
	for i := range s.Actors {
		a := &s.Actors[i]
		a.MaxHP = 1000
		a.Attack, a.Defense, a.Agility = 150, 150, 150
		a.BaseStats = BattleBaseStats{Attack: 100, Defense: 100, Agility: 100}
		a.Tension, a.Hit = 1, 95
	}
	return s, EffectContext{TargetContext: c}
}

func TestProportionalStatUsesBaseUpCurrentDown(t *testing.T) {
	s, c := recoveryFixture()
	r, err := s.ApplyStatUp(&c, TargetEffect{TargetID: "ally", Payload: CallbackPayload{Number: .25}}, StatAttack, &targetRNG{t: t})
	if err != nil || !r.Applied || r.Amount != 25 || s.actor("ally").Attack != 175 {
		t.Fatal(r, err, s)
	}
	r, err = s.ApplyStatDown(&c, TargetEffect{TargetID: "enemy", Payload: CallbackPayload{Number: .25}}, StatAttack, &targetRNG{t: t})
	if err != nil || !r.Applied || r.Amount != 37 || s.actor("enemy").Attack != 113 {
		t.Fatal(r, err, s)
	}
}

func TestPartyHealingConsumesTensionInOrder(t *testing.T) {
	s, c := recoveryFixture()
	s.actor("self").Tension = 3
	rng := &targetRNG{t: t, draws: []float64{0, 0, 0}}
	results, err := s.ApplyPartyHealing(&c, 100, CategoryMagic, rng)
	if err != nil || len(results) != 3 || s.actor("self").HP != 460 || s.actor("ally").HP != 190 || s.actor("fallen").HP != 0 || c.PartyHealCalls != 1 || c.HealCalls != 3 {
		t.Fatal(results, err, s, c)
	}
	rng.done()
}

func TestHealingSourceBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name             string
		power            float64
		category         CallbackCategory
		items            []string
		tmp              TemporaryEffect
		field            string
		boss             bool
		damage           int
		hp, want, amount int
		draws            []float64
	}{
		{"stone", 100, CategoryMagic, []string{"item-146"}, "", "", false, 0, 100, 262, 162, []float64{0}},
		{"song49 doubled", 49, CategorySong, []string{"item-148"}, TemporaryDouble, "", false, 0, 100, 982, 882, []float64{0}},
		{"song50 doubled", 50, CategorySong, []string{"item-148"}, TemporaryDouble, "", false, 0, 100, 190, 90, []float64{0}},
		{"full", 100, CategoryMagic, nil, TemporaryRage, "", false, 0, 1000, 1000, 90, []float64{0}},
		{"sanctuary below", 100, CategoryMagic, nil, "", "sanctuary", false, 0, 100, 208, 108, []float64{0}},
		{"sanctuary at", 100, CategoryMagic, nil, "", "sanctuary", false, 0, 892, 1000, 108, []float64{0}},
		{"sanctuary overflow", 100, CategoryMagic, nil, "", "sanctuary", false, 0, 950, 942, 108, []float64{0}},
		{"sanctuary floor", 3000, CategoryMagic, nil, "", "sanctuary", false, 0, 950, 1, 3240, []float64{0}},
		{"boss half cap", 2000, CategoryMagic, nil, "", "", true, 1, 100, 550, 450, []float64{0}},
		{"boss sanctuary exception", 100, CategoryMagic, nil, "", "sanctuary", true, 1, 100, 208, 108, []float64{0}},
		{"boss untyped no exception", 100, CategoryUntyped, nil, "", "sanctuary", true, 1, 100, 145, 45, []float64{0}},
		{"damage zero modifier", 100, CategoryMagic, []string{"item-239"}, TemporaryRage, "", false, 1, 100, 100, 0, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, c := recoveryFixture()
			s.actor("self").ItemDefinitionIDs = tc.items
			s.actor("ally").HP, s.actor("ally").Temporary = tc.hp, tc.tmp
			s.Fields.Default, s.SpecialBossField, c.DamageCalls = tc.field, tc.boss, tc.damage
			rng := &targetRNG{t: t, draws: tc.draws}
			r, err := s.ApplyHealing(&c, TargetEffect{TargetID: "ally", Category: tc.category, Payload: CallbackPayload{Number: tc.power}}, rng)
			if err != nil || s.actor("ally").HP != tc.want || r.Amount != tc.amount || r.Applied != (tc.want != tc.hp) {
				t.Fatal(r, err, s)
			}
			if tc.name == "full" && (s.actor("ally").Temporary != "" || !r.Returned) {
				t.Fatal(r, s)
			}
			rng.done()
		})
	}
}

func TestRecoveryModifierOrder(t *testing.T) {
	s, c := recoveryFixture()
	s.actor("self").ItemDefinitionIDs = []string{"item-186", "item-144", "item-239", "item-190"}
	c.Item185Activated, c.BowEffect, c.AreaDamageCalls, c.DamageCalls = true, true, 1, 1
	s.Fields.Default = "sanctuary"
	rng := &targetRNG{t: t, draws: []float64{0}}
	r, err := s.ApplyStatUp(&c, TargetEffect{TargetID: "ally", Category: CategoryMagic, Payload: CallbackPayload{Number: 1}}, StatAttack, rng)
	// Truncation retains the floating source order, BEFORE144 and239.
	if err != nil || r.Applied || r.Returned || s.actor("ally").HP != 180 || s.actor("ally").Attack != 150 || len(r.IncidentalHealing) != 1 || r.IncidentalHealing[0].Amount != 80 {
		t.Fatal(r, err, s)
	}
	rng.done()
	s, c = recoveryFixture()
	s.actor("self").Status = StatusSabaku
	s.actor("ally").Status = StatusVirulentPoison
	rng = &targetRNG{t: t, draws: []float64{.5, 0}}
	r, err = s.ApplyHealing(&c, TargetEffect{TargetID: "ally", Payload: CallbackPayload{Number: 100}}, rng)
	if err != nil || r.Amount != 7 || s.actor("ally").HP != 107 {
		t.Fatal(r, err, s)
	}
	rng.done()
	// Shared MP modifier: post-zero den +1, without writing MP.
	s, c = recoveryFixture()
	s.Fields.Default = "den"
	s.actor("self").ItemDefinitionIDs = []string{"item-239"}
	c.DamageCalls = 1
	value, _, err := s.ModifyEffect(&c, TargetEffect{TargetID: "ally", Payload: CallbackPayload{Number: 100}}, ModifierMPHealing, "", &targetRNG{t: t})
	if err != nil || value != 1 || s.actor("ally").MP != 0 {
		t.Fatal(value, err, s)
	}
}

func TestHealingSplashOnceAndReflection(t *testing.T) {
	s, c := recoveryFixture()
	s.actor("self").ItemDefinitionIDs = []string{"item-256"}
	s.actor("ally").Temporary = TemporaryReflectMagic
	rng := &targetRNG{t: t, draws: []float64{0, 0}}
	e := TargetEffect{TargetID: "ally", Category: CategoryMagic, Payload: CallbackPayload{Number: 100}}
	r, err := s.ApplyHealing(&c, e, rng)
	if err != nil || r.TargetID != "self" || r.Amount != 108 || s.actor("self").HP != 208 || s.actor("ally").HP != 154 || c.PartyHealCalls != 1 {
		t.Fatal(r, err, s, c)
	}
	_, err = s.ApplyHealing(&c, e, rng)
	if err != nil || s.actor("ally").HP != 154 || c.PartyHealCalls != 1 {
		t.Fatal(err, s, c)
	}
	rng.done()
	for _, damage := range []int{0, 1} {
		s, c = recoveryFixture()
		c.DamageCalls = damage
		s.actor("self").ItemDefinitionIDs = []string{"item-256"}
		r, err = s.ApplyHealing(&c, TargetEffect{TargetID: "ally", Payload: CallbackPayload{Number: 2000}}, &targetRNG{t: t, draws: []float64{0}})
		want := 100
		if damage == 0 {
			want = 400
		}
		if err != nil || s.actor("self").HP != want {
			t.Fatal(damage, r, err, s)
		}
	}
}

func TestToolRecoverySeparatePath(t *testing.T) {
	for _, hook := range []bool{false, true} {
		s, c := recoveryFixture()
		s.Fields.Default = "sanctuary"
		s.actor("self").Tension = 5
		s.actor("self").ItemDefinitionIDs = []string{"item-146", "item-239"}
		s.actor("ally").Status = StatusVirulentPoison
		s.actor("ally").Temporary = TemporaryDouble
		r, err := s.ApplyToolHealing(c.TargetContext, "ally", 100, hook, &targetRNG{t: t, draws: []float64{0}})
		want := 90
		if hook {
			want = 180
		}
		if err != nil || r.Amount != want || s.actor("ally").HP != 100+want || s.actor("self").Tension != 5 {
			t.Fatal(r, err, s)
		}
	}
	// Cover has a target-only tuple: power becomes0, but still rolls.
	s, c := recoveryFixture()
	s.actor("ally").Temporary, s.actor("fallen").Temporary = TemporaryCover, TemporaryCovering
	r, err := s.ApplyToolHealing(c.TargetContext, "ally", 100, true, &targetRNG{t: t, draws: []float64{0}})
	if err != nil || !r.Returned || r.TargetID != "fallen" || r.Amount != 0 || s.actor("fallen").HP != 0 {
		t.Fatal(r, err, s)
	}
	// Phantom can redirect to fallen AFTER the tool's sole living precheck.
	s, c = recoveryFixture()
	s.actor("ally").Temporary = TemporaryPhantom
	r, err = s.ApplyToolHealing(c.TargetContext, "ally", 100, true, &targetRNG{t: t, draws: []float64{.4, .8, 0}})
	if err != nil || r.TargetID != "fallen" || s.actor("fallen").HP != 90 {
		t.Fatal(r, err, s)
	}
	for _, tmp := range []TemporaryEffect{TemporaryReflectMagic, TemporaryCover, TemporaryReduceAttack} {
		s, c = recoveryFixture()
		s.actor("ally").Temporary = tmp
		before := reloadLifetime(t, s)
		_, err = s.ApplyToolHealing(c.TargetContext, "ally", 100, true, &targetRNG{t: t})
		if !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(s, before) {
			t.Fatal(tmp, err, s)
		}
	}
}

func TestRecoveryRejectsInvalidInputWithoutDraws(t *testing.T) {
	for _, power := range []float64{-1, math.NaN(), math.Inf(1)} {
		s, c := recoveryFixture()
		before := reloadLifetime(t, s)
		_, err := s.ApplyHealing(&c, TargetEffect{TargetID: "ally", Payload: CallbackPayload{Number: power}}, &targetRNG{t: t})
		if !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(s, before) {
			t.Fatal(power, err, s)
		}
	}
}

func TestFullHPConsumptionAndNullification(t *testing.T) {
	for _, null := range []bool{false, true} {
		s, c := recoveryFixture()
		s.actor("self").Tension = 3
		s.actor("ally").HP = 1000
		rng := &targetRNG{t: t}
		if null {
			s.actor("ally").Temporary = TemporaryNullMagic
		} else {
			rng.draws = []float64{0}
		}
		r, err := s.ApplyHealing(&c, TargetEffect{TargetID: "ally", Category: CategoryMagic, Payload: CallbackPayload{Number: 100}}, rng)
		wantTension := 1.0
		if null {
			wantTension = 3
		}
		if err != nil || r.Applied || r.Returned == null || s.actor("self").Tension != wantTension || s.actor("ally").HP != 1000 {
			t.Fatal(r, err, s)
		}
		rng.done()
	}
	s, c := recoveryFixture()
	s.Fields.Default = "sanctuary"
	s.actor("ally").HP = 950
	r, err := s.ApplyToolHealing(c.TargetContext, "ally", 100, false, &targetRNG{t: t, draws: []float64{0}})
	if err != nil || r.Amount != 90 || r.Delta != 50 || s.actor("ally").HP != 1000 {
		t.Fatal(r, err, s)
	}
}

func TestSharedNondamageModifierKinds(t *testing.T) {
	for _, tc := range []struct {
		kind     ModifierKind
		category CallbackCategory
		want     float64
	}{
		{ModifierHPHealing, CategoryMagic, 90}, {ModifierMPHealing, CategoryMagic, 90},
		{ModifierStatUp, CategoryMagic, 90}, {ModifierStatDown, CategoryMagic, 75},
		{ModifierHPHealing, CategoryDance, 112.5}, {ModifierMPHealing, CategoryDance, 75},
		{ModifierStatUp, CategoryDance, 112.5}, {ModifierStatDown, CategoryDance, 75},
	} {
		s, c := recoveryFixture()
		s.Fields.Default = "sanctuary"
		s.Fields.AntiTurns = 2
		c.Item185Activated, c.BowEffect = true, true
		s.actor("self").ItemDefinitionIDs = []string{"item-190"}
		v, _, err := s.ModifyEffect(&c, TargetEffect{TargetID: "ally", Category: tc.category, Payload: CallbackPayload{Number: 100}}, tc.kind, StatAttack, &targetRNG{t: t})
		if err != nil || math.Abs(v-tc.want) > 1e-10 {
			t.Fatal(tc, v, err)
		}
	}
}
