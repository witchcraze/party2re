package battle

import (
	"errors"
	"reflect"
	"testing"
)

func nativeDefeatContext(c TargetContext) *DefeatContext {
	return &DefeatContext{EquipmentResetContext: EquipmentResetContext{TargetContext: c}, Rewards: &DefeatRewards{
		Validate: func(*LifetimeState, TargetContext, string) error { return nil }, Apply: func(*LifetimeState, TargetContext, string) error { return nil },
	}}
}

func TestStatusShieldAndEquipmentPairDrawOrder(t *testing.T) {
	for _, tc := range []struct {
		name    string
		death   bool
		tension float64
		draws   []float64
		applied bool
	}{
		{"death shield no draw", true, 5, nil, false},
		{"status shield after pair", false, 5, []float64{0, .499}, false},
		{"death pair success", true, 4.99, []float64{0, .499}, true},
		{"death pair failure", true, 4.99, []float64{0, .5}, false},
		{"status pair success", false, 4.99, []float64{0, .499}, true},
		{"status pair failure", false, 4.99, []float64{0, .5}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, c := statusFixture()
			c.Defeat = nativeDefeatContext(c.TargetContext)
			enemy := s.actor("enemy")
			raw := 180
			enemy.Gear = &BattleGearStats{WeaponID: "weapon-68", WeaponAttack: &raw, ArmorID: "armor-53", AccessoryID: "item-161"}
			enemy.ItemDefinitionIDs = []string{"item-161"}
			enemy.Tension = tc.tension
			rng := &targetRNG{t: t, draws: tc.draws}
			r, err := s.ApplyStatus(&c, TargetEffect{TargetID: "enemy", Payload: CallbackPayload{Symbol: StatusSleep}}, 100, false, tc.death, rng)
			if err != nil || r.Applied != tc.applied || r.Returned == tc.death {
				t.Fatal(r, err)
			}
			rng.done()
		})
	}
}

func TestStatusProbabilityModifiersAndDen(t *testing.T) {
	for _, tc := range []struct {
		name               string
		death              bool
		field              string
		items              []string
		activated, bow     bool
		areaDamage, damage int
		chance, draw       float64
		applied            bool
	}{
		{"sanctuary excludes status", false, "sanctuary", nil, false, false, 0, 0, 20, .21, false},
		{"sanctuary excludes death", true, "sanctuary", nil, false, false, 0, 0, 20, .21, false},
		{"activated mirror halves", false, "", nil, true, false, 0, 0, 20, .1, false},
		{"death inclusive half", true, "", nil, true, false, 0, 0, 20, .1, true},
		{"bow and plural ring", false, "", []string{"item-144"}, false, true, 1, 0, 20, .389, true},
		{"zero status", false, "", []string{"item-239"}, false, false, 0, 1, 100, 0, false},
		{"zero death", true, "", []string{"item-239"}, false, false, 0, 1, 100, 0, true},
		{"den floor strict", false, "den", nil, false, false, 0, 0, 20.8, .24, false},
		{"den below floor", false, "den", nil, false, false, 0, 0, 20.8, .239, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, c := statusFixture()
			c.Defeat = nativeDefeatContext(c.TargetContext)
			s.Fields.Default = tc.field
			s.actor("self").ItemDefinitionIDs = tc.items
			c.Item185Activated, c.BowEffect, c.AreaDamageCalls, c.DamageCalls = tc.activated, tc.bow, tc.areaDamage, tc.damage
			rng := &targetRNG{t: t, draws: []float64{tc.draw}}
			r, err := s.ApplyStatus(&c, TargetEffect{TargetID: "enemy", Category: CategoryMagic, Payload: CallbackPayload{Symbol: StatusSleep}}, tc.chance, false, tc.death, rng)
			if err != nil || r.Applied != tc.applied {
				t.Fatal(r, err)
			}
			rng.done()
		})
	}
	for _, npc := range []bool{false, true} {
		s, c := statusFixture()
		s.Fields.Default = "den"
		s.actor("enemy").HP = 1000
		s.actor("enemy").MaxHP = 1000
		s.actor("enemy").NPC = npc
		draws := []float64{}
		if !npc {
			draws = []float64{0}
		}
		rng := &targetRNG{t: t, draws: draws}
		r, err := s.ApplyStatus(&c, TargetEffect{TargetID: "enemy", Payload: CallbackPayload{Symbol: StatusSleep}}, 100, false, false, rng)
		if err != nil || r.Applied == npc {
			t.Fatal(r, err)
		}
		rng.done()
	}
}

