package skill

import (
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

// RegisterMartial binds complete job4/23/25 tables using supplied operation ports.
func (d *BattleDefinitions) RegisterMartial(c *corebattle.MartialContext) {
	if d.Invocations.Entries == nil {
		d.Invocations.Entries = make(map[string]corebattle.CallbackDefinition)
	}
	if d.Invocations.JobTables == nil {
		d.Invocations.JobTables = make(map[int]corebattle.CallbackTable)
	}
	for _, job := range []int{4, 23, 25} {
		entries := corebattle.MartialEntries(job)
		d.Invocations.JobTables[job] = corebattle.CallbackTable{Variants: [][]corebattle.CallbackEntry{entries}}
		for _, entry := range entries {
			d.Invocations.Entries[entry.ID] = martialInvocation(c, entry.ID)
		}
	}
}

func martialInvocation(c *corebattle.MartialContext, id string) corebattle.CallbackDefinition {
	context := func(f *corebattle.CallbackContext) (corebattle.MartialContext, error) {
		if c == nil {
			return corebattle.MartialContext{}, corebattle.ErrUnsupportedEffect
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
			if f.AfterEffects && id != "job-4-entry-1" && id != "job-25-entry-3" && id != "job-25-entry-4" && id != "job-25-entry-6" && id != "job-25-entry-7" {
				return corebattle.ErrUnsupportedEffect
			}
			return s.ValidateMartial(&child, id, random.Default())
		},
		Project: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext) (bool, error) {
			child, err := context(f)
			if err != nil {
				return false, err
			}
			return s.ProjectMartial(&child, id)
		},
		Execute: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext, rng random.Generator) error {
			if c == nil {
				return corebattle.ErrUnsupportedEffect
			}
			c.TargetContext, c.Arguments = f.TargetContext, append([]string(nil), f.Arguments...)
			err := s.ApplyMartial(c, id, rng)
			f.TargetContext, f.Arguments = c.TargetContext, c.Arguments
			return err
		},
	}
}
