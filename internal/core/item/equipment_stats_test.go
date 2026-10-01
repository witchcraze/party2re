package item_test

import (
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/item"
)

func TestNominalEquipmentStats_AuthenticCatalog(t *testing.T) {
	// Authentic static weapons from party2/lib/_data.cgi:394-500
	weaponTests := []struct {
		defID      string
		wantAttack int
		wantWeight int
	}{
		{"weapon-01", 2, 1},    // ひのきの棒: atk 2, r(2) fallback = 1
		{"weapon-02", 4, 2},    // 竹の槍: atk 4, wt 2
		{"weapon-08", 14, 9},   // 銅の剣: atk 14, wt 9
		{"weapon-13", 30, 20},  // 鋼鉄の剣: atk 30, wt 20
		{"weapon-15", 44, 24},  // ゾンビキラー: atk 44, wt 24
		{"weapon-18", 54, 34},  // クサナギの剣: atk 54, wt 34
		{"weapon-23", 75, 45},  // ドラゴンキラー: atk 75, wt 45
		{"weapon-25", 99, 66},  // デーモンスピア: atk 99, wt 66
		{"weapon-40", 150, 75}, // ハグレメタルの剣: atk 150, wt 75
		{"weapon-41", 1, 30},   // 古びた剣: atk 1, wt 30
		{"weapon-42", 22, 12},  // 鉄の槍: atk 22, wt 12
		{"weapon-45", 31, 19},  // ロングスピア: atk 31, wt 19
		{"weapon-49", 52, 26},  // 氷の刃: atk 52, wt 26
		{"weapon-50", 80, 39},  // 吹雪の剣: atk 80, wt 39
	}

	for _, tt := range weaponTests {
		t.Run(tt.defID, func(t *testing.T) {
			atk, wt := item.NominalWeaponStats(tt.defID)
			if atk != tt.wantAttack || wt != tt.wantWeight {
				t.Errorf("NominalWeaponStats(%s) = (%d, %d), want (%d, %d)", tt.defID, atk, wt, tt.wantAttack, tt.wantWeight)
			}
		})
	}

	// Authentic static armors from party2/lib/_data.cgi:506-613
	armorTests := []struct {
		defID       string
		wantDefense int
		wantWeight  int
	}{
		{"armor-01", 3, 0},    // 布の服: def 3, wt 0
		{"armor-04", 12, 4},   // 皮の鎧: def 12, wt 4
		{"armor-07", 24, 8},   // 鎖かたびら: def 24, wt 8
		{"armor-11", 43, 17},  // 鉄の鎧: def 43, wt 17
		{"armor-16", 52, 20},  // 鋼鉄の鎧: def 52, wt 20
		{"armor-19", 73, 24},  // 銀の胸当て: def 73, wt 24
		{"armor-26", 76, 30},  // ドラゴンメイル: def 76, wt 30
		{"armor-29", 90, 34},  // 炎の鎧: def 90, wt 34
		{"armor-40", 150, 45}, // ハグレメタルの鎧: def 150, wt 45
		{"armor-41", 1, 30},   // 古びた鎧: def 1, wt 30
		{"armor-43", 55, 17},  // シルバーメイル: def 55, wt 17
		{"armor-45", 75, 40},  // あつでの鎧: def 75, wt 40
		{"armor-52", 165, 50}, // メタルキングの鎧: def 165, wt 50
		{"armor-53", 180, 55}, // ロトの鎧: def 180, wt 55
	}

	for _, tt := range armorTests {
		t.Run(tt.defID, func(t *testing.T) {
			def, wt := item.NominalArmorStats(tt.defID)
			if def != tt.wantDefense || wt != tt.wantWeight {
				t.Errorf("NominalArmorStats(%s) = (%d, %d), want (%d, %d)", tt.defID, def, wt, tt.wantDefense, tt.wantWeight)
			}
		})
	}
}

func TestCalculateWeaponStats_CharacterScaling(t *testing.T) {
	char := corecharacter.Character{
		Stats: corecharacter.Stats{
			Agility: 50,
			Defense: 60,
			Attack:  100,
			MaxMP:   80,
			MaxHP:   200,
		},
	}

	// weapon-32: 正義のソロバン (int(rand(df*1.2)), 40) -> df*1.2 = 72, r(72) = 36
	atk, wt := item.CalculateWeaponStats("weapon-32", char, nil, false)
	if atk != 36 || wt != 40 {
		t.Errorf("weapon-32 stats = (%d, %d), want (36, 40)", atk, wt)
	}

	// weapon-33: ガイアの剣 (int(rand(at*1.2)), 40) -> at*1.2 = 120, r(120) = 60
	atk, wt = item.CalculateWeaponStats("weapon-33", char, nil, false)
	if atk != 60 || wt != 40 {
		t.Errorf("weapon-33 stats = (%d, %d), want (60, 40)", atk, wt)
	}
}
