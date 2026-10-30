package battle

// IncidentalJobEntries preserves whole source tables, including separately
// owned wallet, inspection and sacrifice positions (_skill.cgi:1278–1538).
func IncidentalJobEntries(job int) []CallbackEntry {
	switch job {
	case 7:
		return []CallbackEntry{
			{ID: "job-7-entry-0", Name: "wallet-strike", Extended: true},
			{ID: "job-7-entry-1", Name: "fortify-defense", RequiredSP: 2, MPCost: 2},
			{ID: "job-7-entry-2", Name: "coin-strike", RequiredSP: 7, MPCost: 1},
			{ID: "job-7-entry-3", Name: "accuracy-dust", RequiredSP: 12, MPCost: 6},
			{ID: "job-7-entry-4", Name: "member-content-inspection", RequiredSP: 20},
			{ID: "job-7-entry-5", Name: "defense-charge", RequiredSP: 35, MPCost: 4},
			{ID: "job-7-entry-6", Name: "share-mana", RequiredSP: 45, MPCost: 1},
			{ID: "job-7-entry-7", Name: "confusion-dance", RequiredSP: 65, MPCost: 7},
			{ID: "job-7-entry-8", Name: "献命終止", RequiredSP: 80, MPCost: 1},
		}
	case 8:
		return []CallbackEntry{
			{ID: "job-8-entry-0", Name: "wallet-strike", Extended: true},
			{ID: "job-8-entry-1", Name: "sleep-recovery", RequiredSP: 1},
			{ID: "job-8-entry-2", Name: "random-paralysis", RequiredSP: 4},
			{ID: "job-8-entry-3", Name: "random-immobilization", RequiredSP: 8},
			{ID: "job-8-entry-4", Name: "random-poison", RequiredSP: 12},
			{ID: "job-8-entry-5", Name: "group-taunt", RequiredSP: 18},
			{ID: "job-8-entry-6", Name: "target-taunt", RequiredSP: 24},
			{ID: "job-8-entry-7", Name: "incapacitating-joke", RequiredSP: 36},
			{ID: "job-8-entry-8", Name: "enemy-calm", RequiredSP: 50},
		}
	case 9:
		return []CallbackEntry{
			{ID: "job-9-entry-0", Name: "agility-strike", Extended: true},
			{ID: "job-9-entry-1", Name: "reduce-agility", RequiredSP: 3, MPCost: 3},
			{ID: "job-9-entry-2", Name: "raise-agility", RequiredSP: 6, MPCost: 3},
			{ID: "job-9-entry-3", Name: "confusing-breath", RequiredSP: 12, MPCost: 7},
			{ID: "job-9-entry-4", Name: "member-content-inspection", RequiredSP: 20},
			{ID: "job-9-entry-5", Name: "sleep-breath", RequiredSP: 35, MPCost: 9},
			{ID: "job-9-entry-6", Name: "member-stat-inspection", RequiredSP: 50, MPCost: 1},
			{ID: "job-9-entry-7", Name: "paralyzing-strike", RequiredSP: 70, MPCost: 8},
			{ID: "job-9-entry-8", Name: "armor-break", RequiredSP: 90, MPCost: 15},
			{ID: "job-9-entry-9", Name: "agility-drain", RequiredSP: 90, MPCost: 20, Extended: true},
		}
	}
	return nil
}

func incidentalJobOwns(job, pos int) bool {
	return job == 7 && (pos == 1 || pos == 3 || pos >= 5 && pos <= 7) ||
		job == 8 && pos > 0 || job == 9 && pos != 4 && pos != 6
}
