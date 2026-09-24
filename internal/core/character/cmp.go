package character

// CMPTierRate returns the multiplier for the given job CMP growth rate tier (0..5),
// as defined in legacy Party2 _data.cgi (@c_mp_rate:2200-2207).
func CMPTierRate(tier int) float64 {
	switch tier {
	case 1:
		return 0.5
	case 2:
		return 0.8
	case 3:
		return 1.0
	case 4:
		return 1.5
	case 5:
		return 2.0
	default:
		return 0.0
	}
}

// CalculateCMP calculates a character's Custom Skill MP (c_mp) based on their level
// and current/old job growth rate tiers.
// Formula (system.cgi:73-78): int(original_lv(level) * (rate[jobTier] + rate[oldJobTier]))
// where original_lv caps at 99 (system.cgi:1705-1708).
func CalculateCMP(level int, jobTierRate, oldJobTierRate float64) int {
	origLv := level
	if origLv > 99 {
		origLv = 99
	}
	if origLv < 0 {
		origLv = 0
	}
	return int(float64(origLv) * (jobTierRate + oldJobTierRate))
}

// CMP calculates the character's CMP using the provided job and old job tiers.
func (c *Character) CMP(jobTier, oldJobTier int) int {
	if c == nil {
		return 0
	}
	return CalculateCMP(c.Level, CMPTierRate(jobTier), CMPTierRate(oldJobTier))
}
