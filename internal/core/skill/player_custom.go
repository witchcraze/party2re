package skill

import (
	"slices"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

// PlayerCustomAction applies the player custom command's listing gates and
// admits every occupied slot, repeats included, before any slot or cost. names
// are the commands listed before it; a colliding custom name is not listed.
// The submitted target resolves once, falling back to the actor, and every slot
// receives it. _skill.cgi:288–309.
func (d BattleDefinitions) PlayerCustomAction(s *corebattle.LifetimeState, c *corebattle.CallbackContext, names []string, rng random.Generator) (corebattle.CallbackContext, []corebattle.CallbackInvocation, error) {
	if s == nil || c == nil || rng == nil {
		return corebattle.CallbackContext{}, nil, corebattle.ErrInvalidRequest
	}
	a := playerActor(s, c.ActorID)
	if a == nil {
		return corebattle.CallbackContext{}, nil, corebattle.ErrInvalidParticipant
	}
	load := a.Custom
	if load == nil || load.Name == "" || len(c.EnemyIDs) == 0 || slices.Contains(names, load.Name) || a.CMP < load.Cost || slices.Contains(a.Abilities, "cursed") {
		return corebattle.CallbackContext{}, nil, ErrUnavailable
	}
	var ops []corebattle.CallbackInvocation
	for _, id := range load.Gems {
		if id != "" {
			ops = append(ops, corebattle.CallbackInvocation{Kind: "entry", EntryID: id})
		}
	}
	if len(ops) == 0 {
		return corebattle.CallbackContext{}, nil, ErrUnavailable
	}
	frame := *c
	frame.Custom = true
	frame.Arguments = []string{c.ActorID}
	for _, x := range s.Actors {
		if len(c.Arguments) > 0 && x.ID == c.Arguments[0] {
			frame.Arguments[0] = x.ID
		}
	}
	if err := s.ValidateInvocations(&frame, d.Invocations, ops, rng); err != nil {
		return corebattle.CallbackContext{}, nil, err
	}
	return frame, ops, nil
}

// ApplyPlayerCustomAction executes the admitted entry, then charges the custom
// cost floored at zero, even when a slot felled the actor. A rejected slot
// leaves state, context and CMP unchanged. _battle.cgi:759–789.
func (d BattleDefinitions) ApplyPlayerCustomAction(s *corebattle.LifetimeState, c *corebattle.CallbackContext, names []string, rng random.Generator) error {
	frame, ops, err := d.PlayerCustomAction(s, c, names, rng)
	if err != nil {
		return err
	}
	if err := s.ApplyInvocations(&frame, d.Invocations, ops, rng); err != nil {
		return err
	}
	a := playerActor(s, c.ActorID)
	a.CMP = max(0, a.CMP-a.Custom.Cost)
	*c = frame
	return nil
}

func playerActor(s *corebattle.LifetimeState, id string) *corebattle.LifetimeActor {
	for i := range s.Actors {
		if s.Actors[i].ID == id && !s.Actors[i].NPC {
			return &s.Actors[i]
		}
	}
	return nil
}