func TestCureExactAndPoisonVariantsAndReturnedTuple(t *testing.T) {
	for _, cure := range []string{StatusPoison, StatusDeadlyPoison, StatusSleep} {
		for _, status := range []string{StatusPoison, StatusDeadlyPoison, StatusVirulentPoison, StatusSleep} {
			s, c := statusFixture()
			s.actor("ally").Status = status
			r, err := s.ApplyCure(&c, TargetEffect{TargetID: "ally", Payload: CallbackPayload{Symbol: cure}}, &targetRNG{t: t})
			want := cure == status || (cure == StatusDeadlyPoison && (status == StatusPoison || status == StatusVirulentPoison))
			if err != nil || !r.Returned || r.Applied != want || c.CureCalls != 1 || (s.actor("ally").Status == "") != want {
				t.Fatal(cure, status, r, err)
			}
		}
	}
	s, c := statusFixture()
	s.actor("self").Status = StatusConfusion
	s.actor("ally").Status = StatusSleep
	s.actor("enemy").Status = StatusPoison
	// Gem62 resolves once to read the current state, then invokes the cure
	// helper, which performs its own independent confusion/temporary resolution.
	rng := &targetRNG{t: t, draws: []float64{.4, .6}}
	r, err := s.ApplyStatusEntry(&c, nil, "gem_cure_all_abnormal", "ally", rng)
	if err != nil || len(r) != 1 || r[0].TargetID != "enemy" || r[0].Applied || s.actor("ally").Status != StatusSleep || s.actor("enemy").Status != StatusPoison {
		t.Fatal(s, r, err)
	}
	rng.done()
	s, c = statusFixture()
	s.actor("self").Status = StatusConfusion
	s.actor("fallen").Status = StatusSleep
	if r, err := s.ApplyCure(&c, TargetEffect{TargetID: "fallen", Payload: CallbackPayload{Symbol: StatusSleep}}, &targetRNG{t: t, draws: []float64{.2}}); err != nil || r.Returned || s.actor("fallen").Status != StatusSleep {
		t.Fatal(r, err)
	}
}

func TestStatusAreaReflectionChangesLaterTargetResolution(t *testing.T) {
	s, c := statusFixture()
	s.actor("enemy").Temporary = TemporaryReflectMagic
	rng := &targetRNG{t: t, draws: []float64{.1, 0, .1}}
	r, err := s.ApplyStatusEntry(&c, nil, "gem_inflict_confusion", "", rng)
	if err != nil || len(r) != 2 || r[0].TargetID != "self" || r[1].TargetID != "self" || s.actor("self").Status != StatusConfusion || c.StatusCalls != 2 || c.AreaStatusCalls != 1 {
		t.Fatal(s, c, r, err)
	}
	rng.done()
}

func TestAreaDeathAdmitsRewardAfterEarlierAccessoryRevival(t *testing.T) {
	s, c := statusFixture()
	c.Defeat = nativeDefeatContext(c.TargetContext)
	c.Defeat.Rewards = nil
	s.actor("self").Status = StatusConfusion
	s.actor("ally").HP = 0
	for _, id := range c.EnemyIDs {
		a := s.actor(id)
		a.HP = 100
		a.Gear.AccessoryID = "item-193"
		a.ItemDefinitionIDs = []string{"item-193"}
	}
	before := reloadLifetime(t, s)
	if _, err := s.ApplyStatusEntry(&c, nil, "gem_instant_death_sky", "", &targetRNG{t: t}); !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(before, s) || c.DeathCalls != 0 || c.AreaDeathCalls != 0 {
		t.Fatal(s, c, err)
	}
}

