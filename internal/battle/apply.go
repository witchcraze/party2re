package battle

import (
	"context"
	"maps"
	"sort"
	"strings"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/progression"
	"github.com/witchcraze/party2re/internal/economy"
)

// ApplyPostBattleRequest specifies combatants, resolution, and additional rewards to persist.
type ApplyPostBattleRequest struct {
	CharacterIDs          []string
	BattleResult          corebattle.PartyBattleResult
	PreserveEarnedRewards bool                // TotalReward currency/EXP/crystals were earned before the final outcome.
	DropItems             []string            // Extra dropped item definition IDs (e.g. stage/boss drops)
	RecipientDrops        map[string][]string // Optional recipient-targeted drop item definition IDs (characterID -> []itemDefID)
	RecipientCharacterID  string              // Optional recipient character ID for DropItems / BattleResult.TotalReward drops
	DefeatedEnemies       []corebattle.Participant
	Habitat               string // Habitat/stage name for monster book recording
	ExitFatigue           int    // Crawl exit fatigue added to each participant (lib/_battle.cgi:1187-1188, 1216)
	UnsealDemonKing       bool   // Completing Stage EX / unsealing demon king (vs_monster.cgi:218)
}

// ApplyPostBattleResponse contains the committed state for each participating character.
type ApplyPostBattleResponse struct {
	UpdatedCharacters map[string]corecharacter.Character
	GainedExperience  map[string]int
	GainedGold        map[string]int
	GainedCrystals    map[string]int
	LevelUpResults    map[string]progression.LevelUpResult
	InventoryDrops    map[string][]coreitem.Instance
	DepotDeliveries   map[string][]coreitem.Instance
	LostDrops         map[string][]coreitem.Instance
	ConsumedItems     map[string][]corebattle.ConsumedItem
	RemainingStatus   map[string]string
	MonsterTames      map[string][]MonsterTameResult
}

// ApplyPostBattleResult applies battle outcomes (HP, MP, fatigue, consumed items, EXP, gold, crystals, drops)
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
	resp := newApplyPostBattleResponse(req.BattleResult)
	txReq := economy.TransactionRequest{
		CharacterID:   charID,
		LockInventory: true,
	}

	var finalChar corecharacter.Character
	var finalInv coreinventory.Inventory
	var finalEquip coreequipment.Equipment

	_, err := s.txRunner.ExecuteTransaction(ctx, txReq, func(tc *economy.TxContext) error {
		// 1. Load equipment (Rank 3) if available
		var equip coreequipment.Equipment
		var equipModified bool
		if s.equipRepo != nil {
			loadedEquip, err := s.equipRepo.FindByCharacterID(tc.Context, charID)
			if err != nil {
				return err
			}
			equip = loadedEquip
		}

		// 2. Apply HP, MP, fatigue
		s.applyResourceUpdates(&tc.Character, charID, req.BattleResult, req.ExitFatigue)

		// 3. Process item consumption
		slots := equip.Slots
		if err := consumeBufferedItems(&tc.Inventory, &equip, req.BattleResult.ConsumedItems[charID]); err != nil {
			return err
		}
		equipModified = !maps.Equal(slots, equip.Slots)
		resp.ConsumedItems[charID] = req.BattleResult.ConsumedItems[charID]

		// 4. Save equipment if modified
		if equipModified && s.equipRepo != nil {
			if err := s.equipRepo.Save(tc.Context, equip); err != nil {
				return err
			}
		}

		// 5. Apply earned assets independently of the final encounter outcome.
		if req.BattleResult.Outcome == corebattle.OutcomeWin || req.PreserveEarnedRewards {
			var charDrops []string
			if req.BattleResult.Outcome == corebattle.OutcomeWin {
				charDrops = req.dropsForCharacter(charID)
			}
			gainedGold, gainedEXP, gainedCrystals, lvlRes, invDrops, depotDrops, err := s.applyRewardsForCharacter(
				tc.Context,
				&tc.Character,
				&tc.Inventory,
				req.BattleResult.TotalReward,
				charDrops,
			)
			if err != nil {
				return err
			}
			resp.GainedGold[charID] = gainedGold
			resp.GainedExperience[charID] = gainedEXP
			resp.GainedCrystals[charID] = gainedCrystals
			resp.LevelUpResults[charID] = lvlRes
			resp.InventoryDrops[charID] = invDrops

			// Deliver overflow drops to Depot (Rank 5)
			if err := s.deliverToDepot(tc.Context, charID, tc.Character, depotDrops, &resp); err != nil {
				return err
			}
		}

		if req.BattleResult.Outcome == corebattle.OutcomeWin {
			s.applyMonsterKills(&tc.Character, tc.Inventory, equip, req.BattleResult, req.DefeatedEnemies)
			s.applyMaoCount(&tc.Character, req)

			if s.monsterRecorder != nil && len(req.DefeatedEnemies) > 0 {
				if err := s.recordDefeatedMonsters(tc.Context, []string{charID}, req.DefeatedEnemies, req.Habitat); err != nil {
					return err
				}
			}
		}

		resp.UpdatedCharacters[charID] = tc.Character
		finalChar = tc.Character
		finalInv = tc.Inventory
		finalEquip = equip
		return nil
	})
	if err != nil {
		return ApplyPostBattleResponse{}, err
	}

	if req.BattleResult.Outcome == corebattle.OutcomeWin && len(req.DefeatedEnemies) > 0 {
		if tames := s.processMonsterTaming(ctx, finalChar, finalInv, finalEquip, req.DefeatedEnemies); len(tames) > 0 {
			resp.MonsterTames[charID] = tames
		}
	}

	return resp, nil
}

