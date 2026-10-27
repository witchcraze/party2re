package battle

// WeaponWeatherStanceEntries retains source order (_skill.cgi:2272–2289,
// 2408–2445,2455–2486), including the separately owned wallet position28.3.
func WeaponWeatherStanceEntries(job int) []CallbackEntry {
	switch job {
	case 24:
		return []CallbackEntry{
			{ID: "job-24-entry-0", Name: "flame-strike", RequiredSP: 10, MPCost: 6},
			{ID: "job-24-entry-1", Name: "metal-strike", RequiredSP: 30, MPCost: 6},
			{ID: "job-24-entry-2", Name: "attack-double", RequiredSP: 50, MPCost: 16},
			{ID: "job-24-entry-3", Name: "lightning-strike", RequiredSP: 70, MPCost: 14},
			{ID: "job-24-entry-4", Name: "physical-230", RequiredSP: 130, MPCost: 25},
			{ID: "job-24-entry-5", Name: "weapon-enchantment", RequiredSP: 130, MPCost: 15, Extended: true},
		}
	case 27:
		return []CallbackEntry{
			{ID: "job-27-entry-0", Name: "hit-down-wave", RequiredSP: 5, MPCost: 6},
			{ID: "job-27-entry-1", Name: "neutral-80", RequiredSP: 15, MPCost: 5},
			{ID: "job-27-entry-2", Name: "agility-down-wave", RequiredSP: 25, MPCost: 6},
			{ID: "job-27-entry-3", Name: "magic-wave-80", RequiredSP: 40, MPCost: 11},
			{ID: "job-27-entry-4", Name: "sleep-cure", RequiredSP: 55, MPCost: 4},
			{ID: "job-27-entry-5", Name: "party-breath-reflection", RequiredSP: 70, MPCost: 5},
			{ID: "job-27-entry-6", Name: "magic-wave-160", RequiredSP: 90, MPCost: 27},
			{ID: "job-27-entry-7", Name: "weapon-break", RequiredSP: 110, MPCost: 15},
			{ID: "job-27-entry-8", Name: "party-heal-cure", RequiredSP: 110, MPCost: 45, Extended: true},
		}
	case 28:
		return []CallbackEntry{
			{ID: "job-28-entry-0", Name: "accuracy-restore", RequiredSP: 5},
			{ID: "job-28-entry-1", Name: "immobilization", RequiredSP: 15, MPCost: 3},
			{ID: "job-28-entry-2", Name: "deflection", RequiredSP: 30, MPCost: 5},
			{ID: "job-28-entry-3", Name: "wallet-wave", RequiredSP: 50},
			{ID: "job-28-entry-4", Name: "physical-immunity", RequiredSP: 70, MPCost: 4},
			{ID: "job-28-entry-5", Name: "immobilization-wave", RequiredSP: 100, MPCost: 20},
			{ID: "job-28-entry-6", Name: "physical-death", RequiredSP: 140, MPCost: 10},
		}
	}
	return nil
}
