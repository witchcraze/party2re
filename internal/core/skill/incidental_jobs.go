package skill

import (
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

// RegisterIncidentalJobs binds 21 bodies and contributes whole ordered tables.
// Wallet, inspection and sacrifice definitions retain their existing owners.
func (d *BattleDefinitions) RegisterIncidentalJobs(c *corebattle.IncidentalJobContext) {
	if d.Invocations.Entries == nil {
		d.Invocations.Entries = make(map[string]corebattle.CallbackDefinition)
	}
	if d.Invocations.JobTables == nil {
		d.Invocations.JobTables = make(map[int]corebattle.CallbackTable)
	}
	for job := 7; job <= 9; job++ {
		entries := corebattle.IncidentalJobEntries(job)
		d.Invocations.JobTables[job] = corebattle.CallbackTable{Variants: [][]corebattle.CallbackEntry{entries}}
		for pos, entry := range entries {
			if job == 7 && (pos == 0 || pos == 2 || pos == 4 || pos == 8) || job == 8 && pos == 0 || job == 9 && (pos == 4 || pos == 6) {
				continue
			}
			d.Invocations.Entries[entry.ID] = incidentalJobInvocation(c, entry.ID)
		}
	}
}

func incidentalJobInvocation(c *corebattle.IncidentalJobContext, id string) corebattle.CallbackDefinition {
	raw := id == "job-8-entry-2" || id == "job-8-entry-3" || id == "job-8-entry-4" || id == "job-8-entry-7" || id == "job-8-entry-8"
	return corebattle.CallbackDefinition{
		Validate: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext) error {
			if c == nil || f.AfterEffects && !raw {
				return corebattle.ErrUnsupportedEffect
			}
			child := *c
			child.TargetContext, child.Arguments = f.TargetContext, append([]string(nil), f.Arguments...)
			return s.ValidateIncidentalJob(&child, id, random.Default())
		},
		Project: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext) (bool, error) {
			if c == nil {
				return false, corebattle.ErrUnsupportedEffect
			}
			child := *c
			child.TargetContext, child.Arguments = f.TargetContext, append([]string(nil), f.Arguments...)
			return s.ProjectIncidentalJob(&child, id)
		},
		Execute: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext, rng random.Generator) error {
			if c == nil {
				return corebattle.ErrUnsupportedEffect
			}
			c.TargetContext, c.Arguments = f.TargetContext, append([]string(nil), f.Arguments...)
			err := s.ApplyIncidentalJob(c, id, rng)
			f.TargetContext, f.Arguments = c.TargetContext, c.Arguments
			return err
		},
	}
}
