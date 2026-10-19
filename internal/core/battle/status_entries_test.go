package battle

import (
	"reflect"
	"testing"
)

func TestEveryStatusGemConcreteCoefficientCategoryRoleAndCost(t *testing.T) {
	for _, tc := range []struct {
		id            string
		source, cmp   int
		scope, status string
		category      CallbackCategory
		chance        float64
	}{
		{"gem_cure_poison", 55, 6, TargetScopeAllAllies, StatusDeadlyPoison, CategoryUntyped, 0},
		{"gem_cure_paralysis", 56, 6, TargetScopeAllAllies, StatusParalyze, CategoryUntyped, 0},
		{"gem_cure_sleep", 57, 6, TargetScopeAllAllies, StatusSleep, CategoryUntyped, 0},
		{"gem_cure_confusion", 58, 6, TargetScopeAllAllies, StatusConfusion, CategoryUntyped, 0},
		{"gem_cure_paralyze_action", 59, 6, TargetScopeAllAllies, StatusDofuu, CategoryUntyped, 0},
		{"gem_cure_atk_seal", 60, 11, TargetScopeAllAllies, StatusPhysicalSeal, CategoryUntyped, 0},
		{"gem_cure_magic_seal", 61, 11, TargetScopeAllAllies, StatusMagicSeal, CategoryUntyped, 0},
		{"gem_cure_all_abnormal", 62, 38, TargetScopeSingleAlly, StatusSabaku, CategoryUntyped, 0},
		{"gem_instant_death_1", 70, 15, TargetScopeSingleEnemy, "death", CategoryMagic, 20},
		{"gem_instant_death_sky", 71, 26, TargetScopeAllEnemies, "death", CategoryMagic, 20},
		{"gem_inflict_poison", 75, 6, TargetScopeAllEnemies, StatusDeadlyPoison, CategoryBreath, 60},
		{"gem_inflict_paralysis", 76, 6, TargetScopeAllEnemies, StatusParalyze, CategoryBreath, 35},
		{"gem_inflict_sleep", 77, 6, TargetScopeAllEnemies, StatusSleep, CategoryBreath, 35},
		{"gem_inflict_confusion", 78, 6, TargetScopeAllEnemies, StatusConfusion, CategoryMagic, 60},
		{"gem_inflict_seal_act", 79, 6, TargetScopeAllEnemies, StatusDofuu, CategoryPhysical, 50},
		{"gem_inflict_seal_atk", 80, 6, TargetScopeAllEnemies, StatusPhysicalSeal, CategoryPhysical, 60},
		{"gem_inflict_seal_magic", 81, 6, TargetScopeAllEnemies, StatusMagicSeal, CategoryPhysical, 60},
	} {
		t.Run(tc.id, func(t *testing.T) {
			var entry StatusEntry
			for _, e := range StatusEntries() {
				if e.ID == tc.id {
					entry = e
				}
			}
			if entry.SourceID != tc.source || entry.CMP != tc.cmp || entry.TargetScope != tc.scope || entry.MPCost != 0 {
				t.Fatal(entry)
			}
			for _, success := range []bool{false, true} {
				s, c := statusFixture()
				c.Defeat = nativeDefeatContext(c.TargetContext)
				target := "enemy"
				draws := []float64{}
				if tc.source < 70 {
					target = "ally"
					s.actor("ally").Status = tc.status
					s.actor("fallen").Status = tc.status
				} else if success {
					draws = []float64{(tc.chance - .01) / 100}
				} else {
					draws = []float64{(tc.chance + .01) / 100}
				}
				beforeMP, beforeCMP := s.actor("self").MP, s.actor("self").CMP
				rng := &targetRNG{t: t, draws: draws}
				r, err := s.ApplyStatusEntry(&c, nil, tc.id, target, rng)
				if err != nil {
					t.Fatal(err)
				}
				rng.done()
				if tc.source < 70 {
					if s.actor("ally").Status != "" || s.actor("fallen").Status != tc.status {
						t.Fatal(s)
					}
				} else {
					if r[0].Applied != success {
						t.Fatal(r)
					}
					if success && tc.source < 75 && s.actor("enemy").HP != 0 {
						t.Fatal(s)
					}
					if success && tc.source >= 75 && s.actor("enemy").Status != tc.status {
						t.Fatal(s)
					}
					if r[0].Category != tc.category {
						t.Fatal(r)
					}
				}
				if s.actor("self").MP != beforeMP || s.actor("self").CMP != beforeCMP {
					t.Fatal("intrinsic body charged outer cost", s)
				}
			}
		})
	}
	if len(StatusEntries()) != 18 {
		t.Fatal(len(StatusEntries()))
	}
}

