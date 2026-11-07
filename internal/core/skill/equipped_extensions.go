package skill

import (
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

// RegisterEquippedExtensions binds the certified generic identities. Rebuild the
// closures after reload. They are never appended to NPC or job tables.
func (d *BattleDefinitions) RegisterEquippedExtensions(c *corebattle.EquippedExtensionContext) {
	if d.Invocations.Entries == nil {
		d.Invocations.Entries = make(map[string]corebattle.CallbackDefinition)
	}
	for _, entry := range corebattle.EquippedExtensionEntries() {
		d.Invocations.Entries[entry.ID] = corebattle.CallbackDefinition{
			Validate: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext) error {
				if c == nil || f.AfterEffects {
					return corebattle.ErrUnsupportedEffect
				}
				child := *c
				child.TargetContext, child.Arguments = f.TargetContext, append([]string(nil), f.Arguments...)
				_, err := s.ValidateEquippedExtension(&child, entry.ID, random.Default())
				return err
			},
			Execute: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext, rng random.Generator) error {
				if c == nil {
					return corebattle.ErrUnsupportedEffect
				}
				c.TargetContext, c.Arguments = f.TargetContext, append([]string(nil), f.Arguments...)
				err := s.ApplyEquippedExtension(c, entry.ID, rng)
				f.TargetContext, f.Arguments = c.TargetContext, c.Arguments
				return err
			},
		}
	}
}

// EquippedExtensionAction requires explicit registration and the same whole-entry
// gate as execution. Its adjusted MP cost is charged by the consumer afterward.
func (d BattleDefinitions) EquippedExtensionAction(s *corebattle.LifetimeState, c *corebattle.EquippedExtensionContext, id string, rng random.Generator) (corebattle.CallbackEntry, error) {
	if s == nil || c == nil {
		return corebattle.CallbackEntry{}, corebattle.ErrInvalidRequest
	}
	frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: c.Arguments}
	if _, err := d.InvocationAction(s, &frame, []string{id}, rng); err != nil {
		return corebattle.CallbackEntry{}, err
	}
	return s.ValidateEquippedExtension(c, id, rng)
}
