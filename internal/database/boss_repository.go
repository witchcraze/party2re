package database

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/witchcraze/party2re/internal/boss"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
)

type BossRepository struct {
	db *sql.DB
}

func NewBossRepository(db *sql.DB) (*BossRepository, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	return &BossRepository{db: db}, nil
}

func (r *BossRepository) GetOrCreateRecord(ctx context.Context, characterID string) (boss.CharacterBossRecord, error) {
	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	var rec boss.CharacterBossRecord
	var firstClearedAt, lastChallengedAt sql.NullTime

	query := `
		SELECT character_id, highest_tier_cleared, total_boss_defeats,
		       first_cleared_at, last_challenged_at, daily_attempts_used, daily_attempts_reset_at,
		       created_at, updated_at
		FROM character_boss_records
		WHERE character_id = ?
	`
	err := r.db.QueryRowContext(ctx, query, characterID).Scan(
		&rec.CharacterID,
		&rec.HighestTierCleared,
		&rec.TotalBossDefeats,
		&firstClearedAt,
		&lastChallengedAt,
		&rec.DailyAttemptsUsed,
		&rec.DailyAttemptsResetAt,
		&rec.CreatedAt,
		&rec.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		insertQuery := `
			INSERT INTO character_boss_records (
				character_id, highest_tier_cleared, total_boss_defeats,
				daily_attempts_used, daily_attempts_reset_at, created_at, updated_at
			) VALUES (?, 0, 0, 0, ?, ?, ?)
		`
		_, insertErr := r.db.ExecContext(ctx, insertQuery, characterID, today, now, now)
		if insertErr != nil {
			return boss.CharacterBossRecord{}, insertErr
		}
		return boss.CharacterBossRecord{
			CharacterID:          characterID,
			HighestTierCleared:   0,
			TotalBossDefeats:     0,
			DailyAttemptsUsed:    0,
			DailyAttemptsResetAt: today,
			CreatedAt:            now,
			UpdatedAt:            now,
		}, nil
	}
	if err != nil {
		return boss.CharacterBossRecord{}, err
	}

	if firstClearedAt.Valid {
		rec.FirstClearedAt = &firstClearedAt.Time
	}
	if lastChallengedAt.Valid {
		rec.LastChallengedAt = &lastChallengedAt.Time
	}

	return rec, nil
}

