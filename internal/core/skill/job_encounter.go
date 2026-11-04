package skill

import (
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

// RegisterJobEncounters supplies the four intrinsic room/order bodies. The
// existing player table assembler retains their source order and SP/MP metadata.
func (d *BattleDefinitions) RegisterJobEncounters(c *corebattle.JobEncounterContext) {
	if d.Invocations.Entries == nil {
		d.Invocations.Entries = map[string]corebattle.CallbackDefinition{}
	}
	for _, id := range []string{"job-78-entry-2", "job-78-entry-4", "job-78-entry-6", "job-79-entry-2"} {
		context := func(f *corebattle.CallbackContext) (corebattle.JobEncounterContext, error) {
			if c == nil {
				return corebattle.JobEncounterContext{}, corebattle.ErrUnsupportedEffect
			}
			child := *c
			if f.Damage != nil {
				child.DamageContext = *f.Damage
			}
			if f.Defeat != nil {
				child.DefeatContext = f.Defeat
				child.Cursor = f.Defeat.Cursor
			}
			child.TargetContext, child.Arguments, child.Invoke = f.TargetContext, append([]string(nil), f.Arguments...), f.Invoke
			return child, nil
		}
		delegates := func(s *corebattle.LifetimeState, f *corebattle.CallbackContext) []corebattle.CallbackInvocation {
			if id == "job-79-entry-2" {
				for _, a := range s.Actors {
					if a.ID == f.ActorID && a.NPC {
						return []corebattle.CallbackInvocation{{Kind: "job-79-random"}}
					}
				}
			}
			return nil
		}
		d.Invocations.Entries[id] = corebattle.CallbackDefinition{
			Delegates: delegates,
			Validate: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext) error {
				child, err := context(f)
				if err != nil {
					return err
				}
				// shortcut: #1733 owns room/order predecessor projections.
				if f.AfterEffects {
					return corebattle.ErrUnsupportedEffect
				}
				// Admission traverses declared children without an executable port.
				child.Invoke = func(corebattle.CallbackInvocation) error { return corebattle.ErrUnsupportedEffect }
				return s.ValidateJobEncounter(&child, id, random.Default())
			},
			Execute: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext, rng random.Generator) error {
				child, err := context(f)
				if err != nil {
					return err
				}
				delegated := len(delegates(s, f)) > 0
				if err := s.ApplyJobEncounter(&child, id, rng); err != nil {
					return err
				}
				if delegated {
					// Nested invocation already updated f; never restore the pre-call snapshot.
					updated, err := context(f)
					if err != nil {
						return err
					}
					*c = updated
					return nil
				}
				*c = child
				f.TargetContext, f.Arguments = child.TargetContext, child.Arguments
				if f.Damage != nil {
					*f.Damage = child.DamageContext
				}
				if f.Defeat != nil {
					f.Defeat.Cursor = child.Cursor
				}
				return nil
			},
		}
	}
}
