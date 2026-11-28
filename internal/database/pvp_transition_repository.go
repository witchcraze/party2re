package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"time"

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
	var lastActivity sql.NullTime
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx,
		"SELECT transition_json, pending, last_activity_at FROM pvp_room_transitions WHERE room_id = ?", roomID).Scan(&data, &pending, &lastActivity)
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
	t.Detail.Room.PendingPvPKills = maps.Clone(t.PendingPvPKills)
	t.Detail.Room.FinancialVersion = t.Version
	if lastActivity.Valid {
		t.LastActivityAt = lastActivity.Time.UTC()
	} else if t.LastActivityAt.IsZero() && !t.Detail.Room.CreatedAt.IsZero() {
		t.LastActivityAt = t.Detail.Room.CreatedAt.UTC()
	}
	t.Detail.Room.LastActivityAt = t.LastActivityAt
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

// ListOverdueRooms also returns pending rooms so failed publication converges. It drives from claims, which only live rooms and unacknowledged departures hold,
// so the scan stays bounded by participants rather than retained disbanded rows.
func (r *PvPTransitionRepository) ListOverdueRooms(ctx context.Context, cutoff time.Time, limit int) ([]string, error) {
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, `SELECT t.room_id FROM pvp_room_transitions t
		WHERE (t.last_activity_at < ? OR t.pending) AND t.room_id IN (SELECT room_id FROM pvp_character_claims)
		ORDER BY t.last_activity_at, t.room_id LIMIT ?`, cutoff.UTC(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
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
		var lastActivity sql.NullTime
		var oldData []byte
		if err := executor.QueryRowContext(txCtx, `SELECT version, pending, last_activity_at, transition_json
			FROM pvp_room_transitions WHERE room_id = ? FOR UPDATE`, roomID).Scan(&version, &pending, &lastActivity, &oldData); err != nil {
			return err
		}
		var previous pvp.Transition
		if len(oldData) > 0 {
			if err := json.Unmarshal(oldData, &previous); err != nil {
				return err
			}
		}
		if version+1 != t.Version || pending || !pvp.CanTransitionFrom(previous.Detail.Room.Status, t.Kind) {
			return pvp.ErrTransitionConflict
		}
		effectiveLast := previous.LastActivityAt
		if lastActivity.Valid {
			effectiveLast = lastActivity.Time.UTC()
		} else if effectiveLast.IsZero() {
			effectiveLast = previous.Detail.Room.CreatedAt
		}
		if pvp.IsActivityTransitionKind(t.Kind) && !effectiveLast.IsZero() && t.LastActivityAt.After(effectiveLast.Add(pvp.InactivityTimeout)) {
			return pvp.ErrRoomInactive
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
		nextActivity := sql.NullTime{Time: t.LastActivityAt.UTC(), Valid: !t.LastActivityAt.IsZero()}
		_, err := executor.ExecContext(txCtx, `UPDATE pvp_room_transitions
			SET version = ?, pending = TRUE, last_activity_at = ?, transition_json = ? WHERE room_id = ?`, t.Version, nextActivity, data, roomID)
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
		for _, characterID := range t.RemovedIDs {
			if _, err := executor.ExecContext(txCtx, "DELETE FROM pvp_character_claims WHERE character_id = ? AND room_id = ?", characterID, roomID); err != nil {
				return err
			}
		}
		_, err = executor.ExecContext(txCtx, "UPDATE pvp_room_transitions SET pending = FALSE WHERE room_id = ?", roomID)
		return err
	})
}
