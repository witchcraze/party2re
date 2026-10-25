package skill

import (
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

// RegisterControlClimax contributes full ordered tables while binding only this
// owner's 17 bodies. Field53.6 and revival70.3 retain their existing owners.
func (d *BattleDefinitions) RegisterControlClimax(c *corebattle.ControlClimaxContext) {
	if d.Invocations.Entries == nil {
		d.Invocations.Entries = make(map[string]corebattle.CallbackDefinition)
	}
	if d.Invocations.JobTables == nil {
		d.Invocations.JobTables = make(map[int]corebattle.CallbackTable)
	}
	for _, job := range []int{53, 58, 70} {
		entries := corebattle.ControlClimaxEntries(job)
		d.Invocations.JobTables[job] = corebattle.CallbackTable{Variants: [][]corebattle.CallbackEntry{entries}}
		for _, entry := range entries {
			if entry.ID == "job-53-entry-6" || entry.ID == "job-70-entry-3" {
				continue
			}
			d.Invocations.Entries[entry.ID] = controlClimaxInvocation(c, entry.ID)
		}
	}
}

func controlClimaxInvocation(c *corebattle.ControlClimaxContext, id string) corebattle.CallbackDefinition {
	return corebattle.CallbackDefinition{
		Validate: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext) error {
			if c == nil || f.AfterEffects {
				return corebattle.ErrUnsupportedEffect
			}
			child := *c
			child.TargetContext, child.Arguments = f.TargetContext, append([]string(nil), f.Arguments...)
			return s.ValidateControlClimax(&child, id, random.Default())
		},
		Execute: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext, rng random.Generator) error {
			if c == nil {
				return corebattle.ErrUnsupportedEffect
			}
			c.TargetContext, c.Arguments = f.TargetContext, append([]string(nil), f.Arguments...)
			err := s.ApplyControlClimax(c, id, rng)
			f.TargetContext, f.Arguments = c.TargetContext, c.Arguments
			return err
		},
	}
}
