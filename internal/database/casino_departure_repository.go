package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"

	"github.com/witchcraze/party2re/internal/casino"
)

func (r *CasinoRepository) FindDeparture(ctx context.Context, roomID string) (casino.Departure, error) {
	var data []byte
	var pending bool
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx,
		"SELECT departure_json, pending FROM casino_departures WHERE room_id = ?", roomID).Scan(&data, &pending)
	if errors.Is(err, sql.ErrNoRows) {
		return casino.Departure{}, nil
	}
	if err != nil {
		return casino.Departure{}, err
	}
	var d casino.Departure
	if len(data) > 0 {
		if err := json.Unmarshal(data, &d); err != nil {
			return casino.Departure{}, err
		}
	}
	d.Pending = pending
	d.Detail.Room.PasswordHash = d.PasswordHash
	d.Detail.Room.DepartureVersion = d.Version
	return d, nil
}

func (r *CasinoRepository) FindPendingDeparture(ctx context.Context, characterID string) (string, error) {
	var roomID string
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx,
		"SELECT room_id FROM casino_departure_claims WHERE character_id = ?", characterID).Scan(&roomID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return roomID, err
}

// CommitDeparture owns commit: publication must never precede an outer transaction's commit.
// Transaction: RunInTx
// Lock Order: casino_departures(0) -> characters(2, ascending ID) -> casino_departure_claims(8, ascending ID)
func (r *CasinoRepository) CommitDeparture(ctx context.Context, d casino.Departure, mutate func(context.Context) error) error {
	if TxFromContext(ctx) != nil || d.Detail.Room.ID == "" || d.Member.CharacterID == "" || d.Version < 1 || !d.Pending || mutate == nil {
		return casino.ErrDepartureRecovery
	}
	data, err := json.Marshal(d)
	if err != nil {
		return err
	}
	return RunInTx(ctx, r.db, func(txCtx context.Context) error {
		exec := ExecutorFromContext(txCtx, r.db)
		if _, err := exec.ExecContext(txCtx, `INSERT INTO casino_departures (room_id) VALUES (?)
			ON DUPLICATE KEY UPDATE room_id = room_id`, d.Detail.Room.ID); err != nil {
			return err
		}
		var version int
		var pending bool
		if err := exec.QueryRowContext(txCtx, "SELECT version, pending FROM casino_departures WHERE room_id = ? FOR UPDATE", d.Detail.Room.ID).Scan(&version, &pending); err != nil {
			return err
		}
		if pending || version+1 != d.Version {
			return casino.ErrDepartureRecovery
		}
		if err := mutate(txCtx); err != nil {
			return err
		}
		ids := slices.Clone(d.ClaimedIDs)
		slices.Sort(ids)
		for _, characterID := range slices.Compact(ids) {
			if _, err := exec.ExecContext(txCtx, "INSERT INTO casino_departure_claims (character_id, room_id) VALUES (?, ?)", characterID, d.Detail.Room.ID); err != nil {
				return err
			}
		}
		_, err := exec.ExecContext(txCtx, "UPDATE casino_departures SET version = ?, pending = TRUE, departure_json = ? WHERE room_id = ?", d.Version, data, d.Detail.Room.ID)
		return err
	})
}

// AcknowledgeDeparture drops only pending claims; the latest receipt remains until replaced.
// Transaction: RunInTx
// Lock Order: casino_departures(0) -> casino_departure_claims(8)
func (r *CasinoRepository) AcknowledgeDeparture(ctx context.Context, roomID string, version int) error {
	if TxFromContext(ctx) != nil {
		return casino.ErrDepartureRecovery
	}
	return RunInTx(ctx, r.db, func(txCtx context.Context) error {
		exec := ExecutorFromContext(txCtx, r.db)
		var current int
		if err := exec.QueryRowContext(txCtx, "SELECT version FROM casino_departures WHERE room_id = ? FOR UPDATE", roomID).Scan(&current); err != nil {
			return err
		}
		if current != version {
			return casino.ErrDepartureRecovery
		}
		if _, err := exec.ExecContext(txCtx, "DELETE FROM casino_departure_claims WHERE room_id = ?", roomID); err != nil {
			return err
		}
		_, err := exec.ExecContext(txCtx, "UPDATE casino_departures SET pending = FALSE WHERE room_id = ?", roomID)
		return err
	})
}
