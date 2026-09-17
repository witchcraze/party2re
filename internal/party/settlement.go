package party

import (
	"context"
	"fmt"
	"slices"

	"github.com/witchcraze/party2re/internal/adventure"
	battleadapter "github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/progression"
	"github.com/witchcraze/party2re/internal/depot"
)

// PostBattleSettler applies post-battle state, rewards, and item drops via the Battle Adapter.
type PostBattleSettler interface {
	ApplyPostBattleResult(ctx context.Context, req battleadapter.ApplyPostBattleRequest) (battleadapter.ApplyPostBattleResponse, error)
}

// DepotRepository defines persistence operations on Depot (Rank 5).
type DepotRepository interface {
	FindByCharacterIDForUpdate(ctx context.Context, characterID string) (depot.Depot, error)
	Save(ctx context.Context, d depot.Depot) error
}

// WithPostBattleSettler configures the PostBattleSettler for the service.
func WithPostBattleSettler(settler PostBattleSettler) Option {
	return func(s *Service) {
		s.battleSettler = settler
	}
}

// WithDepotRepository configures the DepotRepository for the service.
func WithDepotRepository(repo DepotRepository) Option {
	return func(s *Service) {
		s.depotRepo = repo
	}
}

func (s *Service) settlePostBattle(
	ctx context.Context,
	members []Member,
	charMap map[string]corecharacter.Character,
	crawlResult *adventure.DungeonCrawlResult,
	lastBattleRes corebattle.PartyBattleResult,
) ([]MemberRewardSummary, map[string][]string, error) {
	if s.battleSettler != nil {
		return s.settleWithBattleSettler(ctx, members, charMap, crawlResult, lastBattleRes)
	}
	return s.settleFallback(ctx, members, charMap, crawlResult, lastBattleRes)
}

func (s *Service) settleWithBattleSettler(
	ctx context.Context,
	members []Member,
	charMap map[string]corecharacter.Character,
	crawlResult *adventure.DungeonCrawlResult,
	lastBattleRes corebattle.PartyBattleResult,
) ([]MemberRewardSummary, map[string][]string, error) {
	recipientDrops := make(map[string][]string)
	for _, box := range crawlResult.TreasureBoxes {
		if box.OpenedBy != "" && box.ItemID != "" {
			recipientDrops[box.OpenedBy] = append(recipientDrops[box.OpenedBy], box.ItemID)
		}
	}

	charIDs := make([]string, len(members))
	for i, m := range members {
		charIDs[i] = m.CharacterID
	}

	postBattleReq := battleadapter.ApplyPostBattleRequest{
		CharacterIDs: charIDs,
		BattleResult: corebattle.PartyBattleResult{
			Outcome: crawlResult.Outcome,
			Turns:   crawlResult.TotalTurns,
			TotalReward: corebattle.Reward{
				Experience: crawlResult.TotalEXP,
				Currency:   crawlResult.TotalGold,
				Crystals:   crawlResult.TotalCrystals,
			},
			RemainingHP:   crawlResult.ParticipantHPs,
			RemainingMP:   crawlResult.ParticipantMPs,
			AlliesFallen:  lastBattleRes.AlliesFallen,
			BanishedIDs:   lastBattleRes.BanishedIDs,
			ConsumedItems: lastBattleRes.ConsumedItems,
		},
		RecipientDrops: recipientDrops,
	}

	resp, err := s.battleSettler.ApplyPostBattleResult(ctx, postBattleReq)
	if err != nil {
		return nil, nil, err
	}

	updateTreasureDeliveries(crawlResult.TreasureBoxes, resp)

	lostDrops := make(map[string][]string)
	if len(resp.LostDrops) > 0 {
		for cID, items := range resp.LostDrops {
			for _, inst := range items {
				lostDrops[cID] = append(lostDrops[cID], inst.DefinitionID)
			}
		}
	}

	var rewardSummaries []MemberRewardSummary
	for _, m := range members {
		cID := m.CharacterID
		c := charMap[cID]
		levelAfter := c.Level
		if updatedChar, ok := resp.UpdatedCharacters[cID]; ok {
			levelAfter = updatedChar.Level
		}

		var acquiredDrops []coreitem.Instance
		if invDrops, ok := resp.InventoryDrops[cID]; ok {
			acquiredDrops = append(acquiredDrops, invDrops...)
		}
		if depotDrops, ok := resp.DepotDeliveries[cID]; ok {
			acquiredDrops = append(acquiredDrops, depotDrops...)
		}

		rewardSummaries = append(rewardSummaries, MemberRewardSummary{
			CharacterID:    cID,
			Name:           c.Name,
			GainedEXP:      resp.GainedExperience[cID],
			GainedGold:     resp.GainedGold[cID],
			GainedCrystals: resp.GainedCrystals[cID],
			LevelBefore:    c.Level,
			LevelAfter:     levelAfter,
			Drops:          acquiredDrops,
		})
	}

	return rewardSummaries, lostDrops, nil
}

