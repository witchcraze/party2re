package skill

import (
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

// RegisterDamageGems binds complete native bodies to the existing invocation
// catalog. Rebuild operation-local closures after loading saved battle facts.
func (d *BattleDefinitions) RegisterDamageGems(c *corebattle.DamageContext) {
	if d.DamageGems == nil {
		d.DamageGems = make(map[string]bool)
	}
	if d.Invocations.Entries == nil {
		d.Invocations.Entries = make(map[string]corebattle.CallbackDefinition)
	}
	for _, entry := range corebattle.DamageGemEntries() {
		d.DamageGems[entry.ID] = true
		d.Invocations.Entries[entry.ID] = d.DamageGemInvocation(c, entry.ID)
	}
}

// DamageGemAction shares discovery/submission admission. The consumer owns
// late outer CMP charging; the returned CMP contribution is never an MP cost.
func (d BattleDefinitions) DamageGemAction(s *corebattle.LifetimeState, c *corebattle.DamageContext, id, targetID string, rng random.Generator) (corebattle.DamageGemEntry, error) {
	if !d.DamageGems[id] {
		return corebattle.DamageGemEntry{}, corebattle.ErrUnsupportedEffect
	}
	for _, entry := range corebattle.DamageGemEntries() {
		if entry.ID == id {
			if err := s.ValidateDamageGem(entry.SourceID, c, targetID, rng); err != nil {
				return corebattle.DamageGemEntry{}, err
			}
			return entry, nil
		}
	}
	return corebattle.DamageGemEntry{}, corebattle.ErrUnsupportedEffect
}

// DamageGemInvocation preserves source argument consumption. Mixed preceding
// callback transitions require #1436's projection and remain wholly gated.
func (d BattleDefinitions) DamageGemInvocation(c *corebattle.DamageContext, id string) corebattle.CallbackDefinition {
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
			_, err := d.DamageGemAction(s, &child, id, target, random.Default())
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
			entry, err := d.DamageGemAction(s, c, id, target, rng)
			if err != nil {
				return err
			}
			_, err = s.ApplyDamageGem(entry.SourceID, c, target, rng)
			if entry.TargetScope == corebattle.TargetScopeSingleEnemy && len(frame.Arguments) > 0 {
				frame.Arguments = frame.Arguments[1:]
			}
			frame.TargetContext = c.TargetContext
			return err
		},
	}
}
