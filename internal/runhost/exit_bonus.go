package runhost

import (
	"slices"

	"github.com/witchcraze/party2re/internal/core/random"
	"github.com/witchcraze/party2re/internal/economy"
)

// ExitBonus evaluates equipped accessory effects after cumulative consumption.
// draw is a uniform sample in [0,1); the accepted command fixes it in its receipt.
func ExitBonus(definition string, round int, draw float64) (exp, gold int, err error) {
	if definition == "item-105" {
		exp, err = economy.SafeMultiply(round, 20)
	}
	if definition == "item-106" {
		if _, err = economy.SafeMultiply(round, 77); err != nil {
			return 0, 0, err
		}
		gold = int(float64(round) * draw * 77)
	}
	return
}

// DefaultExit is nigeru's part for a room without calm mode (_battle.cgi:1208–1218):
// fatigue for the current round and the held item105/item106 bonus.
func (r *State) DefaultExit(actor string, rng random.Generator) (Earnings, int, error) {
	bonus := Earnings{}
	if a := r.Actor(actor); a != nil && a.Gear != nil && slices.Contains(a.ItemDefinitionIDs, a.Gear.AccessoryID) {
		draw := 0.0
		if a.Gear.AccessoryID == "item-106" {
			draw = rng.Float64()
		}
		var err error
		if bonus.EXP, bonus.Gold, err = ExitBonus(a.Gear.AccessoryID, r.Round, draw); err != nil {
			return bonus, 0, err
		}
	}
	return bonus, r.Round*3 + 1, nil
}
