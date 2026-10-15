package skill

import (
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

// RosterAction admits only explicitly registered callbacks. Catalog presence
// and batch skill DTOs do not certify the explicit roster effect or its tails.
func (d BattleDefinitions) RosterAction(s *corebattle.LifetimeState, c *corebattle.RosterContext, id, targetID string, rng random.Generator) error {
	if s == nil || c == nil {
		return corebattle.ErrInvalidRequest
	}
	if !d.RosterSkills[id] {
		return corebattle.ErrUnsupportedEffect
	}
	return s.ValidateRoster(c, id, targetID, rng)
}

// RosterInvocation connects the existing indirect executor to the same native
// gate. The operation owner supplies requester/gear/reset/defeat prerequisites.
// A preceding arbitrary callback can change helpers, stats or appearances;
// without its transition projection the enclosing continuation stays rejected.
func (d BattleDefinitions) RosterInvocation(c *corebattle.RosterContext, id string) corebattle.CallbackDefinition {
	return corebattle.CallbackDefinition{
		Validate: func(s *corebattle.LifetimeState, frame *corebattle.CallbackContext) error {
			if c == nil || frame.AfterEffects {
				return corebattle.ErrUnsupportedEffect
			}
			child := *c
			child.TargetContext = frame.TargetContext
			target := ""
			if len(frame.Arguments) > 0 {
				target = frame.Arguments[0]
			}
			return d.RosterAction(s, &child, id, target, random.Default())
		},
		Execute: func(s *corebattle.LifetimeState, frame *corebattle.CallbackContext, rng random.Generator) error {
			if c == nil {
				return corebattle.ErrUnsupportedEffect
			}
			c.TargetContext = frame.TargetContext
			target := ""
			if len(frame.Arguments) > 0 {
				target = frame.Arguments[0]
			}
			if err := d.RosterAction(s, c, id, target, rng); err != nil {
				return err
			}
			result, err := s.ApplyRoster(c, id, target, rng)
			if result.TargetConsumed && len(frame.Arguments) > 0 {
				frame.Arguments = frame.Arguments[1:]
			}
			frame.TargetContext = c.TargetContext
			return err
		},
	}
}
