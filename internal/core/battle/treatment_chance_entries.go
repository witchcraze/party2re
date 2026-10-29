package battle

// TreatmentChanceEntries preserves source43/45/46 order, including the full
// revival retained by its existing owner (_skill.cgi:3184–3413, ff97fc5).
func TreatmentChanceEntries(job int) []CallbackEntry {
	switch job {
	case 43:
		return []CallbackEntry{
			{ID: "job-43-entry-0", Name: "party-poison-treatment", RequiredSP: 5, MPCost: 2},
			{ID: "job-43-entry-1", Name: "party-paralysis-treatment", RequiredSP: 10, MPCost: 3},
			{ID: "job-43-entry-2", Name: "party-waking-song", RequiredSP: 20, MPCost: 5},
			{ID: "job-43-entry-3", Name: "regeneration", RequiredSP: 40, MPCost: 7},
			{ID: "job-43-entry-4", Name: "current-status-treatment", RequiredSP: 70, MPCost: 10},
			{ID: "job-43-entry-5", Name: "healing-400", RequiredSP: 110, MPCost: 35},
			{ID: "job-43-entry-6", Name: "完全蘇生", RequiredSP: 150, MPCost: 70},
		}
	case 45:
		return []CallbackEntry{
			{ID: "job-45-entry-0", Name: "accuracy-restoration", RequiredSP: 10, MPCost: 5},
			{ID: "job-45-entry-1", Name: "magic-immobilization", RequiredSP: 30, MPCost: 8},
			{ID: "job-45-entry-2", Name: "magic-armor", RequiredSP: 50, MPCost: 7},
			{ID: "job-45-entry-3", Name: "mana-dance", RequiredSP: 70, MPCost: 3},
			{ID: "job-45-entry-4", Name: "weakening-song", RequiredSP: 90, MPCost: 30},
			{ID: "job-45-entry-5", Name: "regeneration", RequiredSP: 120, MPCost: 7},
			{ID: "job-45-entry-6", Name: "direct-charge", RequiredSP: 150, MPCost: 40},
			{ID: "job-45-entry-7", Name: "party-restoration-dance", RequiredSP: 150, MPCost: 20, Extended: true},
		}
	case 46:
		return []CallbackEntry{
			{ID: "job-46-entry-0", Name: "three-symbols", RequiredSP: 10, MPCost: 7},
			{ID: "job-46-entry-1", Name: "death-dart", RequiredSP: 30, MPCost: 14},
			{ID: "job-46-entry-2", Name: "recoil-dice", RequiredSP: 60, MPCost: 6},
			{ID: "job-46-entry-3", Name: "death-roulette", RequiredSP: 80, MPCost: 4},
			{ID: "job-46-entry-4", Name: "weighted-dice", RequiredSP: 140, MPCost: 36},
			{ID: "job-46-entry-5", Name: "danger-coin", RequiredSP: 140, MPCost: 44, Extended: true},
		}
	}
	return nil
}