func TestStatusConcreteSealGuardsAndBreathIndependence(t *testing.T) {
	for _, id := range []string{"gem_instant_death_1", "gem_instant_death_sky", "gem_inflict_confusion", "gem_inflict_seal_act", "gem_inflict_seal_atk", "gem_inflict_seal_magic", "gem_inflict_poison"} {
		s, c := statusFixture()
		c.Defeat = nativeDefeatContext(c.TargetContext)
		s.actor("self").Status = StatusMagicSeal
		if id == "gem_inflict_seal_act" || id == "gem_inflict_seal_atk" || id == "gem_inflict_seal_magic" || id == "gem_inflict_poison" {
			s.actor("self").Status = StatusPhysicalSeal
		}
		rng := &targetRNG{t: t, draws: []float64{.25}}
		r, err := s.ApplyStatusEntry(&c, nil, id, "enemy", rng)
		if err != nil {
			t.Fatal(id, err)
		}
		rng.done()
		if id == "gem_inflict_poison" {
			if !r[0].Applied {
				t.Fatal(id, r)
			}
		} else {
			for _, v := range r {
				if v.Applied {
					t.Fatal(id, r)
				}
			}
			if c.AreaDeathCalls != 0 || c.AreaStatusCalls != 0 {
				t.Fatal(id, c)
			}
		}
	}
}

func TestNativeStatusDeathReloadedContinuationMatchesUninterrupted(t *testing.T) {
	var reference LifetimeState
	for _, reload := range []bool{false, true} {
		s, c := statusFixture()
		c.Defeat = nativeDefeatContext(c.TargetContext)
		level := 99
		c.Defeat.RequesterLevel = &level
		c.Defeat.Seals = &WeaponSealTable{DragonAttack: 25, PhoenixWeight: -41}
		a := s.actor("enemy")
		raw := 37
		a.Gear = &BattleGearStats{WeaponID: "weapon-69", WeaponAttack: &raw, AccessoryID: "item-193"}
		a.ItemDefinitionIDs = []string{"item-193"}
		a.WeaponSeal = 3
		rng := &targetRNG{t: t, draws: []float64{.1, .1}}
		for phase := range 2 {
			if _, err := s.ApplyStatus(&c, TargetEffect{TargetID: "enemy", Payload: CallbackPayload{Symbol: "death"}}, 100, false, true, rng); err != nil {
				t.Fatal(err)
			}
			if phase == 0 && reload {
				s = reloadLifetime(t, s)
				fresh := nativeDefeatContext(c.TargetContext)
				fresh.RequesterLevel = &level
				fresh.Seals = &WeaponSealTable{DragonAttack: 25, PhoenixWeight: -41}
				c.Defeat = fresh
			}
		}
		rng.done()
		if s.actor("enemy").HP != 0 || len(s.actor("enemy").ConsumedItems) != 1 || s.actor("enemy").Attack != 310 {
			t.Fatal(s)
		}
		if !reload {
			reference = s
		} else if !reflect.DeepEqual(reference, s) {
			t.Fatal("reloaded continuation differs", reference, s)
		}
	}
}
