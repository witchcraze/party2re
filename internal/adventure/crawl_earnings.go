package adventure

import (
	"context"
	"fmt"
	"maps"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/runhost"
)

// LoadBlessings queries active chapel blessings for all participants and records treasure eligibility.
func (s *CrawlSession) LoadBlessings(ctx context.Context, provider BlessingProvider) error {
	if provider == nil {
		return nil
	}
	blessings := make(map[string]string, len(s.CharacterIDs))
	for _, cID := range s.CharacterIDs {
		b, err := provider.GetActiveBlessing(ctx, cID)
		if err != nil {
			return fmt.Errorf("get active blessing: %w", err)
		}
		blessings[cID] = b
	}
	s.Blessings = blessings
	actorID := s.LeaderID
	if actorID == "" && len(s.CharacterIDs) > 0 {
		actorID = s.CharacterIDs[0]
	}
	if actorID != "" && IsTreasureBlessing(blessings[actorID]) {
		s.HasTreasureBlessing = true
	}
	return nil
}

func (r DungeonCrawlResult) RewardForCharacter(id string) corebattle.Reward {
	if r.RecipientRewards != nil {
		return r.RecipientRewards[id]
	}
	return corebattle.Reward{Experience: r.TotalEXP, Currency: r.TotalGold, Crystals: r.TotalCrystals}
}

func (s *CrawlSession) recordDefeatEarnings(result corebattle.PartyBattleResult) error {
	floor := make(map[string]runhost.Earnings)
	for _, defeat := range result.DefeatEarnings {
		r := defeat.Reward
		base, err := runhost.AddEarnings(runhost.Earnings{}, runhost.Earnings{EXP: r.Experience, Gold: r.Currency, Crystals: r.Crystals})
		if err != nil {
			return err
		}
		base, err = runhost.AddEarnings(base, chapelDefeatBonus(base, s.Blessings[defeat.ActorID], s.rngIntn))
		if err != nil {
			return err
		}
		for _, id := range defeat.RecipientIDs {
			e, err := runhost.AddEarnings(floor[id], base)
			if err != nil {
				return err
			}
			floor[id] = e
		}
	}
	earnings := maps.Clone(s.Earnings)
	if earnings == nil {
		earnings = make(map[string]runhost.Earnings)
	}
	for id, e := range floor {
		// Keep the existing floor-level cooperative rounding, applied once.
		e, err := scaleCooperativeEarnings(e, result.BonusPercent)
		if err != nil {
			return err
		}
		earnings[id], err = runhost.AddEarnings(earnings[id], e)
		if err != nil {
			return err
		}
	}
	s.Earnings = earnings
	return nil
}
