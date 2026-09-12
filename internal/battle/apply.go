package battle

import (
	"context"
	"errors"
	"sort"
	"strings"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/job"
	"github.com/witchcraze/party2re/internal/core/progression"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/economy"
)

// ApplyPostBattleRequest specifies combatants, resolution, and additional rewards to persist.
type ApplyPostBattleRequest struct {
	CharacterIDs []string
	BattleResult corebattle.PartyBattleResult
	DropItems    []string // Extra dropped item definition IDs (e.g. stage/boss drops)
}

// ApplyPostBattleResponse contains the committed state for each participating character.
type ApplyPostBattleResponse struct {
	UpdatedCharacters map[string]corecharacter.Character
	GainedExperience  map[string]int
	GainedGold        map[string]int
	LevelUpResults    map[string]progression.LevelUpResult
	InventoryDrops    map[string][]coreitem.Instance
	DepotDeliveries   map[string][]coreitem.Instance
	ConsumedItems     map[string][]corebattle.ConsumedItem
}

// ApplyPostBattleResult applies battle outcomes (HP, MP, CMP, status, consumed items, EXP, gold, drops)
// atomically adhering to the deterministic row-lock hierarchy (Rank 2: Character -> Rank 3: Inventory/Equip -> Rank 5: Depot).
func (s *Service) ApplyPostBattleResult(ctx context.Context, req ApplyPostBattleRequest) (ApplyPostBattleResponse, error) {
	if len(req.CharacterIDs) == 0 {
		return ApplyPostBattleResponse{}, ErrInvalidRequest
	}

	// Deduplicate and sort character IDs in ascending lexicographical order for Rank 2 locking
	sortedIDs := deduplicateAndSortIDs(req.CharacterIDs)

	// Single character and TransactionRunner configured -> route through economy.TransactionRunner
	if len(sortedIDs) == 1 && s.txRunner != nil {
		return s.applySingleCharacterWithRunner(ctx, sortedIDs[0], req)
	}

	// Multi-character party or fallback to TransactionProvider
	return s.applyMultiCharacterWithProvider(ctx, sortedIDs, req)
}

func (s *Service) applySingleCharacterWithRunner(ctx context.Context, charID string, req ApplyPostBattleRequest) (ApplyPostBattleResponse, error) {
	resp := newApplyPostBattleResponse()
	txReq := economy.TransactionRequest{
		CharacterID:   charID,
		LockInventory: true,
	}

	_, err := s.txRunner.ExecuteTransaction(ctx, txReq, func(tc *economy.TxContext) error {
		// 1. Load equipment (Rank 3) if available
		var equip coreequipment.Equipment
		var equipModified bool
		if s.equipRepo != nil {
			if loadedEquip, err := s.equipRepo.FindByCharacterID(tc.Context, charID); err == nil {
				equip = loadedEquip
			}
		}

		// 2. Apply status, HP, MP, CMP
		s.applyResourceUpdates(&tc.Character, charID, req.BattleResult)

		// 3. Process item consumption
		consumed := s.applyConsumedItems(charID, req.BattleResult.ConsumedItems[charID], &tc.Inventory, &equip, &equipModified)
		resp.ConsumedItems[charID] = consumed

		// 4. Save equipment if modified
		if equipModified && s.equipRepo != nil {
			if err := s.equipRepo.Save(tc.Context, equip); err != nil {
				return err
			}
		}

		// 5. Apply victory rewards
		if req.BattleResult.Outcome == corebattle.OutcomeWin {
			gainedGold, gainedEXP, lvlRes, invDrops, depotDrops, err := s.applyRewardsForCharacter(
				tc.Context,
				&tc.Character,
				&tc.Inventory,
				req.BattleResult.TotalReward,
				req.DropItems,
			)
			if err != nil {
				return err
			}
			resp.GainedGold[charID] = gainedGold
			resp.GainedExperience[charID] = gainedEXP
			resp.LevelUpResults[charID] = lvlRes
			resp.InventoryDrops[charID] = invDrops
			resp.DepotDeliveries[charID] = depotDrops

			// Deliver overflow drops to Depot (Rank 5)
			if len(depotDrops) > 0 && s.depotRepo != nil {
				dep, err := s.depotRepo.FindByCharacterIDForUpdate(tc.Context, charID)
				if errors.Is(err, depot.ErrNotFound) {
					dep, err = depot.NewDepotWithCapacity(charID, tc.Character.JobLevel, 0, tc.Character.OverDepot)
					if err != nil {
						return err
					}
				} else if err != nil {
					return err
				}
				for _, inst := range depotDrops {
					_ = dep.AddItem(inst)
				}
				if err := s.depotRepo.Save(tc.Context, dep); err != nil {
					return err
				}
			}
		}

		resp.UpdatedCharacters[charID] = tc.Character
		return nil
	})
	if err != nil {
		return ApplyPostBattleResponse{}, err
	}
	return resp, nil
}

