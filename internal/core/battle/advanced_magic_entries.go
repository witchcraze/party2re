package battle

// AdvancedMagicEntries preserves source order at _skill.cgi:1764–2031.
// Inspection16.1 and quarter-revival16.6 retain their separate owners.
func AdvancedMagicEntries(job int) []CallbackEntry {
	switch job {
	case 15:
		return []CallbackEntry{
			{ID: "job-15-entry-0", Name: "poison-strike", RequiredSP: 5, MPCost: 6},
			{ID: "job-15-entry-1", Name: "magic-35", RequiredSP: 10, MPCost: 5},
			{ID: "job-15-entry-2", Name: "sleep-55", RequiredSP: 20, MPCost: 8},
			{ID: "job-15-entry-3", Name: "magic-reflection", RequiredSP: 40, MPCost: 7},
			{ID: "job-15-entry-4", Name: "mp-theft-20", RequiredSP: 60, MPCost: 3},
			{ID: "job-15-entry-5", Name: "health-drain", RequiredSP: 80, MPCost: 16},
			{ID: "job-15-entry-6", Name: "magic-260", RequiredSP: 130, MPCost: 40},
			{ID: "job-15-entry-7", Name: "tension-magic-500", RequiredSP: 130, MPCost: 64, Extended: true},
		}
	case 16:
		return []CallbackEntry{
			{ID: "job-16-entry-0", Name: "heal-60", RequiredSP: 5, MPCost: 5},
			{ID: "job-16-entry-1", Name: "inspect-stats", RequiredSP: 10, MPCost: 2},
			{ID: "job-16-entry-2", Name: "magic-seal-90", RequiredSP: 15, MPCost: 6},
			{ID: "job-16-entry-3", Name: "heal-180", RequiredSP: 30, MPCost: 18},
			{ID: "job-16-entry-4", Name: "revival-guard", RequiredSP: 60, MPCost: 20},
			{ID: "job-16-entry-5", Name: "magic-mitigation", RequiredSP: 80, MPCost: 5},
			{ID: "job-16-entry-6", Name: "quarter-revival", RequiredSP: 100, MPCost: 40},
			{ID: "job-16-entry-7", Name: "magic-180", RequiredSP: 140, MPCost: 35},
			{ID: "job-16-entry-8", Name: "physical-immunity-party-heal", RequiredSP: 140, MPCost: 46, Extended: true},
		}
	case 19:
		return []CallbackEntry{
			{ID: "job-19-entry-0", Name: "enemy-defense-down", RequiredSP: 4, MPCost: 7},
			{ID: "job-19-entry-1", Name: "magic-reflection", RequiredSP: 8, MPCost: 5},
			{ID: "job-19-entry-2", Name: "area-confusion-50", RequiredSP: 16, MPCost: 10},
			{ID: "job-19-entry-3", Name: "instant-death-20", RequiredSP: 25, MPCost: 14},
			{ID: "job-19-entry-4", Name: "area-magic-seal-70", RequiredSP: 40, MPCost: 10},
			{ID: "job-19-entry-5", Name: "magic-wave-125", RequiredSP: 60, MPCost: 18},
			{ID: "job-19-entry-6", Name: "mp-theft-25", RequiredSP: 70, MPCost: 2},
			{ID: "job-19-entry-7", Name: "area-instant-death-20", RequiredSP: 110, MPCost: 32},
			{ID: "job-19-entry-8", Name: "mixed-death-magic", RequiredSP: 110, MPCost: 88, Extended: true},
		}
	}
	return nil
}
