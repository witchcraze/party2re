package database

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/witchcraze/party2re/internal/challenge"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/progression"
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
	var partyID sql.NullString
	if s.PartyID != "" {
		partyID = sql.NullString{String: s.PartyID, Valid: true}
	}
	var membersJSON sql.NullString
	if len(s.Members) > 0 {
		membersJSON = sql.NullString{String: challenge.EncodeJSON(s.Members), Valid: true}
	}
	query := `
		INSERT INTO challenge_sessions (
			id, character_id, party_id, members_json, tier_id, current_round, character_current_hp,
			accumulated_exp, accumulated_gold, accumulated_items_json, status,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(
		ctx,
		query,
		s.ID,
		s.CharacterID,
		partyID,
		membersJSON,
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
		SELECT id, character_id, party_id, members_json, tier_id, current_round, character_current_hp,
		       accumulated_exp, accumulated_gold, accumulated_items_json, status,
		       created_at, updated_at
		FROM challenge_sessions
		WHERE id = ?
	`
	var s challenge.ChallengeSession
	var statusStr, itemsJSON string
	var partyID, membersJSON sql.NullString

	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, query, id).Scan(
		&s.ID,
		&s.CharacterID,
		&partyID,
		&membersJSON,
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

	if partyID.Valid {
		s.PartyID = partyID.String
	}
	if membersJSON.Valid && membersJSON.String != "" {
		members, err := challenge.DecodeJSON[[]challenge.ChallengeMember](membersJSON.String)
		if err == nil {
			s.Members = members
		}
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
		SELECT id, character_id, party_id, members_json, tier_id, current_round, character_current_hp,
		       accumulated_exp, accumulated_gold, accumulated_items_json, status,
		       created_at, updated_at
		FROM challenge_sessions
		WHERE character_id = ? AND status = 'active'
		ORDER BY created_at DESC
		LIMIT 1
	`
	var s challenge.ChallengeSession
	var statusStr, itemsJSON string
	var partyID, membersJSON sql.NullString

	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, query, characterID).Scan(
		&s.ID,
		&s.CharacterID,
		&partyID,
		&membersJSON,
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

	if partyID.Valid {
		s.PartyID = partyID.String
	}
	if membersJSON.Valid && membersJSON.String != "" {
		members, err := challenge.DecodeJSON[[]challenge.ChallengeMember](membersJSON.String)
		if err == nil {
			s.Members = members
		}
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
	var membersJSON sql.NullString
	if len(s.Members) > 0 {
		membersJSON = sql.NullString{String: challenge.EncodeJSON(s.Members), Valid: true}
	}
	query := `
		UPDATE challenge_sessions
		SET current_round = ?, character_current_hp = ?, accumulated_exp = ?,
		    accumulated_gold = ?, accumulated_items_json = ?, members_json = ?, status = ?,
		    updated_at = ?
		WHERE id = ? AND character_id = ?
	`
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(
		ctx,
		query,
		s.CurrentRound,
		s.CharacterCurrentHP,
		s.AccumulatedExp,
		s.AccumulatedGold,
		itemsJSON,
		membersJSON,
		string(s.Status),
		s.UpdatedAt,
		s.ID,
		s.CharacterID,
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

func (r *ChallengeRepository) FindRecordsByCharacter(ctx context.Context, characterID string) ([]challenge.CharacterChallengeRecord, error) {
	query := `
		SELECT character_id, tier_id, highest_round, total_attempts,
		       total_victories, best_cleared_at
		FROM character_challenge_records
		WHERE character_id = ?
		ORDER BY tier_id ASC
	`
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, query, characterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []challenge.CharacterChallengeRecord
	for rows.Next() {
		var rec challenge.CharacterChallengeRecord
		if err := rows.Scan(
			&rec.CharacterID,
			&rec.TierID,
			&rec.HighestRound,
			&rec.TotalAttempts,
			&rec.TotalVictories,
			&rec.BestClearedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, rec)
	}
	return list, rows.Err()
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
		var partyID sql.NullString
		if s.PartyID != "" {
			partyID = sql.NullString{String: s.PartyID, Valid: true}
		}
		var membersJSON sql.NullString
		if len(s.Members) > 0 {
			membersJSON = sql.NullString{String: challenge.EncodeJSON(s.Members), Valid: true}
		}
		upsertSessionQuery := `
			INSERT INTO challenge_sessions (
				id, character_id, party_id, members_json, tier_id, current_round, character_current_hp,
				accumulated_exp, accumulated_gold, accumulated_items_json, status,
				created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE
				party_id = VALUES(party_id),
				members_json = VALUES(members_json),
				current_round = VALUES(current_round),
				character_current_hp = VALUES(character_current_hp),
				accumulated_exp = VALUES(accumulated_exp),
				accumulated_gold = VALUES(accumulated_gold),
				accumulated_items_json = VALUES(accumulated_items_json),
				status = VALUES(status),
				updated_at = VALUES(updated_at)
		`
		if _, err := executor.ExecContext(
			txCtx,
			upsertSessionQuery,
			s.ID,
			s.CharacterID,
			partyID,
			membersJSON,
			s.TierID,
			s.CurrentRound,
			s.CharacterCurrentHP,
			s.AccumulatedExp,
			s.AccumulatedGold,
			itemsJSON,
			string(s.Status),
			s.CreatedAt,
			s.UpdatedAt,
		); err != nil {
			return err
		}

		// 2. Lock Character (Rank 2) & Award EXP and Gold
		charRepo := &CharacterRepository{db: r.db}
		char, err := charRepo.FindByIDForUpdate(txCtx, s.CharacterID)
		if err != nil {
			return err
		}

		if goldReward > 0 {
			if err := char.AddMoney(goldReward); err != nil {
				return err
			}
		}
		if expReward > 0 {
			if _, err := progression.ApplyExperience(&char, expReward); err != nil {
				return err
			}
		}
		if err := charRepo.Update(txCtx, char); err != nil {
			return err
		}

		// 3. Award Items with capacity enforcement and depot overflow (Rank 3 Inventory -> Rank 5 Depot)
		if len(items) > 0 {
			rewardInstances := make([]coreitem.Instance, 0, len(items))
			for _, itemDefID := range items {
				if itemDefID == "" {
					continue
				}
				inst, err := coreitem.NewInstance(itemDefID, 1)
				if err != nil {
					return err
				}
				rewardInstances = append(rewardInstances, inst)
			}
			if err := deliverRewardItems(txCtx, r.db, char, rewardInstances); err != nil {
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
