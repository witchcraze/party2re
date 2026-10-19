package battle

import (
	"errors"
	"reflect"
	"testing"
)

func statusFixture() (LifetimeState, StatusContext) {
	s, c := revivalFixture()
	for i := range s.Actors {
		s.Actors[i].BaseStats = BattleBaseStats{100, 100, 100}
		s.Actors[i].Gear = &BattleGearStats{}
	}
	return s, StatusContext{EffectContext: EffectContext{TargetContext: c.TargetContext}}
}

func TestStatusDeathProbabilityAndImmunityBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name               string
		death, npc, bypass bool
		hp, maxHP, defense int
		chance, draw       float64
		applied            bool
	}{
		{"status equality", false, false, false, 999, 1000, 999, 0, 0, false},
		{"death equality", true, false, false, 999, 1000, 999, 0, 0, true},
		{"player current HP", false, false, false, 999, 1000, 999, 100, 0, true},
		{"player immune", false, false, false, 1000, 1000, 999, 100, 0, false},
		{"player bypass", false, false, true, 1000, 1000, 1000, 100, 0, true},
		{"NPC maximum HP", false, true, true, 1, 1000, 999, 100, 0, false},
		{"NPC defense", false, true, true, 1, 999, 1000, 100, 0, false},
		{"death current HP", true, true, true, 1, 1000, 999, 100, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, c := statusFixture()
			target := s.actor("enemy")
			target.HP, target.MaxHP, target.BaseStats.Defense, target.NPC = tc.hp, tc.maxHP, tc.defense, tc.npc
			c.Defeat = &DefeatContext{EquipmentResetContext: EquipmentResetContext{TargetContext: c.TargetContext}, Rewards: &DefeatRewards{
				Validate: func(*LifetimeState, TargetContext, string) error { return nil },
				Apply:    func(*LifetimeState, TargetContext, string) error { return nil },
			}}
			draws := []float64{}
			immune := statusImmune(target, tc.bypass, tc.death, "")
			if !immune {
				draws = append(draws, tc.draw)
			}
			rng := &targetRNG{t: t, draws: draws}
			e := TargetEffect{TargetID: "enemy", Payload: CallbackPayload{Symbol: StatusSleep}}
			r, err := s.ApplyStatus(&c, e, tc.chance, tc.bypass, tc.death, rng)
			if err != nil || r.Applied != tc.applied || (tc.applied && !tc.death && target.Status != StatusSleep) || (tc.applied && tc.death && target.HP != 0) {
				t.Fatal(r, target, err)
			}
			rng.done()
		})
	}
}

func TestStatusUnavailableDefeatRejectsBeforeDrawsOrMutation(t *testing.T) {
	s, c := statusFixture()
	s.actor("self").Status = StatusMagicSeal
	before := reloadLifetime(t, s)
	rng := &targetRNG{t: t}
	_, err := s.ApplyStatus(&c, TargetEffect{TargetID: "enemy", Category: CategoryMagic, Payload: CallbackPayload{Symbol: "death"}}, 20, false, true, rng)
	if !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(before, s) {
		t.Fatal(s, err)
	}
}

func TestAbilityCureSpecificThenGeneralAndPoisonVariants(t *testing.T) {
	for _, status := range []string{StatusPoison, StatusDeadlyPoison, StatusVirulentPoison, StatusParalyze, StatusSleep, StatusConfusion, StatusDofuu, StatusPhysicalSeal, StatusMagicSeal, StatusDanceSeal} {
		for _, specific := range []bool{false, true} {
			a := LifetimeActor{Participant: Participant{Status: status, Abilities: []string{"poison1", "paralysis1", "sleep1", "confusion1", "sealed1", "attack_sealed1", "magic_sealed1", "state1"}}}
			draws := []float64{.5, .199}
			if specific {
				draws = []float64{.499}
			}
			if status == StatusDanceSeal {
				draws = []float64{.199}
			}
			rng := &targetRNG{t: t, draws: draws}
			if err := a.ApplyAbilityCure(rng); err != nil || a.Status != "" {
				t.Fatal(a, err)
			}
			rng.done()
		}
	}
}
