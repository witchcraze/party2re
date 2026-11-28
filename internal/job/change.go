package job

import (
	"context"
	"errors"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	corejob "github.com/witchcraze/party2re/internal/core/job"
	"github.com/witchcraze/party2re/internal/economy"
)

// ErrRequiredArmor is returned when changing to 炎闘士 (job-84) without armor-29 equipped.
var ErrRequiredArmor = errors.New("equipped armor-29 (炎の鎧) is required")

// ErrRequiredItem is returned when a required job-change item is missing.
var ErrRequiredItem = errors.New("required job-change item is missing")

// ChangeJob performs a normal Dharma job change for characterID into targetJobID.
// Transaction: delegates to economy.TransactionRunner.ExecuteTransaction.
// Lock Order: characters(2) -> inventory_items(3) -> equipment_slots(3) -> character_jobs(4).
func (s *Service) ChangeJob(ctx context.Context, characterID string, targetJobID string) (corecharacter.Character, corejob.CharacterJob, error) {
	if s.characters == nil {
		return corecharacter.Character{}, corejob.CharacterJob{}, errors.New("character repository not configured")
	}
	if s.economy == nil {
		return corecharacter.Character{}, corejob.CharacterJob{}, errors.New("economy service not configured")
	}
	char, err := s.characters.FindByID(ctx, characterID)
	if err != nil {
		return corecharacter.Character{}, corejob.CharacterJob{}, err
	}
	targetDef, err := s.GetDefinition(targetJobID)
	if err != nil {
		return corecharacter.Character{}, corejob.CharacterJob{}, err
	}
	if char.JobMemory != nil {
		return corecharacter.Character{}, corejob.CharacterJob{}, corejob.ErrJobUnavailable
	}

	requiredItem := targetDef.RequiredItem()
	req := economy.TransactionRequest{
		CharacterID:   characterID,
		LockInventory: requiredItem != "",
		CostFunc: func(lockedChar corecharacter.Character) (economy.ResourceCost, error) {
			if lockedChar.JobMemory != nil {
				return economy.ResourceCost{}, corejob.ErrJobUnavailable
			}
			// Evaluate character-level prerequisites before item deduction so
			// non-item rejections return ErrJobUnavailable rather than ErrRequiredItem.
			preCtx := s.buildChangeContextNoItems(lockedChar, corejob.CharacterJob{AllJobsMastered: true})
			if err := corejob.ValidateRequirements(targetDef, preCtx); err != nil {
				return economy.ResourceCost{}, err
			}
			needsItemPossession := corejob.RequiresItemPossession(targetDef, lockedChar.JobID, lockedChar.OldJobID)
			consumesItem := needsItemPossession && !corejob.IsItemExempt(targetJobID, lockedChar.JobID, lockedChar.OldJobID)
			if !consumesItem {
				return economy.ResourceCost{}, nil
			}
			return economy.ResourceCost{
				ItemDefinitionID:  requiredItem,
				ItemDefinitionQty: 1,
			}, nil
		},
	}

	var updated corecharacter.Character
	var updatedState corejob.CharacterJob
	_, err = s.economy.ExecuteTransaction(ctx, req, func(tc *economy.TxContext) error {
		if tc.Character.JobMemory != nil {
			return corejob.ErrJobUnavailable
		}
		needsItemPossession := corejob.RequiresItemPossession(targetDef, tc.Character.JobID, tc.Character.OldJobID)
		consumesItem := needsItemPossession && !corejob.IsItemExempt(targetJobID, tc.Character.JobID, tc.Character.OldJobID)
		if needsItemPossession && !consumesItem && tc.Inventory.Quantity(requiredItem) < 1 {
			return ErrRequiredItem
		}
		needArmor := corejob.IsArmorConsumed(targetJobID, tc.Character.JobID, tc.Character.OldJobID)
		var lockedEquip *coreequipment.Equipment
		if needArmor {
			equip, err := s.lockEquippedArmor(tc.Context, characterID, tc.Inventory)
			if err != nil {
				return err
			}
			lockedEquip = &equip
		}
		currentState, err := s.loadState(tc.Context, tc.Character)
		if err != nil {
			return err
		}
		// Only current/previous job matches (_is_need_job in _data.cgi:303-309) bypass
		// prerequisites and item/armor requirements; previously mastered jobs still require them.
		reqCtx := s.buildChangeContextNoItems(tc.Character, currentState)
		if err := corejob.ValidateRequirements(targetDef, reqCtx); err != nil {
			return err
		}
		if lockedEquip != nil {
			if err := s.consumeEquippedArmor(tc.Context, lockedEquip, &tc.Inventory); err != nil {
				return err
			}
		}
		// Legacy order: the change, then the departing job's mastery, then completion.
		if err := s.memories.Record(tc.Context, characterID, corecharacter.MemoryJobChange,
			s.jobName(tc.Character.JobID)+"から"+targetDef.Name+"に転職"); err != nil {
			return err
		}
		if err := s.recordDeparture(tc.Context, &currentState, tc.Character.SP); err != nil {
			return err
		}
		if err := s.checkAndNotifyCompletion(tc.Context, tc.Character.Name, &currentState); err != nil {
			return err
		}
		if err := currentState.ChangeTo(targetDef, tc.Character.Level, tc.Character.Gender); err != nil {
			return err
		}
		targetSPValue := targetSP(currentState, tc.Character, targetJobID)
		if err := s.recordPopularity(tc.Context, tc.Character); err != nil {
			return err
		}
		jobChanged := tc.Character.JobID != targetJobID
		if err := tc.Character.ApplyJobChange(targetJobID, targetSPValue); err != nil {
			return err
		}
		if jobChanged {
			if err := s.resetCostume(tc.Context, characterID); err != nil {
				return err
			}
		}
		if err := s.repository.Save(tc.Context, currentState); err != nil {
			return err
		}
		if s.jobTracker != nil {
			if err := s.jobTracker.RecordJobChange(tc.Context, characterID); err != nil {
				return err
			}
		}
		if s.guildPoints != nil {
			if err := s.guildPoints.AddGuildPoints(tc.Context, characterID, 50); err != nil {
				return err
			}
		}
		updated, updatedState = tc.Character, currentState
		return nil
	})
	if err != nil {
		if errors.Is(err, economy.ErrItemNotFound) || errors.Is(err, economy.ErrInsufficientItemQuantity) {
			return corecharacter.Character{}, corejob.CharacterJob{}, ErrRequiredItem
		}
		return corecharacter.Character{}, corejob.CharacterJob{}, err
	}
	return updated, updatedState, nil
}