func (s *Service) applyMultiCharacterWithProvider(ctx context.Context, sortedIDs []string, req ApplyPostBattleRequest) (ApplyPostBattleResponse, error) {
	resp := newApplyPostBattleResponse()

	err := s.runInTx(ctx, func(txCtx context.Context) error {
		// Phase 1: Lock characters in ascending lexicographical order (Rank 2)
		chars := make(map[string]corecharacter.Character, len(sortedIDs))
		for _, id := range sortedIDs {
			char, err := s.charRepo.FindByIDForUpdate(txCtx, id)
			if err != nil {
				return ErrCharacterNotFound
			}
			chars[id] = char
		}

		// Phase 2: Lock inventories in ascending order (Rank 3) & load equipment
		invs := make(map[string]coreinventory.Inventory, len(sortedIDs))
		equips := make(map[string]coreequipment.Equipment, len(sortedIDs))
		equipsModified := make(map[string]bool, len(sortedIDs))

		for _, id := range sortedIDs {
			if s.invRepo != nil {
				inv, err := s.invRepo.FindByCharacterIDForUpdate(txCtx, id)
				if err != nil {
					return ErrInventoryNotFound
				}
				invs[id] = inv
			}
			if s.equipRepo != nil {
				if equip, err := s.equipRepo.FindByCharacterID(txCtx, id); err == nil {
					equips[id] = equip
				}
			}
		}

		// Staged depot deliveries per character
		pendingDepotDeliveries := make(map[string][]coreitem.Instance)

		// Phase 3: Mutate character state, consumables, and rewards
		for _, id := range sortedIDs {
			char := chars[id]
			inv := invs[id]
			equip := equips[id]
			var equipModified bool

			s.applyResourceUpdates(&char, id, req.BattleResult)

			consumed := s.applyConsumedItems(id, req.BattleResult.ConsumedItems[id], &inv, &equip, &equipModified)
			resp.ConsumedItems[id] = consumed

			if equipModified {
				equipsModified[id] = true
				equips[id] = equip
			}

			if req.BattleResult.Outcome == corebattle.OutcomeWin {
				gainedGold, gainedEXP, lvlRes, invDrops, depotDrops, err := s.applyRewardsForCharacter(
					txCtx,
					&char,
					&inv,
					req.BattleResult.TotalReward,
					req.DropItems,
				)
				if err != nil {
					return err
				}
				resp.GainedGold[id] = gainedGold
				resp.GainedExperience[id] = gainedEXP
				resp.LevelUpResults[id] = lvlRes
				resp.InventoryDrops[id] = invDrops
				if len(depotDrops) > 0 {
					pendingDepotDeliveries[id] = append(pendingDepotDeliveries[id], depotDrops...)
					resp.DepotDeliveries[id] = depotDrops
				}
			}

			chars[id] = char
			invs[id] = inv
			resp.UpdatedCharacters[id] = char
		}

		// Phase 4: Lock & deliver to Depots in ascending order (Rank 5)
		for _, id := range sortedIDs {
			items := pendingDepotDeliveries[id]
			if len(items) == 0 || s.depotRepo == nil {
				continue
			}

			dep, err := s.depotRepo.FindByCharacterIDForUpdate(txCtx, id)
			if errors.Is(err, depot.ErrNotFound) {
				char := chars[id]
				dep, err = depot.NewDepotWithCapacity(id, char.JobLevel, 0, char.OverDepot)
				if err != nil {
					return err
				}
			} else if err != nil {
				return err
			}

			for _, inst := range items {
				_ = dep.AddItem(inst)
			}
			if err := s.depotRepo.Save(txCtx, dep); err != nil {
				return err
			}
		}

		// Phase 5: Persist entities in deterministic order (Rank 2 -> Rank 3)
		for _, id := range sortedIDs {
			if err := s.charRepo.Update(txCtx, chars[id]); err != nil {
				return err
			}
		}
		for _, id := range sortedIDs {
			if s.invRepo != nil {
				if err := s.invRepo.Save(txCtx, invs[id]); err != nil {
					return err
				}
			}
			if equipsModified[id] && s.equipRepo != nil {
				if err := s.equipRepo.Save(txCtx, equips[id]); err != nil {
					return err
				}
			}
		}

		return nil
	})
	if err != nil {
		return ApplyPostBattleResponse{}, err
	}
	return resp, nil
}

