package database

import (
	"context"
	"database/sql"
	"errors"
	"time"

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
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		INSERT INTO adventures
			(id, character_id, adventure_type, started_at, experience_reward,
			 outcome, winner_id, loser_id, battle_turns, floors_cleared, is_cleared,
			 party_size, reward_experience, reward_currency,
			 reward_item_definition_id, reward_item_quantity, resolved)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			outcome = VALUES(outcome), winner_id = VALUES(winner_id), loser_id = VALUES(loser_id),
			battle_turns = VALUES(battle_turns), floors_cleared = VALUES(floors_cleared),
			is_cleared = VALUES(is_cleared), party_size = VALUES(party_size),
			reward_experience = VALUES(reward_experience),
			reward_currency = VALUES(reward_currency),
			reward_item_definition_id = VALUES(reward_item_definition_id),
			reward_item_quantity = VALUES(reward_item_quantity),
			resolved = VALUES(resolved)
	`, value.ID, value.CharacterID, value.Type, value.StartedAt,
		value.ExperienceReward, nullableString(string(value.BattleResult.Outcome), value.Resolved),
		nullableString(value.BattleResult.WinnerID, value.Resolved),
		nullableString(value.BattleResult.LoserID, value.Resolved),
		nullableInt(value.BattleResult.Turns, value.Resolved),
		value.FloorsCleared, value.IsCleared, value.PartySize,
		value.BattleResult.Reward.Experience, value.BattleResult.Reward.Currency,
		nullableString(value.BattleResult.Reward.ItemDefinitionID, value.Resolved),
		value.BattleResult.Reward.ItemQuantity, value.Resolved)
	return err
}

func (r *AdventureRepository) FindByID(ctx context.Context, id string) (adventure.Adventure, error) {
	var value adventure.Adventure
	var outcome, winnerID, loserID, rewardItemID sql.NullString
	var turns sql.NullInt64
	var rewardExperience, rewardCurrency, rewardItemQuantity int
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT id, character_id, adventure_type, started_at, experience_reward,
			outcome, winner_id, loser_id, battle_turns, floors_cleared, is_cleared,
			party_size, reward_experience, reward_currency,
			reward_item_definition_id, reward_item_quantity, resolved
		FROM adventures
		WHERE id = ?
	`, id).Scan(&value.ID, &value.CharacterID, &value.Type, &value.StartedAt,
		&value.ExperienceReward, &outcome, &winnerID, &loserID, &turns,
		&value.FloorsCleared, &value.IsCleared, &value.PartySize,
		&rewardExperience, &rewardCurrency, &rewardItemID, &rewardItemQuantity, &value.Resolved)
	if errors.Is(err, sql.ErrNoRows) {
		return adventure.Adventure{}, adventure.ErrNotFound
	}
	if err != nil {
		return adventure.Adventure{}, err
	}
	value.StageID = value.Type
	value.BattleResult = corebattle.Result{
		Outcome:  corebattle.Outcome(outcome.String),
		WinnerID: winnerID.String,
		LoserID:  loserID.String,
		Turns:    int(turns.Int64),
		Reward: corebattle.Reward{
			Experience:       rewardExperience,
			Currency:         rewardCurrency,
			ItemDefinitionID: rewardItemID.String,
			ItemQuantity:     rewardItemQuantity,
		},
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

func (r *AdventureRepository) ListByCharacterID(ctx context.Context, characterID string, limit, offset int) ([]adventure.Adventure, int, error) {
	var total int
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT COUNT(*) FROM adventures WHERE character_id = ?
	`, characterID).Scan(&total)
	if err != nil {
		return nil, 0, err
	}
	if total == 0 || offset >= total {
		return []adventure.Adventure{}, total, nil
	}

	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, `
		SELECT id, character_id, adventure_type, started_at, experience_reward,
			outcome, winner_id, loser_id, battle_turns, floors_cleared, is_cleared,
			party_size, reward_experience, reward_currency,
			reward_item_definition_id, reward_item_quantity, resolved
		FROM adventures
		WHERE character_id = ?
		ORDER BY started_at DESC, id DESC
		LIMIT ? OFFSET ?
	`, characterID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var result []adventure.Adventure
	for rows.Next() {
		var value adventure.Adventure
		var outcome, winnerID, loserID, rewardItemID sql.NullString
		var turns sql.NullInt64
		var rewardExperience, rewardCurrency, rewardItemQuantity int
		if err := rows.Scan(&value.ID, &value.CharacterID, &value.Type, &value.StartedAt,
			&value.ExperienceReward, &outcome, &winnerID, &loserID, &turns,
			&value.FloorsCleared, &value.IsCleared, &value.PartySize,
			&rewardExperience, &rewardCurrency, &rewardItemID, &rewardItemQuantity, &value.Resolved); err != nil {
			return nil, 0, err
		}
		value.StageID = value.Type
		value.BattleResult = corebattle.Result{
			Outcome:  corebattle.Outcome(outcome.String),
			WinnerID: winnerID.String,
			LoserID:  loserID.String,
			Turns:    int(turns.Int64),
			Reward: corebattle.Reward{
				Experience:       rewardExperience,
				Currency:         rewardCurrency,
				ItemDefinitionID: rewardItemID.String,
				ItemQuantity:     rewardItemQuantity,
			},
		}
		result = append(result, value)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return result, total, nil
}

func (r *AdventureRepository) ListByCharacterIDByCursor(ctx context.Context, characterID string, limit int, beforeTime time.Time, beforeID string) ([]adventure.Adventure, error) {
	executor := ExecutorFromContext(ctx, r.db)
	var rows *sql.Rows
	var err error

	if beforeTime.IsZero() && beforeID == "" {
		rows, err = executor.QueryContext(ctx, `
			SELECT id, character_id, adventure_type, started_at, experience_reward,
				outcome, winner_id, loser_id, battle_turns, floors_cleared, is_cleared,
				party_size, reward_experience, reward_currency,
				reward_item_definition_id, reward_item_quantity, resolved
			FROM adventures
			WHERE character_id = ?
			ORDER BY started_at DESC, id DESC
			LIMIT ?
		`, characterID, limit)
	} else if !beforeTime.IsZero() && beforeID != "" {
		rows, err = executor.QueryContext(ctx, `
			SELECT id, character_id, adventure_type, started_at, experience_reward,
				outcome, winner_id, loser_id, battle_turns, floors_cleared, is_cleared,
				party_size, reward_experience, reward_currency,
				reward_item_definition_id, reward_item_quantity, resolved
			FROM adventures
			WHERE character_id = ? AND (started_at < ? OR (started_at = ? AND id < ?))
			ORDER BY started_at DESC, id DESC
			LIMIT ?
		`, characterID, beforeTime.UTC(), beforeTime.UTC(), beforeID, limit)
	} else if !beforeTime.IsZero() {
		rows, err = executor.QueryContext(ctx, `
			SELECT id, character_id, adventure_type, started_at, experience_reward,
				outcome, winner_id, loser_id, battle_turns, floors_cleared, is_cleared,
				party_size, reward_experience, reward_currency,
				reward_item_definition_id, reward_item_quantity, resolved
			FROM adventures
			WHERE character_id = ? AND started_at < ?
			ORDER BY started_at DESC, id DESC
			LIMIT ?
		`, characterID, beforeTime.UTC(), limit)
	} else {
		rows, err = executor.QueryContext(ctx, `
			SELECT id, character_id, adventure_type, started_at, experience_reward,
				outcome, winner_id, loser_id, battle_turns, floors_cleared, is_cleared,
				party_size, reward_experience, reward_currency,
				reward_item_definition_id, reward_item_quantity, resolved
			FROM adventures
			WHERE character_id = ? AND id < ?
			ORDER BY id DESC
			LIMIT ?
		`, characterID, beforeID, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []adventure.Adventure
	for rows.Next() {
		var value adventure.Adventure
		var outcome, winnerID, loserID, rewardItemID sql.NullString
		var turns sql.NullInt64
		var rewardExperience, rewardCurrency, rewardItemQuantity int
		if err := rows.Scan(&value.ID, &value.CharacterID, &value.Type, &value.StartedAt,
			&value.ExperienceReward, &outcome, &winnerID, &loserID, &turns,
			&value.FloorsCleared, &value.IsCleared, &value.PartySize,
			&rewardExperience, &rewardCurrency, &rewardItemID, &rewardItemQuantity, &value.Resolved); err != nil {
			return nil, err
		}
		value.StageID = value.Type
		value.BattleResult = corebattle.Result{
			Outcome:  corebattle.Outcome(outcome.String),
			WinnerID: winnerID.String,
			LoserID:  loserID.String,
			Turns:    int(turns.Int64),
			Reward: corebattle.Reward{
				Experience:       rewardExperience,
				Currency:         rewardCurrency,
				ItemDefinitionID: rewardItemID.String,
				ItemQuantity:     rewardItemQuantity,
			},
		}
		result = append(result, value)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *AdventureRepository) GetAggregatedStats(ctx context.Context, characterID string) (adventure.AggregatedStats, error) {
	var stats adventure.AggregatedStats
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT
			COUNT(*),
			COALESCE(SUM(CASE WHEN outcome = 'win' AND winner_id = ? THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN (outcome = 'win' AND winner_id != ?) OR (outcome = 'loss') THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN outcome = 'draw' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(battle_turns), 0),
			COALESCE(SUM(reward_experience), 0),
			COALESCE(SUM(reward_currency), 0)
		FROM adventures
		WHERE character_id = ? AND resolved = TRUE
	`, characterID, characterID, characterID).Scan(
		&stats.TotalAdventures,
		&stats.TotalVictories,
		&stats.TotalDefeats,
		&stats.TotalDraws,
		&stats.TotalTurns,
		&stats.TotalExpEarned,
		&stats.TotalGoldEarned,
	)
	if err != nil {
		return adventure.AggregatedStats{}, err
	}

	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, `
		SELECT
			adventure_type,
			COUNT(*) AS total_attempts,
			COALESCE(SUM(CASE WHEN (outcome = 'win' AND winner_id = ?) OR is_cleared = TRUE THEN 1 ELSE 0 END), 0) AS clear_count
		FROM adventures
		WHERE character_id = ? AND resolved = TRUE
		GROUP BY adventure_type
		ORDER BY adventure_type ASC
	`, characterID, characterID)
	if err != nil {
		return adventure.AggregatedStats{}, err
	}
	defer rows.Close()

	for rows.Next() {
		var item adventure.StageStatData
		if err := rows.Scan(&item.StageID, &item.TotalAttempts, &item.ClearCount); err != nil {
			return adventure.AggregatedStats{}, err
		}
		stats.StageStats = append(stats.StageStats, item)
	}
	if err := rows.Err(); err != nil {
		return adventure.AggregatedStats{}, err
	}

	return stats, nil
}
