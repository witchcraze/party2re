package skill

import corebattle "github.com/witchcraze/party2re/internal/core/battle"

// RegisterCallingJobs contributes complete ordered calling tables, retaining
// unimplemented positions for their owners. Only the 88 certified roster bodies
// are bound here; missing descendants still reject through the shared gate.
func (d *BattleDefinitions) RegisterCallingJobs(c *corebattle.RosterContext) {
	if d.RosterSkills == nil {
		d.RosterSkills = make(map[string]bool)
	}
	if d.Invocations.JobTables == nil {
		d.Invocations.JobTables = make(map[int]corebattle.CallbackTable)
	}
	if d.Invocations.Entries == nil {
		d.Invocations.Entries = make(map[string]corebattle.CallbackDefinition)
	}
	for _, job := range []int{59, 60, 61, 62, 63, 64, 65, 66, 67, 68, 69, 71, 80, 87} {
		entries := corebattle.RosterEntries(job)
		d.Invocations.JobTables[job] = corebattle.CallbackTable{Variants: [][]corebattle.CallbackEntry{entries}}
		for _, entry := range entries {
			if entry.Name == "grave" || entry.Name == "revive" {
				continue
			}
			d.RosterSkills[entry.ID] = true
			d.Invocations.Entries[entry.ID] = d.RosterInvocation(c, entry.ID)
		}
	}
}
