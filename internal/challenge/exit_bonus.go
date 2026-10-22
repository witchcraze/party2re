package challenge

import "github.com/witchcraze/party2re/internal/economy"

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
