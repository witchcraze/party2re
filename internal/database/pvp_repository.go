package database

import (
	"context"
	"database/sql"
	"errors"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/pvp"
)

type PvPRepository struct {
	db *sql.DB
}

func NewPvPRepository(db *sql.DB) (*PvPRepository, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	return &PvPRepository{db: db}, nil
}

func (r *PvPRepository) GetOrCreateRating(ctx context.Context, characterID string) (pvp.ArenaRating, error) {
	var rating pvp.ArenaRating
	var lastMatchedAt sql.NullTime

	err := r.db.QueryRowContext(ctx, `
		SELECT character_id, rating, wins, losses, draws, last_matched_at, created_at, updated_at
		FROM arena_ratings
		WHERE character_id = ?
	`, characterID).Scan(
		&rating.CharacterID,
		&rating.Rating,
		&rating.Wins,
		&rating.Losses,
		&rating.Draws,
		&lastMatchedAt,
		&rating.CreatedAt,
		&rating.UpdatedAt,
	)

	if errors.Is(err, sql.ErrNoRows) {
		now := time.Now().UTC()
		_, insertErr := r.db.ExecContext(ctx, `
			INSERT INTO arena_ratings (
				character_id, rating, wins, losses, draws, created_at, updated_at
			) VALUES (?, ?, 0, 0, 0, ?, ?)
			ON DUPLICATE KEY UPDATE updated_at = VALUES(updated_at)
		`, characterID, pvp.DefaultRating, now, now)
		if insertErr != nil {
			return pvp.ArenaRating{}, insertErr
		}
		return pvp.ArenaRating{
			CharacterID: characterID,
			Rating:      pvp.DefaultRating,
			Wins:        0,
			Losses:      0,
			Draws:       0,
			CreatedAt:   now,
			UpdatedAt:   now,
		}, nil
	}
	if err != nil {
		return pvp.ArenaRating{}, err
	}

	if lastMatchedAt.Valid {
		rating.LastMatchedAt = &lastMatchedAt.Time
	}
	return rating, nil
}

