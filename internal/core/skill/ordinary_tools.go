package skill

import (
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

// RegisterOrdinaryTools binds whole tool bodies and six shared inspection
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

func mergeInspection(base, overlay map[string]corebattle.InspectionContent) map[string]corebattle.InspectionContent {
	if len(base) == 0 {
		return overlay
	}
	merged := make(map[string]corebattle.InspectionContent, len(base)+len(overlay))
	for k, v := range base {
		merged[k] = v
	}
	for k, v := range overlay {
		merged[k] = v
	}
	return merged
}

func ordinaryToolInvocation(c *corebattle.OrdinaryToolContext, id string, tool bool) corebattle.CallbackDefinition {
	return corebattle.CallbackDefinition{
		Validate: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext) error {
			if c == nil {
				return corebattle.ErrUnsupportedEffect
			}
			child := *c
			child.TargetContext, child.Arguments = f.TargetContext, append([]string(nil), f.Arguments...)
			child.Inspection = mergeInspection(f.Inspection, c.Inspection)
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
			savedInspection := c.Inspection
			c.TargetContext, c.Arguments = f.TargetContext, append([]string(nil), f.Arguments...)
			c.Inspection = mergeInspection(f.Inspection, savedInspection)
			defer func() { c.Inspection = savedInspection }()
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
