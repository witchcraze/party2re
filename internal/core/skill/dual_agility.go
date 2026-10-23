package skill

import (
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

// RegisterDualAgility binds the complete job77/job83 tables and both captured
// variants. Consumers rebuild functions from saved facts and the frozen entry ID.
func (d *BattleDefinitions) RegisterDualAgility(c *corebattle.DualAgilityContext) {
	if d.Invocations.Entries == nil {
		d.Invocations.Entries = make(map[string]corebattle.CallbackDefinition)
	}
	if d.Invocations.JobTables == nil {
		d.Invocations.JobTables = make(map[int]corebattle.CallbackTable)
	}
	for _, job := range []int{77, 83} {
		table := corebattle.CallbackTable{Variants: [][]corebattle.CallbackEntry{corebattle.DualAgilityEntries(job, false)}}
		if job == 77 {
			table.Variants = append(table.Variants, corebattle.DualAgilityEntries(job, true))
			table.Select = func(s *corebattle.LifetimeState, frame *corebattle.CallbackContext) int {
				for _, a := range s.Actors {
					if a.ID == frame.ActorID && a.Temporary != "" {
						return 1
					}
				}
				return 0
			}
		}
		d.Invocations.JobTables[job] = table
		for _, entries := range table.Variants {
			for _, entry := range entries {
				d.Invocations.Entries[entry.ID] = dualAgilityInvocation(c, entry.ID)
			}
		}
	}
}

func dualAgilityInvocation(c *corebattle.DualAgilityContext, id string) corebattle.CallbackDefinition {
	context := func(frame *corebattle.CallbackContext) (corebattle.DualAgilityContext, error) {
		if c == nil {
			return corebattle.DualAgilityContext{}, corebattle.ErrUnsupportedEffect
		}
		child := *c
		child.TargetContext = frame.TargetContext
		child.Arguments = append([]string(nil), frame.Arguments...)
		return child, nil
	}
	return corebattle.CallbackDefinition{
		Validate: func(s *corebattle.LifetimeState, frame *corebattle.CallbackContext) error {
			child, err := context(frame)
			if err != nil {
				return err
			}
			if frame.AfterEffects && id != "job-83-entry-1" && id != "job-83-entry-4" {
				return corebattle.ErrUnsupportedEffect
			}
			return s.ValidateDualAgility(&child, id, random.Default())
		},
		Project: func(s *corebattle.LifetimeState, frame *corebattle.CallbackContext) (bool, error) {
			child, err := context(frame)
			if err != nil {
				return false, err
			}
			return s.ProjectDualAgility(&child, id)
		},
		Execute: func(s *corebattle.LifetimeState, frame *corebattle.CallbackContext, rng random.Generator) error {
			if c == nil {
				return corebattle.ErrUnsupportedEffect
			}
			c.TargetContext = frame.TargetContext
			c.Arguments = append([]string(nil), frame.Arguments...)
			err := s.ApplyDualAgility(c, id, rng)
			frame.TargetContext, frame.Arguments = c.TargetContext, c.Arguments
			return err
		},
	}
}
