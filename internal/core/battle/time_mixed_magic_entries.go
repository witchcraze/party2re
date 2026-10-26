package battle

// TimeMixedMagicEntries preserves full source order (_skill.cgi:2490–2715),
// including the separately owned field and arithmetic positions.
func TimeMixedMagicEntries(job int) []CallbackEntry {
	switch job {
	case 29:
		return []CallbackEntry{
			{ID: "job-29-entry-0", Name: "agility-down", RequiredSP: 10, MPCost: 4},
			{ID: "job-29-entry-1", Name: "agility-up", RequiredSP: 20, MPCost: 4},
			{ID: "job-29-entry-2", Name: "two-three-magic", RequiredSP: 30, MPCost: 14},
			{ID: "job-29-entry-3", Name: "enemy-agility-down", RequiredSP: 50, MPCost: 9},
			{ID: "job-29-entry-4", Name: "party-agility-up", RequiredSP: 70, MPCost: 9},
			{ID: "job-29-entry-5", Name: "hp-reduction", RequiredSP: 110, MPCost: 30},
			{ID: "job-29-entry-6", Name: "four-six-magic", RequiredSP: 150, MPCost: 50},
			{ID: "job-29-entry-7", Name: "cosmic-field", RequiredSP: 150, MPCost: 1, Extended: true},
		}
	case 30:
		return []CallbackEntry{
			{ID: "job-30-entry-0", Name: "heal-60", RequiredSP: 10, MPCost: 5},
			{ID: "job-30-entry-1", Name: "magic-35", RequiredSP: 20, MPCost: 5},
			{ID: "job-30-entry-2", Name: "magic-guard", RequiredSP: 40, MPCost: 5},
			{ID: "job-30-entry-3", Name: "magic-80", RequiredSP: 60, MPCost: 11},
			{ID: "job-30-entry-4", Name: "magic-reflection", RequiredSP: 80, MPCost: 7},
			{ID: "job-30-entry-5", Name: "heal-180", RequiredSP: 100, MPCost: 18},
			{ID: "job-30-entry-6", Name: "revival-guard", RequiredSP: 120, MPCost: 20},
			{ID: "job-30-entry-7", Name: "paired-magic-180", RequiredSP: 150, MPCost: 40},
			{ID: "job-30-entry-8", Name: "mixed-magic", RequiredSP: 150, MPCost: 58, Extended: true},
		}
	case 31:
		arithmetic := ArithmeticEntries("job-31")
		return []CallbackEntry{
			{ID: "job-31-entry-0", Name: "self-destruct", RequiredSP: 11, MPCost: 11},
			{ID: "job-31-entry-1", Name: "death-roulette", RequiredSP: 44, MPCost: 4},
			{ID: "job-31-entry-2", Name: "missing-hp", RequiredSP: 66, MPCost: 18},
			{ID: "job-31-entry-3", Name: "party-guard", RequiredSP: 77, MPCost: 34},
			CallbackEntry(arithmetic[0]),
			{ID: "job-31-entry-5", Name: "party-hp-heal", RequiredSP: 121, MPCost: 36},
			CallbackEntry(arithmetic[1]), CallbackEntry(arithmetic[2]),
		}
	}
	return nil
}
