package skill

import (
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

// RegisterJobAssets binds only the intrinsic wallet, stored/enhanced-tool and scavenging
// positions. Other table owners retain their bodies and full source order.
func (d *BattleDefinitions) RegisterJobAssets(c *corebattle.JobAssetContext) {
	if d.Invocations.Entries == nil {
		d.Invocations.Entries = make(map[string]corebattle.CallbackDefinition)
	}
	for _, id := range []string{"job-7-entry-0", "job-7-entry-2", "job-8-entry-0", "job-28-entry-3", "job-50-entry-7", "job-78-entry-0", "job-78-entry-5", "job-81-entry-2"} {
		d.Invocations.Entries[id] = corebattle.CallbackDefinition{
			Validate: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext) error {
				// shortcut: #1733 owns projections after arbitrary preceding callbacks.
				if c == nil || f.AfterEffects {
					return corebattle.ErrUnsupportedEffect
				}
				child := *c
				child.TargetContext, child.Arguments, child.Assets = f.TargetContext, append([]string(nil), f.Arguments...), f.Assets
				child.Frame, child.Catalog = f, d.Invocations
				if f.Defeat != nil {
					child.DefeatContext = f.Defeat
				}
				if f.Damage != nil {
					child.DamageContext = *f.Damage
					child.TargetContext = f.TargetContext
				}
				return s.ValidateJobAssets(&child, id, random.Default())
			},
			Execute: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext, rng random.Generator) error {
				if c == nil {
					return corebattle.ErrUnsupportedEffect
				}
				c.TargetContext, c.Arguments, c.Assets = f.TargetContext, append([]string(nil), f.Arguments...), f.Assets
				c.Frame, c.Catalog = f, d.Invocations
				if f.Defeat != nil {
					c.DefeatContext = f.Defeat
				}
				if f.Damage != nil {
					c.DamageContext = *f.Damage
					c.TargetContext = f.TargetContext
				}
				err := s.ApplyJobAssets(c, id, rng)
				f.TargetContext, f.Arguments = c.TargetContext, c.Arguments
				if f.Damage != nil {
					*f.Damage = c.DamageContext
				}
				return err
			},
		}
	}
}
