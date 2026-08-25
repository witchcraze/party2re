package database

import (
	"context"
	"database/sql"
	"errors"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/replay"
)

type ReplayRepository struct {
	db *sql.DB
}

func NewReplayRepository(db *sql.DB) (*ReplayRepository, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	return &ReplayRepository{db: db}, nil
}

func (r *ReplayRepository) Save(ctx context.Context, rep replay.BattleReplay) error {
	participantsJSON := replay.EncodeJSON(rep.InitialParticipants)
	turnLogsJSON := replay.EncodeJSON(rep.TurnLogs)

	var winnerID, loserID sql.NullString
	if rep.WinnerID != "" {
		winnerID = sql.NullString{String: rep.WinnerID, Valid: true}
	}
	if rep.LoserID != "" {
		loserID = sql.NullString{String: rep.LoserID, Valid: true}
	}

	query := `
		INSERT INTO battle_replays (
			id, combat_type, initiator_id, initiator_name, opponent_id, opponent_name,
			outcome, winner_id, loser_id, total_turns, initial_participants_json,
			turn_logs_json, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := r.db.ExecContext(
		ctx,
		query,
		rep.ID,
		rep.CombatType,
		rep.InitiatorID,
		rep.InitiatorName,
		rep.OpponentID,
		rep.OpponentName,
		string(rep.Outcome),
		winnerID,
		loserID,
		rep.TotalTurns,
		participantsJSON,
		turnLogsJSON,
		rep.CreatedAt,
	)
	return err
}

func (r *ReplayRepository) FindByID(ctx context.Context, id string) (*replay.BattleReplay, error) {
	query := `
		SELECT id, combat_type, initiator_id, initiator_name, opponent_id, opponent_name,
		       outcome, winner_id, loser_id, total_turns, initial_participants_json,
		       turn_logs_json, created_at
		FROM battle_replays
		WHERE id = ?
	`
	var rep replay.BattleReplay
	var outcomeStr string
	var winnerID, loserID sql.NullString
	var participantsJSON, turnLogsJSON string

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&rep.ID,
		&rep.CombatType,
		&rep.InitiatorID,
		&rep.InitiatorName,
		&rep.OpponentID,
		&rep.OpponentName,
		&outcomeStr,
		&winnerID,
		&loserID,
		&rep.TotalTurns,
		&participantsJSON,
		&turnLogsJSON,
		&rep.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, replay.ErrReplayNotFound
	}
	if err != nil {
		return nil, err
	}

	rep.Outcome = corebattle.Outcome(outcomeStr)
	if winnerID.Valid {
		rep.WinnerID = winnerID.String
	}
	if loserID.Valid {
		rep.LoserID = loserID.String
	}

	participants, err := replay.DecodeJSON[[]replay.ParticipantSnapshot](participantsJSON)
	if err != nil {
		return nil, err
	}
	rep.InitialParticipants = participants

	turnLogs, err := replay.DecodeJSON[[]corebattle.TurnLog](turnLogsJSON)
	if err != nil {
		return nil, err
	}
	rep.TurnLogs = turnLogs

	return &rep, nil
}

func (r *ReplayRepository) FindByCharacter(ctx context.Context, characterID string, combatType string, limit int) ([]replay.ReplayHeader, error) {
	var rows *sql.Rows
	var err error

	if combatType != "" {
		query := `
			SELECT id, combat_type, initiator_id, initiator_name, opponent_id, opponent_name,
			       outcome, winner_id, total_turns, created_at
			FROM battle_replays
			WHERE (initiator_id = ? OR opponent_id = ?) AND combat_type = ?
			ORDER BY created_at DESC
			LIMIT ?
		`
		rows, err = r.db.QueryContext(ctx, query, characterID, characterID, combatType, limit)
	} else {
		query := `
			SELECT id, combat_type, initiator_id, initiator_name, opponent_id, opponent_name,
			       outcome, winner_id, total_turns, created_at
			FROM battle_replays
			WHERE initiator_id = ? OR opponent_id = ?
			ORDER BY created_at DESC
			LIMIT ?
		`
		rows, err = r.db.QueryContext(ctx, query, characterID, characterID, limit)
	}

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanReplayHeaders(rows)
}

func (r *ReplayRepository) FindRecent(ctx context.Context, combatType string, limit int) ([]replay.ReplayHeader, error) {
	var rows *sql.Rows
	var err error

	if combatType != "" {
		query := `
			SELECT id, combat_type, initiator_id, initiator_name, opponent_id, opponent_name,
			       outcome, winner_id, total_turns, created_at
			FROM battle_replays
			WHERE combat_type = ?
			ORDER BY created_at DESC
			LIMIT ?
		`
		rows, err = r.db.QueryContext(ctx, query, combatType, limit)
	} else {
		query := `
			SELECT id, combat_type, initiator_id, initiator_name, opponent_id, opponent_name,
			       outcome, winner_id, total_turns, created_at
			FROM battle_replays
			ORDER BY created_at DESC
			LIMIT ?
		`
		rows, err = r.db.QueryContext(ctx, query, limit)
	}

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanReplayHeaders(rows)
}

func (r *ReplayRepository) DeleteOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	query := `DELETE FROM battle_replays WHERE created_at < ?`
	res, err := r.db.ExecContext(ctx, query, cutoff)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func scanReplayHeaders(rows *sql.Rows) ([]replay.ReplayHeader, error) {
	var list []replay.ReplayHeader
	for rows.Next() {
		var h replay.ReplayHeader
		var outcomeStr string
		var winnerID sql.NullString

		if err := rows.Scan(
			&h.ID,
			&h.CombatType,
			&h.InitiatorID,
			&h.InitiatorName,
			&h.OpponentID,
			&h.OpponentName,
			&outcomeStr,
			&winnerID,
			&h.TotalTurns,
			&h.CreatedAt,
		); err != nil {
			return nil, err
		}

		h.Outcome = corebattle.Outcome(outcomeStr)
		if winnerID.Valid {
			h.WinnerID = winnerID.String
		}
		list = append(list, h)
	}
	return list, rows.Err()
}
