package battle

import (
	"errors"
	"math"
	"reflect"
	"slices"
	"strconv"
	"testing"
)

func finalStrikeFixture() (LifetimeState, *DefeatContext) {
	s, c := statusFixture()
	s.Actors = []LifetimeActor{*s.actor("self"), *s.actor("enemy")}
	c.PartyIDs, c.EnemyIDs = []string{"self"}, []string{"enemy"}
	s.actor("enemy").Gear.AccessoryID = "item-212"
	s.actor("enemy").ItemDefinitionIDs = []string{"item-212"}
	dc := nativeDefeatContext(c.TargetContext)
	level := 50
	dc.RequesterLevel = &level
	dc.Seals = &WeaponSealTable{DragonAttack: 50, PhoenixWeight: -20}
	return s, dc
}

func TestFinalStrikeNonlethalDoesNotRequireDefeat(t *testing.T) {
	for _, accessory := range []string{"item-212", "item-110"} {
		t.Run(accessory, func(t *testing.T) {
			s, c := finalStrikeFixture()
			s.actor("enemy").Attack = 900
			s.actor("self").Gear.AccessoryID = accessory
			s.actor("self").ItemDefinitionIDs = []string{accessory}
			levelBefore, sealsBefore := *c.RequesterLevel, *c.Seals
			partyBefore, enemyBefore := slices.Clone(c.PartyIDs), slices.Clone(c.EnemyIDs)
			applies := 0
			c.Rewards.Apply = func(*LifetimeState, TargetContext, string) error { applies++; return nil }
			before, context := reloadLifetime(t, s), *c
			if err := s.ValidateDefeat(c, "enemy", &targetRNG{t: t}); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, s) || !reflect.DeepEqual(context, *c) ||
				*c.RequesterLevel != levelBefore || *c.Seals != sealsBefore ||
				!slices.Equal(c.PartyIDs, partyBefore) || !slices.Equal(c.EnemyIDs, enemyBefore) || applies != 0 {
				t.Fatal("validation changed state or context")
			}
			for _, reload := range []bool{false, true} {
				s = reloadLifetime(t, before)
				if reload {
					s = reloadLifetime(t, s)
				}
				rng := &targetRNG{t: t, draws: []float64{0, 0}}
				r, err := s.ApplyDefeat(c, "enemy", rng)
				if err != nil || r.FinalStrike == nil || r.FinalStrike.Amount != 67 || s.actor("self").HP != 33 || s.actor("enemy").Attack != 100 {
					t.Fatal(s, r, err)
				}
				rng.done()
			}
		})
	}
}

