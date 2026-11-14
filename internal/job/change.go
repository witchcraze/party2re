package job

import (
	"context"
	"errors"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	corejob "github.com/witchcraze/party2re/internal/core/job"
	"github.com/witchcraze/party2re/internal/economy"
)

// ErrRequiredArmor is returned when changing to 炎闘士 (job-84) without armor-29 equipped.
var ErrRequiredArmor = errors.New("equipped armor-29 (炎の鎧) is required")

// ErrRequiredItem is returned when a required job-change item is missing.
var ErrRequiredItem = errors.New("required job-change item is missing")

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
	state, err := s.loadState(ctx, char)
	if err != nil {
		return corecharacter.Character{}, corejob.CharacterJob{}, err
	}

	// Build a ChangeContext for ValidateRequirements, populating inventory/equipment
	// state only for the specific jobs that need them.
	// Mastered jobs bypass ValidateRequirements: having mastered a job proves requirements
	// were satisfied before, and current/old job matches also bypass (job_change.cgi:164).
	isBypassJob := targetJobID == char.JobID || targetJobID == char.OldJobID || state.IsMastered(targetJobID)
	if !isBypassJob {
		// Set HasRequiredItem and HasEquippedArmor to true so ValidateRequirements only
		// checks non-item conditions (level, gender, prerequisite tree, milestone counters).
		// Actual item/armor possession is verified separately below to produce the
		// correct ErrRequiredItem / ErrRequiredArmor error instead of ErrJobUnavailable.
		reqCtx := s.buildChangeContextNoItems(char, state)
		if err := corejob.ValidateRequirements(targetDef, reqCtx); err != nil {
			return corecharacter.Character{}, corejob.CharacterJob{}, err
		}
	}

	requiredItem := targetDef.RequiredItem()
	needsItemPossession := !isBypassJob && corejob.RequiresItemPossession(targetDef, char.JobID, char.OldJobID)
	consumesItem := needsItemPossession && !corejob.IsItemExempt(targetJobID, char.JobID, char.OldJobID)
	needArmor := !isBypassJob && corejob.IsArmorConsumed(targetJobID, char.JobID, char.OldJobID)

	// Pre-validate required item possession before executing mutations.
	if needsItemPossession {
		if s.inventories == nil {
			return corecharacter.Character{}, corejob.CharacterJob{}, ErrRequiredItem
		}
		inventory, err := s.inventories.FindByCharacterID(ctx, characterID)
		if err != nil || inventory.Quantity(requiredItem) < 1 {
			return corecharacter.Character{}, corejob.CharacterJob{}, ErrRequiredItem
		}
	}

	// Pre-validate equipped armor before executing mutations.
	if needArmor {
		if err := s.verifyEquippedArmor(ctx, characterID); err != nil {
			return corecharacter.Character{}, corejob.CharacterJob{}, err
		}
	}

	req := economy.TransactionRequest{CharacterID: characterID, LockInventory: consumesItem}
	if consumesItem {
		req.Cost.ItemDefinitionID = requiredItem
		req.Cost.ItemDefinitionQty = 1
	}
	var updated corecharacter.Character
	var updatedState corejob.CharacterJob
	_, err = s.economy.ExecuteTransaction(ctx, req, func(tc *economy.TxContext) error {
		if tc.Character.JobMemory != nil {
			return corejob.ErrJobUnavailable
		}
		if needArmor {
			if err := s.consumeEquippedArmor(tc.Context, characterID); err != nil {
				return err
			}
		}
		currentState, err := s.loadState(tc.Context, tc.Character)
		if err != nil {
			return err
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
		if err := tc.Character.ApplyJobChange(targetJobID, targetSPValue); err != nil {
			return err
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
	if s.costume != nil {
		//lint:ignore error-swallow best-effort costume rental return on job change
		_ = s.costume.ResetCostume(ctx, characterID)
	}
	return updated, updatedState, nil
}

// ApplyGrantedJobChange applies a job change another facility awards (god.cgi
// 天竜人) to char inside the caller's transaction, which must also persist char.
// Like ChangeJob it records the departing job and restores retained target SP,
// but applies no eligibility, item, guild-point, weekly-ranking or completion
// effects; the next ChangeJob departure evaluates completion.
func (s *Service) ApplyGrantedJobChange(ctx context.Context, char *corecharacter.Character, targetJobID string) error {
	targetDef, err := s.GetDefinition(targetJobID)
	if err != nil {
		return err
	}
	state, err := s.loadState(ctx, *char)
	if err != nil {
		return err
	}
	if err := s.recordDeparture(ctx, &state, char.SP); err != nil {
		return err
	}
	if err := state.Grant(targetDef); err != nil {
		return err
	}
	if err := s.recordPopularity(ctx, *char); err != nil {
		return err
	}
	if err := char.ApplyJobChange(targetJobID, targetSP(state, *char, targetJobID)); err != nil {
		return err
	}
	return s.repository.Save(ctx, state)
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

// verifyEquippedArmor checks that armor-29 is currently equipped in the body slot.
func (s *Service) verifyEquippedArmor(ctx context.Context, characterID string) error {
	if s.equipment == nil || s.inventories == nil {
		return ErrRequiredArmor
	}
	equip, err := s.equipment.FindByCharacterID(ctx, characterID)
	if err != nil {
		return ErrRequiredArmor
	}
	instID, ok := equip.Equipped(coreitem.SlotBody)
	if !ok {
		return ErrRequiredArmor
	}
	inv, err := s.inventories.FindByCharacterID(ctx, characterID)
	if err != nil {
		return ErrRequiredArmor
	}
	inst, found := inv.Find(instID)
	if !found || inst.DefinitionID != "armor-29" {
		return ErrRequiredArmor
	}
	return nil
}

// consumeEquippedArmor unequips and destroys armor-29 from the body slot (job_change.cgi:181-188).
func (s *Service) consumeEquippedArmor(ctx context.Context, characterID string) error {
	if s.equipment == nil || s.inventories == nil {
		return ErrRequiredArmor
	}
	equip, err := s.equipment.FindByCharacterID(ctx, characterID)
	if err != nil {
		return ErrRequiredArmor
	}
	instID, ok := equip.Equipped(coreitem.SlotBody)
	if !ok {
		return ErrRequiredArmor
	}
	inv, err := s.inventories.FindByCharacterID(ctx, characterID)
	if err != nil {
		return ErrRequiredArmor
	}
	inst, found := inv.Find(instID)
	if !found || inst.DefinitionID != "armor-29" {
		return ErrRequiredArmor
	}
	// Unequip from body slot, then consume the instance from inventory.
	if _, err := equip.Unequip(coreitem.SlotBody); err != nil {
		return err
	}
	if err := inv.Consume(instID, 1); err != nil {
		return err
	}
	if err := s.equipment.Save(ctx, equip); err != nil {
		return err
	}
	return s.inventories.Save(ctx, inv)
}

func (s *Service) Change(ctx context.Context, characterID string, target corejob.Definition, level int, gender string) (corejob.CharacterJob, error) {
	state, err := s.repository.FindByCharacterID(ctx, characterID)
	if err != nil {
		state, err = corejob.NewCharacterJob(characterID, "starter")
		if err != nil {
			return corejob.CharacterJob{}, err
		}
	}
	if err := state.ChangeTo(target, level, gender); err != nil {
		return corejob.CharacterJob{}, err
	}
	if err := s.repository.Save(ctx, state); err != nil {
		return corejob.CharacterJob{}, err
	}
	return state, nil
}
