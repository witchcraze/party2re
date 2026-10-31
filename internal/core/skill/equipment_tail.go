package skill

import (
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

// RegisterEquipmentTails registers passive boundaries, never submitted or random
// commands. Discovery still requires the executor's complete descendant admission.
func (d *BattleDefinitions) RegisterEquipmentTails() {
	if d.EquipmentTails == nil {
		d.EquipmentTails = make(map[string]bool)
	}
	for _, id := range corebattle.EquipmentTailEntries() {
		d.EquipmentTails[id] = true
	}
}

func (d BattleDefinitions) EquipmentTailAction(s *corebattle.LifetimeState, c *corebattle.EquipmentTailContext, id string, rng random.Generator) error {
	if !d.EquipmentTails[id] {
		return corebattle.ErrUnsupportedEffect
	}
	return s.ValidateEquipmentTail(c, id, rng)
}
