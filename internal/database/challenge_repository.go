package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/witchcraze/party2re/internal/challenge"
)

type ChallengeRepository struct {
	db *sql.DB
}

func NewChallengeRepository(db *sql.DB) (*ChallengeRepository, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	return &ChallengeRepository{db: db}, nil
}

func (r *ChallengeRepository) SaveSession(ctx context.Context, s challenge.ChallengeSession) error {
	itemsJSON := challenge.EncodeJSON(s.AccumulatedItems)
	query := `
		INSERT INTO challenge_sessions (
			id, character_id, tier_id, current_round, character_current_hp,
			accumulated_exp, accumulated_gold, accumulated_items_json, status,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(
		ctx,
		query,
		s.ID,
		s.CharacterID,
		s.TierID,
		s.CurrentRound,
		s.CharacterCurrentHP,
		s.AccumulatedExp,
		s.AccumulatedGold,
		itemsJSON,
		string(s.Status),
		s.CreatedAt,
		s.UpdatedAt,
	)
	return err
}

func (r *ChallengeRepository) FindSessionByID(ctx context.Context, id string) (*challenge.ChallengeSession, error) {
	query := `
		SELECT id, character_id, tier_id, current_round, character_current_hp,
		       accumulated_exp, accumulated_gold, accumulated_items_json, status,
		       created_at, updated_at
		FROM challenge_sessions
		WHERE id = ?
	`
	var s challenge.ChallengeSession
	var statusStr, itemsJSON string

	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, query, id).Scan(
		&s.ID,
		&s.CharacterID,
		&s.TierID,
		&s.CurrentRound,
		&s.CharacterCurrentHP,
		&s.AccumulatedExp,
		&s.AccumulatedGold,
		&itemsJSON,
		&statusStr,
		&s.CreatedAt,
		&s.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, challenge.ErrSessionNotFound
	}
	if err != nil {
		return nil, err
	}

	s.Status = challenge.SessionStatus(statusStr)
	items, err := challenge.DecodeJSON[[]string](itemsJSON)
	if err != nil {
		return nil, err
	}
	s.AccumulatedItems = items

	return &s, nil
}

func (r *ChallengeRepository) FindActiveSessionByCharacter(ctx context.Context, characterID string) (*challenge.ChallengeSession, error) {
	query := `
		SELECT id, character_id, tier_id, current_round, character_current_hp,
		       accumulated_exp, accumulated_gold, accumulated_items_json, status,
		       created_at, updated_at
		FROM challenge_sessions
		WHERE character_id = ? AND status = 'active'
		ORDER BY created_at DESC
		LIMIT 1
	`
	var s challenge.ChallengeSession
	var statusStr, itemsJSON string

	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, query, characterID).Scan(
		&s.ID,
		&s.CharacterID,
		&s.TierID,
		&s.CurrentRound,
		&s.CharacterCurrentHP,
		&s.AccumulatedExp,
		&s.AccumulatedGold,
		&itemsJSON,
		&statusStr,
		&s.CreatedAt,
		&s.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	s.Status = challenge.SessionStatus(statusStr)
	items, err := challenge.DecodeJSON[[]string](itemsJSON)
	if err != nil {
		return nil, err
	}
	s.AccumulatedItems = items

	return &s, nil
}

func (r *ChallengeRepository) UpdateSession(ctx context.Context, s challenge.ChallengeSession) error {
	itemsJSON := challenge.EncodeJSON(s.AccumulatedItems)
	query := `
		UPDATE challenge_sessions
		SET current_round = ?, character_current_hp = ?, accumulated_exp = ?,
		    accumulated_gold = ?, accumulated_items_json = ?, status = ?,
		    updated_at = ?
		WHERE id = ?
	`
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(
		ctx,
		query,
		s.CurrentRound,
		s.CharacterCurrentHP,
		s.AccumulatedExp,
		s.AccumulatedGold,
		itemsJSON,
		string(s.Status),
		s.UpdatedAt,
		s.ID,
	)
	return err
}

func (r *ChallengeRepository) SaveRecord(ctx context.Context, rec challenge.CharacterChallengeRecord) error {
	query := `
		INSERT INTO character_challenge_records (
			character_id, tier_id, highest_round, total_attempts,
			total_victories, best_cleared_at
		) VALUES (?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			highest_round = IF(VALUES(highest_round) > highest_round, VALUES(highest_round), highest_round),
			total_attempts = total_attempts + VALUES(total_attempts),
			total_victories = total_victories + VALUES(total_victories),
			best_cleared_at = IF(VALUES(highest_round) > highest_round, VALUES(best_cleared_at), best_cleared_at)
	`
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(
		ctx,
		query,
		rec.CharacterID,
		rec.TierID,
		rec.HighestRound,
		rec.TotalAttempts,
		rec.TotalVictories,
		rec.BestClearedAt,
	)
	return err
}

func (r *ChallengeRepository) FindRecord(ctx context.Context, characterID string, tierID string) (*challenge.CharacterChallengeRecord, error) {
	query := `
		SELECT character_id, tier_id, highest_round, total_attempts,
		       total_victories, best_cleared_at
		FROM character_challenge_records
		WHERE character_id = ? AND tier_id = ?
	`
	var rec challenge.CharacterChallengeRecord
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, query, characterID, tierID).Scan(
		&rec.CharacterID,
		&rec.TierID,
		&rec.HighestRound,
		&rec.TotalAttempts,
		&rec.TotalVictories,
		&rec.BestClearedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

func (r *ChallengeRepository) GetLeaderboard(ctx context.Context, tierID string, limit int) ([]challenge.LeaderboardEntry, error) {
	query := `
		SELECT r.character_id, c.name, c.level, c.job_id, r.highest_round, r.best_cleared_at
		FROM character_challenge_records r
		JOIN characters c ON r.character_id = c.id
		WHERE r.tier_id = ? AND r.highest_round > 0
		ORDER BY r.highest_round DESC, r.best_cleared_at ASC
		LIMIT ?
	`
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, query, tierID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []challenge.LeaderboardEntry
	for rows.Next() {
		var e challenge.LeaderboardEntry
		if err := rows.Scan(
			&e.CharacterID,
			&e.CharacterName,
			&e.Level,
			&e.JobID,
			&e.HighestRound,
			&e.BestClearedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, e)
	}
	return list, rows.Err()
}

func (r *ChallengeRepository) FinalizeSession(ctx context.Context, s challenge.ChallengeSession, expReward int, goldReward int, items []string, newStreak int) error {
	return RunInTx(ctx, r.db, func(txCtx context.Context) error {
		executor := ExecutorFromContext(txCtx, r.db)

		// 1. Update session status
		itemsJSON := challenge.EncodeJSON(s.AccumulatedItems)
		updateSessionQuery := `
			UPDATE challenge_sessions
			SET current_round = ?, character_current_hp = ?, accumulated_exp = ?,
			    accumulated_gold = ?, accumulated_items_json = ?, status = ?,
			    updated_at = ?
			WHERE id = ?
		`
		if _, err := executor.ExecContext(
			txCtx,
			updateSessionQuery,
			s.CurrentRound,
			s.CharacterCurrentHP,
			s.AccumulatedExp,
			s.AccumulatedGold,
			itemsJSON,
			string(s.Status),
			s.UpdatedAt,
			s.ID,
		); err != nil {
			return err
		}

		// 2. Award Character EXP & Gold
		if expReward > 0 || goldReward > 0 {
			updateCharQuery := `UPDATE characters SET experience = experience + ?, money = money + ? WHERE id = ?`
			if _, err := executor.ExecContext(txCtx, updateCharQuery, expReward, goldReward, s.CharacterID); err != nil {
				return err
			}
		}

		// 3. Award Items
		for _, itemDefID := range items {
			if itemDefID == "" {
				continue
			}
			itemInstID := fmt.Sprintf("%032x", time.Now().UnixNano())
			insertItemQuery := `
				INSERT INTO inventory_items (id, character_id, definition_id, quantity)
				VALUES (?, ?, ?, 1)
			`
			if _, err := executor.ExecContext(txCtx, insertItemQuery, itemInstID, s.CharacterID, itemDefID); err != nil {
				return err
			}
		}

		// 4. Update Character Challenge Record
		now := time.Now().UTC()
		upsertRecordQuery := `
			INSERT INTO character_challenge_records (
				character_id, tier_id, highest_round, total_attempts,
				total_victories, best_cleared_at
			) VALUES (?, ?, ?, 1, ?, ?)
			ON DUPLICATE KEY UPDATE
				best_cleared_at = IF(VALUES(highest_round) > highest_round, VALUES(best_cleared_at), best_cleared_at),
				highest_round = IF(VALUES(highest_round) > highest_round, VALUES(highest_round), highest_round),
				total_attempts = total_attempts + 1,
				total_victories = total_victories + VALUES(total_victories)
		`
		if _, err := executor.ExecContext(txCtx, upsertRecordQuery, s.CharacterID, s.TierID, newStreak, newStreak, now); err != nil {
			return err
		}

		return nil
	})
}
