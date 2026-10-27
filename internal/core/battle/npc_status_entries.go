package battle

// Independent positions and original SP/listed MP at _skill.cgi:7043–7178.
// Job88 stage27 starts at SP20; later entries remain intrinsic callbacks only.
func npcStatusEntries(job int) []CallbackEntry {
	switch job {
	case 88:
		return []CallbackEntry{
			{ID: "npc-job-88-position-0", Name: "immobilizing-roar", RequiredSP: 5, MPCost: 12},
			{ID: "npc-job-88-position-1", Name: "wild-claw", RequiredSP: 20, MPCost: 24},
			{ID: "npc-job-88-position-2", Name: "grooming", RequiredSP: 60, MPCost: 15},
			{ID: "npc-job-88-position-3", Name: "idle-claw", RequiredSP: 110, MPCost: 38},
		}
	case 90:
		return []CallbackEntry{
			{ID: "npc-job-90-position-0", Name: "poison-strike", RequiredSP: 10, MPCost: 4},
			{ID: "npc-job-90-position-1", Name: "poison-magic", RequiredSP: 20, MPCost: 6},
			{ID: "npc-job-90-position-2", Name: "poison-mist", RequiredSP: 30, MPCost: 8},
		}
	case 91:
		return []CallbackEntry{
			{ID: "npc-job-91-position-0", Name: "paralyzing-strike", RequiredSP: 10, MPCost: 8},
			{ID: "npc-job-91-position-1", Name: "numbing-strike", RequiredSP: 20, MPCost: 11},
			{ID: "npc-job-91-position-2", Name: "paralyzing-breath", RequiredSP: 30, MPCost: 9},
		}
	case 92:
		return []CallbackEntry{
			{ID: "npc-job-92-position-0", Name: "sleep-magic", RequiredSP: 10, MPCost: 8},
			{ID: "npc-job-92-position-1", Name: "sleep-strike", RequiredSP: 20, MPCost: 15},
			{ID: "npc-job-92-position-2", Name: "sleep-breath", RequiredSP: 30, MPCost: 9},
		}
	case 93:
		return []CallbackEntry{
			{ID: "npc-job-93-position-0", Name: "instant-death", RequiredSP: 10, MPCost: 14},
			{ID: "npc-job-93-position-1", Name: "death-wave", RequiredSP: 20, MPCost: 32},
			{ID: "npc-job-93-position-2", Name: "death-dance", RequiredSP: 30, MPCost: 24},
			{ID: "npc-job-93-position-3", Name: "poisoned-life-reduction", RequiredSP: 40, MPCost: 42},
		}
	case 94:
		return []CallbackEntry{
			{ID: "npc-job-94-position-0", Name: "death-sacrifice", RequiredSP: 10, MPCost: 1},
			{ID: "npc-job-94-position-1", Name: "rest", RequiredSP: 20},
		}
	}
	return nil
}
