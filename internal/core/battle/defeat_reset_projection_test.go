package battle

import (
	"math"
	"reflect"
	"testing"
)

func TestDefeatResetProjectionBoundsIndependentStatDraws(t *testing.T) {
	for _, accessory := range []string{"item-115", "item-145", "item-147", "item-159", "item-187"} {
		t.Run(accessory, func(t *testing.T) {
			s, c := finalStrikeFixture()
			a := s.actor("enemy")
			a.Gear.AccessoryID, a.ItemDefinitionIDs = accessory, []string{accessory}
			a.Jobs.JobID = "job-01"
			a.Temporary = TemporaryRevival
			before := reloadLifetime(t, s)
			states, prevented, err := s.projectDefeatReset(c, "enemy")
			if err != nil || !prevented || !reflect.DeepEqual(before, s) {
				t.Fatal(states, err)
			}
			for _, draw := range []float64{0, math.Nextafter(1, 0)} {
				p := reloadLifetime(t, s)
				draws := []float64{.9}
				switch accessory {
				case "item-115", "item-147":
					draws = append(draws, draw)
				case "item-159":
					draws = append(draws, draw, math.Nextafter(1, 0)-draw, draw)
				}
				rng := &targetRNG{t: t, draws: draws}
				auto := AutomaticRevivalContext{TargetContext: c.TargetContext, Reset: func(id string) error { return p.ApplyEquipmentReset(&c.EquipmentResetContext, id, rng) }}
				if _, err := p.ApplyAutomaticRevival(&auto, "enemy", rng); err != nil {
					t.Fatal(err)
				}
				rng.done()
				actual := p.actor("enemy")
				covered := false
				for _, state := range states {
					bound := state.actor("enemy")
					if bound.Temporary == actual.Temporary && bound.Attack >= actual.Attack && bound.Defense <= actual.Defense && bound.HP > 0 && bound.HP <= actual.HP {
						covered = true
					}
				}
				if !covered {
					t.Fatal("runtime outcome outside projection", accessory, actual, states)
				}
			}
		})
	}
}

func TestDefeatResetProjectionKeepsAllManaAndZeroHPRevivalOutcomes(t *testing.T) {
	for _, maxHP := range []int{6, 101} {
		s, c := finalStrikeFixture()
		a := s.actor("enemy")
		a.MaxHP, a.MP, a.MaxMP, a.Temporary = maxHP, 0, 100, TemporaryRevival
		a.Gear.AccessoryID, a.ItemDefinitionIDs = "item-111", []string{"item-111"}
		states, _, err := s.projectDefeatReset(c, "enemy")
		if err != nil {
			t.Fatal(err)
		}
		for _, prefix := range []float64{0, .9} {
			for i := 0; i < 9; i++ {
				p := reloadLifetime(t, s)
				draws := []float64{prefix}
				if maxHP != 6 || prefix != 0 {
					draws = append(draws, (float64(i)+.5)/9)
				}
				rng := &targetRNG{t: t, draws: draws}
				auto := AutomaticRevivalContext{TargetContext: c.TargetContext, Reset: func(id string) error { return p.ApplyEquipmentReset(&c.EquipmentResetContext, id, rng) }}
				if _, err := p.ApplyAutomaticRevival(&auto, "enemy", rng); err != nil {
					t.Fatal(err)
				}
				rng.done()
				actual, covered := p.actor("enemy"), false
				for _, state := range states {
					bound := state.actor("enemy")
					if bound.MP == actual.MP && (bound.HP > 0) == (actual.HP > 0) && bound.HP <= actual.HP {
						covered = true
					}
				}
				if !covered {
					t.Fatal("missing mana/living continuation", maxHP, actual, states)
				}
			}
		}
	}
}
