package battle

import (
	"strconv"
	"strings"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
)

// EquipmentStats aggregates stat modifications calculated from equipped gear.
type EquipmentStats struct {
	AttackBonus  int
	DefenseBonus int
	AgilityBonus int
}

// CalculateEquipmentStats computes attack, defense, and agility modifications from equipped items
// based on authentic legacy Party2 formulas (party2/lib/_data.cgi:394-613, quest.cgi:1182-1188).
func CalculateEquipmentStats(
	char corecharacter.Character,
	inv coreinventory.Inventory,
	equip coreequipment.Equipment,
	rng corecharacter.RandomSource,
) EquipmentStats {
	var stats EquipmentStats

	// Detect Ex Amulet (item-158) or Awakening gems (item-240..242) in accessory slot (_battle.cgi:1754)
	hasExAmuletOrAwakening := false
	if instID, ok := equip.Equipped(coreitem.SlotAccessory); ok {
		if inst, found := inv.Find(instID); found {
			no := parseItemIndex(inst.DefinitionID, "item-")
			if no == 158 || no == 240 || no == 241 || no == 242 {
				hasExAmuletOrAwakening = true
			}
		}
	}

	// 1. Weapon (SlotMainHand -> @weas in _data.cgi:394-500, _battle.cgi:1726-1745)
	var weaponAtk int
	hasWeapon := false
	if instID, ok := equip.Equipped(coreitem.SlotMainHand); ok {
		if inst, found := inv.Find(instID); found {
			hasWeapon = true
			atk, wt := calculateWeaponStats(inst.DefinitionID, char, rng, hasExAmuletOrAwakening)
			weaponAtk = atk
			stats.AttackBonus += atk
			stats.AgilityBonus -= wt
		}
	}

	// 2. Armor (SlotBody -> @arms in _data.cgi:506-613)
	if instID, ok := equip.Equipped(coreitem.SlotBody); ok {
		if inst, found := inv.Find(instID); found {
			def, wt := calculateArmorStats(inst.DefinitionID, char, rng)
			stats.DefenseBonus += def
			stats.AgilityBonus -= wt
		}
	}

	// 3. Accessory (SlotAccessory -> @ites in _data.cgi:1440-1477)
	if instID, ok := equip.Equipped(coreitem.SlotAccessory); ok {
		if inst, found := inv.Find(instID); found {
			atk, def, agi := calculateAccessoryStats(inst.DefinitionID)
			stats.AttackBonus += atk
			stats.DefenseBonus += def
			stats.AgilityBonus += agi
		}
	}

	// 4. Weapon Seal Stat Modifiers (party2/lib/_data.cgi:2209-2230, party2/lib/_battle.cgi:1405-1406)
	if hasWeapon && char.WeaponSeal > 0 {
		switch char.WeaponSeal {
		case 1: // 爪: 攻撃力+10
			stats.AttackBonus += 10
		case 2: // 牙: 攻撃力+30, 重さ+20 (Agility -20)
			stats.AttackBonus += 30
			stats.AgilityBonus -= 20
		case 3: // 竜: 攻撃力+武器の攻撃力の20%, 重さ+30 (Agility -30)
			stats.AttackBonus += int(float64(weaponAtk) * 0.2)
			stats.AgilityBonus -= 30
		case 4: // 羽: 重さ-10 (Agility +10)
			stats.AgilityBonus += 10
		case 5: // 翼: 攻撃力-10, 重さ-30 (Agility +30)
			stats.AttackBonus -= 10
			stats.AgilityBonus += 30
		case 6: // 鳳: 攻撃力-20, 重さ-50〜0 (Agility +0..49)
			stats.AttackBonus -= 20
			stats.AgilityBonus += randInt(rng, 50)
		}
	}

	return stats
}

func randInt(rng corecharacter.RandomSource, n int) int {
	if n <= 0 {
		return 0
	}
	if rng != nil {
		if v, err := rng.Intn(n); err == nil {
			return v
		}
	}
	// Fallback deterministic if rng is unavailable or errs
	return n / 2
}

func parseItemIndex(defID, prefix string) int {
	clean := strings.TrimPrefix(defID, prefix)
	no, err := strconv.Atoi(clean)
	if err != nil {
		return 0
	}
	return no
}

