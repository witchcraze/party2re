package skill

import (
	"maps"

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

func mergeOrdinaryInspection(base, overlay map[string]corebattle.InspectionContent) map[string]corebattle.InspectionContent {
	if len(overlay) == 0 {
		return base
	}
	merged := maps.Clone(base)
	if merged == nil {
		merged = make(map[string]corebattle.InspectionContent, len(overlay))
	}
	for id, content := range overlay {
		if _, exists := merged[id]; exists && content == (corebattle.InspectionContent{Empty: true}) {
			continue
		}
		merged[id] = content
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
			child.Inspection = mergeOrdinaryInspection(c.Inspection, f.Inspection)
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
			rollbackContext(f, c)
			c.TargetContext, c.Arguments = f.TargetContext, append([]string(nil), f.Arguments...)
			c.Inspection = mergeOrdinaryInspection(c.Inspection, f.Inspection)
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