func (s *Service) applyMultiCharacterWithProvider(ctx context.Context, sortedIDs []string, req ApplyPostBattleRequest) (ApplyPostBattleResponse, error) {
	resp := newApplyPostBattleResponse(req.BattleResult)

	var finalChars map[string]corecharacter.Character
	var finalInvs map[string]coreinventory.Inventory
	var finalEquips map[string]coreequipment.Equipment

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
				equip, err := s.equipRepo.FindByCharacterID(txCtx, id)
				if err != nil {
					return err
				}
				equips[id] = equip
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

			s.applyResourceUpdates(&char, id, req.BattleResult, req.ExitFatigue)

			slots := equip.Slots
			if err := consumeBufferedItems(&inv, &equip, req.BattleResult.ConsumedItems[id]); err != nil {
				return err
			}
			equipModified = !maps.Equal(slots, equip.Slots)
			resp.ConsumedItems[id] = req.BattleResult.ConsumedItems[id]

			if equipModified {
				equipsModified[id] = true
				equips[id] = equip
			}

			if req.BattleResult.Outcome == corebattle.OutcomeWin || req.PreserveEarnedRewards {
				var charDrops []string
				if req.BattleResult.Outcome == corebattle.OutcomeWin {
					charDrops = req.dropsForCharacter(id)
				}
				gainedGold, gainedEXP, gainedCrystals, lvlRes, invDrops, depotDrops, err := s.applyRewardsForCharacter(
					txCtx,
					&char,
					&inv,
					req.BattleResult.TotalReward,
					charDrops,
				)
				if err != nil {
					return err
				}
				resp.GainedGold[id] = gainedGold
				resp.GainedExperience[id] = gainedEXP
				resp.GainedCrystals[id] = gainedCrystals
				resp.LevelUpResults[id] = lvlRes
				resp.InventoryDrops[id] = invDrops
				if len(depotDrops) > 0 {
					pendingDepotDeliveries[id] = append(pendingDepotDeliveries[id], depotDrops...)
				}
			}

			if req.BattleResult.Outcome == corebattle.OutcomeWin {
				s.applyMonsterKills(&char, inv, equip, req.BattleResult, req.DefeatedEnemies)
				s.applyMaoCount(&char, req)
			}

			chars[id] = char
			invs[id] = inv
			resp.UpdatedCharacters[id] = char
		}

		// Phase 4: Lock & deliver to Depots in ascending order (Rank 5)
		for _, id := range sortedIDs {
			items := pendingDepotDeliveries[id]
			if err := s.deliverToDepot(txCtx, id, chars[id], items, &resp); err != nil {
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
			// Remove equipped references before deleting consumed inventory rows.
			if equipsModified[id] && s.equipRepo != nil {
				if err := s.equipRepo.Save(txCtx, equips[id]); err != nil {
					return err
				}
			}
			if s.invRepo != nil {
				if err := s.invRepo.Save(txCtx, invs[id]); err != nil {
					return err
				}
			}
		}

		// Phase 6: Record defeated monsters (Rank 8) if won
		if req.BattleResult.Outcome == corebattle.OutcomeWin && len(req.DefeatedEnemies) > 0 {
			if s.monsterRecorder != nil {
				if err := s.recordDefeatedMonsters(txCtx, sortedIDs, req.DefeatedEnemies, req.Habitat); err != nil {
					return err
				}
			}
		}

		finalChars = chars
		finalInvs = invs
		finalEquips = equips
		return nil
	})
	if err != nil {
		return ApplyPostBattleResponse{}, err
	}

	if req.BattleResult.Outcome == corebattle.OutcomeWin && len(req.DefeatedEnemies) > 0 {
		recipientID := req.RecipientCharacterID
		if recipientID == "" && len(sortedIDs) > 0 {
			recipientID = sortedIDs[0]
		}
		if char, ok := finalChars[recipientID]; ok {
			inv := finalInvs[recipientID]
			equip := finalEquips[recipientID]
			if tames := s.processMonsterTaming(ctx, char, inv, equip, req.DefeatedEnemies); len(tames) > 0 {
				resp.MonsterTames[recipientID] = tames
			}
		}
	}

	return resp, nil
}

