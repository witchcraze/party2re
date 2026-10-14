package battle

import (
	"errors"
	"reflect"
	"testing"
)

func admitDamage(s *LifetimeState, c *DamageContext) {
	c.Defeat = func(id string) error { s.actor(id).HP = 0; return nil }
	c.Reset = func(id string) error {
		a := s.actor(id)
		a.ResetEffects(LifetimeReset{Attack: a.BaseStats.Attack, Defense: a.BaseStats.Defense, Agility: a.BaseStats.Agility})
		return nil
	}
}

func TestDamageModifierBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name               string
		power              float64
		category           CallbackCategory
		items, targetItems []string
		tmp                TemporaryEffect
		status, field      string
		extended           ExtendedEffect
		direct, normal     bool
		calls, area        int
		want               float64
		draws              []float64
	}{
		{"magic below", 299, CategoryMagic, []string{"item-143"}, nil, "", "", "", "", true, false, 1, 0, 598, nil},
		{"magic at", 300, CategoryMagic, []string{"item-143"}, nil, "", "", "", "", true, false, 1, 0, 300, nil},
		{"doubled below", 599, CategoryMagic, []string{"item-143"}, nil, TemporaryDouble, "", "", "", true, false, 1, 0, 1198, nil},
		{"doubled at", 600, CategoryMagic, []string{"item-143"}, nil, TemporaryDouble, "", "", "", true, false, 1, 0, 600, nil},
		{"breath truncates", 101, CategoryBreath, []string{"item-189"}, nil, "", "", "", "", true, false, 1, 0, 151, nil},
		{"target temporary", 101, CategoryMagic, []string{"item-192"}, nil, TemporaryDouble, "", "", "", true, false, 1, 0, 166, []float64{.5}},
		{"category untyped", 100, CategoryUntyped, []string{"item-143"}, nil, "", "", "", "", true, false, 1, 0, 100, nil},
		{"count2", 100, CategoryPhysical, []string{"item-239"}, nil, "", "", "", "", false, false, 2, 0, 120, nil},
		{"count3", 100, CategoryPhysical, []string{"item-239"}, nil, "", "", "", "", false, false, 3, 0, 140, nil},
		{"count4", 100, CategoryPhysical, []string{"item-239"}, nil, "", "", "", "", false, false, 4, 0, 160, nil},
		{"count area", 100, CategoryPhysical, []string{"item-239"}, nil, "", "", "", "", false, false, 4, 1, 100, nil},
		{"normal excludes", 100, CategoryPhysical, nil, []string{"item-200"}, "", "", "", ExtendedWarGod, false, true, 1, 0, 100, nil},
		{"direct excludes", 100, CategoryPhysical, nil, []string{"item-200"}, "", "", "", ExtendedWarGod, true, false, 1, 0, 100, nil},
		{"both modifiers", 100, CategoryPhysical, nil, []string{"item-200"}, "", "", "", ExtendedWarGod, false, false, 1, 0, 156, nil},
		{"dark normal", 100, CategoryPhysical, nil, nil, "", "", "darkness", "", false, true, 1, 0, 100, nil},
		{"dark skill", 100, CategoryPhysical, nil, nil, "", "", "darkness", "", false, false, 1, 0, 130, nil},
		{"defense items", 100, CategoryMagic, nil, []string{"item-161"}, TemporaryDefense, "", "", "", true, false, 1, 0, 60, nil},
		{"kinju", 100, CategoryMagic, nil, nil, "", StatusKinju, "", "", true, false, 1, 0, 125, nil},
		{"dance", 100, CategoryDance, []string{"item-190"}, nil, "", "", "", "", true, false, 1, 0, 150, nil},
		{"diamond below", 299, CategoryMagic, []string{"item-144"}, nil, "", "", "", "", true, false, 1, 1, 388.7, nil},
		{"diamond at", 300, CategoryMagic, []string{"item-144"}, nil, "", "", "", "", true, false, 1, 1, 300, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, c := damageFixture()
			a, target := s.actor("self"), s.actor("enemy")
			a.ItemDefinitionIDs, a.Extended = tc.items, tc.extended
			target.ItemDefinitionIDs, target.Temporary, target.Status = tc.targetItems, tc.tmp, tc.status
			s.Fields.Default = tc.field
			c.DamageCalls, c.AreaDamageCalls = tc.calls, tc.area
			rng := &targetRNG{t: t, draws: tc.draws}
			v := s.modifyDamage(&c, target, tc.power, tc.category, tc.direct, tc.normal, rng.Float64)
			if v-tc.want > 1e-9 || tc.want-v > 1e-9 {
				t.Fatal(v, tc.want)
			}
			rng.done()
		})
	}
}