func (r *BossRepository) RecordChallenge(
	ctx context.Context,
	history boss.BossChallengeHistory,
	record boss.CharacterBossRecord,
	character corecharacter.Character,
	rewardItem *coreitem.Instance,
) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	now := time.Now().UTC()

	// 1. Upsert character_boss_records
	var firstClearedAt, lastChallengedAt sql.NullTime
	if record.FirstClearedAt != nil {
		firstClearedAt = sql.NullTime{Time: *record.FirstClearedAt, Valid: true}
	}
	if record.LastChallengedAt != nil {
		lastChallengedAt = sql.NullTime{Time: *record.LastChallengedAt, Valid: true}
	}

	upsertRecordQuery := `
		INSERT INTO character_boss_records (
			character_id, highest_tier_cleared, total_boss_defeats,
			first_cleared_at, last_challenged_at, daily_attempts_used, daily_attempts_reset_at,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			highest_tier_cleared = VALUES(highest_tier_cleared),
			total_boss_defeats = VALUES(total_boss_defeats),
			first_cleared_at = VALUES(first_cleared_at),
			last_challenged_at = VALUES(last_challenged_at),
			daily_attempts_used = VALUES(daily_attempts_used),
			daily_attempts_reset_at = VALUES(daily_attempts_reset_at),
			updated_at = VALUES(updated_at)
	`
	_, err = tx.ExecContext(
		ctx,
		upsertRecordQuery,
		record.CharacterID,
		record.HighestTierCleared,
		record.TotalBossDefeats,
		firstClearedAt,
		lastChallengedAt,
		record.DailyAttemptsUsed,
		record.DailyAttemptsResetAt,
		now,
		now,
	)
	if err != nil {
		return err
	}

	// 2. Insert boss_challenge_history
	var rewardItemID sql.NullString
	if history.RewardItemID != "" {
		rewardItemID = sql.NullString{String: history.RewardItemID, Valid: true}
	}

	insertHistoryQuery := `
		INSERT INTO boss_challenge_history (
			id, character_id, boss_id, tier, outcome, turns,
			reward_exp, reward_gold, reward_item_id, is_first_clear, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err = tx.ExecContext(
		ctx,
		insertHistoryQuery,
		history.ID,
		history.CharacterID,
		history.BossID,
		history.Tier,
		string(history.Outcome),
		history.Turns,
		history.RewardExp,
		history.RewardGold,
		rewardItemID,
		history.IsFirstClear,
		history.CreatedAt,
	)
	if err != nil {
		return err
	}

	// 3. Update character progression/stats/money/medals
	updateCharQuery := `
		UPDATE characters
		SET level = ?, experience = ?, money = ?, small_medals = ?,
		    hp = ?, max_hp = ?,
		    attack = ?, defense = ?, agility = ?,
		    rebirth_count = ?
		WHERE id = ?
	`
	_, err = tx.ExecContext(
		ctx,
		updateCharQuery,
		character.Level,
		character.Experience,
		character.Money,
		character.SmallMedals,
		character.Stats.HP,
		character.Stats.MaxHP,
		character.Stats.Attack,
		character.Stats.Defense,
		character.Stats.Agility,
		character.RebirthCount,
		character.ID,
	)
	if err != nil {
		return err
	}

	// 4. Save item drop if present
	if rewardItem != nil {
		insertItemQuery := `
			INSERT INTO inventory_items (id, character_id, definition_id, quantity, enhancement_level)
			VALUES (?, ?, ?, ?, ?)
		`
		_, err = tx.ExecContext(
			ctx,
			insertItemQuery,
			rewardItem.ID,
			character.ID,
			rewardItem.DefinitionID,
			rewardItem.Quantity,
			rewardItem.EnhancementLevel,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (r *BossRepository) GetHistory(ctx context.Context, characterID string, limit int) ([]boss.BossChallengeHistory, error) {
	query := `
		SELECT id, character_id, boss_id, tier, outcome, turns,
		       reward_exp, reward_gold, reward_item_id, is_first_clear, created_at
		FROM boss_challenge_history
		WHERE character_id = ?
		ORDER BY created_at DESC
		LIMIT ?
	`
	rows, err := r.db.QueryContext(ctx, query, characterID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []boss.BossChallengeHistory
	for rows.Next() {
		var h boss.BossChallengeHistory
		var outcomeStr string
		var rewardItemID sql.NullString

		if err := rows.Scan(
			&h.ID,
			&h.CharacterID,
			&h.BossID,
			&h.Tier,
			&outcomeStr,
			&h.Turns,
			&h.RewardExp,
			&h.RewardGold,
			&rewardItemID,
			&h.IsFirstClear,
			&h.CreatedAt,
		); err != nil {
			return nil, err
		}

		h.Outcome = corebattle.Outcome(outcomeStr)
		if rewardItemID.Valid {
			h.RewardItemID = rewardItemID.String
		}
		list = append(list, h)
	}

	return list, rows.Err()
}

func (r *BossRepository) GetLeaderboard(ctx context.Context, limit int) ([]boss.BossLeaderboardEntry, error) {
	query := `
		SELECT c.id, c.name, c.level, c.job_id,
		       r.highest_tier_cleared, r.total_boss_defeats, r.first_cleared_at
		FROM character_boss_records r
		JOIN characters c ON r.character_id = c.id
		WHERE r.total_boss_defeats > 0
		ORDER BY r.highest_tier_cleared DESC, r.total_boss_defeats DESC, r.first_cleared_at ASC
		LIMIT ?
	`
	rows, err := r.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var leaderboard []boss.BossLeaderboardEntry
	for rows.Next() {
		var entry boss.BossLeaderboardEntry
		var firstClearedAt sql.NullTime

		if err := rows.Scan(
			&entry.CharacterID,
			&entry.CharacterName,
			&entry.CharacterLevel,
			&entry.JobID,
			&entry.HighestTierCleared,
			&entry.TotalBossDefeats,
			&firstClearedAt,
		); err != nil {
			return nil, err
		}

		if firstClearedAt.Valid {
			entry.FirstClearedAt = &firstClearedAt.Time
		}
		leaderboard = append(leaderboard, entry)
	}

	return leaderboard, rows.Err()
}
