package database

import (
	"context"
	"database/sql"
	"errors"

	"github.com/witchcraze/party2re/internal/party"
)

// PartyRepository implements party.AdventureLogRepository by persisting durable
// party adventure logs in MariaDB. Ephemeral party wait lobbies and member wait states
// are managed exclusively in Valkey Master (RFC #356, Issue #368, Issue #380).
type PartyRepository struct {
	db *sql.DB
}

// NewPartyRepository constructs a new PartyRepository backed by MariaDB.
func NewPartyRepository(db *sql.DB) (*PartyRepository, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	return &PartyRepository{db: db}, nil
}

// SaveAdventureLog persists a completed multiplayer party adventure run in MariaDB.
func (r *PartyRepository) SaveAdventureLog(ctx context.Context, log party.PartyAdventureLog) error {
	var details sql.NullString
	if log.DetailsJSON != "" {
		details.String = log.DetailsJSON
		details.Valid = true
	}
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		INSERT INTO party_adventure_logs
			(id, party_id, stage_id, outcome, turns, total_exp, total_gold, synergy_bonus_percent, details_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, log.ID, log.PartyID, log.StageID, log.Outcome, log.Turns, log.TotalEXP, log.TotalGold, log.SynergyBonusPercent, details, log.CreatedAt)
	return err
}

// GetAdventureLogsByPartyID retrieves all adventure run logs for a party, ordered newest first.
func (r *PartyRepository) GetAdventureLogsByPartyID(ctx context.Context, partyID string) ([]party.PartyAdventureLog, error) {
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, `
		SELECT id, party_id, stage_id, outcome, turns, total_exp, total_gold, synergy_bonus_percent, details_json, created_at
		FROM party_adventure_logs
		WHERE party_id = ?
		ORDER BY created_at DESC
	`, partyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []party.PartyAdventureLog
	for rows.Next() {
		var l party.PartyAdventureLog
		var details sql.NullString
		if err := rows.Scan(
			&l.ID, &l.PartyID, &l.StageID, &l.Outcome, &l.Turns,
			&l.TotalEXP, &l.TotalGold, &l.SynergyBonusPercent, &details, &l.CreatedAt,
		); err != nil {
			return nil, err
		}
		if details.Valid {
			l.DetailsJSON = details.String
		}
		logs = append(logs, l)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return logs, nil
}

// Ensure interface compliance
var _ party.AdventureLogRepository = (*PartyRepository)(nil)
