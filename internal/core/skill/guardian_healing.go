package skill

import (
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

// RegisterGuardianHealing binds eleven bodies and full ordered job17/18 tables.
// RegisterRevivals owns17.4/17.6/18.4/18.6, regardless of binding order.
func (d *BattleDefinitions) RegisterGuardianHealing(c *corebattle.GuardianHealingContext) {
	if d.Invocations.Entries == nil {
		d.Invocations.Entries = make(map[string]corebattle.CallbackDefinition)
	}
	if d.Invocations.JobTables == nil {
		d.Invocations.JobTables = make(map[int]corebattle.CallbackTable)
	}
	for _, job := range []int{17, 18} {
		entries := corebattle.GuardianHealingEntries(job)
		d.Invocations.JobTables[job] = corebattle.CallbackTable{Variants: [][]corebattle.CallbackEntry{entries}}
		for _, entry := range entries {
			if entry.ID == "job-17-entry-4" || entry.ID == "job-17-entry-6" || entry.ID == "job-18-entry-4" || entry.ID == "job-18-entry-6" {
				continue
			}
			d.Invocations.Entries[entry.ID] = guardianHealingInvocation(c, entry.ID)
		}
	}
}

func guardianHealingInvocation(c *corebattle.GuardianHealingContext, id string) corebattle.CallbackDefinition {
	context := func(f *corebattle.CallbackContext) (corebattle.GuardianHealingContext, error) {
		if c == nil {
			return corebattle.GuardianHealingContext{}, corebattle.ErrUnsupportedEffect
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
			if f.AfterEffects && id != "job-17-entry-2" {
				return corebattle.ErrUnsupportedEffect
			}
			return s.ValidateGuardianHealing(&child, id, random.Default())
		},
		Project: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext) (bool, error) {
			child, err := context(f)
			if err != nil {
				return false, err
			}
			return s.ProjectGuardianHealing(&child, id)
		},
		Execute: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext, rng random.Generator) error {
			if c == nil {
				return corebattle.ErrUnsupportedEffect
			}
			c.TargetContext, c.Arguments = f.TargetContext, append([]string(nil), f.Arguments...)
			err := s.ApplyGuardianHealing(c, id, rng)
			f.TargetContext, f.Arguments = c.TargetContext, c.Arguments
			return err
		},
	}
}
