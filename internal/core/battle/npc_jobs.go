package battle

// NPCJobEntries retains source positions, including duplicate-name weights.
// The NPC selector binds names to the last eligible entry; it must not dedupe
// this list or treat it as the full NPC catalog. Evidence: _skill.cgi:7183–7189,
// 7215–7412,8077–8121; _npc_action.cgi:177–224,313–322.
func NPCJobEntries(job int) []CallbackEntry {
	switch job {
	case 95:
		return []CallbackEntry{
			{ID: "npc-job-95-position-0", Name: "stage-summon", MPCost: 50},
			{ID: "npc-job-95-position-1", Name: "stage-summon", MPCost: 50},
		}
	case 99:
		return []CallbackEntry{{ID: "npc-job-99-position-0", Name: "escape"}}
	case 97:
		return []CallbackEntry{
			{ID: "npc-job-97-position-0", Name: "scattered-strikes", MPCost: 24},
			{ID: "npc-job-97-position-1", Name: "burst-strikes", MPCost: 20},
			{ID: "npc-job-97-position-2", Name: "effect-dispel", MPCost: 30},
			{ID: "npc-job-97-position-3", Name: "defense-for-strength", MPCost: 9},
			{ID: "npc-job-97-position-4", Name: "reckless-wave", MPCost: 30},
			{ID: "npc-job-97-position-5", Name: "recoil-strike", MPCost: 40},
			{ID: "npc-job-97-position-6", Name: "vulnerable-charge", MPCost: 33},
			{ID: "npc-job-97-position-7", Name: "dark-breath-wave", MPCost: 80},
			{ID: "npc-job-97-position-8", Name: "paralyzing-magic-wave", MPCost: 70},
			{ID: "npc-job-97-position-9", Name: "poisoned-life-reduction", MPCost: 42},
			{ID: "npc-job-97-position-10", Name: "sleeping-magic-wave", MPCost: 66},
		}
	case 98:
		return []CallbackEntry{
			{ID: "npc-job-98-position-0", Name: "life-to-one", MPCost: 44},
			{ID: "npc-job-98-position-1", Name: "effect-dispel", MPCost: 30},
			{ID: "npc-job-98-position-2", Name: "party-magic-reflection", MPCost: 30},
			{ID: "npc-job-98-position-3", Name: "fortified-magic-barrier", MPCost: 23},
			{ID: "npc-job-98-position-4", Name: "vulnerable-charge", MPCost: 44},
			{ID: "npc-job-98-position-5", Name: "life-to-one", MPCost: 44},
			{ID: "npc-job-98-position-6", Name: "draining-rain", MPCost: 40},
			{ID: "npc-job-98-position-7", Name: "vulnerable-breath", MPCost: 40},
			{ID: "npc-job-98-position-8", Name: "dark-breath-wave", MPCost: 80},
			{ID: "npc-job-98-position-9", Name: "meteor-flurry", MPCost: 99},
			{ID: "npc-job-98-position-10", Name: "great-magic-wave", MPCost: 99},
		}
	case 403:
		return []CallbackEntry{
			{ID: "npc-job-403-position-0", Name: "neutral-wave", RequiredSP: 10, MPCost: 34},
			{ID: "npc-job-403-position-1", Name: "reducing-wall", RequiredSP: 40, MPCost: 27},
			{ID: "npc-job-403-position-2", Name: "three-pronged-strike", RequiredSP: 70, MPCost: 40},
			{ID: "npc-job-403-position-3", Name: "cleansing-vortex", RequiredSP: 130, MPCost: 76},
		}
	}
	return nil
}

func npcRosterCommand(id string) string {
	switch id {
	case "npc-job-95-position-0", "npc-job-95-position-1":
		return "summon"
	case "npc-job-99-position-0":
		return "escape"
	}
	return ""
}
