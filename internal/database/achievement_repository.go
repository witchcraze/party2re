package database

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/witchcraze/party2re/internal/medal"
)

type AchievementRepository struct {
	db *sql.DB
}

func NewAchievementRepository(db *sql.DB) (*AchievementRepository, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	return &AchievementRepository{db: db}, nil
}

func (r *AchievementRepository) RecordProgress(
	ctx context.Context,
	characterID string,
	metric medal.MetricType,
	amount int,
	matchingAchievements []medal.Achievement,
) error {
	if len(matchingAchievements) == 0 || amount <= 0 {
		return nil
	}

	return RunInTx(ctx, r.db, func(txCtx context.Context) error {
		exec := ExecutorFromContext(txCtx, r.db)
		now := time.Now().UTC()

		for _, ach := range matchingAchievements {
			isCompletedInit := amount >= ach.Threshold
			var completedAtInit *time.Time
			if isCompletedInit {
				completedAtInit = &now
			}

			query := `
				INSERT INTO character_achievements (
					character_id, achievement_id, current_progress, is_completed, completed_at
				) VALUES (
					?, ?, ?, ?, ?
				) ON DUPLICATE KEY UPDATE
					current_progress = current_progress + ?,
					completed_at = IF(is_completed = FALSE AND (current_progress + ? >= ?), UTC_TIMESTAMP(), completed_at),
					is_completed = IF(current_progress + ? >= ?, TRUE, is_completed)
			`
			if _, err := exec.ExecContext(
				txCtx,
				query,
				characterID,
				ach.ID,
				amount,
				isCompletedInit,
				completedAtInit,
				amount,
				0, // current_progress was already incremented by amount in MySQL row update
				ach.Threshold,
				0,
				ach.Threshold,
			); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *AchievementRepository) GetCharacterAchievements(ctx context.Context, characterID string) ([]medal.AchievementRecord, error) {
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, `
		SELECT character_id, achievement_id, current_progress, is_completed, completed_at, is_claimed, claimed_at
		FROM character_achievements
		WHERE character_id = ?
		ORDER BY achievement_id ASC
	`, characterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []medal.AchievementRecord
	for rows.Next() {
		var rec medal.AchievementRecord
		var compAt, claimAt sql.NullTime
		if err := rows.Scan(
			&rec.CharacterID,
			&rec.AchievementID,
			&rec.CurrentProgress,
			&rec.IsCompleted,
			&compAt,
			&rec.IsClaimed,
			&claimAt,
		); err != nil {
			return nil, err
		}
		if compAt.Valid {
			t := compAt.Time.UTC()
			rec.CompletedAt = &t
		}
		if claimAt.Valid {
			t := claimAt.Time.UTC()
			rec.ClaimedAt = &t
		}
		records = append(records, rec)
	}
	return records, rows.Err()
}

func (r *AchievementRepository) GetAchievementForUpdate(ctx context.Context, characterID string, achievementID string) (medal.AchievementRecord, error) {
	var rec medal.AchievementRecord
	var compAt, claimAt sql.NullTime

	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT character_id, achievement_id, current_progress, is_completed, completed_at, is_claimed, claimed_at
		FROM character_achievements
		WHERE character_id = ? AND achievement_id = ?
		FOR UPDATE
	`, characterID, achievementID).Scan(
		&rec.CharacterID,
		&rec.AchievementID,
		&rec.CurrentProgress,
		&rec.IsCompleted,
		&compAt,
		&rec.IsClaimed,
		&claimAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return medal.AchievementRecord{
			CharacterID:   characterID,
			AchievementID: achievementID,
		}, nil
	}
	if err != nil {
		return medal.AchievementRecord{}, err
	}
	if compAt.Valid {
		t := compAt.Time.UTC()
		rec.CompletedAt = &t
	}
	if claimAt.Valid {
		t := claimAt.Time.UTC()
		rec.ClaimedAt = &t
	}
	return rec, nil
}

func (r *AchievementRepository) MarkAchievementClaimed(ctx context.Context, characterID string, achievementID string, claimedAt time.Time) error {
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		UPDATE character_achievements
		SET is_claimed = TRUE, claimed_at = ?
		WHERE character_id = ? AND achievement_id = ?
	`, claimedAt.UTC(), characterID, achievementID)
	return err
}

func (r *AchievementRepository) SaveMedal(ctx context.Context, m medal.CharacterMedal) error {
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		INSERT INTO character_medals (
			character_id, medal_id, medal_name, category, description, awarded_at
		) VALUES (
			?, ?, ?, ?, ?, ?
		) ON DUPLICATE KEY UPDATE
			medal_name = VALUES(medal_name),
			category = VALUES(category),
			description = VALUES(description),
			awarded_at = VALUES(awarded_at)
	`, m.CharacterID, m.MedalID, m.MedalName, m.Category, m.Description, m.AwardedAt.UTC())
	return err
}

func (r *AchievementRepository) GetCharacterMedals(ctx context.Context, characterID string) ([]medal.CharacterMedal, error) {
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, `
		SELECT character_id, medal_id, medal_name, category, description, awarded_at
		FROM character_medals
		WHERE character_id = ?
		ORDER BY awarded_at DESC
	`, characterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var medals []medal.CharacterMedal
	for rows.Next() {
		var m medal.CharacterMedal
		var awardedAt time.Time
		if err := rows.Scan(
			&m.CharacterID,
			&m.MedalID,
			&m.MedalName,
			&m.Category,
			&m.Description,
			&awardedAt,
		); err != nil {
			return nil, err
		}
		m.AwardedAt = awardedAt.UTC()
		medals = append(medals, m)
	}
	return medals, rows.Err()
}
