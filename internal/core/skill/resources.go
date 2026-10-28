package skill

import (
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

// RegisterResources binds intrinsic bodies only, without a partial job50 table
// or scalar batch effects. Rebuild operation closures after loading run facts.
func (d *BattleDefinitions) RegisterResources(c *corebattle.DamageContext) {
	if d.Invocations.Entries == nil {
		d.Invocations.Entries = make(map[string]corebattle.CallbackDefinition)
	}
	for _, entry := range corebattle.ResourceEntries() {
		d.Invocations.Entries[entry.ID] = resourceInvocation(c, entry)
	}
}

func resourceInvocation(c *corebattle.DamageContext, e corebattle.ResourceEntry) corebattle.CallbackDefinition {
	target := func(f *corebattle.CallbackContext) string {
		if len(f.Arguments) > 0 {
			return f.Arguments[0]
		}
		return ""
	}
	return corebattle.CallbackDefinition{
		Validate: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext) error {
			if c == nil || (f.AfterEffects && e.SourceID != 103 && e.SourceID != 104) {
				return corebattle.ErrUnsupportedEffect
			}
			child := *c
			child.TargetContext = f.TargetContext
			return s.ValidateResource(&child, e.ID, target(f), random.Default())
		},
		Project: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext) (bool, error) {
			if c == nil {
				return false, corebattle.ErrUnsupportedEffect
			}
			child := *c
			child.TargetContext = f.TargetContext
			return s.ProjectResource(&child, e.ID), nil
		},
		Execute: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext, rng random.Generator) error {
			if c == nil {
				return corebattle.ErrUnsupportedEffect
			}
			c.TargetContext = f.TargetContext
			_, err := s.ApplyResource(c, e.ID, target(f), rng)
			if err == nil && len(f.Arguments) > 0 && (e.TargetScope == corebattle.TargetScopeSingleAlly || e.TargetScope == corebattle.TargetScopeSingleEnemy) {
				f.Arguments = f.Arguments[1:]
			}
			f.TargetContext = c.TargetContext
			return err
		},
	}
}
