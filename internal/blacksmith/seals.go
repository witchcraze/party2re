package blacksmith

import (
	"slices"
	"strings"
)

// Seal represents an authentic weapon seal (刻印) from legacy Party2 (_data.cgi:2209-2230).
type Seal struct {
	ID                  int      `json:"id"`
	Name                string   `json:"name"`
	CrystalCost         int      `json:"crystal_cost"`
	AttackBonus         int      `json:"attack_bonus"`
	WeightBonus         int      `json:"weight_bonus"`
	Description         string   `json:"description"`
	ApplicableWeaponIDs []string `json:"applicable_weapon_ids,omitempty"` // empty means all weapons (1..71)
}

// Seals is the canonical catalog of all 12 authentic weapon seals.
var Seals = []Seal{
	{
		ID:          1,
		Name:        "爪",
		CrystalCost: 50,
		AttackBonus: 10,
		WeightBonus: 0,
		Description: "攻撃力+10",
	},
	{
		ID:          2,
		Name:        "牙",
		CrystalCost: 500,
		AttackBonus: 30,
		WeightBonus: 20,
		Description: "攻撃力+30 重さ+20",
	},
	{
		ID:          3,
		Name:        "竜",
		CrystalCost: 5000,
		AttackBonus: 0, // Dynamic in combat: +20% of weapon attack
		WeightBonus: 30,
		Description: "攻撃力+武器の攻撃力の20% 重さ+30",
	},
	{
		ID:          4,
		Name:        "羽",
		CrystalCost: 50,
		AttackBonus: 0,
		WeightBonus: -10,
		Description: "重さ-10",
	},
	{
		ID:          5,
		Name:        "翼",
		CrystalCost: 500,
		AttackBonus: -10,
		WeightBonus: -30,
		Description: "攻撃力-10 重さ-30",
	},
	{
		ID:          6,
		Name:        "鳳",
		CrystalCost: 5000,
		AttackBonus: -20,
		WeightBonus: 0, // Dynamic in combat: rand(50) * -1 (-50..0)
		Description: "攻撃力-20 重さ-50〜0",
	},
	{
		ID:                  7,
		Name:                "炎",
		CrystalCost:         100,
		Description:         "＠しゃくねつ が使えるようになる",
		ApplicableWeaponIDs: []string{"weapon-36"},
	},
	{
		ID:                  8,
		Name:                "氷",
		CrystalCost:         100,
		Description:         "＠マヒャド  が使えるようになる",
		ApplicableWeaponIDs: []string{"weapon-50", "weapon-51"},
	},
	{
		ID:                  9,
		Name:                "雷",
		CrystalCost:         100,
		Description:         "＠ギガデイン  が使えるようになる",
		ApplicableWeaponIDs: []string{"weapon-68", "weapon-69", "weapon-70"},
	},
	{
		ID:                  10,
		Name:                "神速",
		CrystalCost:         100,
		Description:         "通常攻撃が2回出せる",
		ApplicableWeaponIDs: []string{"weapon-29", "weapon-31", "weapon-51", "weapon-66"},
	},
	{
		ID:                  11,
		Name:                "空",
		CrystalCost:         100,
		Description:         "攻撃した敵のステータスを元に戻すことがある",
		ApplicableWeaponIDs: []string{"weapon-35"},
	},
	{
		ID:                  12,
		Name:                "理",
		CrystalCost:         100,
		Description:         "MPを消費して、物理攻撃が魔法攻撃になる",
		ApplicableWeaponIDs: []string{"weapon-34"},
	},
}

// GetSeal returns the seal definition by ID (1..12).
func GetSeal(id int) (Seal, bool) {
	if id < 1 || id > len(Seals) {
		return Seal{}, false
	}
	return Seals[id-1], true
}

// CanApplySeal checks if a seal can be placed on a specific weapon definition ID (_can_add_wea_seals in blacksmith.cgi).
func CanApplySeal(weaponDefID string, sealID int) bool {
	seal, ok := GetSeal(sealID)
	if !ok {
		return false
	}
	cleanID := strings.TrimSpace(weaponDefID)
	if cleanID == "" {
		return false
	}
	// Universal seals (IDs 1..6) apply to all weapons
	if len(seal.ApplicableWeaponIDs) == 0 {
		return true
	}
	return slices.Contains(seal.ApplicableWeaponIDs, cleanID)
}

// ListAvailableSeals returns all seals applicable to the specified weapon.
func ListAvailableSeals(weaponDefID string) []Seal {
	cleanID := strings.TrimSpace(weaponDefID)
	var available []Seal
	for _, seal := range Seals {
		if len(seal.ApplicableWeaponIDs) == 0 || slices.Contains(seal.ApplicableWeaponIDs, cleanID) {
			available = append(available, seal)
		}
	}
	return available
}
