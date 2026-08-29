package database

import (
	"context"
	"database/sql"
	"errors"

	"github.com/witchcraze/party2re/internal/adventure"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
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
			 outcome, winner_id, loser_id, battle_turns, reward_experience, reward_currency,
			 reward_item_definition_id, reward_item_quantity, resolved, claimed)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			outcome = VALUES(outcome), winner_id = VALUES(winner_id), loser_id = VALUES(loser_id),
			battle_turns = VALUES(battle_turns), reward_experience = VALUES(reward_experience),
			reward_currency = VALUES(reward_currency),
			reward_item_definition_id = VALUES(reward_item_definition_id),
			reward_item_quantity = VALUES(reward_item_quantity),
			resolved = VALUES(resolved), claimed = VALUES(claimed)
	`, value.ID, value.CharacterID, value.Type, value.StartedAt, value.AvailableAt,
		value.ExperienceReward, nullableString(string(value.BattleResult.Outcome), value.Resolved),
		nullableString(value.BattleResult.WinnerID, value.Resolved),
		nullableString(value.BattleResult.LoserID, value.Resolved),
		nullableInt(value.BattleResult.Turns, value.Resolved),
		value.BattleResult.Reward.Experience, value.BattleResult.Reward.Currency,
		nullableString(value.BattleResult.Reward.ItemDefinitionID, value.Resolved),
		value.BattleResult.Reward.ItemQuantity, value.Resolved, value.Claimed)
	return err
}

func (r *AdventureRepository) FindByID(ctx context.Context, id string) (adventure.Adventure, error) {
	var value adventure.Adventure
	var outcome, winnerID, loserID, rewardItemID sql.NullString
	var turns sql.NullInt64
	var rewardExperience, rewardCurrency, rewardItemQuantity int
	err := r.db.QueryRowContext(ctx, `
		SELECT id, character_id, adventure_type, started_at, available_at, experience_reward,
			outcome, winner_id, loser_id, battle_turns, reward_experience, reward_currency,
			reward_item_definition_id, reward_item_quantity, resolved, claimed
		FROM adventures
		WHERE id = ?
	`, id).Scan(&value.ID, &value.CharacterID, &value.Type, &value.StartedAt, &value.AvailableAt,
		&value.ExperienceReward, &outcome, &winnerID, &loserID, &turns, &rewardExperience,
		&rewardCurrency, &rewardItemID, &rewardItemQuantity, &value.Resolved, &value.Claimed)
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

func (r *AdventureRepository) ClaimAndApply(ctx context.Context, value adventure.Adventure, character corecharacter.Character) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	result, err := tx.ExecContext(ctx, `
		UPDATE adventures
		SET outcome = ?, winner_id = ?, loser_id = ?, battle_turns = ?,
			reward_experience = ?, reward_currency = ?, reward_item_definition_id = ?,
			reward_item_quantity = ?, resolved = TRUE, claimed = TRUE
		WHERE id = ? AND claimed = FALSE
	`, nullableString(string(value.BattleResult.Outcome), value.Resolved),
		nullableString(value.BattleResult.WinnerID, value.Resolved),
		nullableString(value.BattleResult.LoserID, value.Resolved),
		nullableInt(value.BattleResult.Turns, value.Resolved),
		value.BattleResult.Reward.Experience, value.BattleResult.Reward.Currency,
		nullableString(value.BattleResult.Reward.ItemDefinitionID, value.Resolved),
		value.BattleResult.Reward.ItemQuantity, value.ID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return claimFailure(ctx, tx, "adventures", value.ID, adventure.ErrNotFound, adventure.ErrAlreadyClaimed)
	}
	if err := updateCharacterAtomically(ctx, tx, character); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true
	return nil
}

func (r *AdventureRepository) ListByCharacterID(ctx context.Context, characterID string, limit, offset int) ([]adventure.Adventure, int, error) {
	var total int
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM adventures WHERE character_id = ?
	`, characterID).Scan(&total)
	if err != nil {
		return nil, 0, err
	}
	if total == 0 || offset >= total {
		return []adventure.Adventure{}, total, nil
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT id, character_id, adventure_type, started_at, available_at, experience_reward,
			outcome, winner_id, loser_id, battle_turns, reward_experience, reward_currency,
			reward_item_definition_id, reward_item_quantity, resolved, claimed
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
		if err := rows.Scan(&value.ID, &value.CharacterID, &value.Type, &value.StartedAt, &value.AvailableAt,
			&value.ExperienceReward, &outcome, &winnerID, &loserID, &turns, &rewardExperience,
			&rewardCurrency, &rewardItemID, &rewardItemQuantity, &value.Resolved, &value.Claimed); err != nil {
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

func (r *AdventureRepository) GetAggregatedStats(ctx context.Context, characterID string) (adventure.AggregatedStats, error) {
	var stats adventure.AggregatedStats
	err := r.db.QueryRowContext(ctx, `
		SELECT
			COUNT(*),
			COALESCE(SUM(CASE WHEN outcome = 'win' AND winner_id = ? THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN (outcome = 'win' AND winner_id != ?) OR (outcome = 'loss') THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN outcome = 'draw' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(battle_turns), 0),
			COALESCE(SUM(reward_experience), 0),
			COALESCE(SUM(reward_currency), 0)
		FROM adventures
		WHERE character_id = ? AND claimed = TRUE
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

	rows, err := r.db.QueryContext(ctx, `
		SELECT
			adventure_type,
			COUNT(*) AS total_attempts,
			COALESCE(SUM(CASE WHEN outcome = 'win' AND winner_id = ? THEN 1 ELSE 0 END), 0) AS clear_count
		FROM adventures
		WHERE character_id = ? AND claimed = TRUE
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
