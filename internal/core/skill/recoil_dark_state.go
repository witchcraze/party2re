package skill

import (
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

// RegisterRecoilDarkState contributes ordered tables21/22/38 and eighteen
// intrinsic bodies. The field owner retains22.5 and its registration.
func (d *BattleDefinitions) RegisterRecoilDarkState(c *corebattle.RecoilDarkStateContext) {
	if d.Invocations.Entries == nil {
		d.Invocations.Entries = make(map[string]corebattle.CallbackDefinition)
	}
	if d.Invocations.JobTables == nil {
		d.Invocations.JobTables = make(map[int]corebattle.CallbackTable)
	}
	for _, job := range []int{21, 22, 38} {
		entries := corebattle.RecoilDarkStateEntries(job)
		d.Invocations.JobTables[job] = corebattle.CallbackTable{Variants: [][]corebattle.CallbackEntry{entries}}
		for _, entry := range entries {
			if entry.ID != "job-22-entry-5" {
				d.Invocations.Entries[entry.ID] = recoilDarkStateInvocation(c, entry.ID)
			}
		}
	}
}

func recoilDarkStateInvocation(c *corebattle.RecoilDarkStateContext, id string) corebattle.CallbackDefinition {
	context := func(f *corebattle.CallbackContext) (corebattle.RecoilDarkStateContext, error) {
		if c == nil {
			return corebattle.RecoilDarkStateContext{}, corebattle.ErrUnsupportedEffect
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
			if f.AfterEffects && id != "job-21-entry-1" && id != "job-38-entry-2" {
				return corebattle.ErrUnsupportedEffect
			}
			return s.ValidateRecoilDarkState(&child, id, random.Default())
		},
		Project: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext) (bool, error) {
			child, err := context(f)
			if err != nil {
				return false, err
			}
			return s.ProjectRecoilDarkState(&child, id)
		},
		Execute: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext, rng random.Generator) error {
			if c == nil {
				return corebattle.ErrUnsupportedEffect
			}
			c.TargetContext, c.Arguments = f.TargetContext, append([]string(nil), f.Arguments...)
			err := s.ApplyRecoilDarkState(c, id, rng)
			f.TargetContext, f.Arguments = c.TargetContext, c.Arguments
			return err
		},
	}
}