// ApplyGrantedJobChange applies a job change another facility awards (god.cgi
// 天竜人) to char inside the caller's transaction, which must also persist char.
// Like ChangeJob it records the departing job and restores retained target SP,
// but applies no eligibility, item, guild-point, weekly-ranking or completion
// effects; the next ChangeJob departure evaluates completion.
func (s *Service) ApplyGrantedJobChange(ctx context.Context, char *corecharacter.Character, targetJobID string) error {
	return economy.WithRollbackScope(ctx, func(txCtx context.Context) error {
		targetDef, err := s.GetDefinition(targetJobID)
		if err != nil {
			return err
		}
		state, err := s.loadState(txCtx, *char)
		if err != nil {
			return err
		}
		if err := s.recordDeparture(txCtx, &state, char.SP); err != nil {
			return err
		}
		if err := state.Grant(targetDef); err != nil {
			return err
		}
		if err := s.recordPopularity(txCtx, *char); err != nil {
			return err
		}
		if err := char.ApplyJobChange(targetJobID, targetSP(state, *char, targetJobID)); err != nil {
			return err
		}
		if err := s.resetCostume(txCtx, char.ID); err != nil {
			return err
		}
		return s.repository.Save(txCtx, state)
	})
}

func (s *Service) resetCostume(ctx context.Context, characterID string) error {
	if s.costume == nil {
		return nil
	}
	return s.costume.ResetCostume(ctx, characterID)
}

// recordDeparture stores the leaving job's latest SP and first-time mastery.
func (s *Service) recordDeparture(ctx context.Context, state *corejob.CharacterJob, sp int) error {
	jobID := state.CurrentJobID
	if def, err := s.GetDefinition(jobID); err == nil && state.RecordMastery(jobID, sp, s.masterySP(def)) {
		if err := s.memories.Record(ctx, state.CharacterID, corecharacter.MemoryJobMaster, "★ "+def.Name+" Job Master! ★"); err != nil {
			return err
		}
	}
	return state.RecordJobSP(jobID, sp)
}

func (s *Service) recordPopularity(ctx context.Context, char corecharacter.Character) error {
	if s.jobPopularity == nil {
		return nil
	}
	return s.jobPopularity.RecordJobPopularity(ctx, char.JobID, char.Gender, char.Level/2)
}

// buildChangeContextNoItems assembles corejob.ChangeContext for ValidateRequirements calls.
// HasRequiredItem and HasEquippedArmor are always set to true so that ValidateRequirements
// only evaluates non-item conditions (level, gender, prerequisite tree, milestone counters).
// Actual item/armor presence is checked separately in the ChangeJob flow.
func (s *Service) buildChangeContextNoItems(char corecharacter.Character, state corejob.CharacterJob) corejob.ChangeContext {
	return corejob.ChangeContext{
		Level:            char.Level,
		Gender:           char.Gender,
		CurrentJobID:     char.JobID,
		OldJobID:         char.OldJobID,
		SP:               char.SP,
		JobLevel:         char.JobLevel,
		PvPWins:          char.PvPWins,
		HeroCount:        char.HeroCount,
		CasinoWins:       char.CasinoWins,
		MonsterKills:     char.MonsterKills,
		MaoCount:         char.MaoCount,
		AllJobsMastered:  state.AllJobsMastered || state.HasMasteredAllCompletionJobs(),
		HasRequiredItem:  true, // item possession is verified separately (see needsItemPossession above)
		HasEquippedArmor: true, // armor presence is verified separately (see needArmor above)
	}
}

// lockEquippedArmor locks equipment_slots and checks that armor-29 is equipped in the body slot.
func (s *Service) lockEquippedArmor(ctx context.Context, characterID string, inv coreinventory.Inventory) (coreequipment.Equipment, error) {
	if s.equipment == nil {
		return coreequipment.Equipment{}, ErrRequiredArmor
	}
	equip, err := s.equipment.FindByCharacterIDForUpdate(ctx, characterID)
	if err != nil {
		return coreequipment.Equipment{}, err
	}
	if !armorEquipped(equip, inv) {
		return coreequipment.Equipment{}, ErrRequiredArmor
	}
	return equip, nil
}

// consumeEquippedArmor unequips and destroys armor-29 from the body slot (job_change.cgi:181-188).
func (s *Service) consumeEquippedArmor(ctx context.Context, equip *coreequipment.Equipment, inv *coreinventory.Inventory) error {
	if equip == nil || inv == nil {
		return ErrRequiredArmor
	}
	instID, err := equip.Unequip(coreitem.SlotBody)
	if err != nil {
		return err
	}
	if err := inv.Consume(instID, 1); err != nil {
		return err
	}
	return s.equipment.Save(ctx, *equip)
}
