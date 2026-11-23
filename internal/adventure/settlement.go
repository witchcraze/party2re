package adventure

import (
	"context"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/runhost"
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

	var defeatedEnemies []corebattle.Participant
	for _, floor := range result.FloorResults {
		if floor.Cleared && len(floor.Enemies) > 0 {
			defeatedEnemies = append(defeatedEnemies, floor.Enemies...)
		}
	}

	habitat := result.StageName
	if habitat == "" {
		habitat = result.StageID
	}

	postBattleReq := battle.ApplyPostBattleRequest{
		CharacterIDs:          req.CharacterIDs,
		PreserveEarnedRewards: result.Mode != CrawlModeTry,
		RecipientRewards:      result.RecipientRewards,
		BattleResult: corebattle.PartyBattleResult{
			Outcome: result.Outcome,
			Turns:   result.TotalTurns,
			TotalReward: corebattle.Reward{
				Experience: result.TotalEXP,
				Currency:   result.TotalGold,
				Crystals:   result.TotalCrystals,
			},
			RemainingHP:   result.ParticipantHPs,
			RemainingMP:   result.ParticipantMPs,
			ConsumedItems: result.ConsumedItems,
		},
		RecipientDrops:  recipientDrops,
		DefeatedEnemies: defeatedEnemies,
		Habitat:         habitat,
		ExitFatigue:     result.EffectiveExitFatigue(req.Mode),
		UnsealDemonKing: result.UnsealDemonKing,
	}

	resp, err := s.battleSettler.ApplyPostBattleResult(ctx, postBattleReq)
	if err != nil {
		return err
	}

	runhost.UpdateTreasureDeliveries(result.TreasureBoxes, resp)

	if len(resp.LostDrops) > 0 {
		result.LostDrops = make(map[string][]string)
		for cID, items := range resp.LostDrops {
			for _, inst := range items {
				result.LostDrops[cID] = append(result.LostDrops[cID], inst.DefinitionID)
			}
		}
	}

	if len(resp.MonsterTames) > 0 {
		result.MonsterTames = resp.MonsterTames
	}

	return nil
}
