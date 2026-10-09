package job

import (
	"context"
	"errors"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	corejob "github.com/witchcraze/party2re/internal/core/job"
	"github.com/witchcraze/party2re/internal/economy"
)

// RestoreActiveJob restores state.CurrentJobID to jobID and saves state to repository.
func (s *Service) RestoreActiveJob(ctx context.Context, characterID, jobID string) error {
	if s.repository == nil {
		return errors.New("job repository is nil")
	}
	state, err := s.repository.FindByCharacterID(ctx, characterID)
	if err != nil {
		if isNotFound(err) {
			state, err = corejob.NewCharacterJob(characterID, jobID)
			if err != nil {
				return err
			}
		} else {
			return err
		}
	}
	state.RestoreCurrentJob(jobID)
	return s.repository.Save(ctx, state)
}

// ExchangeJob swaps a character's current and old job classes using item-168 (persistent) or item-243 (temporary).
func (s *Service) ExchangeJob(ctx context.Context, characterID, targetJobID, targetOldJobID string, itemDefinitionID ...string) (corecharacter.Character, corejob.CharacterJob, error) {
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
	if char.OverLevel {
		return corecharacter.Character{}, corejob.CharacterJob{}, corejob.ErrJobUnavailable
	}
	if char.JobMemory != nil {
		if len(itemDefinitionID) > 0 && itemDefinitionID[0] != "" {
			// Overlapping start with an item is rejected while either memory kind is active.
			return corecharacter.Character{}, corejob.CharacterJob{}, corejob.ErrJobUnavailable
		}
		if char.JobMemory.IsTemporary() {
			// Temporary job memory (item-243) cannot be manually restored via exchange.
			// It only reverts upon resting at home (legacy home.cgi:195, job_change.cgi:121).
			return corecharacter.Character{}, corejob.CharacterJob{}, corejob.ErrJobUnavailable
		}
		req := economy.TransactionRequest{CharacterID: characterID}
		var restored corecharacter.Character
		var restoredState corejob.CharacterJob
		_, err := s.economy.ExecuteTransaction(ctx, req, func(tc *economy.TxContext) error {
			if tc.Character.JobMemory == nil || tc.Character.JobMemory.IsTemporary() {
				return corejob.ErrJobUnavailable
			}
			memory := *tc.Character.JobMemory
			if err := tc.Character.ApplyJobMemory(memory.JobID, memory.SP, memory.OldJobID, memory.OldSP); err != nil {
				return err
			}
			tc.Character.JobMemory = nil
			st, err := s.loadState(tc.Context, tc.Character)
			if err != nil {
				return err
			}
			st.RestoreCurrentJob(memory.JobID)
			if err := s.repository.Save(tc.Context, st); err != nil {
				return err
			}
			restored = tc.Character
			restoredState = st
			return nil
		})
		if err != nil {
			return corecharacter.Character{}, corejob.CharacterJob{}, err
		}
		return restored, restoredState, nil
	}

	state, err := s.loadState(ctx, char)
	if err != nil {
		return corecharacter.Character{}, corejob.CharacterJob{}, err
	}
	if targetJobID == "" || targetOldJobID == "" || targetJobID == targetOldJobID ||
		!state.IsMastered(targetJobID) || !state.IsMastered(targetOldJobID) {
		return corecharacter.Character{}, corejob.CharacterJob{}, corejob.ErrJobUnavailable
	}
	targetDef, err := s.GetDefinition(targetJobID)
	if err != nil {
		return corecharacter.Character{}, corejob.CharacterJob{}, err
	}
	if targetDef.RequiredGender != "" && targetDef.RequiredGender != char.Gender {
		return corecharacter.Character{}, corejob.CharacterJob{}, corejob.ErrJobUnavailable
	}
	targetOldDef, err := s.GetDefinition(targetOldJobID)
	if err != nil {
		return corecharacter.Character{}, corejob.CharacterJob{}, err
	}
	if targetOldDef.RequiredGender != "" && targetOldDef.RequiredGender != char.Gender {
		return corecharacter.Character{}, corejob.CharacterJob{}, corejob.ErrJobUnavailable
	}
	targetSPValue, ok := state.MasteredSP(targetJobID)
	if !ok {
		return corecharacter.Character{}, corejob.CharacterJob{}, corejob.ErrJobUnavailable
	}
	targetOldSPValue, ok := state.MasteredSP(targetOldJobID)
	if !ok {
		return corecharacter.Character{}, corejob.CharacterJob{}, corejob.ErrJobUnavailable
	}

	reqItem := "item-168"
	if len(itemDefinitionID) > 0 && itemDefinitionID[0] != "" {
		reqItem = itemDefinitionID[0]
	}
	if reqItem != "item-168" && reqItem != "item-243" {
		return corecharacter.Character{}, corejob.CharacterJob{}, corejob.ErrJobUnavailable
	}
	kind := corecharacter.JobMemoryKindPersistent
	if reqItem == "item-243" {
		kind = corecharacter.JobMemoryKindTemporary
	}

	req := economy.TransactionRequest{
		CharacterID:   characterID,
		LockInventory: true,
		Cost: economy.ResourceCost{
			ItemDefinitionID:  reqItem,
			ItemDefinitionQty: 1,
		},
	}
	var updated corecharacter.Character
	var updatedState corejob.CharacterJob
	_, err = s.economy.ExecuteTransaction(ctx, req, func(tc *economy.TxContext) error {
		if tc.Character.OverLevel || tc.Character.JobMemory != nil ||
			(targetDef.RequiredGender != "" && targetDef.RequiredGender != tc.Character.Gender) ||
			(targetOldDef.RequiredGender != "" && targetOldDef.RequiredGender != tc.Character.Gender) {
			return corejob.ErrJobUnavailable
		}
		tc.Character.JobMemory = &corecharacter.JobMemory{
			Kind:     kind,
			JobID:    tc.Character.JobID,
			SP:       tc.Character.SP,
			OldJobID: tc.Character.OldJobID,
			OldSP:    tc.Character.OldSP,
		}
		if err := tc.Character.ApplyJobMemory(targetJobID, targetSPValue, targetOldJobID, targetOldSPValue); err != nil {
			return err
		}
		st, err := s.loadState(tc.Context, tc.Character)
		if err != nil {
			return err
		}
		st.RestoreCurrentJob(targetJobID)
		if err := s.repository.Save(tc.Context, st); err != nil {
			return err
		}
		updated = tc.Character
		updatedState = st
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
