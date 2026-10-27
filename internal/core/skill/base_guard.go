package skill

import (
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

// RegisterBaseGuard binds job0–3 in source order, preserving revival3.6's owner
// in either registration order. Ports are rebuilt from saved operation facts.
func (d *BattleDefinitions) RegisterBaseGuard(c *corebattle.BaseGuardContext) {
	if d.Invocations.Entries == nil {
		d.Invocations.Entries = make(map[string]corebattle.CallbackDefinition)
	}
	if d.Invocations.JobTables == nil {
		d.Invocations.JobTables = make(map[int]corebattle.CallbackTable)
	}
	for job := 0; job <= 3; job++ {
		entries := corebattle.BaseGuardEntries(job)
		d.Invocations.JobTables[job] = corebattle.CallbackTable{Variants: [][]corebattle.CallbackEntry{entries}}
		for _, entry := range entries {
			if entry.ID != "job-3-entry-6" {
				d.Invocations.Entries[entry.ID] = baseGuardInvocation(c, entry.ID)
			}
		}
	}
}

func baseGuardInvocation(c *corebattle.BaseGuardContext, id string) corebattle.CallbackDefinition {
	context := func(f *corebattle.CallbackContext) (corebattle.BaseGuardContext, error) {
		if c == nil {
			return corebattle.BaseGuardContext{}, corebattle.ErrUnsupportedEffect
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
			if f.AfterEffects && id != "job-0-entry-4" && id != "job-2-entry-3" && id != "job-3-entry-4" {
				return corebattle.ErrUnsupportedEffect
			}
			return s.ValidateBaseGuard(&child, id, random.Default())
		},
		Project: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext) (bool, error) {
			child, err := context(f)
			if err != nil {
				return false, err
			}
			return s.ProjectBaseGuard(&child, id)
		},
		Execute: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext, rng random.Generator) error {
			if c == nil {
				return corebattle.ErrUnsupportedEffect
			}
			c.TargetContext, c.Arguments = f.TargetContext, append([]string(nil), f.Arguments...)
			err := s.ApplyBaseGuard(c, id, rng)
			f.TargetContext, f.Arguments = c.TargetContext, c.Arguments
			return err
		},
	}
}
