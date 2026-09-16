package adventure

import (
	"context"
	"slices"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

// PostBattleSettler applies post-battle state, rewards, and item drops.
type PostBattleSettler interface {
	ApplyPostBattleResult(ctx context.Context, req battle.ApplyPostBattleRequest) (battle.ApplyPostBattleResponse, error)
}

// settlePostBattle delegates post-battle settlement, item drops, and status updates via PostBattleSettler.
func (s *Service) settlePostBattle(ctx context.Context, req DungeonCrawlRequest, result *DungeonCrawlResult) error {
	if s.battleSettler == nil {
		return nil
	}

	recipientDrops := make(map[string][]string)
	for _, box := range result.TreasureBoxes {
		if box.OpenedBy != "" && box.ItemID != "" {
			recipientDrops[box.OpenedBy] = append(recipientDrops[box.OpenedBy], box.ItemID)
		}
	}

	postBattleReq := battle.ApplyPostBattleRequest{
		CharacterIDs: req.CharacterIDs,
		BattleResult: corebattle.PartyBattleResult{
			Outcome: result.Outcome,
			Turns:   result.TotalTurns,
			TotalReward: corebattle.Reward{
				Experience: result.TotalEXP,
				Currency:   result.TotalGold,
				Crystals:   result.TotalCrystals,
			},
			RemainingHP: result.ParticipantHPs,
			RemainingMP: result.ParticipantMPs,
		},
		RecipientDrops: recipientDrops,
	}

	resp, err := s.battleSettler.ApplyPostBattleResult(ctx, postBattleReq)
	if err != nil {
		return err
	}

	updateTreasureDeliveries(result.TreasureBoxes, resp)

	if len(resp.LostDrops) > 0 {
		result.LostDrops = make(map[string][]string)
		for cID, items := range resp.LostDrops {
			for _, inst := range items {
				result.LostDrops[cID] = append(result.LostDrops[cID], inst.DefinitionID)
			}
		}
	}

	return nil
}

func updateTreasureDeliveries(boxes []TreasureBox, resp battle.ApplyPostBattleResponse) {
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
