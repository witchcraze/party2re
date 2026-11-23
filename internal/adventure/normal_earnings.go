package adventure

import (
	"github.com/witchcraze/party2re/internal/runhost"
)

func scaleTryEarnings(e runhost.Earnings) (runhost.Earnings, error) {
	// int(total * 1.5), without floating-point rounding or intermediate overflow.
	return runhost.AddEarnings(e, runhost.Earnings{EXP: e.EXP / 2, Gold: e.Gold / 2, Crystals: e.Crystals / 2})
}
