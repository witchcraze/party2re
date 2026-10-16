package skill

import (
	"slices"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

// RegisterRecoveryStatGems binds complete native bodies, separately from batch
// scalar Gems. Rebuild operation-local closures after loading saved state.
func (d *BattleDefinitions) RegisterRecoveryStatGems(c *corebattle.EffectContext) {
	if d.RecoveryStatGems == nil {
		d.RecoveryStatGems = make(map[string]bool)
	}
	if d.Invocations.Entries == nil {
		d.Invocations.Entries = make(map[string]corebattle.CallbackDefinition)
	}
	for _, entry := range corebattle.RecoveryStatGemEntries() {
		d.RecoveryStatGems[entry.ID] = true
		d.Invocations.Entries[entry.ID] = d.RecoveryStatInvocation(c, []string{entry.ID})
	}
}

// RecoveryStatAction shares whole-slot admission with the executor. Returned
// CMP contributions are charged only by the consumer after this gate succeeds.
func (d BattleDefinitions) RecoveryStatAction(s *corebattle.LifetimeState, c *corebattle.EffectContext, ids []string, targetID string, rng random.Generator) ([]corebattle.RecoveryStatGemEntry, error) {
	var entries []corebattle.RecoveryStatGemEntry
	var sources []int
	for _, id := range ids {
		if id == "" {
			continue
		}
		if !d.RecoveryStatGems[id] {
			return nil, corebattle.ErrUnsupportedEffect
		}
		found := false
		for _, entry := range corebattle.RecoveryStatGemEntries() {
			if entry.ID == id {
				entries, sources = append(entries, entry), append(sources, entry.SourceID)
				found = true
				break
			}
		}
		if !found {
			return nil, corebattle.ErrUnsupportedEffect
		}
	}
	if err := s.ValidateRecoveryStatGems(sources, c, targetID, rng); err != nil {
		return nil, err
	}
	return entries, nil
}

// RecoveryStatInvocation binds an entire ordered custom body to the existing
// invocation engine. Arbitrary preceding callbacks need their own transition
// projection; the finite slot sequence is admitted together inside this body.
func (d BattleDefinitions) RecoveryStatInvocation(c *corebattle.EffectContext, ids []string) corebattle.CallbackDefinition {
	ids = slices.Clone(ids)
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
			_, err := d.RecoveryStatAction(s, &child, ids, target, random.Default())
			return err
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
			entries, err := d.RecoveryStatAction(s, c, ids, target, rng)
			if err != nil {
				return err
			}
			var sources []int
			for _, entry := range entries {
				sources = append(sources, entry.SourceID)
			}
			_, err = s.ApplyRecoveryStatGems(sources, c, target, rng)
			if err == nil && len(frame.Arguments) > 0 && (c.Custom || len(entries) > 1 || entries[0].TargetScope == corebattle.TargetScopeSingleAlly || entries[0].TargetScope == corebattle.TargetScopeSingleEnemy) {
				frame.Arguments = frame.Arguments[1:]
			}
			frame.TargetContext = c.TargetContext
			return err
		},
	}
}
