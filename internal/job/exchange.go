package job

import (
	"context"
	"errors"
	"fmt"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	corejob "github.com/witchcraze/party2re/internal/core/job"
	"github.com/witchcraze/party2re/internal/economy"
)

// ExchangeJob swaps a character's current and old job classes using item-168.
func (s *Service) ExchangeJob(ctx context.Context, characterID, targetJobID, targetOldJobID string) (corecharacter.Character, corejob.CharacterJob, error) {
	if s.characters == nil {
		return corecharacter.Character{}, corejob.CharacterJob{}, errors.New("character repository not configured")
	}
	char, err := s.characters.FindByID(ctx, characterID)
	if err != nil {
		return corecharacter.Character{}, corejob.CharacterJob{}, err
	}
	state, err := s.loadState(ctx, char)
	if err != nil {
		return corecharacter.Character{}, corejob.CharacterJob{}, err
	}
	if char.JobMemory != nil {
		memory := *char.JobMemory
		if err := char.ApplyJobMemory(memory.JobID, memory.SP, memory.OldJobID, memory.OldSP); err != nil {
			return corecharacter.Character{}, corejob.CharacterJob{}, err
		}
		char.JobMemory = nil
		if err := s.characters.Update(ctx, char); err != nil {
			return corecharacter.Character{}, corejob.CharacterJob{}, err
		}
		return char, state, nil
	}
	if targetJobID == "" || targetOldJobID == "" || targetJobID == targetOldJobID ||
		!state.IsMastered(targetJobID) || !state.IsMastered(targetOldJobID) {
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
	if s.economy != nil {
		req := economy.TransactionRequest{CharacterID: characterID, LockInventory: true,
			Cost: economy.ResourceCost{ItemDefinitionID: "item-168", ItemDefinitionQty: 1}}
		var updated corecharacter.Character
		_, err := s.economy.ExecuteTransaction(ctx, req, func(tc *economy.TxContext) error {
			tc.Character.JobMemory = &corecharacter.JobMemory{JobID: tc.Character.JobID, SP: tc.Character.SP, OldJobID: tc.Character.OldJobID, OldSP: tc.Character.OldSP}
			if err := tc.Character.ApplyJobMemory(targetJobID, targetSPValue, targetOldJobID, targetOldSPValue); err != nil {
				return err
			}
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
	if s.inventories == nil {
		return corecharacter.Character{}, corejob.CharacterJob{}, ErrRequiredItem
	}
	inventory, err := s.inventories.FindByCharacterID(ctx, characterID)
	if err != nil || inventory.Quantity("item-168") < 1 {
		return corecharacter.Character{}, corejob.CharacterJob{}, ErrRequiredItem
	}
	for _, instance := range inventory.Items {
		if instance.DefinitionID == "item-168" {
			if err := inventory.Consume(instance.ID, 1); err != nil {
				return corecharacter.Character{}, corejob.CharacterJob{}, err
			}
			break
		}
	}
	memory := &corecharacter.JobMemory{JobID: char.JobID, SP: char.SP, OldJobID: char.OldJobID, OldSP: char.OldSP}
	if err := char.ApplyJobMemory(targetJobID, targetSPValue, targetOldJobID, targetOldSPValue); err != nil {
		return corecharacter.Character{}, corejob.CharacterJob{}, err
	}
	char.JobMemory = memory
	if err := s.characters.Update(ctx, char); err != nil {
		return corecharacter.Character{}, corejob.CharacterJob{}, err
	}
	if err := s.repository.Save(ctx, state); err != nil {
		return corecharacter.Character{}, corejob.CharacterJob{}, fmt.Errorf("save job memory: %w", err)
	}
	if err := s.inventories.Save(ctx, inventory); err != nil {
		return corecharacter.Character{}, corejob.CharacterJob{}, err
	}
	return char, state, nil
}
