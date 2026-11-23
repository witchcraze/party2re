package adventure

import (
	"strings"

	"github.com/witchcraze/party2re/internal/battle"
	"github.com/witchcraze/party2re/internal/economy"
	"github.com/witchcraze/party2re/internal/runhost"
)

func chapelDefeatBonus(e runhost.Earnings, blessing string, rollIntn func(int) int) runhost.Earnings {
	wish := strings.ToLower(strings.TrimSpace(blessing))
	switch {
	case battle.IsGoldBlessing(wish):
		if rollIntn(4) == 0 {
			return runhost.Earnings{Gold: e.Gold / 2}
		}
	case battle.IsExpBlessing(wish):
		if rollIntn(4) == 0 {
			return runhost.Earnings{EXP: e.EXP / 2}
		}
	}
	return runhost.Earnings{}
}

func scaleTryEarnings(e runhost.Earnings) (runhost.Earnings, error) {
	// int(total * 1.5), without floating-point rounding or intermediate overflow.
	return runhost.AddEarnings(e, runhost.Earnings{EXP: e.EXP / 2, Gold: e.Gold / 2, Crystals: e.Crystals / 2})
}

func scaleCooperativeEarnings(e runhost.Earnings, bonusPercent int) (runhost.Earnings, error) {
	if bonusPercent == 0 {
		return e, nil
	}
	exp, err := economy.SafeMultiply(e.EXP, 100+bonusPercent)
	if err != nil {
		return runhost.Earnings{}, err
	}
	gold, err := economy.SafeMultiply(e.Gold, 100+bonusPercent)
	if err != nil {
		return runhost.Earnings{}, err
	}
	e.EXP, e.Gold = exp/100, gold/100
	return e, nil
}
