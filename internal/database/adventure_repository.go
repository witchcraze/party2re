package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/witchcraze/party2re/internal/adventure"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

const adventureSelectColumns = `id, character_id, adventure_type, started_at, experience_reward,
			outcome, winner_id, loser_id, battle_turns, floors_cleared, is_cleared,
			seasonal_sub_id, season,
			party_size, reward_experience, reward_currency, reward_crystals,
			reward_item_definition_id, reward_item_quantity, resolved`

type adventureRowScanner interface {
	Scan(dest ...any) error
}

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
	season := value.Season
	subID := value.SeasonalSubID
	if subID == 0 && season != "" {
		subID = adventure.SeasonToSubID(season)
	}
	if season == "" && subID != 0 {
		season = adventure.SubIDToSeason(subID)
	}

	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		INSERT INTO adventures
			(id, character_id, adventure_type, started_at, experience_reward,
			 outcome, winner_id, loser_id, battle_turns, floors_cleared, is_cleared,
			 seasonal_sub_id, season,
			 party_size, reward_experience, reward_currency, reward_crystals,
			 reward_item_definition_id, reward_item_quantity, resolved)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			outcome = VALUES(outcome), winner_id = VALUES(winner_id), loser_id = VALUES(loser_id),
			battle_turns = VALUES(battle_turns), floors_cleared = VALUES(floors_cleared),
			is_cleared = VALUES(is_cleared), seasonal_sub_id = VALUES(seasonal_sub_id), season = VALUES(season),
			party_size = VALUES(party_size),
			reward_experience = VALUES(reward_experience),
			reward_currency = VALUES(reward_currency),
			reward_crystals = VALUES(reward_crystals),
			reward_item_definition_id = VALUES(reward_item_definition_id),
			reward_item_quantity = VALUES(reward_item_quantity),
			resolved = VALUES(resolved)
	`, value.ID, value.CharacterID, value.Type, value.StartedAt,
		value.ExperienceReward, nullableString(string(value.BattleResult.Outcome), value.Resolved),
		nullableString(value.BattleResult.WinnerID, value.Resolved),
		nullableString(value.BattleResult.LoserID, value.Resolved),
		nullableInt(value.BattleResult.Turns, value.Resolved),
		value.FloorsCleared, value.IsCleared,
		subID, season,
		value.PartySize,
		value.BattleResult.Reward.Experience, value.BattleResult.Reward.Currency,
		value.BattleResult.Reward.Crystals,
		nullableString(value.BattleResult.Reward.ItemDefinitionID, value.Resolved),
		value.BattleResult.Reward.ItemQuantity, value.Resolved)
	return err
}

func (r *AdventureRepository) FindByID(ctx context.Context, id string) (adventure.Adventure, error) {
	query := fmt.Sprintf("SELECT %s FROM adventures WHERE id = ?", adventureSelectColumns)
	value, err := scanAdventure(ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, query, id))
	if errors.Is(err, sql.ErrNoRows) {
		return adventure.Adventure{}, adventure.ErrNotFound
	}
	if err != nil {
		return adventure.Adventure{}, err
	}
	return value, nil
}

func scanAdventure(scanner adventureRowScanner) (adventure.Adventure, error) {
	var value adventure.Adventure
	var outcome, winnerID, loserID, rewardItemID, seasonStr sql.NullString
	var turns sql.NullInt64
	var seasonalSubID, rewardExperience, rewardCurrency, rewardCrystals, rewardItemQuantity int
	if err := scanner.Scan(&value.ID, &value.CharacterID, &value.Type, &value.StartedAt,
		&value.ExperienceReward, &outcome, &winnerID, &loserID, &turns,
		&value.FloorsCleared, &value.IsCleared,
		&seasonalSubID, &seasonStr,
		&value.PartySize,
		&rewardExperience, &rewardCurrency, &rewardCrystals,
		&rewardItemID, &rewardItemQuantity, &value.Resolved); err != nil {
		return adventure.Adventure{}, err
	}
	value.StageID = value.Type
	value.SeasonalSubID = seasonalSubID
	value.Season = seasonStr.String
	if value.Season == "" && value.SeasonalSubID != 0 {
		value.Season = adventure.SubIDToSeason(value.SeasonalSubID)
	}
	if value.SeasonalSubID == 0 && value.Season != "" {
		value.SeasonalSubID = adventure.SeasonToSubID(value.Season)
	}
	value.BattleResult = corebattle.Result{
		Outcome:  corebattle.Outcome(outcome.String),
		WinnerID: winnerID.String,
		LoserID:  loserID.String,
		Turns:    int(turns.Int64),
		Reward: corebattle.Reward{
			Experience:       rewardExperience,
			Currency:         rewardCurrency,
			Crystals:         rewardCrystals,
			ItemDefinitionID: rewardItemID.String,
			ItemQuantity:     rewardItemQuantity,
		},
	}
	return value, nil
}

func scanAdventureRows(rows *sql.Rows) ([]adventure.Adventure, error) {
	var result []adventure.Adventure
	for rows.Next() {
		value, err := scanAdventure(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
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

	query := fmt.Sprintf(`
		SELECT %s
		FROM adventures
		WHERE character_id = ?
		ORDER BY started_at DESC, id DESC
		LIMIT ? OFFSET ?
	`, adventureSelectColumns)
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, query, characterID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	result, err := scanAdventureRows(rows)
	if err != nil {
		return nil, 0, err
	}
	return result, total, nil
}

func (r *AdventureRepository) ListByCharacterIDByCursor(ctx context.Context, characterID string, limit int, beforeTime time.Time, beforeID string) ([]adventure.Adventure, error) {
	executor := ExecutorFromContext(ctx, r.db)
	var where string
	orderBy := "ORDER BY started_at DESC, id DESC"
	var args []any

	if beforeTime.IsZero() && beforeID == "" {
		where = "character_id = ?"
		args = []any{characterID, limit}
	} else if !beforeTime.IsZero() && beforeID != "" {
		where = "character_id = ? AND (started_at < ? OR (started_at = ? AND id < ?))"
		args = []any{characterID, beforeTime.UTC(), beforeTime.UTC(), beforeID, limit}
	} else if !beforeTime.IsZero() {
		where = "character_id = ? AND started_at < ?"
		args = []any{characterID, beforeTime.UTC(), limit}
	} else {
		where = "character_id = ? AND id < ?"
		orderBy = "ORDER BY id DESC"
		args = []any{characterID, beforeID, limit}
	}

	query := fmt.Sprintf("SELECT %s FROM adventures WHERE %s %s LIMIT ?", adventureSelectColumns, where, orderBy)
	rows, err := executor.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanAdventureRows(rows)
}

func (r *AdventureRepository) GetAggregatedStats(ctx context.Context, characterID string) (adventure.AggregatedStats, error) {
	var stats adventure.AggregatedStats
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT
			COUNT(*),
			COALESCE(SUM(CASE WHEN (outcome = 'win' AND winner_id = ?) OR is_cleared = TRUE THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN (outcome = 'win' AND winner_id != ? AND is_cleared = FALSE) OR outcome IN ('defeat', 'loss') THEN 1 ELSE 0 END), 0),
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
			seasonal_sub_id,
			season,
			COUNT(*) AS total_attempts,
			COALESCE(SUM(CASE WHEN (outcome = 'win' AND winner_id = ?) OR is_cleared = TRUE THEN 1 ELSE 0 END), 0) AS clear_count
		FROM adventures
		WHERE character_id = ? AND resolved = TRUE
		GROUP BY adventure_type, seasonal_sub_id, season
		ORDER BY adventure_type ASC, seasonal_sub_id ASC
	`, characterID, characterID)
	if err != nil {
		return adventure.AggregatedStats{}, err
	}
	defer rows.Close()

	for rows.Next() {
		var item adventure.StageStatData
		var seasonStr string
		if err := rows.Scan(&item.StageID, &item.SeasonalSubID, &seasonStr, &item.TotalAttempts, &item.ClearCount); err != nil {
			return adventure.AggregatedStats{}, err
		}
		item.Season = seasonStr
		if item.Season == "" && item.SeasonalSubID != 0 {
			item.Season = adventure.SubIDToSeason(item.SeasonalSubID)
		}
		if item.SeasonalSubID == 0 && item.Season != "" {
			item.SeasonalSubID = adventure.SeasonToSubID(item.Season)
		}
		stats.StageStats = append(stats.StageStats, item)
	}
	if err := rows.Err(); err != nil {
		return adventure.AggregatedStats{}, err
	}

	return stats, nil
}
