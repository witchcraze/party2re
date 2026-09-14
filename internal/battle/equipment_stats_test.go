package battle_test

import (
	"context"
	"testing"

	"github.com/witchcraze/party2re/internal/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
)

func TestEquipmentStats_ExAmuletAndAwakeningScaling(t *testing.T) {
	// Base character with 100 Attack, 100 Defense, 100 Agility
	newChar := func(level int) corecharacter.Character {
		return corecharacter.Character{
			ID:    "char-ex",
			Name:  "勇者エクス",
			Level: level,
			Stats: corecharacter.Stats{
				HP:      100,
				MaxHP:   100,
				Attack:  100,
				Defense: 100,
				Agility: 100,
			},
		}
	}

	tests := []struct {
		name         string
		level        int
		weaponDefID  string
		accDefID     string
		wantAtkBonus int
		wantDefBonus int
		wantAgiBonus int
	}{
		// 1. Excalibur alone (weapon-71):
		// Deterministic fallback: randFrac = 0.25 -> mult = 0.75 + 0.25 = 1.0 -> weaAt = 100.
		// Lv 40 (< 50): ite_158 = 0.5 -> atkBonus = int(100 * 0.5) = 50
		{
			name:         "Excalibur alone at Lv40 (0.5x scaling)",
			level:        40,
			weaponDefID:  "weapon-71",
			accDefID:     "",
			wantAtkBonus: 50,
			wantDefBonus: 0,
			wantAgiBonus: -60, // weight 60
		},
		// Lv 60 (< 75): ite_158 = 0.7 -> atkBonus = int(100 * 0.7) = 70
		{
			name:         "Excalibur alone at Lv60 (0.7x scaling)",
			level:        60,
			weaponDefID:  "weapon-71",
			accDefID:     "",
			wantAtkBonus: 70,
			wantDefBonus: 0,
			wantAgiBonus: -60,
		},
		// Lv 80 (>= 75): ite_158 = 1.0 -> atkBonus = int(100 * 1.0) = 100
		{
			name:         "Excalibur alone at Lv80 (1.0x scaling)",
			level:        80,
			weaponDefID:  "weapon-71",
			accDefID:     "",
			wantAtkBonus: 100,
			wantDefBonus: 0,
			wantAgiBonus: -60,
		},

		// 2. Excalibur (weapon-71) + Ex Amulet (item-158):
		// Awakened: mult = 1.5 + 0.25 = 1.75 -> weaAt = int(100 * 1.75) = 175.
		// Lv 40 (< 50): ite_158 = 0.5 -> atkBonus = int(175 * 0.5) = 87
		{
			name:         "Excalibur + Ex Amulet at Lv40 (0.5x scaling)",
			level:        40,
			weaponDefID:  "weapon-71",
			accDefID:     battle.ItemExAmulet,
			wantAtkBonus: 87,
			wantDefBonus: 0,
			wantAgiBonus: -60,
		},
		// Lv 60 (< 75): ite_158 = 0.7 -> atkBonus = int(175 * 0.7) = 122
		{
			name:         "Excalibur + Ex Amulet at Lv60 (0.7x scaling)",
			level:        60,
			weaponDefID:  "weapon-71",
			accDefID:     battle.ItemExAmulet,
			wantAtkBonus: 122,
			wantDefBonus: 0,
			wantAgiBonus: -60,
		},
		// Lv 80 (>= 75): ite_158 = 1.0 -> atkBonus = int(175 * 1.0) = 175
		{
			name:         "Excalibur + Ex Amulet at Lv80 (1.0x scaling)",
			level:        80,
			weaponDefID:  "weapon-71",
			accDefID:     battle.ItemExAmulet,
			wantAtkBonus: 175,
			wantDefBonus: 0,
			wantAgiBonus: -60,
		},

		// 3. Excalibur (weapon-71) + Awakening Gems (item-240, 241, 242) at Lv80:
		// Awakened weaAt = 175.
		// item-240 (覚醒の紅玉): +30 atk, -20 def, -20 agi -> atkBonus = 175 + 30 = 205
		{
			name:         "Excalibur + Awakening Ruby (item-240) at Lv80",
			level:        80,
			weaponDefID:  "weapon-71",
			accDefID:     battle.ItemAwakeningRuby,
			wantAtkBonus: 205,
			wantDefBonus: -20,
			wantAgiBonus: -80, // -60 (wea) - 20 (acc)
		},
		// item-241 (覚醒の蒼玉): -20 atk, +30 def, -20 agi -> atkBonus = 175 - 20 = 155
		{
			name:         "Excalibur + Awakening Sapphire (item-241) at Lv80",
			level:        80,
			weaponDefID:  "weapon-71",
			accDefID:     battle.ItemAwakeningSapphire,
			wantAtkBonus: 155,
			wantDefBonus: 30,
			wantAgiBonus: -80,
		},
		// item-242 (覚醒の翠玉): -20 atk, -20 def, +30 agi -> atkBonus = 175 - 20 = 155
		{
			name:         "Excalibur + Awakening Emerald (item-242) at Lv80",
			level:        80,
			weaponDefID:  "weapon-71",
			accDefID:     battle.ItemAwakeningEmerald,
			wantAtkBonus: 155,
			wantDefBonus: -20,
			wantAgiBonus: -30, // -60 (wea) + 30 (acc)
		},

		// 4. Ex Amulet (item-158) with non-Excalibur weapon:
		// weapon-08: 銅の剣 (+14 atk, wt 9). Ex Amulet gives no stat boost to other weapons.
		{
			name:         "Copper Sword + Ex Amulet at Lv80 (no awakening)",
			level:        80,
			weaponDefID:  "weapon-08",
			accDefID:     battle.ItemExAmulet,
			wantAtkBonus: 14,
			wantDefBonus: 0,
			wantAgiBonus: -9,
		},

		// 5. Weapon-69 (竜神の剣) & Weapon-70 (竜神王の剣) with original_lv:
		// weapon-69: r(100*1.2) = 60; plus int(original_lv * 1.5)
		// Lv40: 60 + int(40 * 1.5) = 60 + 60 = 120, wt 40
		{
			name:         "Dragon God Sword (weapon-69) at Lv40",
			level:        40,
			weaponDefID:  "weapon-69",
			accDefID:     "",
			wantAtkBonus: 120,
			wantDefBonus: 0,
			wantAgiBonus: -40,
		},
		// Lv120 (capped at 99): 60 + int(99 * 1.5) = 60 + 148 = 208, wt 40
		{
			name:         "Dragon God Sword (weapon-69) at Lv120 (cap at Lv99)",
			level:        120,
			weaponDefID:  "weapon-69",
			accDefID:     "",
			wantAtkBonus: 208,
			wantDefBonus: 0,
			wantAgiBonus: -40,
		},
		// weapon-70: r(100*1.6) = 80; plus int(original_lv * 2.0)
		// Lv40: 80 + int(40 * 2.0) = 80 + 80 = 160, wt 50
		{
			name:         "Dragon God King Sword (weapon-70) at Lv40",
			level:        40,
			weaponDefID:  "weapon-70",
			accDefID:     "",
			wantAtkBonus: 160,
			wantDefBonus: 0,
			wantAgiBonus: -50,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			char := newChar(tt.level)
			inv, err := coreinventory.New(char.ID)
			if err != nil {
				t.Fatalf("failed to create inventory: %v", err)
			}
			equip, err := coreequipment.New(char.ID)
			if err != nil {
				t.Fatalf("failed to create equipment: %v", err)
			}

			if tt.weaponDefID != "" {
				wInst, _ := coreitem.NewInstance(tt.weaponDefID, 1)
				_ = inv.Add(wInst)
				equip.Slots[coreitem.SlotMainHand] = wInst.ID
			}

			if tt.accDefID != "" {
				aInst, _ := coreitem.NewInstance(tt.accDefID, 1)
				_ = inv.Add(aInst)
				equip.Slots[coreitem.SlotAccessory] = aInst.ID
			}

			stats := battle.CalculateEquipmentStats(char, inv, equip, nil)

			if stats.AttackBonus != tt.wantAtkBonus {
				t.Errorf("AttackBonus = %d, want %d", stats.AttackBonus, tt.wantAtkBonus)
			}
			if stats.DefenseBonus != tt.wantDefBonus {
				t.Errorf("DefenseBonus = %d, want %d", stats.DefenseBonus, tt.wantDefBonus)
			}
			if stats.AgilityBonus != tt.wantAgiBonus {
				t.Errorf("AgilityBonus = %d, want %d", stats.AgilityBonus, tt.wantAgiBonus)
			}
		})
	}
}

