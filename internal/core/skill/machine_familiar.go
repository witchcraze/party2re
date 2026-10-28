package skill

import (
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

// RegisterMachineFamiliar contributes full job79/81/82 tables and binds eleven
// intrinsic bodies. Transformation, time, inspection, wallet and generated
// callbacks retain their existing owners, regardless of registration order.
func (d *BattleDefinitions) RegisterMachineFamiliar(c *corebattle.MachineFamiliarContext) {
	if d.Invocations.Entries == nil {
		d.Invocations.Entries = make(map[string]corebattle.CallbackDefinition)
	}
	if d.Invocations.JobTables == nil {
		d.Invocations.JobTables = make(map[int]corebattle.CallbackTable)
	}
	forms := &corebattle.TransformationContext{}
	if c != nil {
		forms.RequesterGender = c.RequesterGender
	}
	tables := transformationTables(forms)
	tables[81] = corebattle.CallbackTable{Variants: [][]corebattle.CallbackEntry{corebattle.FamiliarEntries()}}
	for _, job := range []int{79, 81, 82} {
		table := tables[job]
		d.Invocations.JobTables[job] = table
		for _, entry := range table.Variants[0] {
			switch entry.ID {
			case "job-79-entry-0", "job-79-entry-1", "job-79-entry-4",
				"job-81-entry-0", "job-81-entry-3", "job-81-entry-4",
				"job-82-entry-1", "job-82-entry-2", "job-82-entry-3", "job-82-entry-4", "job-82-entry-5":
				d.Invocations.Entries[entry.ID] = machineFamiliarInvocation(c, entry.ID)
			}
		}
	}
}

func machineFamiliarInvocation(c *corebattle.MachineFamiliarContext, id string) corebattle.CallbackDefinition {
	return corebattle.CallbackDefinition{
		Validate: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext) error {
			if c == nil || f.AfterEffects {
				return corebattle.ErrUnsupportedEffect
			}
			child := *c
			child.TargetContext, child.Arguments = f.TargetContext, append([]string(nil), f.Arguments...)
			return s.ValidateMachineFamiliar(&child, id, random.Default())
		},
		Execute: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext, rng random.Generator) error {
			if c == nil {
				return corebattle.ErrUnsupportedEffect
			}
			c.TargetContext, c.Arguments = f.TargetContext, append([]string(nil), f.Arguments...)
			err := s.ApplyMachineFamiliar(c, id, rng)
			f.TargetContext, f.Arguments = c.TargetContext, c.Arguments
			return err
		},
	}
}
