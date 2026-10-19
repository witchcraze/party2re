package battle

import (
	"errors"
	"reflect"
	"testing"

	"github.com/witchcraze/party2re/internal/core/item"
)

func resetFixture() (LifetimeState, EquipmentResetContext) {
	s, c := statusFixture()
	return s, EquipmentResetContext{TargetContext: c.TargetContext}
}

func TestEquipmentResetAll117SourceAccessoriesLivingAndFallen(t *testing.T) {
	catalog, err := item.InitialCatalog()
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, definition := range catalog.Definitions() {
		if definition.UsageCategory != item.UsageCategoryCombatPassive {
			continue
		}
		count++
		for _, hp := range []int{0, 50} {
			t.Run(definition.ID+"/"+map[bool]string{true: "living", false: "fallen"}[hp > 0], func(t *testing.T) {
				s, c := resetFixture()
				a := s.actor("ally")
				raw := 30
				a.HP, a.MP = hp, 50
				a.Gear = &BattleGearStats{WeaponID: "weapon-13", WeaponAttack: &raw, WeaponWeight: 20, ArmorDefense: 50, ArmorWeight: 10, AccessoryID: definition.ID}
				a.Jobs.JobID = "job-01"
				a.Tension, a.Hit, a.Status, a.Temporary, a.Extended, a.ExtendedTurns = 8, 1, StatusSleep, TemporaryDefense, ExtendedAwakening, 8
				before := reloadLifetime(t, s)
				draws := []float64{}
				switch definition.ID {
				case "item-115", "item-147", "item-251", "item-252", "item-253", "item-254", "item-255":
					draws = []float64{.9}
				case "item-159":
					draws = []float64{.9, .9, .9}
				case "item-111", "item-112", "item-247":
					if hp > 0 {
						draws = []float64{.9}
					}
				}
				rng := &targetRNG{t: t, draws: draws}
				err := s.ApplyEquipmentReset(&c, a.ID, rng)
				if definition.ID == "item-110" {
					if !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(before, s) {
						t.Fatal(s, err)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				rng.done()
				want := BattleBaseStats{130, 150, 70}
				fixed := map[string]BattleBaseStats{
					"item-116": {160, 180, 70}, "item-117": {130, 180, 70}, "item-118": {130, 165, 70}, "item-119": {130, 200, 70},
					"item-120": {130, 150, 100}, "item-121": {130, 150, 130}, "item-122": {150, 150, 70}, "item-123": {170, 150, 70}, "item-124": {160, 150, 100},
					"item-187": {136, 227, 91}, "item-240": {160, 130, 50}, "item-241": {110, 180, 50}, "item-242": {110, 130, 100},
				}
				if w, ok := fixed[definition.ID]; ok {
					want = w
				}
				if definition.ID == "item-145" {
					want.Defense = 135
					if hp == 0 {
						want.Attack = 156
					}
				}
				if definition.ID == "item-162" && hp > 0 {
					want.Attack, want.Agility = 104, 56
				}
				if definition.ID == "item-209" && hp > 0 {
					want.Defense = 180
				}
				wantStatus := ""
				if definition.ID == "item-116" {
					wantStatus = StatusDofuu
				}
				if definition.ID == "item-143" {
					wantStatus = StatusPhysicalSeal
				}
				wantTmp := TemporaryEffect("")
				if definition.ID == "item-163" && hp > 0 {
					wantTmp = TemporaryReflectMagic
				}
				wantHP, wantMP, wantHit := hp, 50, 95
				if definition.ID == "item-111" && hp > 0 {
					wantMP = 63
				}
				if definition.ID == "item-112" && hp > 0 {
					wantHP = 67
				}
				if definition.ID == "item-149" {
					wantHit = 98
				}
				if definition.ID == "item-197" {
					wantHit = 99
				}
				if (BattleBaseStats{a.Attack, a.Defense, a.Agility}) != want || a.Status != wantStatus || a.Temporary != wantTmp || a.HP != wantHP || a.MP != wantMP || a.Hit != wantHit || a.Tension != 1 {
					t.Fatal(a, want, wantStatus, wantTmp)
				}
				if definition.ID == "item-247" && hp > 0 {
					if a.Extended != ExtendedIncense || a.ExtendedTurns != 4 {
						t.Fatal(a)
					}
				} else if a.Extended != "" || a.ExtendedTurns != 0 {
					t.Fatal(a)
				}
			})
		}
	}
	if count != 117 {
		t.Fatal(count)
	}
}

func TestResetAccessoryInclusiveDrawsAndUnflooredAgility(t *testing.T) {
	for _, tc := range []struct {
		id                       string
		draws                    []float64
		attack, defense, agility int
		tension                  float64
		tmp                      TemporaryEffect
	}{
		{"item-115", []float64{1.0 / 3}, 100, 100, 100, 1.7, TemporaryDouble},
		{"item-147", []float64{.25}, 110, 100, 100, 1, ""},
		{"item-159", []float64{1.0 / 3, .334, 0}, 110, 100, 110, 1, ""},
	} {
		s, c := resetFixture()
		a := s.actor("ally")
		a.Gear.AccessoryID = tc.id
		rng := &targetRNG{t: t, draws: tc.draws}
		if err := s.ApplyEquipmentReset(&c, a.ID, rng); err != nil || a.Attack != tc.attack || a.Defense != tc.defense || a.Agility != tc.agility || a.Tension != tc.tension || a.Temporary != tc.tmp {
			t.Fatal(a, err)
		}
		rng.done()
		if tc.id == "item-159" && c.Cursor != (TargetCursor{Source: "item159", ActorID: a.ID}) {
			t.Fatal(c.Cursor)
		}
	}
	s, c := resetFixture()
	a := s.actor("ally")
	a.BaseStats.Agility = 0
	a.Gear.WeaponWeight = 10
	a.Gear.AccessoryID = "item-120"
	if err := s.ApplyEquipmentReset(&c, a.ID, &targetRNG{t: t}); err != nil || a.Agility != 20 {
		t.Fatal(a, err)
	}
}

func TestResetWeaponsRequesterActorTargetBoundaries(t *testing.T) {
	for _, level := range []int{49, 50, 74, 75, 99, 100, 150} {
		for _, weapon := range []string{"weapon-69", "weapon-70", "weapon-71"} {
			for _, accessory := range []string{"", "item-158", "item-240", "item-241", "item-242", "target-awakening", "actor-awakening"} {
				s, c := resetFixture()
				c.RequesterLevel = &level
				a := s.actor("fallen")
				raw := 31
				a.Gear = &BattleGearStats{WeaponID: weapon, WeaponAttack: &raw}
				if accessory == "target-awakening" {
					a.Extended = ExtendedAwakening
				} else if accessory == "actor-awakening" {
					s.actor("self").Extended = ExtendedAwakening
				} else {
					a.Gear.AccessoryID = accessory
				}
				awakened := accessory != "" && accessory != "target-awakening"
				draws := []float64{}
				if weapon == "weapon-71" && awakened {
					draws = []float64{.999}
				}
				rng := &targetRNG{t: t, draws: draws}
				want := 131
				if accessory == "item-240" {
					want += 30
				}
				if accessory == "item-241" || accessory == "item-242" {
					want -= 20
				}
				switch weapon {
				case "weapon-69":
					want = int(float64(want) + float64(min(99, level))*1.5)
				case "weapon-70":
					want += min(99, level) * 2
				case "weapon-71":
					component := want - 100
					if awakened {
						component = 199
					}
					factor := 1.0
					if level < 50 {
						factor = .5
					} else if level < 75 {
						factor = .7
					}
					want = int(100 + float64(component)*factor)
				}
				if err := s.ApplyEquipmentReset(&c, a.ID, rng); err != nil || a.Attack != want {
					t.Fatal(level, weapon, accessory, a, err)
				}
				rng.done()
			}
		}
	}
}

func TestJob35DispelPreservesEveryOrderedTensionAndRejectsMissingLaterContext(t *testing.T) {
	s, c := resetFixture()
	status := StatusContext{EffectContext: EffectContext{TargetContext: c.TargetContext}}
	for i := range s.Actors {
		a := &s.Actors[i]
		a.Tension = float64(i) + 1.7
		a.Temporary = TemporaryDefense
		a.Status = StatusSleep
		a.Extended = ExtendedAwakening
		a.Gear.AccessoryID = "item-116"
		a.WeaponSeal = 11
	}
	before := reloadLifetime(t, s)
	if _, err := s.ApplyStatusEntry(&status, &c, "job-35-entry-1", "", &targetRNG{t: t}); err != nil {
		t.Fatal(err)
	}
	for i, a := range s.Actors {
		if a.Tension != before.Actors[i].Tension || a.Status != StatusDofuu || a.Temporary != "" || a.Extended != "" || a.Attack != 130 || a.HP != before.Actors[i].HP {
			t.Fatal(a)
		}
	}
	s = before
	s.actor("self").Gear.AccessoryID = "item-111"
	raw := 30
	s.Actors[len(s.Actors)-1].Gear = &BattleGearStats{WeaponID: "weapon-69", WeaponAttack: &raw}
	before = reloadLifetime(t, s)
	if _, err := s.ApplyStatusEntry(&status, &c, "job-35-entry-1", "", &targetRNG{t: t}); !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(before, s) {
		t.Fatal(s, err)
	}
}

func TestResetSealsUseSharedLoadFactsAcrossReload(t *testing.T) {
	for seal := 1; seal <= 6; seal++ {
		s, c := resetFixture()
		a := s.actor("fallen")
		a.WeaponSeal = seal
		raw := 1
		a.Gear = &BattleGearStats{WeaponID: "weapon-1", WeaponAttack: &raw}
		c.Seals = &WeaponSealTable{DragonAttack: 40, PhoenixWeight: -49}
		attack, agility := []int{0, 111, 131, 141, 101, 91, 81}[seal], []int{0, 100, 80, 70, 110, 130, 149}[seal]
		for range 2 {
			if err := s.ApplyEquipmentReset(&c, a.ID, &targetRNG{t: t}); err != nil || a.Attack != attack || a.Agility != agility {
				t.Fatal(seal, a, err)
			}
			s = reloadLifetime(t, s)
			a = s.actor("fallen")
		}
	}
}

func TestItem110AlwaysRejectsBeforeLivingFallenEmptySideOrNPCEffects(t *testing.T) {
	for _, id := range []string{"self", "ally", "fallen", "enemy"} {
		for _, npc := range []bool{false, true} {
			for _, empty := range []bool{false, true} {
				if empty && id == "enemy" {
					continue
				}
				s, c := resetFixture()
				if empty {
					s.Actors = s.Actors[:3]
					c.EnemyIDs = nil
				}
				s.actor(c.ActorID).NPC = npc
				a := s.actor(id)
				a.NPC = npc
				a.Gear.AccessoryID = "item-110"
				a.ItemDefinitionIDs = []string{"item-110"}
				before := reloadLifetime(t, s)
				if err := s.ApplyEquipmentReset(&c, id, &targetRNG{t: t}); !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(before, s) {
					t.Fatal(id, npc, empty, err)
				}
			}
		}
	}
}
