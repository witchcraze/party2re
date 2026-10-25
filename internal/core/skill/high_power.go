package skill

import (
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

// RegisterHighPower contributes all twenty ordered bodies without substituting
// ordinary learning metadata for explicit whole-entry certification.
func (d *BattleDefinitions) RegisterHighPower(c *corebattle.HighPowerContext) {
	if d.Invocations.Entries == nil {
		d.Invocations.Entries = make(map[string]corebattle.CallbackDefinition)
	}
	if d.Invocations.JobTables == nil {
		d.Invocations.JobTables = make(map[int]corebattle.CallbackTable)
	}
	for _, job := range []int{47, 48, 49, 52} {
		entries := corebattle.HighPowerEntries(job)
		d.Invocations.JobTables[job] = corebattle.CallbackTable{Variants: [][]corebattle.CallbackEntry{entries}}
		for _, entry := range entries {
			d.Invocations.Entries[entry.ID] = highPowerInvocation(c, entry.ID)
		}
	}
}

func highPowerInvocation(c *corebattle.HighPowerContext, id string) corebattle.CallbackDefinition {
	context := func(f *corebattle.CallbackContext) (corebattle.HighPowerContext, error) {
		if c == nil {
			return corebattle.HighPowerContext{}, corebattle.ErrUnsupportedEffect
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
			if f.AfterEffects && id != "job-47-entry-5" {
				return corebattle.ErrUnsupportedEffect
			}
			return s.ValidateHighPower(&child, id, random.Default())
		},
		Project: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext) (bool, error) {
			child, err := context(f)
			if err != nil {
				return false, err
			}
			return s.ProjectHighPower(&child, id)
		},
		Execute: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext, rng random.Generator) error {
			if c == nil {
				return corebattle.ErrUnsupportedEffect
			}
			c.TargetContext, c.Arguments = f.TargetContext, append([]string(nil), f.Arguments...)
			err := s.ApplyHighPower(c, id, rng)
			f.TargetContext, f.Arguments = c.TargetContext, c.Arguments
			return err
		},
	}
}
