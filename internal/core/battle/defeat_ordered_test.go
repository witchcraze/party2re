package battle

import (
	"errors"
	"reflect"
	"testing"
)

func twoStrikeFixture() (LifetimeState, StatusContext) {
	s, c := finalStrikeFixture()
	a := *s.actor("enemy")
	a.ID = "enemy2"
	s.Actors = append(s.Actors, a)
	c.EnemyIDs = append(c.EnemyIDs, a.ID)
	return s, StatusContext{EffectContext: EffectContext{TargetContext: c.TargetContext}, Defeat: c}
}

func TestFinalStrikeRegisteredSingleAndAreaAdmission(t *testing.T) {
	for _, entry := range []string{"gem_instant_death_1", "gem_instant_death_sky"} {
		s, dc := finalStrikeFixture()
		s.actor("self").Gear.AccessoryID = "item-212"
		s.actor("self").ItemDefinitionIDs = []string{"item-212"}
		c := StatusContext{EffectContext: EffectContext{TargetContext: dc.TargetContext}, Defeat: dc}
		before := reloadLifetime(t, s)
		if err := s.ValidateStatusEntry(&c, nil, entry, "enemy", &targetRNG{t: t}); err != nil || !reflect.DeepEqual(before, s) {
			t.Fatal(entry, s, err)
		}
		rng := &targetRNG{t: t, draws: []float64{0, 0, 0}}
		r, err := s.ApplyStatusEntry(&c, nil, entry, "enemy", rng)
		if err != nil || len(r) != 1 || !r[0].Applied || s.actor("self").HP != 33 {
			t.Fatal(entry, s, r, err)
		}
		rng.done()
	}
}

func TestFinalStrikeOneHitDefenseIsConsumedInContinuation(t *testing.T) {
	s, c := twoStrikeFixture()
	s.actor("self").HP = 1
	s.actor("self").Temporary = TemporaryOneHitDefense
	s.actor("self").Gear.AccessoryID = "item-110"
	s.actor("self").ItemDefinitionIDs = []string{"item-110"}
	before := reloadLifetime(t, s)
	if _, err := s.ApplyStatusEntry(&c, nil, "gem_instant_death_sky", "", &targetRNG{t: t}); !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(before, s) || c.DeathCalls != 0 || c.AreaDeathCalls != 0 {
		t.Fatal(s, c, err)
	}
	// A single final strike reaches neither damage nor target defeat validation.
	s.Actors = s.Actors[:2]
	c.EnemyIDs = c.EnemyIDs[:1]
	c.Defeat.TargetContext = c.TargetContext
	c.Defeat.ToolCategory.Source = "unsupported"
	rng := &targetRNG{t: t, draws: []float64{0, 0}}
	if _, err := s.ApplyStatusEntry(&c, nil, "gem_instant_death_sky", "", rng); err != nil || s.actor("self").HP != 1 || s.actor("self").Temporary != "" {
		t.Fatal(s, err)
	}
	rng.done()
}

func TestAreaFinalStrikeCandidatesDoNotDamageEachOther(t *testing.T) {
	s, dc := finalStrikeFixture()
	ally := *s.actor("self")
	ally.ID = "ally"
	ally.Gear = &BattleGearStats{AccessoryID: "item-212"}
	ally.ItemDefinitionIDs = []string{"item-212"}
	s.actor("self").Gear.AccessoryID = "item-212"
	s.actor("self").ItemDefinitionIDs = []string{"item-212"}
	s.Actors = append(s.Actors, ally)
	dc.PartyIDs = append(dc.PartyIDs, ally.ID)
	c := StatusContext{EffectContext: EffectContext{TargetContext: dc.TargetContext}, Defeat: dc}
	rng := &targetRNG{t: t, draws: []float64{0, 0, 0}}
	if _, err := s.ApplyStatusEntry(&c, nil, "gem_instant_death_sky", "", rng); err != nil || s.actor("self").HP != 33 || s.actor("ally").HP != 100 {
		t.Fatal(s, err)
	}
	rng.done()
}

