package battle

import (
	"errors"
	"reflect"
	"slices"
	"testing"
)

func TestAreaFinalStrikeProjectsExternalFortressBeforeLaterDamage(t *testing.T) {
	for _, projection := range []bool{false, true} {
		s, c := twoStrikeFixture()
		s.actor("self").HP, s.actor("self").MaxHP = 5000, 1
		s.actor("self").Gear.AccessoryID = "item-110"
		s.actor("self").ItemDefinitionIDs = []string{"item-110"}
		ally := *s.actor("self")
		ally.ID, ally.HP, ally.MaxHP = "ally", 1, 100
		ally.Gear, ally.ItemDefinitionIDs = &BattleGearStats{}, nil
		s.Actors = append(s.Actors, ally)
		c.PartyIDs = append(c.PartyIDs, ally.ID)
		c.Defeat.TargetContext, c.Defeat.FieldAtLoad = c.TargetContext, "fortress"
		calls := 0
		c.Defeat.Fortress = &DefeatRewards{
			Validate: func(*LifetimeState, TargetContext, string) error { return nil },
			Apply: func(p *LifetimeState, _ TargetContext, id string) error {
				calls++
				for i := range p.Actors {
					if p.Actors[i].HP > 0 && p.Actors[i].TeamID == p.actor(id).TeamID {
						p.Actors[i].HP = min(p.Actors[i].MaxHP, p.Actors[i].HP+20)
					}
				}
				return nil
			},
		}
		if projection {
			c.Defeat.Fortress.Project = func(p *LifetimeState, _ TargetContext, id string) ([]LifetimeState, error) {
				for i := range p.Actors {
					if p.Actors[i].HP > 0 && p.Actors[i].TeamID == p.actor(id).TeamID {
						p.Actors[i].HP = min(p.Actors[i].HP, p.Actors[i].MaxHP)
					}
				}
				return []LifetimeState{*p}, nil
			}
		}
		levelBefore, sealsBefore := *c.Defeat.RequesterLevel, *c.Defeat.Seals
		partyBefore, enemyBefore := slices.Clone(c.PartyIDs), slices.Clone(c.EnemyIDs)
		before := reloadLifetime(t, s)
		if _, err := s.ApplyStatusEntry(&c, nil, "gem_instant_death_sky", "", &targetRNG{t: t}); !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(before, s) || c.DeathCalls != 0 || c.AreaDeathCalls != 0 || calls != 0 ||
			*c.Defeat.RequesterLevel != levelBefore || *c.Defeat.Seals != sealsBefore ||
			!slices.Equal(c.PartyIDs, partyBefore) || !slices.Equal(c.EnemyIDs, enemyBefore) {
			t.Fatal("fortress changed later damage after partial execution", projection, s, c, calls, err)
		}
	}
}

func TestOrderedDefeatUsesPureOwnerProjectionWithoutExecutingIt(t *testing.T) {
	s, c := twoStrikeFixture()
	s.actor("self").Gear.AccessoryID = "item-110"
	s.actor("self").ItemDefinitionIDs = []string{"item-110"}
	c.Defeat.FieldAtLoad = "fortress"
	applyCalls, projectCalls := 0, 0
	c.Defeat.Fortress = &DefeatRewards{
		Validate: func(*LifetimeState, TargetContext, string) error { return nil },
		Apply:    func(*LifetimeState, TargetContext, string) error { applyCalls++; return nil },
		Project: func(p *LifetimeState, _ TargetContext, _ string) ([]LifetimeState, error) {
			projectCalls++
			return []LifetimeState{*p}, nil
		},
	}
	c.Defeat.Rewards.Project = func(p *LifetimeState, _ TargetContext, _ string) ([]LifetimeState, error) {
		p.actor("self").HP = 1000
		return []LifetimeState{*p}, nil
	}
	c.Defeat.Rewards.Apply = func(p *LifetimeState, _ TargetContext, _ string) error {
		applyCalls++
		p.actor("self").HP = 1000
		return nil
	}
	levelBefore, sealsBefore := *c.Defeat.RequesterLevel, *c.Defeat.Seals
	partyBefore, enemyBefore := slices.Clone(c.PartyIDs), slices.Clone(c.EnemyIDs)
	before := reloadLifetime(t, s)
	if err := s.ValidateStatusEntry(&c, nil, "gem_instant_death_sky", "", &targetRNG{t: t}); err != nil || !reflect.DeepEqual(before, s) || applyCalls != 0 || projectCalls == 0 ||
		*c.Defeat.RequesterLevel != levelBefore || *c.Defeat.Seals != sealsBefore ||
		!slices.Equal(c.PartyIDs, partyBefore) || !slices.Equal(c.EnemyIDs, enemyBefore) ||
		c.DeathCalls != 0 || c.AreaDeathCalls != 0 {
		t.Fatal(s, applyCalls, projectCalls, err)
	}
	rng := &targetRNG{t: t, draws: []float64{0, 0, 0, 0, 0, 0}}
	if _, err := s.ApplyStatusEntry(&c, nil, "gem_instant_death_sky", "", rng); err != nil || applyCalls != 4 || s.actor("self").HP != 1000 {
		t.Fatal(s, applyCalls, err)
	}
	rng.done()
	// Only the first defeat needs a continuation. Last/self/NPC/revival tails
	// retain their native reachability guards and need no skipped projection.
	s, sc := twoStrikeFixture()
	s.actor("self").NPC = true
	sc.Defeat.FieldAtLoad = "fortress"
	levelBefore, sealsBefore = *sc.Defeat.RequesterLevel, *sc.Defeat.Seals
	partyBefore, enemyBefore = slices.Clone(sc.PartyIDs), slices.Clone(sc.EnemyIDs)
	before = reloadLifetime(t, s)
	if err := s.ValidateStatusEntry(&sc, nil, "gem_instant_death_sky", "", &targetRNG{t: t}); err != nil || !reflect.DeepEqual(before, s) ||
		*sc.Defeat.RequesterLevel != levelBefore || *sc.Defeat.Seals != sealsBefore ||
		!slices.Equal(sc.PartyIDs, partyBefore) || !slices.Equal(sc.EnemyIDs, enemyBefore) ||
		sc.DeathCalls != 0 || sc.AreaDeathCalls != 0 {
		t.Fatal(err)
	}
}
