package character

import (
	"errors"
	"strings"
)

// Orb symbols conforming to legacy Party2 (reborn.cgi / _data.cgi).
const (
	OrbSilver = 's' // 月曜日 / シルバーオーブ
	OrbRed    = 'r' // 火曜日 / レッドオーブ
	OrbBlue   = 'b' // 水曜日 / ブルーオーブ
	OrbGreen  = 'g' // 木曜日 / グリーンオーブ
	OrbYellow = 'y' // 金曜日 / イエローオーブ
	OrbPurple = 'p' // 土曜日 / パープルオーブ
	OrbGold   = 'G' // 不死鳥ラーミァ復活完了状態
)

var (
	ErrInvalidOrbRune = errors.New("invalid orb rune")
)

var ValidOrbRunes = []rune{OrbSilver, OrbRed, OrbBlue, OrbGreen, OrbYellow, OrbPurple}

func IsValidOrbRune(r rune) bool {
	for _, orb := range ValidOrbRunes {
		if orb == r {
			return true
		}
	}
	return false
}

// HasOrb checks whether character has offered or collected the specified orb color.
func (c *Character) HasOrb(orb rune) bool {
	return strings.ContainsRune(c.Orb, orb)
}

// OrbCount returns number of distinct standard orbs (s, r, b, g, y, p) collected.
func (c *Character) OrbCount() int {
	count := 0
	for _, orb := range ValidOrbRunes {
		if strings.ContainsRune(c.Orb, orb) {
			count++
		}
	}
	return count
}

// HasAllOrbs returns true if all 6 standard orbs are collected.
func (c *Character) HasAllOrbs() bool {
	return c.OrbCount() == len(ValidOrbRunes)
}

// IsRamiaAwakened returns true if Ramia has awakened (Orb contains 'G').
func (c *Character) IsRamiaAwakened() bool {
	return strings.ContainsRune(c.Orb, OrbGold)
}

// AddOrb adds an orb symbol to character's collection. Returns false if already collected.
func (c *Character) AddOrb(orb rune) (bool, error) {
	if !IsValidOrbRune(orb) {
		return false, ErrInvalidOrbRune
	}
	if c.HasOrb(orb) {
		return false, nil
	}
	c.Orb += string(orb)
	return true, nil
}

// SetRamiaAwakened sets orb state to 'G', indicating Ramia is revived.
func (c *Character) SetRamiaAwakened() {
	c.Orb = string(OrbGold)
}

// ClearOrbs resets character's orb status to empty string.
func (c *Character) ClearOrbs() {
	c.Orb = ""
}
