package skill

import (
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

// RegisterTreatmentMischief contributes full tables and 29 intrinsic bodies.
// Revival57.4 and the other resource/asset/random bodies retain their owners.
func (d *BattleDefinitions) RegisterTreatmentMischief(c *corebattle.TreatmentMischiefContext) {
	if d.Invocations.Entries == nil {
		d.Invocations.Entries = make(map[string]corebattle.CallbackDefinition)
	}
	if d.Invocations.JobTables == nil {
		d.Invocations.JobTables = make(map[int]corebattle.CallbackTable)
	}
	for _, job := range []int{50, 51, 55, 56, 57, 78} {
		entries := corebattle.TreatmentMischiefEntries(job)
		d.Invocations.JobTables[job] = corebattle.CallbackTable{Variants: [][]corebattle.CallbackEntry{entries}}
		for pos, entry := range entries {
			if job == 50 && pos >= 5 || job == 51 && pos == 5 || job == 55 && pos >= 6 || job == 56 && (pos == 3 || pos == 5) || job == 57 && pos == 4 || job == 78 && pos != 3 {
				continue
			}
			d.Invocations.Entries[entry.ID] = treatmentMischiefInvocation(c, entry.ID)
		}
	}
}

func treatmentMischiefInvocation(c *corebattle.TreatmentMischiefContext, id string) corebattle.CallbackDefinition {
	return corebattle.CallbackDefinition{
		Validate: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext) error {
			if c == nil || f.AfterEffects && id != "job-51-entry-1" {
				return corebattle.ErrUnsupportedEffect
			}
			child := *c
			child.TargetContext, child.Arguments = f.TargetContext, append([]string(nil), f.Arguments...)
			return s.ValidateTreatmentMischief(&child, id, random.Default())
		},
		Project: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext) (bool, error) {
			if c == nil {
				return false, corebattle.ErrUnsupportedEffect
			}
			child := *c
			child.TargetContext = f.TargetContext
			return s.ProjectTreatmentMischief(&child, id), nil
		},
		Execute: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext, rng random.Generator) error {
			if c == nil {
				return corebattle.ErrUnsupportedEffect
			}
			c.TargetContext, c.Arguments = f.TargetContext, append([]string(nil), f.Arguments...)
			err := s.ApplyTreatmentMischief(c, id, rng)
			f.TargetContext, f.Arguments = c.TargetContext, c.Arguments
			return err
		},
	}
}
