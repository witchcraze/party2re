package skill

import (
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

// RegisterTreatmentChance contributes twenty intrinsic bodies and complete
// ordered tables. RegisterRevivals retains43.6 in either registration order.
func (d *BattleDefinitions) RegisterTreatmentChance(c *corebattle.TreatmentChanceContext) {
	if d.Invocations.Entries == nil {
		d.Invocations.Entries = make(map[string]corebattle.CallbackDefinition)
	}
	if d.Invocations.JobTables == nil {
		d.Invocations.JobTables = make(map[int]corebattle.CallbackTable)
	}
	for _, job := range []int{43, 45, 46} {
		entries := corebattle.TreatmentChanceEntries(job)
		d.Invocations.JobTables[job] = corebattle.CallbackTable{Variants: [][]corebattle.CallbackEntry{entries}}
		for _, entry := range entries {
			if entry.ID != "job-43-entry-6" {
				d.Invocations.Entries[entry.ID] = treatmentChanceInvocation(c, entry.ID)
			}
		}
	}
}

func treatmentChanceInvocation(c *corebattle.TreatmentChanceContext, id string) corebattle.CallbackDefinition {
	return corebattle.CallbackDefinition{
		Validate: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext) error {
			if c == nil || f.AfterEffects {
				return corebattle.ErrUnsupportedEffect
			}
			child := *c
			child.TargetContext, child.Arguments = f.TargetContext, append([]string(nil), f.Arguments...)
			return s.ValidateTreatmentChance(&child, id, random.Default())
		},
		Execute: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext, rng random.Generator) error {
			if c == nil {
				return corebattle.ErrUnsupportedEffect
			}
			c.TargetContext, c.Arguments = f.TargetContext, append([]string(nil), f.Arguments...)
			err := s.ApplyTreatmentChance(c, id, rng)
			f.TargetContext, f.Arguments = c.TargetContext, c.Arguments
			return err
		},
	}
}
