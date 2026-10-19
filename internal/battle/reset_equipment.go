package battle

import (
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

// BuildWeaponSealTable captures source table-load values once for a request.
// The caller supplies the requester's evaluated gear, independently of the
// current actor/reset target, and retains these facts through continuation.
func BuildWeaponSealTable(requester corebattle.Participant, rng random.Generator) (corebattle.WeaponSealTable, error) {
	if rng == nil || requester.Gear == nil || (requester.Gear.WeaponID != "" && requester.Gear.WeaponAttack == nil) {
		return corebattle.WeaponSealTable{}, corebattle.ErrUnsupportedEffect
	}
	attack := 0
	if requester.Gear.WeaponAttack != nil {
		attack = *requester.Gear.WeaponAttack
	}
	return corebattle.WeaponSealTable{DragonAttack: int(float64(attack) * .2), PhoenixWeight: int(-rng.Float64() * 50)}, nil
}
