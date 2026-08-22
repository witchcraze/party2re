package database

import (
	"context"
	"database/sql"
	"errors"

	"github.com/witchcraze/party2re/internal/adventure"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

type AdventureRepository struct {
	db *sql.DB
}

func NewAdventureRepository(db *sql.DB) (*AdventureRepository, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	return &AdventureRepository{db: db}, nil
}

func (r *AdventureRepository) Save(ctx context.Context, value adventure.Adventure) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO adventures
			(id, character_id, adventure_type, started_at, available_at, experience_reward,
			 outcome, winner_id, loser_id, battle_turns, resolved, claimed)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			outcome = VALUES(outcome), winner_id = VALUES(winner_id), loser_id = VALUES(loser_id),
			battle_turns = VALUES(battle_turns), resolved = VALUES(resolved), claimed = VALUES(claimed)
	`, value.ID, value.CharacterID, value.Type, value.StartedAt, value.AvailableAt,
		value.ExperienceReward, nullableString(string(value.BattleResult.Outcome), value.Resolved),
		nullableString(value.BattleResult.WinnerID, value.Resolved),
		nullableString(value.BattleResult.LoserID, value.Resolved),
		nullableInt(value.BattleResult.Turns, value.Resolved),
		value.Resolved, value.Claimed)
	return err
}

func (r *AdventureRepository) FindByID(ctx context.Context, id string) (adventure.Adventure, error) {
	var value adventure.Adventure
	var outcome, winnerID, loserID sql.NullString
	var turns sql.NullInt64
	err := r.db.QueryRowContext(ctx, `
		SELECT id, character_id, adventure_type, started_at, available_at, experience_reward,
			outcome, winner_id, loser_id, battle_turns, resolved, claimed
		FROM adventures
		WHERE id = ?
	`, id).Scan(&value.ID, &value.CharacterID, &value.Type, &value.StartedAt, &value.AvailableAt,
		&value.ExperienceReward, &outcome, &winnerID, &loserID, &turns, &value.Resolved, &value.Claimed)
	if errors.Is(err, sql.ErrNoRows) {
		return adventure.Adventure{}, adventure.ErrNotFound
	}
	if err != nil {
		return adventure.Adventure{}, err
	}
	value.BattleResult = corebattle.Result{
		Outcome:  corebattle.Outcome(outcome.String),
		WinnerID: winnerID.String,
		LoserID:  loserID.String,
		Turns:    int(turns.Int64),
	}
	return value, nil
}

func nullableString(value string, valid bool) any {
	if !valid {
		return nil
	}
	return value
}

func nullableInt(value int, valid bool) any {
	if !valid {
		return nil
	}
	return value
}
