package skill

import (
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

// RegisterAdvancedMagic binds24 bodies and ordered tables, retaining the
// inspection16.1 and revival16.6 bindings in either registration order.
func (d *BattleDefinitions) RegisterAdvancedMagic(c *corebattle.AdvancedMagicContext) {
	if d.Invocations.Entries == nil {
		d.Invocations.Entries = make(map[string]corebattle.CallbackDefinition)
	}
	if d.Invocations.JobTables == nil {
		d.Invocations.JobTables = make(map[int]corebattle.CallbackTable)
	}
	for _, job := range []int{15, 16, 19} {
		entries := corebattle.AdvancedMagicEntries(job)
		d.Invocations.JobTables[job] = corebattle.CallbackTable{Variants: [][]corebattle.CallbackEntry{entries}}
		for _, entry := range entries {
			if entry.ID != "job-16-entry-1" && entry.ID != "job-16-entry-6" {
				d.Invocations.Entries[entry.ID] = advancedMagicInvocation(c, entry.ID)
			}
		}
	}
}

func advancedMagicInvocation(c *corebattle.AdvancedMagicContext, id string) corebattle.CallbackDefinition {
	return corebattle.CallbackDefinition{
		Validate: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext) error {
			if c == nil || f.AfterEffects {
				return corebattle.ErrUnsupportedEffect
			}
			child := *c
			child.TargetContext, child.Arguments = f.TargetContext, append([]string(nil), f.Arguments...)
			return s.ValidateAdvancedMagic(&child, id, random.Default())
		},
		Execute: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext, rng random.Generator) error {
			if c == nil {
				return corebattle.ErrUnsupportedEffect
			}
			c.TargetContext, c.Arguments = f.TargetContext, append([]string(nil), f.Arguments...)
			err := s.ApplyAdvancedMagic(c, id, rng)
			f.TargetContext, f.Arguments = c.TargetContext, c.Arguments
			return err
		},
	}
}
