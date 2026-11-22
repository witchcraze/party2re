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
	if err := ctx.Err(); err != nil {
		return err
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
	if err := ctx.Err(); err != nil {
		return err
	}
	state.RestoreCurrentJob(jobID)
	return s.repository.Save(ctx, state)
}

// RestoreJobMemory returns from a persistent job memory to the remembered jobs
// (job_change.cgi job_reverse). Like legacy job_exchange, it is refused while
// limit-broken; a temporary memory reverts only on sleep (home.cgi:195).
func (s *Service) RestoreJobMemory(ctx context.Context, characterID string) (corecharacter.Character, corejob.CharacterJob, error) {
	char, err := s.preflightJobMemory(ctx, characterID)
	if err != nil {
		return corecharacter.Character{}, corejob.CharacterJob{}, err
	}
	if char.JobMemory == nil || char.JobMemory.IsTemporary() {
		return corecharacter.Character{}, corejob.CharacterJob{}, corejob.ErrJobUnavailable
	}
	req := economy.TransactionRequest{CharacterID: characterID}
	var restored corecharacter.Character
	var restoredState corejob.CharacterJob
	_, err = s.economy.ExecuteTransaction(ctx, req, func(tc *economy.TxContext) error {
		if tc.Character.OverLevel || tc.Character.JobMemory == nil || tc.Character.JobMemory.IsTemporary() {
			return corejob.ErrJobUnavailable
		}
		memory := *tc.Character.JobMemory
		if err := tc.Character.ApplyJobMemory(memory.JobID, memory.SP, memory.OldJobID, memory.OldSP); err != nil {
			return err
		}
		tc.Character.JobMemory = nil
		// The trajectory snapshots SQL state before the economy runner's final save.
		if err := s.characters.Update(tc.Context, tc.Character); err != nil {
			return err
		}
		st, err := s.loadState(tc.Context, tc.Character)
		if err != nil {
			return err
		}
		st.RestoreCurrentJob(memory.JobID)
		if err := s.repository.Save(tc.Context, st); err != nil {
			return err
		}
		if err := s.memories.Record(tc.Context, characterID, corecharacter.MemoryJobRevert,
			s.jobName(memory.JobID)+"と"+s.jobName(memory.OldJobID)+"に戻る"); err != nil {
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

// preflightJobMemory loads the character for a memory start or restore and
// applies legacy job_exchange's limit-break refusal (job_change.cgi:361-364).
func (s *Service) preflightJobMemory(ctx context.Context, characterID string) (corecharacter.Character, error) {
	if s.characters == nil {
		return corecharacter.Character{}, errors.New("character repository not configured")
	}
	if s.economy == nil {
		return corecharacter.Character{}, errors.New("economy service not configured")
	}
	char, err := s.characters.FindByID(ctx, characterID)
	if err != nil {
		return corecharacter.Character{}, err
	}
	if char.OverLevel {
		return corecharacter.Character{}, corejob.ErrJobUnavailable
	}
	return char, nil
}

// ExchangeJob starts a job memory with the held recall item: item-243 a
// temporary one, otherwise item-168 a persistent one (job_change.cgi:118-136
// dispatches on the single held item). An active memory blocks another.
func (s *Service) ExchangeJob(ctx context.Context, characterID, targetJobID, targetOldJobID string) (corecharacter.Character, corejob.CharacterJob, error) {
	char, err := s.preflightJobMemory(ctx, characterID)
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
	if targetJobID == "" || targetOldJobID == "" || targetJobID == targetOldJobID ||
		!state.HasRecordedJob(targetJobID) || !state.HasRecordedJob(targetOldJobID) {
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
	if _, ok := state.SavedSP(targetJobID); !ok {
		return corecharacter.Character{}, corejob.CharacterJob{}, corejob.ErrJobUnavailable
	}
	if _, ok := state.SavedSP(targetOldJobID); !ok {
		return corecharacter.Character{}, corejob.CharacterJob{}, corejob.ErrJobUnavailable
	}

	reqItem, err := s.heldRecallItem(ctx, characterID)
	if err != nil {
		return corecharacter.Character{}, corejob.CharacterJob{}, err
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
		st, err := s.loadState(tc.Context, tc.Character)
		if err != nil {
			return err
		}
		if !st.HasRecordedJob(targetJobID) || !st.HasRecordedJob(targetOldJobID) {
			return corejob.ErrJobUnavailable
		}
		spVal, ok := st.SavedSP(targetJobID)
		if !ok {
			return corejob.ErrJobUnavailable
		}
		oldSPVal, ok := st.SavedSP(targetOldJobID)
		if !ok {
			return corejob.ErrJobUnavailable
		}
		tc.Character.JobMemory = &corecharacter.JobMemory{
			Kind:     kind,
			JobID:    tc.Character.JobID,
			SP:       tc.Character.SP,
			OldJobID: tc.Character.OldJobID,
			OldSP:    tc.Character.OldSP,
		}
		if err := tc.Character.ApplyJobMemory(targetJobID, spVal, targetOldJobID, oldSPVal); err != nil {
			return err
		}
		// Recall records the recalled job, unlike a normal job change.
		if err := s.characters.Update(tc.Context, tc.Character); err != nil {
			return err
		}
		st.RestoreCurrentJob(targetJobID)
		if err := s.repository.Save(tc.Context, st); err != nil {
			return err
		}
		if err := s.memories.Record(tc.Context, characterID, corecharacter.MemoryJobRecall,
			targetDef.Name+"と"+targetOldDef.Name+"の記憶を思い出す"); err != nil {
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

// recallItemPreference orders the recall items; legacy assigns the item-243
// action last, so it wins (job_change.cgi:129-134).
var recallItemPreference = []string{"item-243", "item-168"}

// heldRecallItem is the recall item in the held field; the economy runner
// re-checks it under lock when consuming it.
func (s *Service) heldRecallItem(ctx context.Context, characterID string) (string, error) {
	if s.inventories == nil {
		return "", ErrRequiredItem
	}
	inv, err := s.inventories.FindByCharacterID(ctx, characterID)
	if err != nil {
		return "", err
	}
	for _, id := range recallItemPreference {
		if inv.Quantity(id) > 0 {
			return id, nil
		}
	}
	return "", ErrRequiredItem
}
