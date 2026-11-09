package battle

import (
	"context"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/job"
	"github.com/witchcraze/party2re/internal/core/progression"
	"github.com/witchcraze/party2re/internal/depot"
)

type dropItem struct {
	definitionID string
	isTreasure   bool
}

// dropsForCharacter determines which item definition IDs should be delivered to the specified character.
// It tags whether each item originated from a treasure chest (RecipientDrops) or shared battle drops.
// It ensures that items are never cloned across party members.
func (req ApplyPostBattleRequest) dropsForCharacter(charID string) []dropItem {
	var drops []dropItem
	if len(req.RecipientDrops) > 0 {
		if explicit, ok := req.RecipientDrops[charID]; ok {
			for _, defID := range explicit {
				drops = append(drops, dropItem{definitionID: defID, isTreasure: true})
			}
		}
	}

	// Designated recipient receives shared/unpartitioned drops (req.DropItems and TotalReward.ItemDefinitionID)
	if req.designatedRecipient() == charID {
		if req.BattleResult.TotalReward.ItemDefinitionID != "" && req.BattleResult.TotalReward.ItemQuantity > 0 {
			for i := 0; i < req.BattleResult.TotalReward.ItemQuantity; i++ {
				drops = append(drops, dropItem{definitionID: req.BattleResult.TotalReward.ItemDefinitionID, isTreasure: false})
			}
		}
		for _, defID := range req.DropItems {
			drops = append(drops, dropItem{definitionID: defID, isTreasure: false})
		}
	}

	return drops
}

// designatedRecipient returns the character ID that should receive global/shared drops.
// If RecipientCharacterID is explicitly set, it is returned.
// Otherwise, if no explicit RecipientDrops map is provided and CharacterIDs is non-empty,
// the primary character (CharacterIDs[0]) is the default recipient.
func (req ApplyPostBattleRequest) designatedRecipient() string {
	if req.RecipientCharacterID != "" {
		return req.RecipientCharacterID
	}
	if len(req.RecipientDrops) == 0 && len(req.CharacterIDs) > 0 {
		return req.CharacterIDs[0]
	}
	return ""
}

func (s *Service) applyRewardsForCharacter(
	ctx context.Context,
	char *corecharacter.Character,
	inv *coreinventory.Inventory,
	reward corebattle.Reward,
	drops []dropItem,
	finalized bool,
) (int, int, int, progression.LevelUpResult, []coreitem.Instance, []coreitem.Instance, error) {
	blessing := ""
	if !finalized {
		blessing = s.queryBlessing(ctx, char.ID)
	}

	gainedGold := reward.Currency
	if gainedGold > 0 {
		// Chapel Blessing 1 ("お金がほしい"): 25% chance of 1.5x Gold (party2/lib/_battle.cgi:186-189, rand(4) < 1)
		if isGoldBlessing(blessing) && s.rollIntn(4) == 0 {
			gainedGold = int(float64(gainedGold) * 1.5)
		}
		if err := char.AddMoney(gainedGold); err != nil {
			return 0, 0, 0, progression.LevelUpResult{}, nil, nil, err
		}
	}

	gainedEXP := reward.Experience
	var lvlRes progression.LevelUpResult
	if gainedEXP > 0 {
		// Chapel Blessing 2 ("強くなりたい"): 25% chance of 1.5x EXP (party2/lib/_battle.cgi:190-193, rand(4) < 1)
		if isExpBlessing(blessing) && s.rollIntn(4) == 0 {
			gainedEXP = int(float64(gainedEXP) * 1.5)
		}
		opts := ExtractStatOrbOptions(*inv)
		if s.skillProvider != nil && char.JobID != "" {
			opts.JobSkills = s.skillProvider.SkillsForJob(char.JobID)
		}
		var jobDef job.Definition
		if s.jobProvider != nil && char.JobID != "" {
			if def, err := s.jobProvider.FindByID(char.JobID); err == nil {
				jobDef = def
			}
		}
		res, err := progression.ApplyExperienceWithJobFull(char, gainedEXP, jobDef, s.rng, opts)
		if err != nil {
			return 0, 0, 0, progression.LevelUpResult{}, nil, nil, err
		}
		lvlRes = res
	}

	// Crystal rewards (_battle.cgi:145-150, 178-228)
	gainedCrystals := reward.Crystals
	if gainedCrystals > 0 {
		if err := char.AddCrystal(gainedCrystals); err != nil {
			return 0, 0, 0, progression.LevelUpResult{}, nil, nil, err
		}
	}

	var invDrops []coreitem.Instance
	var depotDrops []coreitem.Instance

	for _, drop := range drops {
		inst, err := coreitem.NewInstance(drop.definitionID, 1)
		if err != nil {
			continue
		}

		// Check hand / inventory eligibility
		if s.canAcceptDrop(inv, drop) {
			if err := inv.Add(inst); err == nil {
				invDrops = append(invDrops, inst)
				continue
			}
		}

		// Overflow routes to depot
		depotDrops = append(depotDrops, inst)
	}

	return gainedGold, gainedEXP, gainedCrystals, lvlRes, invDrops, depotDrops, nil
}

func (s *Service) canAcceptDrop(inv *coreinventory.Inventory, drop dropItem) bool {
	if drop.isTreasure {
		return depot.AcceptsTreasure(inv, drop.definitionID)
	}
	return len(inv.Items) < s.maxInventoryCapacity()
}

func (req ApplyPostBattleRequest) rewardForCharacter(charID string) corebattle.Reward {
	if req.RecipientRewards != nil {
		return req.RecipientRewards[charID]
	}
	return req.BattleResult.TotalReward
}

func (s *Service) maxInventoryCapacity() int {
	if s.maxInvCap > 0 {
		return s.maxInvCap
	}
	return 1 // Default 1 matching legacy $m{ite}
}

func (s *Service) deliverToDepot(
	ctx context.Context,
	charID string,
	char corecharacter.Character,
	items []coreitem.Instance,
	resp *ApplyPostBattleResponse,
) error {
	if len(items) == 0 {
		return nil
	}
	if char.ID == "" {
		char.ID = charID
	}
	results, err := depot.DeliverRewardItems(ctx, nil, s.depotRepo, char, items, depot.PolicyTreatOverflowAsLost)
	if err != nil {
		return err
	}
	for _, r := range results {
		if r.DeliveredTo == depot.DeliveredToDepot {
			resp.DepotDeliveries[charID] = append(resp.DepotDeliveries[charID], r.Item)
		} else {
			resp.LostDrops[charID] = append(resp.LostDrops[charID], r.Item)
		}
	}
	return nil
}
