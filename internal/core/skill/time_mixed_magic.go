package skill

import (
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

// RegisterTimeMixedMagic contributes complete tables without certifying the
// separately owned job29 field or job31 arithmetic bodies. Registration order
// with RegisterArithmetic does not change identities, bindings or table order.
func (d *BattleDefinitions) RegisterTimeMixedMagic(c *corebattle.TimeMixedMagicContext) {
	if d.Invocations.Entries == nil {
		d.Invocations.Entries = make(map[string]corebattle.CallbackDefinition)
	}
	if d.Invocations.JobTables == nil {
		d.Invocations.JobTables = make(map[int]corebattle.CallbackTable)
	}
	for _, job := range []int{29, 30, 31} {
		entries := corebattle.TimeMixedMagicEntries(job)
		d.Invocations.JobTables[job] = corebattle.CallbackTable{Variants: [][]corebattle.CallbackEntry{entries}}
		for _, entry := range entries {
			if entry.ID == "job-29-entry-7" || job == 31 && (entry.ID == "job-31-gravity" || entry.ID == "job-31-death" || entry.ID == "job-31-reverse") {
				continue
			}
			d.Invocations.Entries[entry.ID] = timeMixedMagicInvocation(c, entry.ID)
		}
	}
}

func timeMixedMagicInvocation(c *corebattle.TimeMixedMagicContext, id string) corebattle.CallbackDefinition {
	return corebattle.CallbackDefinition{
		Validate: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext) error {
			if c == nil || f.AfterEffects {
				return corebattle.ErrUnsupportedEffect
			}
			child := *c
			child.TargetContext, child.Arguments = f.TargetContext, append([]string(nil), f.Arguments...)
			return s.ValidateTimeMixedMagic(&child, id, random.Default())
		},
		Execute: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext, rng random.Generator) error {
			if c == nil {
				return corebattle.ErrUnsupportedEffect
			}
			c.TargetContext, c.Arguments = f.TargetContext, append([]string(nil), f.Arguments...)
			err := s.ApplyTimeMixedMagic(c, id, rng)
			f.TargetContext, f.Arguments = c.TargetContext, c.Arguments
			return err
		},
	}
}