func (s *Service) recordDefeatedMonsters(ctx context.Context, charIDs []string, defeatedEnemies []corebattle.Participant, habitat string) error {
	for _, enemy := range defeatedEnemies {
		if strings.HasPrefix(enemy.ID, "char-") || strings.HasPrefix(enemy.ID, "user-") {
			continue
		}
		monsterID := ParseDefeatedMonsterID(enemy.ID)
		baseName := CleanMonsterName(enemy.Name)
		hp := enemy.MaxHP
		if hp <= 0 {
			hp = enemy.HP
		}
		mp := enemy.MaxMP
		if mp <= 0 {
			mp = enemy.MP
		}
		strong := Strong(hp, mp, enemy.Attack, enemy.Defense, enemy.Agility)
		rec := DefeatedMonsterRecord{
			MonsterID:        monsterID,
			MonsterName:      baseName,
			Habitat:          habitat,
			Icon:             enemy.Icon,
			Strong:           strong,
			HP:               hp,
			MP:               mp,
			Attack:           enemy.Attack,
			Defense:          enemy.Defense,
			Agility:          enemy.Agility,
			ExperienceReward: enemy.ExperienceReward,
			GoldReward:       enemy.GoldReward,
		}
		for _, cID := range charIDs {
			if err := s.monsterRecorder.RecordMonsterDefeat(ctx, cID, rec); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) applyResourceUpdates(char *corecharacter.Character, charID string, res corebattle.PartyBattleResult, exitFatigue int) {
	if remHP, ok := res.RemainingHP[charID]; ok {
		remMP := -1
		if mp, hasMP := res.RemainingMP[charID]; hasMP && mp >= 0 {
			remMP = mp
		}
		char.ApplyCombatSurvival(remHP, remMP, remHP <= 0)
	} else if remMP, ok := res.RemainingMP[charID]; ok && remMP >= 0 {
		char.Stats.MP = remMP
		char.Stats.ClampVitality()
	}

	if res.BanishedIDs != nil && res.BanishedIDs[charID] {
		char.AddTired(30)
	}
	if exitFatigue > 0 {
		char.AddTired(exitFatigue)
	}
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

func newApplyPostBattleResponse(res corebattle.PartyBattleResult) ApplyPostBattleResponse {
	return ApplyPostBattleResponse{
		UpdatedCharacters: make(map[string]corecharacter.Character),
		GainedExperience:  make(map[string]int),
		GainedGold:        make(map[string]int),
		GainedCrystals:    make(map[string]int),
		LevelUpResults:    make(map[string]progression.LevelUpResult),
		InventoryDrops:    make(map[string][]coreitem.Instance),
		DepotDeliveries:   make(map[string][]coreitem.Instance),
		LostDrops:         make(map[string][]coreitem.Instance),
		ConsumedItems:     make(map[string][]corebattle.ConsumedItem),
		RemainingStatus:   res.RemainingStatus,
		MonsterTames:      make(map[string][]MonsterTameResult),
	}
}
