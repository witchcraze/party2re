package battle

import (
	"slices"

	"github.com/witchcraze/party2re/internal/core/random"
)

// FormationCallback moves the actor within its party and rebuilds opponents
// from the member scan. The source includes fallen opponents in this scan.
func FormationCallback(front bool) CallbackDefinition {
	move := func(s *LifetimeState, c *CallbackContext) {
		party := slices.Clone(c.PartyIDs)
		i := slices.Index(party, c.ActorID)
		party = slices.Delete(party, i, i+1)
		if front {
			party = slices.Insert(party, 0, c.ActorID)
		} else {
			party = append(party, c.ActorID)
		}
		members := make([]LifetimeActor, 0, len(s.Actors))
		for _, id := range party {
			members = append(members, *s.actor(id))
		}
		team := s.actor(c.ActorID).TeamID
		c.PartyIDs, c.EnemyIDs, c.MemberOnlyIDs = party, nil, nil
		for _, a := range s.Actors {
			if slices.Contains(party, a.ID) {
				continue
			}
			if a.TeamID != team {
				members = append(members, a)
				c.EnemyIDs = append(c.EnemyIDs, a.ID)
			} else {
				c.MemberOnlyIDs = append(c.MemberOnlyIDs, a.ID)
			}
		}
		for _, id := range c.MemberOnlyIDs {
			members = append(members, *s.actor(id))
		}
		s.Actors = members
	}
	return CallbackDefinition{
		Validate: func(s *LifetimeState, c *CallbackContext) error {
			if len(c.PartyIDs) < 2 {
				return ErrUnsupportedEffect
			}
			return s.validateTargetContext(c.TargetContext, random.Default())
		},
		Project: func(s *LifetimeState, c *CallbackContext) (bool, error) {
			move(s, c)
			return true, nil
		},
		Execute: func(s *LifetimeState, c *CallbackContext, _ random.Generator) error {
			move(s, c)
			return nil
		},
	}
}

// BetweenEncounterSurvivalCallback uses the ordinary healing owner. Discovery
// supplies job, floor and MP admission, and the enclosing step charges MP.
func BetweenEncounterSurvivalCallback() CallbackDefinition {
	effect := func(c *CallbackContext) TargetEffect { return controlEffect(c.ActorID, 80, CategoryUntyped) }
	return CallbackDefinition{
		Validate: func(s *LifetimeState, c *CallbackContext) error {
			return s.validatePartyTarget(c.TargetContext, effect(c), random.Default())
		},
		Execute: func(s *LifetimeState, c *CallbackContext, rng random.Generator) error {
			if c.Damage == nil {
				c.Damage = &DamageContext{}
			}
			c.Damage.TargetContext = c.TargetContext
			_, err := s.ApplyHealing(&c.Damage.EffectContext, effect(c), rng)
			return err
		},
	}
}