func TestDamageHitStrictBoundaryAndCounters(t *testing.T) {
	for _, tc := range []struct {
		name    string
		draws   []float64
		status  string
		weapon  bool
		want    int
		tension float64
	}{
		{"equal95", []float64{.95, .5, 0}, "", false, 180, 1},
		{"above95", []float64{.95001}, "", false, 0, 1},
		{"kinju draws before guard", []float64{.99, 0}, StatusKinju, false, 225, 1},
		{"weapon draws before guard", []float64{.99, .5, 0}, "", true, 180, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, c := damageFixture()
			admitDamage(&s, &c)
			s.actor("enemy").HP = 1000
			s.actor("enemy").Status = tc.status
			if tc.weapon {
				s.actor("self").ItemDefinitionIDs = []string{"weapon-63"}
			}
			rng := &targetRNG{t: t, draws: tc.draws}
			r, err := s.ApplyDamage(&c, TargetEffect{TargetID: "enemy", Category: CategoryPhysical, Payload: CallbackPayload{Number: 200}, Arguments: []string{"1"}}, rng)
			if err != nil || r.Amount != tc.want || c.DamageCalls != 1 {
				t.Fatal(r, err, c)
			}
			rng.done()
		})
	}
	// A block counts, preserves tension, and never needs the skipped defeat hook.
	s, c := damageFixture()
	s.actor("self").Tension = 3
	s.actor("enemy").Temporary = TemporaryOneHitDefense
	r, err := s.ApplyDamage(&c, TargetEffect{TargetID: "enemy", Payload: CallbackPayload{Number: 100}}, &targetRNG{t: t})
	if err != nil || !r.Returned || r.Applied || c.DamageCalls != 1 || s.actor("self").Tension != 3 || s.actor("enemy").Temporary != "" {
		t.Fatal(r, err, c)
	}
	// A null payload does not even count and does not require any damage context.
	s, c = damageFixture()
	s.actor("enemy").Temporary = TemporaryNullMagic
	s.actor("self").Tension = 3
	r, err = s.ApplyDamage(&c, TargetEffect{TargetID: "enemy", Category: CategoryMagic, Payload: CallbackPayload{Number: 100}}, &targetRNG{t: t})
	if err != nil || r.Returned || c.DamageCalls != 0 || s.actor("self").Tension != 3 {
		t.Fatal(r, err, c)
	}
}

func TestDamageShieldBaseDefenseAndToolContext(t *testing.T) {
	for _, base := range []int{999, 1000} {
		for _, tool := range []bool{false, true} {
			for _, category := range []CallbackCategory{CategoryPhysical, CategoryMagic} {
				s, c := damageFixture()
				admitDamage(&s, &c)
				c.GuaranteedHit = true
				target := s.actor("enemy")
				target.HP = 1000
				target.Defense = 100
				target.BaseStats.Defense = base
				target.ItemDefinitionIDs = []string{"item-162"}
				rng := &targetRNG{t: t, draws: []float64{0}}
				var r EffectApplication
				var err error
				if tool {
					r, err = s.ApplyToolDamage(&c, "enemy", 200, true, false, rng)
				} else {
					r, err = s.ApplyDamage(&c, TargetEffect{TargetID: "enemy", Category: category, Payload: CallbackPayload{Number: 200}, Arguments: []string{"1"}}, rng)
				}
				want := 180
				if base == 1000 || (!tool && category == CategoryPhysical) {
					want = 49
				}
				if err != nil || r.Amount != want {
					t.Fatal(base, tool, category, r, err)
				}
				rng.done()
			}
		}
	}
	for _, source := range []string{"", "job77", "physical", "magic", "unknown"} {
		s, c := damageFixture()
		admitDamage(&s, &c)
		c.ToolCategory = ToolCategoryContext{Source: source}
		if source == "job77" {
			c.ToolCategory.SavedAttack = 150
		}
		rng := &targetRNG{t: t}
		if source == "" || source == "job77" {
			rng.draws = []float64{0}
		}
		before := reloadLifetime(t, s)
		_, err := s.ApplyToolDamage(&c, "enemy", 10, true, true, rng)
		if source != "" && source != "job77" && (!errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(s, before)) {
			t.Fatal(source, err)
		}
		rng.done()
	}
}

