package skill

import (
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

// RegisterCounterReset binds eighteen intrinsic bodies. Status/reset and
// transformation registration retain35.1/36.4; their positions stay in the pool.
func (d *BattleDefinitions) RegisterCounterReset(c *corebattle.CounterResetContext) {
	if d.Invocations.Entries == nil {
		d.Invocations.Entries = make(map[string]corebattle.CallbackDefinition)
	}
	if d.Invocations.JobTables == nil {
		d.Invocations.JobTables = make(map[int]corebattle.CallbackTable)
	}
	for _, job := range []int{35, 36, 37} {
		entries := corebattle.CounterResetEntries(job)
		d.Invocations.JobTables[job] = corebattle.CallbackTable{Variants: [][]corebattle.CallbackEntry{entries}}
		for _, entry := range entries {
			if entry.ID == "job-35-entry-1" || entry.ID == "job-36-copy" {
				continue
			}
			d.Invocations.Entries[entry.ID] = counterResetInvocation(c, entry.ID)
		}
	}
}

func counterResetInvocation(c *corebattle.CounterResetContext, id string) corebattle.CallbackDefinition {
	context := func(f *corebattle.CallbackContext) (corebattle.CounterResetContext, error) {
		if c == nil {
			return corebattle.CounterResetContext{}, corebattle.ErrUnsupportedEffect
		}
		child := *c
		child.TargetContext, child.Arguments = f.TargetContext, append([]string(nil), f.Arguments...)
		return child, nil
	}
	return corebattle.CallbackDefinition{
		Validate: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext) error {
			child, err := context(f)
			if err != nil {
				return err
			}
			if f.AfterEffects && id != "job-35-entry-0" && id != "job-37-entry-3" && id != "job-36-entry-0" && id != "job-36-entry-1" && id != "job-36-entry-2" && id != "job-36-entry-3" {
				return corebattle.ErrUnsupportedEffect
			}
			return s.ValidateCounterReset(&child, id, random.Default())
		},
		Project: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext) (bool, error) {
			child, err := context(f)
			if err != nil {
				return false, err
			}
			return s.ProjectCounterReset(&child, id)
		},
		Execute: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext, rng random.Generator) error {
			if c == nil {
				return corebattle.ErrUnsupportedEffect
			}
			c.TargetContext, c.Arguments = f.TargetContext, append([]string(nil), f.Arguments...)
			err := s.ApplyCounterReset(c, id, rng)
			f.TargetContext, f.Arguments = c.TargetContext, c.Arguments
			return err
		},
	}
}
