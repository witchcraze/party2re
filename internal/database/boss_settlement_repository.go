package database

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/witchcraze/party2re/internal/boss"
)

// LockSealingSettlement must run inside the service transaction, before character locks (Rank 0).
func (r *BossRepository) LockSealingSettlement(ctx context.Context, roomID string) (*boss.SealingSettlement, error) {
	if TxFromContext(ctx) == nil {
		return nil, errors.New("sealing settlement requires a transaction")
	}
	ex := ExecutorFromContext(ctx, r.db)
	if _, err := ex.ExecContext(ctx, `INSERT INTO boss_room_settlements (room_id) VALUES (?) ON DUPLICATE KEY UPDATE room_id = room_id`, roomID); err != nil {
		return nil, err
	}
	var data []byte
	if err := ex.QueryRowContext(ctx, `SELECT settlement_json FROM boss_room_settlements WHERE room_id = ? FOR UPDATE`, roomID).Scan(&data); err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}
	var receipt boss.SealingSettlement
	if err := json.Unmarshal(data, &receipt); err != nil {
		return nil, err
	}
	return &receipt, nil
}

func (r *BossRepository) SaveSealingSettlement(ctx context.Context, receipt boss.SealingSettlement) error {
	if TxFromContext(ctx) == nil {
		return errors.New("sealing settlement requires a transaction")
	}
	data, err := json.Marshal(receipt)
	if err != nil {
		return err
	}
	res, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `UPDATE boss_room_settlements SET settlement_json = ? WHERE room_id = ? AND settlement_json IS NULL`, data, receipt.Lobby.Party.ID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return boss.ErrPartyNotRecruiting
	}
	return nil
}