func (s *Service) applyResourceUpdates(char *corecharacter.Character, charID string, res corebattle.PartyBattleResult) {
	if remHP, ok := res.RemainingHP[charID]; ok {
		if remHP <= 0 {
			char.Stats.HP = 1 // Fallen combatant survives with 1 HP
		} else {
			char.Stats.HP = remHP
			if char.Stats.MaxHP > 0 && char.Stats.HP > char.Stats.MaxHP {
				char.Stats.HP = char.Stats.MaxHP
			}
		}
	}

	if remMP, ok := res.RemainingMP[charID]; ok && remMP >= 0 {
		char.Stats.MP = remMP
		if char.Stats.MaxMP > 0 && char.Stats.MP > char.Stats.MaxMP {
			char.Stats.MP = char.Stats.MaxMP
		}
	}
}

func (s *Service) applyConsumedItems(
	charID string,
	consumed []corebattle.ConsumedItem,
	inv *coreinventory.Inventory,
	equip *coreequipment.Equipment,
	equipModified *bool,
) []corebattle.ConsumedItem {
	if len(consumed) == 0 || inv == nil {
		return nil
	}

	applied := make([]corebattle.ConsumedItem, 0, len(consumed))
	for _, itemEntry := range consumed {
		// 1. Unequip slot if this consumed item was equipped
		if itemEntry.InstanceID != "" && equip != nil && equip.Slots != nil {
			for slot, equippedInstID := range equip.Slots {
				if equippedInstID == itemEntry.InstanceID {
					delete(equip.Slots, slot)
					if equipModified != nil {
						*equipModified = true
					}
					break
				}
			}
		}

		// 2. Consume from inventory
		if itemEntry.InstanceID != "" {
			if err := inv.Consume(itemEntry.InstanceID, itemEntry.Quantity); err == nil {
				applied = append(applied, itemEntry)
				continue
			}
		}

		// Fallback: consume by DefinitionID
		remaining := itemEntry.Quantity
		for _, inst := range inv.Items {
			if inst.DefinitionID == itemEntry.ID && inst.Quantity > 0 {
				toTake := inst.Quantity
				if toTake > remaining {
					toTake = remaining
				}
				_ = inv.Consume(inst.ID, toTake)
				remaining -= toTake
				if remaining <= 0 {
					break
				}
			}
		}
		applied = append(applied, itemEntry)
	}
	return applied
}

func (s *Service) applyRewardsForCharacter(
	ctx context.Context,
	char *corecharacter.Character,
	inv *coreinventory.Inventory,
	reward corebattle.Reward,
	extraDrops []string,
) (int, int, progression.LevelUpResult, []coreitem.Instance, []coreitem.Instance, error) {
	gainedGold := reward.Currency
	_ = char.AddMoney(gainedGold)

	gainedEXP := reward.Experience
	var lvlRes progression.LevelUpResult
	if gainedEXP > 0 {
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
		if err == nil {
			lvlRes = res
		}
	}

	// Item drop collection & depot fallback
	var dropDefIDs []string
	if reward.ItemDefinitionID != "" && reward.ItemQuantity > 0 {
		for i := 0; i < reward.ItemQuantity; i++ {
			dropDefIDs = append(dropDefIDs, reward.ItemDefinitionID)
		}
	}
	dropDefIDs = append(dropDefIDs, extraDrops...)

	var invDrops []coreitem.Instance
	var depotDrops []coreitem.Instance

	for _, defID := range dropDefIDs {
		inst, err := coreitem.NewInstance(defID, 1)
		if err != nil {
			continue
		}

		// Check inventory capacity
		if len(inv.Items) < s.maxInventoryCapacity() {
			if err := inv.Add(inst); err == nil {
				invDrops = append(invDrops, inst)
				continue
			}
		}

		// Overflow routes to depot
		depotDrops = append(depotDrops, inst)
	}

	return gainedGold, gainedEXP, lvlRes, invDrops, depotDrops, nil
}

func (s *Service) maxInventoryCapacity() int {
	if s.maxInvCap > 0 {
		return s.maxInvCap
	}
	return 1 // Default 1 matching legacy $m{ite}
}

func deduplicateAndSortIDs(ids []string) []string {
	seen := make(map[string]bool, len(ids))
	result := make([]string, 0, len(ids))
	for _, id := range ids {
		trimmed := strings.TrimSpace(id)
		if trimmed != "" && !seen[trimmed] {
			seen[trimmed] = true
			result = append(result, trimmed)
		}
	}
	sort.Strings(result)
	return result
}

func newApplyPostBattleResponse() ApplyPostBattleResponse {
	return ApplyPostBattleResponse{
		UpdatedCharacters: make(map[string]corecharacter.Character),
		GainedExperience:  make(map[string]int),
		GainedGold:        make(map[string]int),
		LevelUpResults:    make(map[string]progression.LevelUpResult),
		InventoryDrops:    make(map[string][]coreitem.Instance),
		DepotDeliveries:   make(map[string][]coreitem.Instance),
		ConsumedItems:     make(map[string][]corebattle.ConsumedItem),
	}
}