// calculateWeaponStats returns (attackBonus, weight) according to @weas in _data.cgi:394-500 and _weapon_revision in _battle.cgi:1726-1745.
func calculateWeaponStats(defID string, char corecharacter.Character, rng corecharacter.RandomSource, hasExAmuletOrAwakening bool) (int, int) {
	no := parseItemIndex(defID, "weapon-")
	if no <= 0 {
		no = parseItemIndex(defID, "item-")
	}
	r := func(n int) int { return randInt(rng, n) }

	switch no {
	case 1:
		return 2, r(2)
	case 2:
		return 4, 2
	case 3:
		return r(3) * 8, 6
	case 4:
		return 6, r(5)
	case 5:
		return r(15), 5
	case 6:
		return 9, r(7)
	case 7:
		return r(2) * 40, 12
	case 8:
		return 14, 9
	case 9:
		return r(24), 10
	case 10:
		return 18, r(15)
	case 11:
		return r(3) * 30, 25
	case 12:
		return r(2) * 42, 4
	case 13:
		return 30, 20
	case 14:
		return 27, r(25)
	case 15:
		return 44, 24
	case 16:
		return r(60), 20
	case 17:
		return r(4) * 18, 16
	case 18:
		return 54, 34
	case 19:
		return 47, r(50)
	case 20:
		return r(3) * 50, 38
	case 21:
		return r(90), 32
	case 22:
		return r(2) * 150, 50
	case 23:
		return 75, 45
	case 24:
		return 65, r(70)
	case 25:
		return 99, 66
	case 26:
		return r(120), 45
	case 27:
		return r(2) * 180, 60
	case 28:
		return -20, r(40) * -1
	case 29:
		return 5, r(40) * -1
	case 30:
		return 70, r(2) * 20
	case 31:
		return r(int(float64(char.Stats.Agility) * 1.2)), r(50)
	case 32:
		return r(int(float64(char.Stats.Defense) * 1.2)), 40
	case 33:
		return r(int(float64(char.Stats.Attack) * 1.2)), 40
	case 34:
		return r(int(float64(char.Stats.MaxMP) * 0.6)), r(70)
	case 35:
		return r(int(float64(char.Stats.MaxHP) * 0.6)), 50
	case 36:
		return 90, r(100)
	case 37:
		return r(2) * 300, 70
	case 38:
		return r(200), 55
	case 39:
		return r(3) * 150, 80
	case 40:
		return 150, 75
	case 41:
		return 1, 30
	case 42:
		return 22, 12
	case 43:
		return 14, r(11)
	case 44:
		return r(2) * 60, 15
	case 45:
		return 31, 19
	case 46:
		return 36, 10
	case 47:
		return r(4) * 20, 30
	case 48:
		return r(7) * 20, 55
	case 49:
		return 52, 26
	case 50:
		return 80, 39
	case 51:
		return 45, r(40) * -1
	case 52:
		return -30, r(60) * -1
	case 53:
		return r(3) * 120, 60
	case 54:
		return 85, r(2) * 10
	case 55:
		return 65, 45
	case 56:
		return 96, 50
	case 57:
		return 60, 20
	case 58:
		return 110, 55
	case 59:
		return 84, 40
	case 60:
		return r(2) * 300, 85
	case 61:
		return r(4) * 70, 55
	case 62:
		return r(4) * 90, 65
	case 63:
		return r(4) * 100, 80
	case 64:
		return 54, 16
	case 65:
		return 61, 11
	case 66:
		return 78, r(30) * -1
	case 67:
		return 165, 80
	case 68:
		return 180, r(int(float64(char.Stats.HP) * 0.2))
	case 69:
		origLv := min(char.Level, 99)
		return r(int(float64(char.Stats.Attack)*1.2)) + int(float64(origLv)*1.5), 40
	case 70:
		origLv := min(char.Level, 99)
		return r(int(float64(char.Stats.Attack)*1.6)) + int(float64(origLv)*2.0), 50
	case 71:
		randFrac := 0.25
		if rng != nil {
			if v, err := rng.Intn(500); err == nil {
				randFrac = float64(v) / 1000.0
			}
		}
		mult := 0.75 + randFrac
		if hasExAmuletOrAwakening {
			mult = 1.5 + randFrac
		}
		weaAt := int(float64(char.Stats.Attack) * mult)
		ite158 := 1.0
		if char.Level < 50 {
			ite158 = 0.5
		} else if char.Level < 75 {
			ite158 = 0.7
		}
		return int(float64(weaAt) * ite158), 60
	default:
		return 0, 0
	}
}