func (r *PvPRepository) RecordMatchAndUpdateRatings(
	ctx context.Context,
	match pvp.MatchRecord,
	attackerRating, defenderRating pvp.ArenaRating,
	attacker corecharacter.Character,
) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	now := time.Now().UTC()

	// 1. Update attacker rating
	_, err = tx.ExecContext(ctx, `
		INSERT INTO arena_ratings (
			character_id, rating, wins, losses, draws, last_matched_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			rating = VALUES(rating),
			wins = VALUES(wins),
			losses = VALUES(losses),
			draws = VALUES(draws),
			last_matched_at = VALUES(last_matched_at),
			updated_at = VALUES(updated_at)
	`, attackerRating.CharacterID, attackerRating.Rating, attackerRating.Wins, attackerRating.Losses, attackerRating.Draws, now, now, now)
	if err != nil {
		return err
	}

	// 2. Update defender rating
	_, err = tx.ExecContext(ctx, `
		INSERT INTO arena_ratings (
			character_id, rating, wins, losses, draws, last_matched_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			rating = VALUES(rating),
			wins = VALUES(wins),
			losses = VALUES(losses),
			draws = VALUES(draws),
			last_matched_at = VALUES(last_matched_at),
			updated_at = VALUES(updated_at)
	`, defenderRating.CharacterID, defenderRating.Rating, defenderRating.Wins, defenderRating.Losses, defenderRating.Draws, now, now, now)
	if err != nil {
		return err
	}

	// 3. Update attacker stats, money, level, exp
	if err := updateCharacterAtomically(ctx, tx, attacker); err != nil {
		return err
	}

	// 4. Insert match history record
	var winnerID, loserID sql.NullString
	if match.WinnerID != "" {
		winnerID = sql.NullString{String: match.WinnerID, Valid: true}
	}
	if match.LoserID != "" {
		loserID = sql.NullString{String: match.LoserID, Valid: true}
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO arena_matches (
			id, attacker_id, defender_id, winner_id, loser_id, outcome, turns,
			attacker_rating_before, attacker_rating_after,
			defender_rating_before, defender_rating_after,
			reward_gold, reward_exp, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, match.ID, match.AttackerID, match.DefenderID, winnerID, loserID, string(match.Outcome), match.Turns,
		match.AttackerRatingBefore, match.AttackerRatingAfter,
		match.DefenderRatingBefore, match.DefenderRatingAfter,
		match.RewardGold, match.RewardExp, match.CreatedAt)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (r *PvPRepository) FindOpponents(ctx context.Context, characterID string, limit int) ([]pvp.OpponentCandidate, error) {
	// Query characters and their ratings, excluding self and same player
	rows, err := r.db.QueryContext(ctx, `
		SELECT c.id, c.name, c.job_id, c.level, COALESCE(ar.rating, 1000) as rating,
		       COALESCE(ar.wins, 0) as wins, COALESCE(ar.losses, 0) as losses, c.rebirth_count
		FROM characters c
		LEFT JOIN arena_ratings ar ON c.id = ar.character_id
		WHERE c.id != ?
		  AND (c.player_id = '' OR c.player_id IS NULL OR c.player_id != COALESCE((SELECT player_id FROM characters WHERE id = ?), ''))
		ORDER BY ABS(COALESCE(ar.rating, 1000) - COALESCE((SELECT rating FROM arena_ratings WHERE character_id = ?), 1000)) ASC,
		         c.level DESC
		LIMIT ?
	`, characterID, characterID, characterID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	candidates := make([]pvp.OpponentCandidate, 0)
	for rows.Next() {
		var c pvp.OpponentCandidate
		if err := rows.Scan(&c.CharacterID, &c.Name, &c.JobID, &c.Level, &c.Rating, &c.Wins, &c.Losses, &c.RebirthCount); err != nil {
			return nil, err
		}
		candidates = append(candidates, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return candidates, nil
}

func (r *PvPRepository) GetMatchHistory(ctx context.Context, characterID string, limit int) ([]pvp.MatchRecord, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, attacker_id, defender_id, winner_id, loser_id, outcome, turns,
		       attacker_rating_before, attacker_rating_after,
		       defender_rating_before, defender_rating_after,
		       reward_gold, reward_exp, created_at
		FROM arena_matches
		WHERE attacker_id = ? OR defender_id = ?
		ORDER BY created_at DESC
		LIMIT ?
	`, characterID, characterID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanMatches(rows)
}

func (r *PvPRepository) GetDefenseLogs(ctx context.Context, characterID string, limit int) ([]pvp.MatchRecord, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, attacker_id, defender_id, winner_id, loser_id, outcome, turns,
		       attacker_rating_before, attacker_rating_after,
		       defender_rating_before, defender_rating_after,
		       reward_gold, reward_exp, created_at
		FROM arena_matches
		WHERE defender_id = ?
		ORDER BY created_at DESC
		LIMIT ?
	`, characterID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanMatches(rows)
}

func (r *PvPRepository) GetLeaderboard(ctx context.Context, limit int) ([]pvp.OpponentCandidate, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT c.id, c.name, c.job_id, c.level, ar.rating, ar.wins, ar.losses, c.rebirth_count
		FROM arena_ratings ar
		JOIN characters c ON ar.character_id = c.id
		ORDER BY ar.rating DESC, ar.wins DESC, c.level DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	candidates := make([]pvp.OpponentCandidate, 0)
	for rows.Next() {
		var c pvp.OpponentCandidate
		if err := rows.Scan(&c.CharacterID, &c.Name, &c.JobID, &c.Level, &c.Rating, &c.Wins, &c.Losses, &c.RebirthCount); err != nil {
			return nil, err
		}
		candidates = append(candidates, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return candidates, nil
}

func scanMatches(rows *sql.Rows) ([]pvp.MatchRecord, error) {
	matches := make([]pvp.MatchRecord, 0)
	for rows.Next() {
		var m pvp.MatchRecord
		var winnerID, loserID sql.NullString
		var outcomeStr string

		if err := rows.Scan(
			&m.ID,
			&m.AttackerID,
			&m.DefenderID,
			&winnerID,
			&loserID,
			&outcomeStr,
			&m.Turns,
			&m.AttackerRatingBefore,
			&m.AttackerRatingAfter,
			&m.DefenderRatingBefore,
			&m.DefenderRatingAfter,
			&m.RewardGold,
			&m.RewardExp,
			&m.CreatedAt,
		); err != nil {
			return nil, err
		}

		if winnerID.Valid {
			m.WinnerID = winnerID.String
		}
		if loserID.Valid {
			m.LoserID = loserID.String
		}
		m.Outcome = pvp.MatchOutcome(outcomeStr)
		matches = append(matches, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return matches, nil
}
