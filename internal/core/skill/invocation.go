package skill

import (
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

// InvocationAction shares discovery/submission admission for explicit callback
// commands. Call before any outer cost, item consumption or durable mutation.
// Passing primitive job/gem definitions never enables an indirect closure.
func (d BattleDefinitions) InvocationAction(s *corebattle.LifetimeState, c *corebattle.CallbackContext, ids []string, rng random.Generator) ([]corebattle.CallbackInvocation, error) {
	ops := make([]corebattle.CallbackInvocation, 0, len(ids))
	for _, id := range ids {
		ops = append(ops, corebattle.CallbackInvocation{Kind: "entry", EntryID: id})
	}
	if err := s.ValidateInvocations(c, d.Invocations, ops, rng); err != nil {
		return nil, err
	}
	return ops, nil
}
