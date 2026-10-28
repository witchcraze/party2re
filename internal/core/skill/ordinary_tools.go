package skill

import (
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

// RegisterOrdinaryTools binds 27 whole tool bodies and six shared inspection
// producers. Full job tables and durable instance discovery remain consumer-owned.
func (d *BattleDefinitions) RegisterOrdinaryTools(c *corebattle.OrdinaryToolContext) {
	if d.Invocations.Entries == nil {
		d.Invocations.Entries = make(map[string]corebattle.CallbackDefinition)
	}
	for _, entry := range corebattle.OrdinaryToolEntries() {
		d.Invocations.Entries[entry.ID] = ordinaryToolInvocation(c, entry.ID, true)
	}
	for _, entry := range corebattle.InspectionEntries() {
		d.Invocations.Entries[entry.ID] = ordinaryToolInvocation(c, entry.ID, false)
	}
}

func ordinaryToolInvocation(c *corebattle.OrdinaryToolContext, id string, tool bool) corebattle.CallbackDefinition {
	return corebattle.CallbackDefinition{
		Validate: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext) error {
			if c == nil || f.AfterEffects && id != "tool-130" && id != "job-9-entry-6" && id != "job-16-entry-1" {
				return corebattle.ErrUnsupportedEffect
			}
			child := *c
			child.TargetContext, child.Arguments = f.TargetContext, append([]string(nil), f.Arguments...)
			return s.ValidateOrdinaryTool(&child, id, random.Default())
		},
		Project: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext) (bool, error) {
			// Inspection and comment bodies leave combat and cursor unchanged.
			return !tool || id == "tool-130" || id == "tool-151", nil
		},
		Execute: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext, rng random.Generator) error {
			if c == nil {
				return corebattle.ErrUnsupportedEffect
			}
			c.TargetContext, c.Arguments = f.TargetContext, append([]string(nil), f.Arguments...)
			var err error
			if tool {
				err = s.ApplyOrdinaryTool(c, id, rng)
			} else {
				err = s.ApplyInspection(c, id, rng)
			}
			f.TargetContext, f.Arguments = c.TargetContext, c.Arguments
			return err
		},
	}
}
