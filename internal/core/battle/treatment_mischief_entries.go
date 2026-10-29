package battle

// TreatmentMischiefEntries preserves complete source tables, including bodies
// registered by resource, revival, random, inspection and asset owners.
func TreatmentMischiefEntries(job int) []CallbackEntry {
	switch job {
	case 50:
		return []CallbackEntry{
			{ID: "job-50-entry-0", Name: "healing-80", RequiredSP: 10, MPCost: 5},
			{ID: "job-50-entry-1", Name: "restore-accuracy", RequiredSP: 25, MPCost: 3},
			{ID: "job-50-entry-2", Name: "death-potion", RequiredSP: 40, MPCost: 14},
			{ID: "job-50-entry-3", Name: "breath-armor", RequiredSP: 55, MPCost: 10},
			{ID: "job-50-entry-4", Name: "healing-200", RequiredSP: 75, MPCost: 24},
			{ID: "job-50-entry-5", Name: "mp-restoration-50", RequiredSP: 110, MPCost: 50},
			{ID: "job-50-entry-6", Name: "献命全快", RequiredSP: 145, MPCost: 1},
			{ID: "job-50-entry-7", Name: "stored-treatment", RequiredSP: 145, MPCost: 1, Extended: true},
		}
	case 51:
		return []CallbackEntry{
			{ID: "job-51-entry-0", Name: "accuracy-light", RequiredSP: 10, MPCost: 6},
			{ID: "job-51-entry-1", Name: "party-accuracy", RequiredSP: 30, MPCost: 11},
			{ID: "job-51-entry-2", Name: "healing-light", RequiredSP: 50, MPCost: 14},
			{ID: "job-51-entry-3", Name: "bewildering-light", RequiredSP: 80, MPCost: 16},
			{ID: "job-51-entry-4", Name: "judgment-light", RequiredSP: 110, MPCost: 34},
			{ID: "job-51-entry-5", Name: "希望の光", RequiredSP: 130, MPCost: 30},
			{ID: "job-51-entry-6", Name: "death-light", RequiredSP: 160, MPCost: 46},
			{ID: "job-51-entry-7", Name: "mana-star", RequiredSP: 160, MPCost: 80, Extended: true},
		}
	case 55:
		return []CallbackEntry{
			{ID: "job-55-entry-0", Name: "random-blindfold", RequiredSP: 5},
			{ID: "job-55-entry-1", Name: "mana-arrow", RequiredSP: 10, MPCost: 4},
			{ID: "job-55-entry-2", Name: "random-magic-seal", RequiredSP: 20},
			{ID: "job-55-entry-3", Name: "group-taunt", RequiredSP: 35},
			{ID: "job-55-entry-4", Name: "agility-arrows", RequiredSP: 50, MPCost: 8},
			{ID: "job-55-entry-5", Name: "target-taunt", RequiredSP: 65},
			{ID: "job-55-entry-6", Name: "random-skill", RequiredSP: 80},
			{ID: "job-55-entry-7", Name: "rainbow-light", RequiredSP: 80, MPCost: 24, Extended: true},
		}
	case 56:
		return []CallbackEntry{
			{ID: "job-56-entry-0", Name: "magic-wave-25", RequiredSP: 6, MPCost: 5},
			{ID: "job-56-entry-1", Name: "magic-absorption", RequiredSP: 16, MPCost: 1},
			{ID: "job-56-entry-2", Name: "magic-wave-70", RequiredSP: 36, MPCost: 12},
			{ID: "job-56-entry-3", Name: "finite-random", RequiredSP: 46, MPCost: 10},
			{ID: "job-56-entry-4", Name: "magic-wave-160", RequiredSP: 66, MPCost: 34},
			{ID: "job-56-entry-5", Name: "random-tail", RequiredSP: 96},
		}
	case 57:
		return []CallbackEntry{
			{ID: "job-57-entry-0", Name: "healing-30", RequiredSP: 10, MPCost: 3},
			{ID: "job-57-entry-1", Name: "accuracy-arrow", RequiredSP: 20, MPCost: 6},
			{ID: "job-57-entry-2", Name: "healing-90", RequiredSP: 40, MPCost: 10},
			{ID: "job-57-entry-3", Name: "sleep-arrow", RequiredSP: 60, MPCost: 20},
			{ID: "job-57-entry-4", Name: "半蘇生", RequiredSP: 80, MPCost: 20},
			{ID: "job-57-entry-5", Name: "mana-arrow", RequiredSP: 100, MPCost: 4},
			{ID: "job-57-entry-6", Name: "party-strength", RequiredSP: 120, MPCost: 33},
		}
	case 78:
		return []CallbackEntry{
			{ID: "job-78-entry-0", Name: "enhanced-tool", RequiredSP: 1},
			{ID: "job-78-entry-1", Name: "reward-valuation-inspection", RequiredSP: 15},
			{ID: "job-78-entry-2", Name: "treasure-theft", RequiredSP: 30, MPCost: 10},
			{ID: "job-78-entry-3", Name: "survival-treatment", RequiredSP: 55, MPCost: 14},
			{ID: "job-78-entry-4", Name: "shock-bomb", RequiredSP: 70, MPCost: 18},
			{ID: "job-78-entry-5", Name: "tool-release", RequiredSP: 100, MPCost: 28},
			{ID: "job-78-entry-6", Name: "job-history-reward", RequiredSP: 200},
		}
	}
	return nil
}

func treatmentMischiefOwns(job, pos int) bool {
	return job == 50 && pos < 5 || job == 51 && pos != 5 || job == 55 && pos < 6 ||
		job == 56 && pos != 3 && pos != 5 || job == 57 && pos != 4 || job == 78 && pos == 3
}
