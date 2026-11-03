package battle

import (
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

// ValidateResetAll admits every member before revival, reset or random draws.
func ValidateResetAll(s *corebattle.LifetimeState, c *corebattle.EquipmentResetContext, rng random.Generator) error {
	if s == nil || c == nil {
		return corebattle.ErrUnsupportedEffect
	}
	for _, a := range s.Actors {
		if err := s.ValidateEquipmentReset(c, a.ID, rng); err != nil {
			return err
		}
	}
	return nil
}

// ResetAll shares the source all-member reset, including item188's revival
// pass. Explicit advance separately clears fields; tool150 retains them.
func ResetAll(s *corebattle.LifetimeState, c *corebattle.EquipmentResetContext, rng random.Generator) error {
	if err := ValidateResetAll(s, c, rng); err != nil {
		return err
	}
	if err := s.ReviveOnAdvance(c.PartyIDs, rng); err != nil {
		return err
	}
	for i := range s.Actors {
		a := &s.Actors[i]
		if err := s.ApplyEquipmentReset(c, a.ID, rng); err != nil {
			return err
		}
		if a.Gear.AccessoryID == "item-188" && a.HP > 0 {
			a.Temporary = corebattle.TemporaryRevival
		}
	}
	return nil
}
