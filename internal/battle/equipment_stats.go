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

// calculateWeaponStats delegates to coreitem.CalculateWeaponStats.
func calculateWeaponStats(defID string, char corecharacter.Character, rng corecharacter.RandomSource, hasExAmuletOrAwakening bool) (int, int) {
	return coreitem.CalculateWeaponStats(defID, char, rng, hasExAmuletOrAwakening)
}

// calculateArmorStats delegates to coreitem.CalculateArmorStats.
func calculateArmorStats(defID string, char corecharacter.Character, rng corecharacter.RandomSource) (int, int) {
	return coreitem.CalculateArmorStats(defID, char, rng)
}

// calculateAccessoryStats delegates to coreitem.CalculateAccessoryStats.
func calculateAccessoryStats(defID string) (int, int, int) {
	return coreitem.CalculateAccessoryStats(defID)
}
