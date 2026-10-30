package skill

import (
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

// RegisterPlayerTails registers passive source boundaries independently of
// learnable/job/random commands. Registration alone does not admit a context.
func (d *BattleDefinitions) RegisterPlayerTails() {
	if d.PlayerTails == nil {
		d.PlayerTails = make(map[string]bool)
	}
	for _, id := range corebattle.PlayerTailEntries() {
		d.PlayerTails[id] = true
	}
}

// PlayerTailAction uses the executor's complete admission for discovery. The
// enclosing consumer supplies exact eligibility, original requester and snapshot.
func (d BattleDefinitions) PlayerTailAction(s *corebattle.LifetimeState, c *corebattle.PlayerTailContext, id string, rng random.Generator) error {
	if !d.PlayerTails[id] {
		return corebattle.ErrUnsupportedEffect
	}
	return s.ValidatePlayerTail(c, id, rng)
}