func TestToolNoEffectDoesNotRequireSkippedContext(t *testing.T) {
	for _, blocked := range []bool{false, true} {
		s, c := damageFixture()
		s.actor("enemy").Gear = nil
		power := 0.0
		if blocked {
			s.actor("enemy").Temporary = TemporaryOneHitDefense
			power = 100
		}
		r, err := s.ApplyToolDamage(&c, "enemy", power, true, false, &targetRNG{t: t})
		if err != nil || r.Applied || r.Returned != blocked || s.actor("enemy").HP != 100 || s.actor("enemy").Temporary != "" {
			t.Fatal(blocked, r, err, s)
		}
	}
}

func TestDamageModifierZeroCountsWithoutTails(t *testing.T) {
	s, c := damageFixture()
	s.actor("self").ItemDefinitionIDs = []string{"item-143"}
	s.actor("self").Tension = 3
	s.actor("self").Extended = ExtendedPurification
	s.actor("self").Abilities = []string{"seal_kuu"}
	s.Fields.Default = "darkness"
	r, err := s.ApplyDamage(&c, TargetEffect{TargetID: "enemy", Category: CategoryMagic, Payload: CallbackPayload{Number: .4}, Arguments: []string{"1"}}, &targetRNG{t: t})
	if err != nil || r.Returned || r.Applied || c.DamageCalls != 1 || c.End.DarknessDebit != nil || s.actor("self").Tension != 3 || s.actor("enemy").HP != 100 {
		t.Fatal(r, err, c, s)
	}
}

func TestDamageTensionInterceptionAndResetOrder(t *testing.T) {
	for _, shield := range []bool{false, true} {
		s, c := damageFixture()
		admitDamage(&s, &c)
		c.GuaranteedHit = true
		s.actor("self").ItemDefinitionIDs = []string{"item-149"}
		s.actor("self").Tension = 3
		target := s.actor("enemy")
		target.HP = 1000
		target.Extended = ExtendedInterception
		target.ExtendedTurns = 1
		rng := &targetRNG{t: t, draws: []float64{0, 0}}
		if shield {
			target.ItemDefinitionIDs = []string{"item-162"}
			rng.draws = rng.draws[:1]
		}
		r, err := s.ApplyDamage(&c, TargetEffect{TargetID: "enemy", Category: CategoryPhysical, Payload: CallbackPayload{Number: 200}, Arguments: []string{"1"}}, rng)
		if err != nil || !r.Applied || s.actor("self").Tension != 3 || (!shield && target.Extended != "") || (shield && target.ExtendedTurns != 1) {
			t.Fatal(shield, r, err, s)
		}
		if !shield && s.actor("self").HP != 69 {
			t.Fatal(s.actor("self"))
		}
		rng.done()
	}
	s, c := damageFixture()
	admitDamage(&s, &c)
	s.actor("self").Extended = ExtendedPurification
	s.actor("self").Abilities = []string{"seal_kuu"}
	s.actor("enemy").HP = 1000
	s.actor("enemy").Status = StatusPoison
	s.actor("enemy").Tension = 3
	resets := 0
	c.Reset = func(id string) error { resets++; s.actor(id).Status = ""; s.actor(id).Tension = 1; return nil }
	rng := &targetRNG{t: t, draws: []float64{0, 0}}
	_, err := s.ApplyDamage(&c, TargetEffect{TargetID: "enemy", Category: CategoryMagic, Payload: CallbackPayload{Number: 50}, Arguments: []string{"1"}}, rng)
	if err != nil || resets != 2 || s.actor("enemy").Status != "" || s.actor("enemy").Tension != 3 {
		t.Fatal(resets, err, s)
	}
	rng.done()
}
