package job

import (
	"context"
	"errors"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	corejob "github.com/witchcraze/party2re/internal/core/job"
	"github.com/witchcraze/party2re/internal/economy"
	"github.com/witchcraze/party2re/internal/id"
)

// FutureMemoryRepository defines persistence operations for future memory snapshots.
type FutureMemoryRepository interface {
	Save(ctx context.Context, memory corecharacter.FutureMemory) error
	FindByCharacterID(ctx context.Context, characterID string) ([]corecharacter.FutureMemory, error)
	Delete(ctx context.Context, characterID, memoryID string) error
}

// WithFutureMemoryRepository sets the repository used to persist future memory snapshots.
func WithFutureMemoryRepository(repo FutureMemoryRepository) Option {
	return func(s *Service) {
		s.futureMemories = repo
	}
}

// SaveFutureMemory creates a future memory snapshot of the character's state,
// consuming item-207 (未来のカケラ) and enforcing the OverFuture slot limit.
func (s *Service) SaveFutureMemory(ctx context.Context, characterID string) (corecharacter.FutureMemory, error) {
	if s.characters == nil {
		return corecharacter.FutureMemory{}, errors.New("character repository is nil")
	}
	if s.futureMemories == nil {
		return corecharacter.FutureMemory{}, errors.New("future memory repository is nil")
	}
	char, err := s.characters.FindByID(ctx, characterID)
	if err != nil {
		return corecharacter.FutureMemory{}, err
	}
	if char.JobMemory != nil {
		return corecharacter.FutureMemory{}, corejob.ErrJobUnavailable
	}
	existing, err := s.futureMemories.FindByCharacterID(ctx, characterID)
	if err != nil {
		return corecharacter.FutureMemory{}, err
	}
	if !char.CanSaveFutureMemory(len(existing)) {
		return corecharacter.FutureMemory{}, errors.New("future memory slot limit reached")
	}

	const requiredItem = "item-207"
	if s.economy != nil {
		req := economy.TransactionRequest{
			CharacterID:   characterID,
			LockInventory: true,
			Cost: economy.ResourceCost{
				ItemDefinitionID:  requiredItem,
				ItemDefinitionQty: 1,
			},
		}
		var snapshot corecharacter.FutureMemory
		_, err := s.economy.ExecuteTransaction(ctx, req, func(tc *economy.TxContext) error {
			now := time.Now().UTC()
			snapshot = tc.Character.CreateFutureMemory(id.New(), now)
			if err := s.futureMemories.Save(tc.Context, snapshot); err != nil {
				return err
			}
			state, err := s.loadState(tc.Context, tc.Character)
			if err != nil {
				return err
			}
			if state.MasteredJobSP == nil {
				state.MasteredJobSP = make(map[string]int)
			}
			state.MasteredJobSP[tc.Character.JobID] = tc.Character.SP
			if tc.Character.OldJobID != "" {
				state.MasteredJobSP[tc.Character.OldJobID] = tc.Character.OldSP
			}
			return s.repository.Save(tc.Context, state)
		})
		if err != nil {
			if errors.Is(err, economy.ErrItemNotFound) || errors.Is(err, economy.ErrInsufficientItemQuantity) {
				return corecharacter.FutureMemory{}, ErrRequiredItem
			}
			return corecharacter.FutureMemory{}, err
		}
		return snapshot, nil
	}

	if s.inventories == nil {
		return corecharacter.FutureMemory{}, ErrRequiredItem
	}
	inventory, err := s.inventories.FindByCharacterID(ctx, characterID)
	if err != nil || inventory.Quantity(requiredItem) < 1 {
		return corecharacter.FutureMemory{}, ErrRequiredItem
	}
	for _, instance := range inventory.Items {
		if instance.DefinitionID == requiredItem {
			if err := inventory.Consume(instance.ID, 1); err != nil {
				return corecharacter.FutureMemory{}, err
			}
			break
		}
	}
	if err := s.inventories.Save(ctx, inventory); err != nil {
		return corecharacter.FutureMemory{}, err
	}

	now := time.Now().UTC()
	snapshot := char.CreateFutureMemory(id.New(), now)
	if err := s.futureMemories.Save(ctx, snapshot); err != nil {
		return corecharacter.FutureMemory{}, err
	}
	state, err := s.loadState(ctx, char)
	if err == nil {
		if state.MasteredJobSP == nil {
			state.MasteredJobSP = make(map[string]int)
		}
		state.MasteredJobSP[char.JobID] = char.SP
		if char.OldJobID != "" {
			state.MasteredJobSP[char.OldJobID] = char.OldSP
		}
		if err := s.repository.Save(ctx, state); err != nil {
			return corecharacter.FutureMemory{}, err
		}
	}
	return snapshot, nil
}