func TestFinalStrikeDamageBoundsAndResetContexts(t *testing.T) {
	for _, tc := range []struct {
		name                                            string
		hp, defense, armor, attack, weapon, seal, level int
		shield, awakened, selfAwakened                  bool
		weaponID                                        string
		accept                                          bool
	}{
		{name: "maximum survives", hp: 91, attack: 100, accept: true},
		{name: "maximum lethal", hp: 89, attack: 100},
		{name: "armor", hp: 3, attack: 100, armor: 300, accept: true},
		{name: "minimum fallback lethal", hp: 2, attack: 1},
		{name: "minimum fallback survives", hp: 3, attack: 1, accept: true},
		{name: "one damage without fallback", hp: 2, attack: 2, defense: 1, accept: true},
		{name: "unsupported negative power", hp: 3, attack: -1},
		{name: "shield162", hp: 40, attack: 100, defense: 100, shield: true, accept: true},
		{name: "defense", hp: 40, attack: 100, defense: 100},
		{name: "raw weapon", hp: 150, attack: 100, weapon: 100},
		{name: "seal", hp: 95, attack: 100, seal: 1},
		{name: "seal dragon", hp: 95, attack: 100, seal: 3},
		{name: "seal phoenix", hp: 80, attack: 100, seal: 6, accept: true},
		{name: "weapon69", hp: 100, attack: 100, weaponID: "weapon-69", level: 99},
		{name: "weapon70", hp: 100, attack: 100, weaponID: "weapon-70", level: 99},
		{name: "weapon71 awake", hp: 200, attack: 100, weaponID: "weapon-71", level: 75, awakened: true},
		{name: "weapon71 awake survives", hp: 271, attack: 100, weaponID: "weapon-71", level: 75, awakened: true, accept: true},
		{name: "weapon71 level49", hp: 181, attack: 100, weaponID: "weapon-71", level: 49, awakened: true, accept: true},
		{name: "weapon71 target awakening cleared", hp: 100, attack: 100, weaponID: "weapon-71", level: 75, selfAwakened: true, accept: true},
		{name: "zero power", hp: 1, accept: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, c := finalStrikeFixture()
			a, target := s.actor("enemy"), s.actor("self")
			a.Attack, a.BaseStats.Attack, a.WeaponSeal = 1, tc.attack, tc.seal
			if tc.weaponID != "" || tc.weapon != 0 {
				a.Gear.WeaponID, a.Gear.WeaponAttack = tc.weaponID, &tc.weapon
				if a.Gear.WeaponID == "" {
					a.Gear.WeaponID = "weapon-1"
				}
			}
			c.RequesterLevel, c.Seals = &tc.level, &WeaponSealTable{DragonAttack: 50, PhoenixWeight: -20}
			if tc.awakened {
				target.Extended = ExtendedAwakening
			}
			if tc.selfAwakened {
				a.Extended = ExtendedAwakening
			}
			target.HP, target.Defense, target.Gear.ArmorDefense = tc.hp, tc.defense, tc.armor
			target.Gear.AccessoryID = "item-110"
			target.ItemDefinitionIDs = []string{"item-110"}
			if tc.shield {
				target.ItemDefinitionIDs = append(target.ItemDefinitionIDs, "item-162")
			}
			levelBefore, sealsBefore := *c.RequesterLevel, *c.Seals
			partyBefore, enemyBefore := slices.Clone(c.PartyIDs), slices.Clone(c.EnemyIDs)
			applies := 0
			c.Rewards.Apply = func(*LifetimeState, TargetContext, string) error { applies++; return nil }
			before := reloadLifetime(t, s)
			err := s.ValidateDefeat(c, "enemy", &targetRNG{t: t})
			if *c.Seals != sealsBefore {
				t.Fatal("validation changed shared seal facts")
			}
			if (err == nil) != tc.accept || (!tc.accept && !errors.Is(err, ErrUnsupportedEffect)) ||
				!reflect.DeepEqual(before, s) || *c.RequesterLevel != levelBefore ||
				!slices.Equal(c.PartyIDs, partyBefore) || !slices.Equal(c.EnemyIDs, enemyBefore) || applies != 0 {
				t.Fatal(tc.accept, err, s)
			}
			if tc.accept && tc.attack != 0 {
				draws := []float64{0, math.Nextafter(1, 0)}
				if tc.awakened && tc.weaponID == "weapon-71" {
					draws = append([]float64{math.Nextafter(1, 0)}, draws...)
				}
				if tc.armor != 0 || tc.attack == 1 {
					draws = append(draws, math.Nextafter(1, 0))
				}
				rng := &targetRNG{t: t, draws: draws}
				if _, err := s.ApplyDefeat(c, "enemy", rng); err != nil || target.HP <= 0 {
					t.Fatal(s, err)
				}
				rng.done()
			}
		})
	}
}

func TestAreaDeathFinalStrikeAccumulatesDamage(t *testing.T) {
	for _, hp := range []int{120, 181} {
		t.Run(strconv.Itoa(hp), func(t *testing.T) {
			s, c := finalStrikeFixture()
			a := *s.actor("enemy")
			a.ID = "enemy2"
			s.Actors = append(s.Actors, a)
			c.EnemyIDs = []string{"enemy", "enemy2"}
			s.actor("self").HP = hp
			s.actor("self").Gear.AccessoryID = "item-110"
			s.actor("self").ItemDefinitionIDs = []string{"item-110"}
			levelBefore, sealsBefore := *c.RequesterLevel, *c.Seals
			partyBefore, enemyBefore := slices.Clone(c.PartyIDs), slices.Clone(c.EnemyIDs)
			applies := 0
			c.Rewards.Apply = func(*LifetimeState, TargetContext, string) error { applies++; return nil }
			sc := StatusContext{EffectContext: EffectContext{TargetContext: c.TargetContext}, Defeat: c}
			before := reloadLifetime(t, s)
			err := s.ValidateAreaStatus(&sc, TargetEffect{Payload: CallbackPayload{Symbol: "death"}}, 100, true, true, &targetRNG{t: t})
			if !reflect.DeepEqual(before, s) || sc.DeathCalls != 0 || sc.AreaDeathCalls != 0 ||
				*c.RequesterLevel != levelBefore || *c.Seals != sealsBefore ||
				!slices.Equal(c.PartyIDs, partyBefore) || !slices.Equal(c.EnemyIDs, enemyBefore) ||
				sc.Defeat != c || applies != 0 {
				t.Fatal("impure area validation")
			}
			if hp == 120 {
				if !errors.Is(err, ErrUnsupportedEffect) {
					t.Fatal(err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			rng := &targetRNG{t: t, draws: []float64{0, 0, 0, 0, 0, 0}}
			_, err = s.ApplyAreaStatus(&sc, TargetEffect{Payload: CallbackPayload{Symbol: "death"}}, 100, true, true, rng)
			if err != nil || s.actor("self").HP != 47 || sc.DeathCalls != 2 {
				t.Fatal(s, sc, err)
			}
			rng.done()
		})
	}
}
