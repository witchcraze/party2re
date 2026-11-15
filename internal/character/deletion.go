package character

import (
	"context"
	"slices"
	"strings"
)

// Delete validates character ownership (if playerID is provided) and deletes the character,
// its hook-owned associations and its records in one transaction.
func (s *Service) Delete(ctx context.Context, playerID, characterID string) error {
	characterID = strings.TrimSpace(characterID)
	if characterID == "" {
		return ErrNotFound
	}
	playerID = strings.TrimSpace(playerID)
	return s.runInTx(ctx, func(txCtx context.Context) error {
		char, err := s.repository.FindByID(txCtx, characterID)
		if err != nil {
			return err
		}
		if playerID != "" && char.PlayerID != playerID {
			return ErrForbidden
		}
		return s.deleteCharacters(txCtx, []string{characterID})
	})
}

// DeleteByPlayerID deletes every character of the player in one transaction; a failure
// leaves all of them intact.
func (s *Service) DeleteByPlayerID(ctx context.Context, playerID string) error {
	playerID = strings.TrimSpace(playerID)
	if playerID == "" {
		return ErrInvalidPlayer
	}
	return s.runInTx(ctx, func(txCtx context.Context) error {
		chars, err := s.repository.FindByPlayerID(txCtx, playerID)
		if err != nil {
			return err
		}
		ids := make([]string, len(chars))
		for i, c := range chars {
			ids[i] = c.ID
		}
		return s.deleteCharacters(txCtx, ids)
	})
}

// deleteCharacters runs inside the caller's transaction in lock-rank order: shared rows
// (Rank 0), cleanup hooks (characters ascending, then feature rows), then
// the complete mailbox set, then each character's records in ascending ID order.
func (s *Service) deleteCharacters(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	slices.Sort(ids)
	for _, id := range ids {
		if err := s.repository.DeleteSharedRows(ctx, id); err != nil {
			return err
		}
	}
	for _, hook := range s.cleanupHooks {
		if err := hook.CleanupCharacters(ctx, ids...); err != nil {
			return err
		}
	}
	if err := s.repository.LockDeletionLetters(ctx, ids...); err != nil {
		return err
	}
	for _, id := range ids {
		if err := s.repository.Delete(ctx, id); err != nil {
			return err
		}
	}
	return nil
}
