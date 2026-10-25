package skill

import (
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

// RegisterProtectiveMagic binds nineteen bodies and complete job32/33/34 tables.
// RegisterRevivals retains ownership of32.4/33.6, including either binding order.
func (d *BattleDefinitions) RegisterProtectiveMagic(c *corebattle.ProtectiveMagicContext) {
	if d.Invocations.Entries == nil {
		d.Invocations.Entries = make(map[string]corebattle.CallbackDefinition)
	}
	if d.Invocations.JobTables == nil {
		d.Invocations.JobTables = make(map[int]corebattle.CallbackTable)
	}
	for _, job := range []int{32, 33, 34} {
		entries := corebattle.ProtectiveMagicEntries(job)
		d.Invocations.JobTables[job] = corebattle.CallbackTable{Variants: [][]corebattle.CallbackEntry{entries}}
		for _, entry := range entries {
			if entry.ID == "job-32-entry-4" || entry.ID == "job-33-entry-6" {
				continue
			}
			d.Invocations.Entries[entry.ID] = protectiveMagicInvocation(c, entry.ID)
		}
	}
}

func protectiveMagicInvocation(c *corebattle.ProtectiveMagicContext, id string) corebattle.CallbackDefinition {
	context := func(f *corebattle.CallbackContext) (corebattle.ProtectiveMagicContext, error) {
		if c == nil {
			return corebattle.ProtectiveMagicContext{}, corebattle.ErrUnsupportedEffect
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
			if f.AfterEffects && id != "job-32-entry-2" && id != "job-32-entry-3" && id != "job-33-entry-0" && id != "job-33-entry-2" && id != "job-34-entry-4" {
				return corebattle.ErrUnsupportedEffect
			}
			return s.ValidateProtectiveMagic(&child, id, random.Default())
		},
		Project: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext) (bool, error) {
			child, err := context(f)
			if err != nil {
				return false, err
			}
			return s.ProjectProtectiveMagic(&child, id)
		},
		Execute: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext, rng random.Generator) error {
			if c == nil {
				return corebattle.ErrUnsupportedEffect
			}
			c.TargetContext, c.Arguments = f.TargetContext, append([]string(nil), f.Arguments...)
			err := s.ApplyProtectiveMagic(c, id, rng)
			f.TargetContext, f.Arguments = c.TargetContext, c.Arguments
			return err
		},
	}
}
