package character

import (
	"errors"
	"strings"
)

// SPExchangeStat identifies which base stat will be increased by sacrificing SP.
type SPExchangeStat string

const (
	SPExchangeMaxHP   SPExchangeStat = "mhp"
	SPExchangeMaxMP   SPExchangeStat = "mmp"
	SPExchangeAttack  SPExchangeStat = "at"
	SPExchangeDefense SPExchangeStat = "df"
	SPExchangeAgility SPExchangeStat = "ag"
)

var (
	ErrInvalidSPAmount     = errors.New("sp amount must be at least 1")
	ErrInsufficientSP      = errors.New("insufficient skill points")
	ErrJobMemoryActive     = errors.New("cannot exchange sp while recalling a job")
	ErrOverLevelRestricted = errors.New("overlevel characters cannot exchange sp for stats")
	ErrInvalidTargetStat   = errors.New("invalid target stat for sp exchange")
)

// ParseSPExchangeStat parses and normalizes a stat name/alias to canonical SPExchangeStat.
// Accepts canonical legacy keys ("mhp", "mmp", "at", "df", "ag") and common aliases ("hp", "max_hp", etc.).
func ParseSPExchangeStat(s string) (SPExchangeStat, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "mhp", "hp", "max_hp", "maxhp", "たいりょく", "体力", "ｈｐ":
		return SPExchangeMaxHP, nil
	case "mmp", "mp", "max_mp", "maxmp", "まりょく", "魔力", "ｍｐ":
		return SPExchangeMaxMP, nil
	case "at", "atk", "attack", "こうげき", "攻撃", "攻撃力":
		return SPExchangeAttack, nil
	case "df", "def", "defense", "ぼうぎょ", "防御", "守備力":
		return SPExchangeDefense, nil
	case "ag", "agi", "agility", "すばやさ", "素早さ":
		return SPExchangeAgility, nil
	default:
		return "", ErrInvalidTargetStat
	}
}

// SPExchangeRate returns the multiplier for stat increase per 1 SP.
// Legacy parity: MHP = 2, MMP = 2, AT = 1, DF = 1, AG = 1.
func SPExchangeRate(stat SPExchangeStat) int {
	switch stat {
	case SPExchangeMaxHP, SPExchangeMaxMP:
		return 2
	case SPExchangeAttack, SPExchangeDefense, SPExchangeAgility:
		return 1
	default:
		return 0
	}
}

// JapaneseName returns the authentic Japanese display name for the stat (%e2j in legacy Party2).
func (s SPExchangeStat) JapaneseName() string {
	switch s {
	case SPExchangeMaxHP:
		return "ＨＰ"
	case SPExchangeMaxMP:
		return "ＭＰ"
	case SPExchangeAttack:
		return "攻撃力"
	case SPExchangeDefense:
		return "守備力"
	case SPExchangeAgility:
		return "素早さ"
	default:
		return string(s)
	}
}

// ApplySPExchange executes the legacy Goddess SP exchange (sp_change.cgi).
// It validates preconditions:
// - sp >= 1
// - sp <= c.SP
// - c.JobMemory == nil (remembered jobs cannot spend SP)
// - !c.OverLevel (OverLevel limit-broken characters cannot spend SP)
// Then it deducts sp and increases the specified stat by sp * multiplier,
// capping with Clamp(c.OverLevel, c.Level).
// Returns the actual stat increase amount.
func (c *Character) ApplySPExchange(stat SPExchangeStat, sp int) (int, error) {
	if c == nil {
		return 0, errors.New("character is nil")
	}
	if sp < 1 {
		return 0, ErrInvalidSPAmount
	}
	if sp > c.SP {
		return 0, ErrInsufficientSP
	}
	if c.JobMemory != nil {
		return 0, ErrJobMemoryActive
	}
	if c.OverLevel {
		return 0, ErrOverLevelRestricted
	}

	rate := SPExchangeRate(stat)
	if rate == 0 {
		return 0, ErrInvalidTargetStat
	}

	increase := sp * rate
	c.SP -= sp

	switch stat {
	case SPExchangeMaxHP:
		c.Stats.MaxHP += increase
	case SPExchangeMaxMP:
		c.Stats.MaxMP += increase
	case SPExchangeAttack:
		c.Stats.Attack += increase
	case SPExchangeDefense:
		c.Stats.Defense += increase
	case SPExchangeAgility:
		c.Stats.Agility += increase
	}

	c.Stats.Clamp(c.OverLevel, c.Level)

	return increase, nil
}