func (s *Service) settleFallback(
	ctx context.Context,
	members []Member,
	charMap map[string]corecharacter.Character,
	crawlResult *adventure.DungeonCrawlResult,
	lastBattleRes corebattle.PartyBattleResult,
) ([]MemberRewardSummary, map[string][]string, error) {
	lostDrops := make(map[string][]string)
	var rewardSummaries []MemberRewardSummary

	for _, m := range members {
		cID := m.CharacterID
		c := charMap[cID]
		levelBefore := c.Level
		gainedEXP := crawlResult.TotalEXP
		gainedGold := crawlResult.TotalGold
		gainedCrystals := crawlResult.TotalCrystals

		if err := c.AddMoney(gainedGold); err != nil {
			return nil, nil, fmt.Errorf("adding gold for character %s: %w", cID, err)
		}
		if gainedCrystals > 0 {
			if err := c.AddCrystal(gainedCrystals); err != nil {
				return nil, nil, fmt.Errorf("adding crystals for character %s: %w", cID, err)
			}
		}
		if gainedEXP > 0 {
			if _, err := progression.ApplyExperience(&c, gainedEXP); err != nil {
				return nil, nil, err
			}
		}

		// Apply HP changes
		if remHP, ok := crawlResult.ParticipantHPs[cID]; ok {
			if remHP <= 0 {
				c.Stats.HP = 1 // Fallen members survive with 1 HP
			} else {
				c.Stats.HP = remHP
				if c.Stats.MaxHP > 0 && c.Stats.HP > c.Stats.MaxHP {
					c.Stats.HP = c.Stats.MaxHP
				}
			}
		} else {
			for _, fallenID := range lastBattleRes.AlliesFallen {
				if fallenID == cID {
					c.Stats.HP = 1
					break
				}
			}
		}

		// Apply MP changes
		if remMP, ok := crawlResult.ParticipantMPs[cID]; ok && remMP >= 0 {
			c.Stats.MP = remMP
			if c.Stats.MaxMP > 0 && c.Stats.MP > c.Stats.MaxMP {
				c.Stats.MP = c.Stats.MaxMP
			}
		}

		if err := s.charRepo.Update(ctx, c); err != nil {
			return nil, nil, err
		}

		// Award drops from Floor 11 treasure boxes
		var drops []coreitem.Instance
		if crawlResult.Outcome == corebattle.OutcomeWin {
			for i := range crawlResult.TreasureBoxes {
				box := &crawlResult.TreasureBoxes[i]
				if box.OpenedBy != cID || box.ItemID == "" {
					continue
				}

				res, err := depot.DeliverRewardItem(ctx, s.invRepo, s.depotRepo, c, box.ItemID, 1, depot.PolicyTreatOverflowAsLost)
				if err != nil {
					continue
				}
				box.DeliveredTo = string(res.DeliveredTo)
				if res.DeliveredTo == depot.DeliveredToInventory || res.DeliveredTo == depot.DeliveredToDepot {
					drops = append(drops, res.Item)
				} else {
					lostDrops[cID] = append(lostDrops[cID], box.ItemID)
				}
			}
		}

		rewardSummaries = append(rewardSummaries, MemberRewardSummary{
			CharacterID:    cID,
			Name:           c.Name,
			GainedEXP:      gainedEXP,
			GainedGold:     gainedGold,
			GainedCrystals: gainedCrystals,
			LevelBefore:    levelBefore,
			LevelAfter:     c.Level,
			Drops:          drops,
		})
	}

	return rewardSummaries, lostDrops, nil
}

func updateTreasureDeliveries(boxes []adventure.TreasureBox, resp battleadapter.ApplyPostBattleResponse) {
	remainingInv := make(map[string][]string)
	for cID, items := range resp.InventoryDrops {
		for _, inst := range items {
			remainingInv[cID] = append(remainingInv[cID], inst.DefinitionID)
		}
	}
	remainingDepot := make(map[string][]string)
	for cID, items := range resp.DepotDeliveries {
		for _, inst := range items {
			remainingDepot[cID] = append(remainingDepot[cID], inst.DefinitionID)
		}
	}
	remainingLost := make(map[string][]string)
	for cID, items := range resp.LostDrops {
		for _, inst := range items {
			remainingLost[cID] = append(remainingLost[cID], inst.DefinitionID)
		}
	}

	for i := range boxes {
		box := &boxes[i]
		if box.OpenedBy == "" || box.ItemID == "" {
			continue
		}
		cID := box.OpenedBy
		if idx := slices.Index(remainingInv[cID], box.ItemID); idx != -1 {
			box.DeliveredTo = "inventory"
			remainingInv[cID] = append(remainingInv[cID][:idx], remainingInv[cID][idx+1:]...)
			continue
		}
		if idx := slices.Index(remainingDepot[cID], box.ItemID); idx != -1 {
			box.DeliveredTo = "depot"
			remainingDepot[cID] = append(remainingDepot[cID][:idx], remainingDepot[cID][idx+1:]...)
			continue
		}
		if idx := slices.Index(remainingLost[cID], box.ItemID); idx != -1 {
			box.DeliveredTo = "lost"
			remainingLost[cID] = append(remainingLost[cID][:idx], remainingLost[cID][idx+1:]...)
			continue
		}
		box.DeliveredTo = "inventory"
	}
}
