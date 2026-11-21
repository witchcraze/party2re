package database

import (
	"context"
	"errors"

	"github.com/witchcraze/party2re/internal/gvg"
)

// ClaimRoomExitForUpdate locks and records a character's GvG exit claim within the active transaction.
// Transaction: joins ambient RunInTx
// Lock Order: gvg_exit_claims(0)
func (r *GvGRepository) ClaimRoomExitForUpdate(ctx context.Context, roomID, characterID string) (gvg.ExitClaimState, error) {
	if roomID == "" || characterID == "" {
		return gvg.ExitClaimState{}, errors.New("room_id and character_id are required")
	}
	executor := ExecutorFromContext(ctx, r.db)
	res, err := executor.ExecContext(ctx, `
		INSERT IGNORE INTO gvg_exit_claims (room_id, character_id, delivery_done)
		VALUES (?, ?, FALSE)
	`, roomID, characterID)
	if err != nil {
		return gvg.ExitClaimState{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return gvg.ExitClaimState{}, err
	}

	var deliveryDone bool
	if err := executor.QueryRowContext(ctx, `
		SELECT delivery_done FROM gvg_exit_claims WHERE room_id = ? AND character_id = ? FOR UPDATE
	`, roomID, characterID).Scan(&deliveryDone); err != nil {
		return gvg.ExitClaimState{}, err
	}

	return gvg.ExitClaimState{
		AlreadyClaimed: affected == 0,
		DeliveryDone:   deliveryDone,
	}, nil
}

// MarkExitDeliveryDone marks post-exit tavern delivery/fullness processing as completed.
func (r *GvGRepository) MarkExitDeliveryDone(ctx context.Context, roomID, characterID string) error {
	if roomID == "" || characterID == "" {
		return errors.New("room_id and character_id are required")
	}
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		UPDATE gvg_exit_claims SET delivery_done = TRUE WHERE room_id = ? AND character_id = ?
	`, roomID, characterID)
	return err
}
