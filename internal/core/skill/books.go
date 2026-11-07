package skill

import (
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

// RegisterBooks binds the certified book identities. Rebuild the closures after
// reload. They are never appended to NPC or job tables.
func (d *BattleDefinitions) RegisterBooks(c *corebattle.BookContext) {
	if d.Invocations.Entries == nil {
		d.Invocations.Entries = make(map[string]corebattle.CallbackDefinition)
	}
	for _, entry := range corebattle.BookEntries() {
		d.Invocations.Entries[entry.ID] = corebattle.CallbackDefinition{
			Validate: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext) error {
				if c == nil || f.AfterEffects {
					return corebattle.ErrUnsupportedEffect
				}
				child := *c
				child.TargetContext, child.Arguments = f.TargetContext, append([]string(nil), f.Arguments...)
				return s.ValidateBook(&child, entry.ID, random.Default())
			},
			Execute: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext, rng random.Generator) error {
				if c == nil {
					return corebattle.ErrUnsupportedEffect
				}
				c.TargetContext, c.Arguments = f.TargetContext, append([]string(nil), f.Arguments...)
				err := s.ApplyBook(c, entry.ID, rng)
				f.TargetContext, f.Arguments = c.TargetContext, c.Arguments
				return err
			},
		}
	}
}

// BookAction requires explicit registration and the same whole-entry gate as
// execution. Its adjusted MP cost is charged by the consumer afterward.
func (d BattleDefinitions) BookAction(s *corebattle.LifetimeState, c *corebattle.BookContext, id string, rng random.Generator) (corebattle.CallbackEntry, error) {
	if s == nil || c == nil {
		return corebattle.CallbackEntry{}, corebattle.ErrInvalidRequest
	}
	frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: c.Arguments}
	if _, err := d.InvocationAction(s, &frame, []string{id}, rng); err != nil {
		return corebattle.CallbackEntry{}, err
	}
	return s.ValidateBookAction(c, id, rng)
}
