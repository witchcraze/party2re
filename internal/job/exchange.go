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
		state, err = corejob.NewCharacterJob(characterID, jobID)
		if err != nil {
			return err
		}
	}
	state.RestoreCurrentJob(jobID)
	return s.repository.Save(ctx, state)
}

// ExchangeJob swaps a character's current and old job classes using item-168.
func (s *Service) ExchangeJob(ctx context.Context, characterID, targetJobID, targetOldJobID string) (corecharacter.Character, corejob.CharacterJob, error) {
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
	state, err := s.loadState(ctx, char)
	if err != nil {
		return corecharacter.Character{}, corejob.CharacterJob{}, err
	}
	if char.JobMemory != nil {
		req := economy.TransactionRequest{CharacterID: characterID}
		var restored corecharacter.Character
		var restoredState corejob.CharacterJob
		_, err := s.economy.ExecuteTransaction(ctx, req, func(tc *economy.TxContext) error {
			if tc.Character.JobMemory == nil {
				return errors.New("no job memory to restore")
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
	req := economy.TransactionRequest{
		CharacterID:   characterID,
		LockInventory: true,
		Cost: economy.ResourceCost{
			ItemDefinitionID:  "item-168",
			ItemDefinitionQty: 1,
		},
	}
	var updated corecharacter.Character
	_, err = s.economy.ExecuteTransaction(ctx, req, func(tc *economy.TxContext) error {
		if tc.Character.OverLevel ||
			(targetDef.RequiredGender != "" && targetDef.RequiredGender != tc.Character.Gender) ||
			(targetOldDef.RequiredGender != "" && targetOldDef.RequiredGender != tc.Character.Gender) {
			return corejob.ErrJobUnavailable
		}
		tc.Character.JobMemory = &corecharacter.JobMemory{
			JobID:    tc.Character.JobID,
			SP:       tc.Character.SP,
			OldJobID: tc.Character.OldJobID,
			OldSP:    tc.Character.OldSP,
		}
		if err := tc.Character.ApplyJobMemory(targetJobID, targetSPValue, targetOldJobID, targetOldSPValue); err != nil {
			return err
		}
		state.RestoreCurrentJob(targetJobID)
		if err := s.repository.Save(tc.Context, state); err != nil {
			return err
		}
		updated = tc.Character
		return nil
	})
	if err != nil {
		if errors.Is(err, economy.ErrItemNotFound) || errors.Is(err, economy.ErrInsufficientItemQuantity) {
			return corecharacter.Character{}, corejob.CharacterJob{}, ErrRequiredItem
		}
		return corecharacter.Character{}, corejob.CharacterJob{}, err
	}
	return updated, state, nil
}
