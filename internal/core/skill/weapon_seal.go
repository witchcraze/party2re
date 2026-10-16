package skill

import (
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

// RegisterWeaponSeals binds exactly the three source-backed active callbacks.
// Rebuild these operation-local closures after reload; supply fully admitted
// reset/defeat descendants in c. This does not append seals to any NPC/job table.
func (d *BattleDefinitions) RegisterWeaponSeals(c *corebattle.DamageContext) {
	if d.Invocations.Entries == nil {
		d.Invocations.Entries = make(map[string]corebattle.CallbackDefinition)
	}
	for _, entry := range corebattle.WeaponSealEntries() {
		d.Invocations.Entries[entry.ID] = corebattle.CallbackDefinition{
			Validate: func(s *corebattle.LifetimeState, frame *corebattle.CallbackContext) error {
				if c == nil || frame.AfterEffects {
					return corebattle.ErrUnsupportedEffect
				}
				child := *c
				child.TargetContext = frame.TargetContext
				_, err := s.ValidateWeaponSeal(&child, entry.ID, random.Default())
				return err
			},
			Execute: func(s *corebattle.LifetimeState, frame *corebattle.CallbackContext, rng random.Generator) error {
				c.TargetContext = frame.TargetContext
				_, err := s.ApplyWeaponSeal(c, entry.ID, rng)
				frame.TargetContext = c.TargetContext
				return err
			},
		}
	}
}

// WeaponSealAction requires explicit registration and the same whole-entry
// gate as execution. Its adjusted MP cost is charged by the consumer afterward.
func (d BattleDefinitions) WeaponSealAction(s *corebattle.LifetimeState, c *corebattle.DamageContext, id string, rng random.Generator) (corebattle.CallbackEntry, error) {
	if s == nil || c == nil {
		return corebattle.CallbackEntry{}, corebattle.ErrInvalidRequest
	}
	frame := corebattle.CallbackContext{TargetContext: c.TargetContext}
	if _, err := d.InvocationAction(s, &frame, []string{id}, rng); err != nil {
		return corebattle.CallbackEntry{}, err
	}
	return s.ValidateWeaponSeal(c, id, rng)
}
