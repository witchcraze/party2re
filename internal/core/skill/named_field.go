package skill

import (
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

// RegisterNamedFields adds the eight complete bodies. Existing owners supply
// full ordered tables; catalog assembly must not replace them with this subset.
func (d *BattleDefinitions) RegisterNamedFields(c *corebattle.NamedFieldContext) {
	if d.Invocations.Entries == nil {
		d.Invocations.Entries = make(map[string]corebattle.CallbackDefinition)
	}
	for _, entry := range corebattle.NamedFieldEntries() {
		id := entry.ID
		context := func(f *corebattle.CallbackContext) corebattle.NamedFieldContext {
			child := *c
			child.TargetContext, child.Arguments = f.TargetContext, append([]string(nil), f.Arguments...)
			child.Catalog = d.Invocations
			return child
		}
		d.Invocations.Entries[id] = corebattle.CallbackDefinition{
			Validate: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext) error {
				if c == nil || f.AfterEffects {
					return corebattle.ErrUnsupportedEffect
				}
				child := context(f)
				return s.ValidateNamedField(&child, id, random.Default())
			},
			Execute: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext, rng random.Generator) error {
				if c == nil {
					return corebattle.ErrUnsupportedEffect
				}
				child := context(f)
				err := s.ApplyNamedField(&child, id, rng)
				*c = child
				f.TargetContext, f.Arguments = child.TargetContext, child.Arguments
				return err
			},
		}
	}
}