// RecallFutureMemory restores the character's state from a saved future memory snapshot
// (legacy "よびおこす" action), restoring stats and retained mastery SP, and consumes the snapshot.
func (s *Service) RecallFutureMemory(ctx context.Context, characterID, memoryID string) (corecharacter.Character, corejob.CharacterJob, error) {
	if s.characters == nil {
		return corecharacter.Character{}, corejob.CharacterJob{}, errors.New("character repository is nil")
	}
	if s.futureMemories == nil {
		return corecharacter.Character{}, corejob.CharacterJob{}, errors.New("future memory repository is nil")
	}

	if s.economy != nil {
		var recalledChar corecharacter.Character
		var recalledState corejob.CharacterJob

		req := economy.TransactionRequest{
			CharacterID: characterID,
		}

		_, err := s.economy.ExecuteTransaction(ctx, req, func(tc *economy.TxContext) error {
			if tc.Character.JobMemory != nil {
				return corejob.ErrJobUnavailable
			}
			memories, err := s.futureMemories.FindByCharacterID(tc.Context, characterID)
			if err != nil {
				return err
			}
			var targetMemory *corecharacter.FutureMemory
			for _, m := range memories {
				if m.ID == memoryID {
					targetMemory = &m
					break
				}
			}
			if targetMemory == nil {
				return errors.New("future memory not found")
			}

			state, err := s.loadState(tc.Context, tc.Character)
			if err != nil {
				return err
			}
			jobSP, _ := state.MasteredSP(targetMemory.JobID)
			oldJobSP, _ := state.MasteredSP(targetMemory.OldJobID)

			c := tc.Character
			if err := c.ApplyFutureMemory(*targetMemory, jobSP, oldJobSP); err != nil {
				return err
			}
			tc.Character = c

			if err := s.futureMemories.Delete(tc.Context, characterID, memoryID); err != nil {
				return err
			}
			state.RestoreCurrentJob(tc.Character.JobID)
			if err := s.repository.Save(tc.Context, state); err != nil {
				return err
			}

			recalledChar = tc.Character
			recalledState = state
			return nil
		})
		if err != nil {
			return corecharacter.Character{}, corejob.CharacterJob{}, err
		}
		return recalledChar, recalledState, nil
	}

	char, err := s.characters.FindByID(ctx, characterID)
	if err != nil {
		return corecharacter.Character{}, corejob.CharacterJob{}, err
	}
	if char.JobMemory != nil {
		return char, corejob.CharacterJob{}, corejob.ErrJobUnavailable
	}
	memories, err := s.futureMemories.FindByCharacterID(ctx, characterID)
	if err != nil {
		return char, corejob.CharacterJob{}, err
	}
	var targetMemory *corecharacter.FutureMemory
	for _, m := range memories {
		if m.ID == memoryID {
			targetMemory = &m
			break
		}
	}
	if targetMemory == nil {
		return char, corejob.CharacterJob{}, errors.New("future memory not found")
	}

	state, err := s.loadState(ctx, char)
	if err != nil {
		return char, corejob.CharacterJob{}, err
	}
	jobSP, _ := state.MasteredSP(targetMemory.JobID)
	oldJobSP, _ := state.MasteredSP(targetMemory.OldJobID)

	if err := char.ApplyFutureMemory(*targetMemory, jobSP, oldJobSP); err != nil {
		return char, corejob.CharacterJob{}, err
	}
	if err := s.characters.Update(ctx, char); err != nil {
		return char, corejob.CharacterJob{}, err
	}
	if err := s.futureMemories.Delete(ctx, characterID, memoryID); err != nil {
		return char, corejob.CharacterJob{}, err
	}
	state.RestoreCurrentJob(char.JobID)
	if err := s.repository.Save(ctx, state); err != nil {
		return char, corejob.CharacterJob{}, err
	}
	return char, state, nil
}

// ListFutureMemories returns all saved future memory snapshots for a character.
func (s *Service) ListFutureMemories(ctx context.Context, characterID string) ([]corecharacter.FutureMemory, error) {
	if s.futureMemories == nil {
		return nil, nil
	}
	return s.futureMemories.FindByCharacterID(ctx, characterID)
}