// calculateArmorStats returns (defenseBonus, weight) according to @arms in _data.cgi:506-613.
func calculateArmorStats(defID string, char corecharacter.Character, rng corecharacter.RandomSource) (int, int) {
	no := parseItemIndex(defID, "armor-")
	if no <= 0 {
		no = parseItemIndex(defID, "item-")
	}
	r := func(n int) int { return randInt(rng, n) }

	switch no {
	case 1:
		return 3, 0
	case 2:
		return 5, r(2)
	case 3:
		return -2, r(12) * -1
	case 4:
		return 12, 4
	case 5:
		return r(11), r(6)
	case 6:
		return r(10) + 7, 6
	case 7:
		return 24, 8
	case 8:
		return r(20), r(10)
	case 9:
		return 30, r(18)
	case 10:
		return r(20) + 15, 12
	case 11:
		return 43, 17
	case 12:
		return 34, r(20)
	case 13:
		return r(2) * 90, 22
	case 14:
		return 0, r(55) * -1
	case 15:
		return r(30) + 10, r(20)
	case 16:
		return 52, 20
	case 17:
		return r(30) + 30, 22
	case 18:
		return 45, r(25)
	case 19:
		return 73, 24
	case 20:
		return r(2) * 140, 22
	case 21:
		return 62, r(30)
	case 22:
		return r(3) * 40, 27
	case 23:
		return r(25), r(30) * -1
	case 24:
		return 70, r(35)
	case 25:
		return r(2) * 180, 33
	case 26:
		return 76, 30
	case 27:
		return r(50) + 30, r(40)
	case 28:
		return r(40), r(40) * -1
	case 29:
		return 90, 34
	case 30:
		return r(70) + 40, 32
	case 31:
		return r(2) * 220, 44
	case 32:
		return r(10) + 5, r(20) * -1
	case 33:
		return r(2) * 77, r(2) * -77
	case 34:
		return r(30) * -1, r(100) * -1
	case 35:
		return r(50) + 70, 10 * r(3)
	case 36:
		return 100, r(50)
	case 37:
		return r(50) + 100, 42
	case 38:
		return r(2) * 300, 55
	case 39:
		return r(150), r(40)
	case 40:
		return 150, 45
	case 41:
		return 1, 30
	case 42:
		return 32, r(14)
	case 43:
		return 55, 17
	case 44:
		return r(35) + 25, 20
	case 45:
		return 75, 40
	case 46:
		return 92, 40
	case 47:
		return 95, 38
	case 48:
		return r(20), r(10) * -5
	case 49:
		return r(40), r(15) * -4
	case 50:
		return 90, r(50)
	case 51:
		return r(3) * 160, 60
	case 52:
		return 165, 50
	case 53:
		return 180, 55
	case 54:
		return r(10) * 25, r(15) * 5
	case 55:
		return r(15) * 25, r(10) * 5
	default:
		return 0, 0
	}
}

// calculateAccessoryStats returns (atk, def, agi) according to @ites in _data.cgi:1440-1477.
func calculateAccessoryStats(defID string) (int, int, int) {
	no := parseItemIndex(defID, "item-")
	switch no {
	case 116: // ドクロの指輪
		return 30, 30, 0
	case 117: // 金のロザリオ
		return 0, 30, 0
	case 118: // 金の指輪
		return 0, 15, 0
	case 119: // 金のブレスレット
		return 0, 50, 0
	case 120: // はやてのリング
		return 0, 0, 30
	case 121: // ほしふる腕輪
		return 0, 0, 60
	case 122: // 力の指輪
		return 20, 0, 0
	case 123: // ごうけつの腕輪
		return 40, 0, 0
	case 124: // アルゴンリング
		return 30, 0, 30
	case 240: // 覚醒の紅玉 (_data.cgi:1997: at += 30, df -= 20, ag -= 20)
		return 30, -20, -20
	case 241: // 覚醒の蒼玉 (_data.cgi:1998: at -= 20, df += 30, ag -= 20)
		return -20, 30, -20
	case 242: // 覚醒の翠玉 (_data.cgi:1999: at -= 20, df -= 20, ag += 30)
		return -20, -20, 30
	default:
		return 0, 0, 0
	}
}
