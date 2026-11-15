package adventure

import (
	"maps"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/economy"
)

func (r DungeonCrawlResult) RewardForCharacter(id string) corebattle.Reward {
	if r.RecipientRewards != nil {
		return r.RecipientRewards[id]
	}
	return corebattle.Reward{Experience: r.TotalEXP, Currency: r.TotalGold, Crystals: r.TotalCrystals}
}

func (s *CrawlSession) recordDefeatEarnings(result corebattle.PartyBattleResult) error {
	floor := make(map[string]RunEarnings)
	for _, defeat := range result.DefeatEarnings {
		r := defeat.Reward
		for _, id := range defeat.RecipientIDs {
			e, err := addRunEarnings(floor[id], RunEarnings{EXP: r.Experience, Gold: r.Currency, Crystals: r.Crystals})
			if err != nil {
				return err
			}
			floor[id] = e
		}
	}
	earnings := maps.Clone(s.Earnings)
	if earnings == nil {
		earnings = make(map[string]RunEarnings)
	}
	for id, e := range floor {
		// Keep the existing floor-level cooperative rounding, applied once.
		exp, err := economy.SafeMultiply(e.EXP, 100+result.BonusPercent)
		if err != nil {
			return err
		}
		gold, err := economy.SafeMultiply(e.Gold, 100+result.BonusPercent)
		if err != nil {
			return err
		}
		e.EXP, e.Gold = exp/100, gold/100
		earnings[id], err = addRunEarnings(earnings[id], e)
		if err != nil {
			return err
		}
	}
	s.Earnings = earnings
	return nil
}
