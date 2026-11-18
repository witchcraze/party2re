package battle

import (
	"github.com/witchcraze/party2re/internal/core/random"
)

// HasExtendedSkillAmulet is the source's item158 equivalence for player
// extension and shift-gear listing (_battle.cgi:1747–1758).
func (a LifetimeActor) HasExtendedSkillAmulet() bool {
	return a.Extended == ExtendedAwakening || hasItem(a.ItemDefinitionIDs, "item-158") ||
		hasItem(a.ItemDefinitionIDs, "item-240") || hasItem(a.ItemDefinitionIDs, "item-241") ||
		hasItem(a.ItemDefinitionIDs, "item-242")
}

// ShiftGearCallback is the ordinary item158 body, distinct from gem86's
// handled job73 branch. The consumer charges ordinary MP1 after the callback.
func ShiftGearCallback(previous bool) CallbackDefinition {
	validate := func(s *LifetimeState, c *CallbackContext) error {
		a := s.actor(c.ActorID)
		if a == nil {
			return ErrInvalidRequest
		}
		job, sp := a.Jobs.JobID, a.Jobs.SP
		if previous {
			job, sp = a.Jobs.OldJobID, a.Jobs.OldSP
		}
		if job != "job-73" || sp == 0 || !a.HasExtendedSkillAmulet() || a.MP < 1 {
			return ErrUnsupportedEffect
		}
		return nil
	}
	apply := func(s *LifetimeState, c *CallbackContext) {
		a := s.actor(c.ActorID)
		if a.Attack < 999 {
			a.Attack += len(c.PartyIDs)
		}
	}
	return CallbackDefinition{
		Validate: validate,
		Project: func(s *LifetimeState, c *CallbackContext) (bool, error) {
			if err := validate(s, c); err != nil {
				return false, err
			}
			apply(s, c)
			return true, nil
		},
		Execute: func(s *LifetimeState, c *CallbackContext, _ random.Generator) error {
			if err := validate(s, c); err != nil {
				return err
			}
			apply(s, c)
			return nil
		},
	}
}