func TestAreaFinalStrikeUsesDefenseAfterRevival(t *testing.T) {
	for _, defense := range []int{100, 1000} {
		s, c := twoStrikeFixture()
		ally := *s.actor("self")
		ally.ID, ally.HP, ally.Defense, ally.Tension = "ally", 1, 9000, 8
		ally.BaseStats.Defense = defense
		ally.Gear = &BattleGearStats{AccessoryID: "item-161"}
		ally.ItemDefinitionIDs = []string{"item-161"}
		s.Actors = append(s.Actors, ally)
		s.actor("self").HP = 5000
		c.PartyIDs = append(c.PartyIDs, ally.ID)
		c.Defeat.TargetContext = c.TargetContext
		c.Defeat.Rewards.Validate = func(_ *LifetimeState, _ TargetContext, id string) error {
			if id == "ally" {
				return ErrUnsupportedEffect
			}
			return nil
		}
		before := reloadLifetime(t, s)
		err := s.ValidateStatusEntry(&c, nil, "gem_instant_death_sky", "", &targetRNG{t: t})
		if !reflect.DeepEqual(before, s) {
			t.Fatal("impure revival admission")
		}
		if defense == 100 {
			if !errors.Is(err, ErrUnsupportedEffect) {
				t.Fatal(err)
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		rng := &targetRNG{t: t, draws: []float64{0, .9, 0, 0, 0, .9, 0, 0}}
		if _, err := s.ApplyStatusEntry(&c, nil, "gem_instant_death_sky", "", rng); err != nil || s.actor("ally").HP != 49 || s.actor("ally").Defense != defense {
			t.Fatal(s, err)
		}
		rng.done()
	}
}

func TestFinalStrikeZeroPowerNeedsNoToolCategoryOrTargetDefeat(t *testing.T) {
	s, c := finalStrikeFixture()
	s.actor("enemy").BaseStats.Attack = 0
	s.actor("self").HP = 1
	s.actor("self").Gear.AccessoryID = "item-110"
	c.ToolCategory.Source = "unsupported"
	rng := &targetRNG{t: t, draws: []float64{0}}
	if _, err := s.ApplyDefeat(c, "enemy", rng); err != nil || s.actor("self").HP != 1 {
		t.Fatal(s, err)
	}
	rng.done()
}

func TestAreaDeathAdmissionRetainsTargetHelperManaBeforeRevival(t *testing.T) {
	s, dc := finalStrikeFixture()
	a := s.actor("enemy")
	a.Abilities, a.MP, a.Temporary = []string{"pharaoh"}, 0, TemporaryAbsorbMagic
	s.actor("self").Gear.AccessoryID = "item-110"
	c := StatusContext{EffectContext: EffectContext{TargetContext: dc.TargetContext}, Defeat: dc}
	before := reloadLifetime(t, s)
	if err := s.ValidateStatusEntry(&c, nil, "gem_instant_death_sky", "", &targetRNG{t: t}); err != nil || !reflect.DeepEqual(before, s) {
		t.Fatal(s, err)
	}
	rng := &targetRNG{t: t, draws: []float64{.5, 0}}
	if _, err := s.ApplyStatusEntry(&c, nil, "gem_instant_death_sky", "", rng); err != nil || a.HP != 101 || a.MP != 20 || s.actor("self").HP != 100 {
		t.Fatal(s, err)
	}
	rng.done()
}

func TestFinalStrikeReloadBetweenDefeatsMatchesContinuation(t *testing.T) {
	var reference LifetimeState
	for _, reload := range []bool{false, true} {
		s, c := twoStrikeFixture()
		s.actor("self").HP = 181
		s.actor("self").Gear.AccessoryID = "item-110"
		s.actor("self").ItemDefinitionIDs = []string{"item-110"}
		rng := &targetRNG{t: t, draws: []float64{0, 0, 0, 0}}
		for i, id := range []string{"enemy", "enemy2"} {
			if i == 1 && reload {
				s = reloadLifetime(t, s)
				c.Defeat = nativeDefeatContext(c.TargetContext)
			}
			if _, err := s.ApplyDefeat(c.Defeat, id, rng); err != nil {
				t.Fatal(s, err)
			}
		}
		rng.done()
		if s.actor("self").HP != 47 {
			t.Fatal(s)
		}
		if reload && !reflect.DeepEqual(reference, s) {
			t.Fatal("reload changed continuation", reference, s)
		}
		reference = s
	}
}