func TestAreaDeathProjectsFinalStrikeRevivalRemovingLaterCover(t *testing.T) {
	s, c := statusFixture()
	c.Defeat = nativeDefeatContext(c.TargetContext)
	s.actor("enemy").ItemDefinitionIDs = []string{"item-212"}
	s.actor("enemy").Gear.AccessoryID = "item-212"
	s.actor("dead_enemy").HP = 100
	s.actor("dead_enemy").Temporary = TemporaryCover
	s.actor("ally").HP = 1
	s.actor("ally").Temporary = TemporaryCovering
	s.actor("ally").ItemDefinitionIDs = []string{"item-193"}
	s.actor("ally").Gear.AccessoryID = "item-193"
	c.Defeat.Rewards.Validate = func(_ *LifetimeState, _ TargetContext, id string) error {
		if id == "dead_enemy" {
			return ErrUnsupportedEffect
		}
		return nil
	}
	before := reloadLifetime(t, s)
	if _, err := s.ApplyStatusEntry(&c, nil, "gem_instant_death_sky", "", &targetRNG{t: t}); !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(before, s) || c.DeathCalls != 0 {
		t.Fatal(s, c, err)
	}
}

func TestAbilityCureRunsBeforeIncapacitation(t *testing.T) {
	for _, draws := range [][]float64{{.49}, {.5, .19}} {
		a := LifetimeActor{Participant: Participant{Status: StatusSleep, Abilities: []string{"sleep1", "state1"}}}
		rng := &targetRNG{t: t, draws: draws}
		r, err := a.CheckIncapacitation(IncapacitationContext{}, rng)
		if err != nil || !r.NormalAction || a.Status != "" {
			t.Fatal(a, r, err)
		}
		rng.done()
	}
}

func TestZeroStatusAndImmuneDeathNeedNoSkippedEquipmentOrDefeatContext(t *testing.T) {
	for _, death := range []bool{false, true} {
		s, c := statusFixture()
		s.actor("enemy").Gear = nil
		draws := []float64{0}
		if death {
			s.actor("enemy").HP = 1000
			draws = nil
		}
		rng := &targetRNG{t: t, draws: draws}
		r, err := s.ApplyStatus(&c, TargetEffect{TargetID: "enemy", Payload: CallbackPayload{Symbol: StatusSleep}}, 0, false, death, rng)
		if err != nil || r.Applied {
			t.Fatal(r, err)
		}
		rng.done()
	}
}

func TestAdditionalStatusImmunityReturnsNoTupleButMissStillReturns(t *testing.T) {
	for _, immune := range []bool{false, true} {
		s, c := statusFixture()
		c.AdditionalEffect = true
		if immune {
			s.actor("enemy").HP = 1000
		}
		draws := []float64{.9}
		if immune {
			draws = nil
		}
		rng := &targetRNG{t: t, draws: draws}
		r, err := s.ApplyStatus(&c, TargetEffect{TargetID: "enemy", Payload: CallbackPayload{Symbol: StatusSleep}}, 20, false, false, rng)
		if err != nil || r.Applied || r.Returned == immune {
			t.Fatal(r, err)
		}
		rng.done()
	}
}

func TestDamageSeal11UsesNativeResetAndPreservesTensionAndCursor(t *testing.T) {
	s, c := statusFixture()
	defeat := nativeDefeatContext(c.TargetContext)
	s.actor("self").Abilities = []string{"seal_kuu"}
	s.actor("enemy").Gear.AccessoryID = "item-159"
	s.actor("enemy").ItemDefinitionIDs = []string{"item-159"}
	s.actor("enemy").Tension = 5
	rng := &targetRNG{t: t, draws: []float64{0, 0, .9, .9, .9}}
	damage := DamageContext{EffectContext: c.EffectContext}
	damage.Reset = func(id string) error {
		err := s.ApplyEquipmentReset(&defeat.EquipmentResetContext, id, rng)
		damage.Cursor = defeat.Cursor
		return err
	}
	damage.Defeat = func(id string) error { _, err := s.ApplyDefeat(defeat, id, rng); return err }
	if err := s.ValidateEquipmentReset(&defeat.EquipmentResetContext, "enemy", rng); err != nil {
		t.Fatal(err)
	}
	r, err := s.ApplyDamage(&damage, TargetEffect{TargetID: "enemy", Category: CategoryMagic, Payload: CallbackPayload{Number: 100}, Arguments: []string{"1"}}, rng)
	if err != nil || !r.Applied || s.actor("enemy").HP != 10 || s.actor("enemy").Tension != 5 || s.actor("enemy").Attack != 100 || damage.Cursor != (TargetCursor{Source: "item159", ActorID: "enemy"}) {
		t.Fatal(s, r, damage.Cursor, err)
	}
	rng.done()
}
