package skill

import corebattle "github.com/witchcraze/party2re/internal/core/battle"

// RegisterNPCJobs binds only the complete job95 and job99 tables to the shared
// roster admission/executor. Rebuild closures after reload; NPC eligibility,
// name binding, selection, post-callback cost and tails belong to the consumer.
func (d *BattleDefinitions) RegisterNPCJobs(c *corebattle.RosterContext) {
	if d.RosterSkills == nil {
		d.RosterSkills = make(map[string]bool)
	}
	if d.Invocations.JobTables == nil {
		d.Invocations.JobTables = make(map[int]corebattle.CallbackTable)
	}
	if d.Invocations.Entries == nil {
		d.Invocations.Entries = make(map[string]corebattle.CallbackDefinition)
	}
	for _, job := range []int{95, 99} {
		entries := corebattle.NPCJobEntries(job)
		d.Invocations.JobTables[job] = corebattle.CallbackTable{Variants: [][]corebattle.CallbackEntry{entries}}
		for _, entry := range entries {
			d.RosterSkills[entry.ID] = true
			d.Invocations.Entries[entry.ID] = d.RosterInvocation(c, entry.ID)
		}
	}
}
