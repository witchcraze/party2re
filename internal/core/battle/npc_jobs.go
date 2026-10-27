package battle

// NPCJobEntries retains source positions, including duplicate-name weights.
// The NPC selector binds names to the last eligible entry; it must not dedupe
// this list or treat it as the full NPC catalog. Evidence: _skill.cgi:7183–7189,
// 7409–7412,8077–8121; _npc_action.cgi:177–224,313–322.
func NPCJobEntries(job int) []CallbackEntry {
	switch job {
	case 95:
		return []CallbackEntry{
			{ID: "npc-job-95-position-0", Name: "stage-summon", MPCost: 50},
			{ID: "npc-job-95-position-1", Name: "stage-summon", MPCost: 50},
		}
	case 99:
		return []CallbackEntry{{ID: "npc-job-99-position-0", Name: "escape"}}
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