func TestBuildParticipant_ExcaliburAndExAmulet(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharRepo()
	invRepo := newMockInvRepo()
	equipRepo := newMockEquipRepo()

	charID := "char-hero-ex"
	char := corecharacter.Character{
		ID:    charID,
		Name:  "勇者アーサー",
		Level: 80,
		Stats: corecharacter.Stats{
			HP:      150,
			MaxHP:   150,
			Attack:  100,
			Defense: 100,
			Agility: 100,
		},
	}
	_ = charRepo.Update(ctx, char)

	inv, _ := coreinventory.New(charID)
	wea, _ := coreitem.NewInstance("weapon-71", 1)
	amulet, _ := coreitem.NewInstance(battle.ItemExAmulet, 1)
	_ = inv.Add(wea)
	_ = inv.Add(amulet)
	_ = invRepo.Save(ctx, inv)

	equip, _ := coreequipment.New(charID)
	equip.Slots[coreitem.SlotMainHand] = wea.ID
	equip.Slots[coreitem.SlotAccessory] = amulet.ID
	_ = equipRepo.Save(ctx, equip)

	svc := battle.NewService(
		battle.WithCharacterRepository(charRepo),
		battle.WithInventoryRepository(invRepo),
		battle.WithEquipmentRepository(equipRepo),
	)

	p, err := svc.BuildParticipant(ctx, charID)
	if err != nil {
		t.Fatalf("BuildParticipant failed: %v", err)
	}

	// Base 100 + 175 (Excalibur + Ex Amulet at Lv80) = 275
	if p.Attack != 275 {
		t.Errorf("Participant.Attack = %d, want 275", p.Attack)
	}
	// Base 100 - 60 (Excalibur weight) = 40
	if p.Agility != 40 {
		t.Errorf("Participant.Agility = %d, want 40", p.Agility)
	}
}

