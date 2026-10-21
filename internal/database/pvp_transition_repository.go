package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/go-sql-driver/mysql"
	"github.com/witchcraze/party2re/internal/pvp"
)

type PvPTransitionRepository struct{ db *sql.DB }

func NewPvPTransitionRepository(db *sql.DB) (*PvPTransitionRepository, error) {
	if db == nil {
		return nil, errors.New("nil database connection")
	}
	return &PvPTransitionRepository{db: db}, nil
}

func (r *PvPTransitionRepository) FindTransition(ctx context.Context, roomID string) (pvp.Transition, error) {
	var data []byte
	var pending bool
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx,
		"SELECT transition_json, pending FROM pvp_room_transitions WHERE room_id = ?", roomID).Scan(&data, &pending)
	if errors.Is(err, sql.ErrNoRows) {
		return pvp.Transition{}, nil
	}
	if err != nil {
		return pvp.Transition{}, err
	}
	var t pvp.Transition
	if err := json.Unmarshal(data, &t); err != nil {
		return pvp.Transition{}, err
	}
	t.Pending = pending
	t.Detail.Room.PasswordHash = t.PasswordHash
	t.Detail.Room.FinancialVersion = t.Version
	return t, nil
}

func (r *PvPTransitionRepository) FindCharacterClaim(ctx context.Context, characterID string) (string, error) {
	var roomID string
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx,
		"SELECT room_id FROM pvp_character_claims WHERE character_id = ?", characterID).Scan(&roomID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return roomID, err
}

// CommitTransition commits the financial mutation, unique participation and recovery state.
// Transaction: RunInTx
// Lock Order: pvp_room_transitions(0) -> characters(2, ascending ID) -> pvp_character_claims(8)
func (r *PvPTransitionRepository) CommitTransition(ctx context.Context, t pvp.Transition, mutate func(context.Context) error) error {
	roomID := t.Detail.Room.ID
	if roomID == "" || t.Version < 1 || !t.Pending || mutate == nil {
		return fmt.Errorf("invalid pvp transition")
	}
	data, err := json.Marshal(t)
	if err != nil {
		return err
	}
	return RunInTx(ctx, r.db, func(txCtx context.Context) error {
		executor := ExecutorFromContext(txCtx, r.db)
		if _, err := executor.ExecContext(txCtx, `INSERT INTO pvp_room_transitions (room_id) VALUES (?)
			ON DUPLICATE KEY UPDATE room_id = room_id`, roomID); err != nil {
			return err
		}
		var version int
		var pending bool
		var oldData []byte
		if err := executor.QueryRowContext(txCtx, `SELECT version, pending, transition_json
			FROM pvp_room_transitions WHERE room_id = ? FOR UPDATE`, roomID).Scan(&version, &pending, &oldData); err != nil {
			return err
		}
		var previous pvp.Transition
		if len(oldData) > 0 {
			if err := json.Unmarshal(oldData, &previous); err != nil {
				return err
			}
		}
		if version+1 != t.Version || pending || previous.Detail.Room.Status == pvp.StatusCompleted || previous.Detail.Room.Status == pvp.StatusDisbanded {
			return pvp.ErrTransitionConflict
		}
		if err := mutate(txCtx); err != nil {
			return err
		}
		if t.AddedID != "" {
			if _, err := executor.ExecContext(txCtx, "INSERT INTO pvp_character_claims (character_id, room_id) VALUES (?, ?)", t.AddedID, roomID); err != nil {
				var mysqlErr *mysql.MySQLError
				if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
					return pvp.ErrAlreadyInRoom
				}
				return err
			}
		}
		_, err := executor.ExecContext(txCtx, `UPDATE pvp_room_transitions
			SET version = ?, pending = TRUE, transition_json = ? WHERE room_id = ?`, t.Version, data, roomID)
		return err
	})
}

// AcknowledgeTransition releases departed claims only after every cache write succeeded.
// Transaction: RunInTx
// Lock Order: pvp_room_transitions(0) -> pvp_character_claims(8)
func (r *PvPTransitionRepository) AcknowledgeTransition(ctx context.Context, roomID string, version int) error {
	return RunInTx(ctx, r.db, func(txCtx context.Context) error {
		executor := ExecutorFromContext(txCtx, r.db)
		var current int
		if err := executor.QueryRowContext(txCtx, "SELECT version FROM pvp_room_transitions WHERE room_id = ? FOR UPDATE", roomID).Scan(&current); err != nil {
			return err
		}
		if current != version {
			return pvp.ErrTransitionConflict
		}
		t, err := r.FindTransition(txCtx, roomID)
		if err != nil {
			return err
		}
		removed := append([]string{}, t.RemovedIDs...)
		if t.Detail.Room.Status == pvp.StatusCompleted {
			for _, m := range t.Detail.Members {
				removed = append(removed, m.CharacterID)
			}
		}
		for _, characterID := range removed {
			if _, err := executor.ExecContext(txCtx, "DELETE FROM pvp_character_claims WHERE character_id = ? AND room_id = ?", characterID, roomID); err != nil {
				return err
			}
		}
		_, err = executor.ExecContext(txCtx, "UPDATE pvp_room_transitions SET pending = FALSE WHERE room_id = ?", roomID)
		return err
	})
}
