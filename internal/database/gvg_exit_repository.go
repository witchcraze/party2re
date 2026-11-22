package database

import (
	"context"
	"errors"

	"github.com/witchcraze/party2re/internal/gvg"
)

// ClaimRoomExitForUpdate locks and records a character's per-join GvG exit claim within the active transaction.
// Transaction: joins ambient RunInTx
// Lock Order: gvg_exit_claims(0)
func (r *GvGRepository) ClaimRoomExitForUpdate(ctx context.Context, roomID, characterID, joinID string) (gvg.ExitClaimState, error) {
	if roomID == "" || characterID == "" || joinID == "" {
		return gvg.ExitClaimState{}, errors.New("room_id, character_id, and join_id are required")
	}
	executor := ExecutorFromContext(ctx, r.db)
	res, err := executor.ExecContext(ctx, `
		INSERT IGNORE INTO gvg_exit_claims (room_id, character_id, join_id, delivery_done)
		VALUES (?, ?, ?, FALSE)
	`, roomID, characterID, joinID)
	if err != nil {
		return gvg.ExitClaimState{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return gvg.ExitClaimState{}, err
	}

	var deliveryDone bool
	if err := executor.QueryRowContext(ctx, `
		SELECT delivery_done FROM gvg_exit_claims WHERE room_id = ? AND character_id = ? AND join_id = ? FOR UPDATE
	`, roomID, characterID, joinID).Scan(&deliveryDone); err != nil {
		return gvg.ExitClaimState{}, err
	}

	return gvg.ExitClaimState{
		AlreadyClaimed: affected == 0,
		DeliveryDone:   deliveryDone,
	}, nil
}

// MarkExitDeliveryDone marks post-exit tavern delivery/fullness processing as completed for this join.
// Transaction: joins ambient RunInTx (expects the gvg_exit_claims row locked by ClaimRoomExitForUpdate).
func (r *GvGRepository) MarkExitDeliveryDone(ctx context.Context, roomID, characterID, joinID string) error {
	if roomID == "" || characterID == "" || joinID == "" {
		return errors.New("room_id, character_id, and join_id are required")
	}
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		UPDATE gvg_exit_claims SET delivery_done = TRUE WHERE room_id = ? AND character_id = ? AND join_id = ?
	`, roomID, characterID, joinID)
	return err
}