type mockRNG struct {
	val int
}

func (m mockRNG) Intn(max int) (int, error) {
	if m.val >= max {
		return max - 1, nil
	}
	return m.val, nil
}

func TestEquipmentStats_WeaponSeals(t *testing.T) {
	// Base character with 100 Attack, 100 Defense, 100 Agility
	// Using weapon-40 (巨人の斧: Atk 150, Wt 75)
	tests := []struct {
		name         string
		sealID       int
		rngVal       int
		wantAtkBonus int
		wantDefBonus int
		wantAgiBonus int
	}{
		{
			name:         "Seal 1 (爪): Atk +10, Agi 0",
			sealID:       1,
			wantAtkBonus: 150 + 10,
			wantDefBonus: 0,
			wantAgiBonus: -75,
		},
		{
			name:         "Seal 2 (牙): Atk +30, Agi -20",
			sealID:       2,
			wantAtkBonus: 150 + 30,
			wantDefBonus: 0,
			wantAgiBonus: -75 - 20,
		},
		{
			name:         "Seal 3 (竜): Atk + int(150 * 0.2) = +30, Agi -30",
			sealID:       3,
			wantAtkBonus: 150 + 30,
			wantDefBonus: 0,
			wantAgiBonus: -75 - 30,
		},
		{
			name:         "Seal 4 (羽): Atk +0, Agi +10",
			sealID:       4,
			wantAtkBonus: 150,
			wantDefBonus: 0,
			wantAgiBonus: -75 + 10,
		},
		{
			name:         "Seal 5 (翼): Atk -10, Agi +30",
			sealID:       5,
			wantAtkBonus: 150 - 10,
			wantDefBonus: 0,
			wantAgiBonus: -75 + 30,
		},
		{
			name:         "Seal 6 (鳳): Atk -20, Agi +rand(50)",
			sealID:       6,
			rngVal:       25,
			wantAtkBonus: 150 - 20,
			wantDefBonus: 0,
			wantAgiBonus: -75 + 25,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			char := corecharacter.Character{
				ID:         "char-seal",
				Name:       "刻印戦士",
				Level:      50,
				WeaponSeal: tt.sealID,
				Stats: corecharacter.Stats{
					HP:      100,
					MaxHP:   100,
					Attack:  100,
					Defense: 100,
					Agility: 100,
				},
			}
			inv, _ := coreinventory.New(char.ID)
			wea, _ := coreitem.NewInstance("weapon-40", 1)
			_ = inv.Add(wea)
			equip, _ := coreequipment.New(char.ID)
			equip.Slots[coreitem.SlotMainHand] = wea.ID

			stats := battle.CalculateEquipmentStats(char, inv, equip, mockRNG{val: tt.rngVal})

			if stats.AttackBonus != tt.wantAtkBonus {
				t.Errorf("AttackBonus = %d, want %d", stats.AttackBonus, tt.wantAtkBonus)
			}
			if stats.DefenseBonus != tt.wantDefBonus {
				t.Errorf("DefenseBonus = %d, want %d", stats.DefenseBonus, tt.wantDefBonus)
			}
			if stats.AgilityBonus != tt.wantAgiBonus {
				t.Errorf("AgilityBonus = %d, want %d", stats.AgilityBonus, tt.wantAgiBonus)
			}
		})
	}

	t.Run("No weapon equipped with seal", func(t *testing.T) {
		char := corecharacter.Character{
			ID:         "char-seal-noweap",
			Name:       "素手刻印",
			Level:      50,
			WeaponSeal: 1, // 爪
			Stats: corecharacter.Stats{
				HP:      100,
				MaxHP:   100,
				Attack:  100,
				Defense: 100,
				Agility: 100,
			},
		}
		inv, _ := coreinventory.New(char.ID)
		equip, _ := coreequipment.New(char.ID)

		stats := battle.CalculateEquipmentStats(char, inv, equip, nil)
		if stats.AttackBonus != 0 || stats.AgilityBonus != 0 {
			t.Errorf("expected 0 bonuses when no weapon equipped, got atk=%d, agi=%d", stats.AttackBonus, stats.AgilityBonus)
		}
	})
}
