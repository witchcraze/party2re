package database

import (
	"context"
	"database/sql"
	"errors"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/economy"
)

// LockDeletionLetters runs inside the deletion transaction after cleanup hooks.
// All characters must be locked before discovering letters, as delivery locks both
// owners. Locking the union prevents per-character cleanup from inverting letter order.
func (r *CharacterRepository) LockDeletionLetters(ctx context.Context, characterIDs ...string) error {
	if err := economy.LockInOrder(ctx, characterIDs, func(ctx context.Context, id string) error {
		_, err := r.FindByIDForUpdate(ctx, id)
		if errors.Is(err, corecharacter.ErrNotFound) {
			return nil
		}
		return err
	}); err != nil {
		return err
	}
	exec := ExecutorFromContext(ctx, r.db)
	var ids []string
	for _, characterID := range characterIDs {
		letters, err := letterIDs(ctx, exec, `
			SELECT id FROM character_letters WHERE sender_character_id = ?
			UNION SELECT id FROM character_letters WHERE recipient_character_id = ?
		`, characterID, characterID)
		if err != nil {
			return err
		}
		ids = append(ids, letters...)
	}
	return economy.LockInOrder(ctx, ids, func(ctx context.Context, id string) error {
		var lockedID string
		err := exec.QueryRowContext(ctx, `SELECT id FROM character_letters WHERE id = ? FOR UPDATE`, id).Scan(&lockedID)
		// A counterpart may have purged both copies since discovery.
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	})
}
