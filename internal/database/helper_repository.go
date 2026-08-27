package database

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/witchcraze/party2re/internal/helper"
)

type HelperRepository struct {
	db *sql.DB
}

func NewHelperRepository(db *sql.DB) (*HelperRepository, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	return &HelperRepository{db: db}, nil
}

func (r *HelperRepository) Save(ctx context.Context, q helper.Quest) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO helper_quests (
			id, title, kind, target_id, target_name, required_count,
			reward_item_id, is_rare, is_guild, expires_at, completed_at, completed_by, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			completed_at = VALUES(completed_at),
			completed_by = VALUES(completed_by)
	`, q.ID, q.Title, int(q.Kind), q.TargetID, q.TargetName, q.RequiredCount,
		q.RewardItemID, q.IsRare, q.IsGuild, q.ExpiresAt, q.CompletedAt, q.CompletedBy, q.CreatedAt)
	return err
}

func (r *HelperRepository) FindByID(ctx context.Context, id string) (helper.Quest, error) {
	var q helper.Quest
	var kindInt int
	var completedBy sql.NullString
	var completedAt sql.NullTime

	err := r.db.QueryRowContext(ctx, `
		SELECT id, title, kind, target_id, target_name, required_count,
		       reward_item_id, is_rare, is_guild, expires_at, completed_at, completed_by, created_at
		FROM helper_quests
		WHERE id = ?
	`, id).Scan(
		&q.ID, &q.Title, &kindInt, &q.TargetID, &q.TargetName, &q.RequiredCount,
		&q.RewardItemID, &q.IsRare, &q.IsGuild, &q.ExpiresAt, &completedAt, &completedBy, &q.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return helper.Quest{}, helper.ErrQuestNotFound
	}
	if err != nil {
		return helper.Quest{}, err
	}

	q.Kind = helper.QuestKind(kindInt)
	if completedAt.Valid {
		q.CompletedAt = &completedAt.Time
	}
	if completedBy.Valid {
		q.CompletedBy = completedBy.String
	}
	return q, nil
}

func (r *HelperRepository) ListActive(ctx context.Context, now time.Time) ([]helper.Quest, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, title, kind, target_id, target_name, required_count,
		       reward_item_id, is_rare, is_guild, expires_at, completed_at, completed_by, created_at
		FROM helper_quests
		WHERE completed_at IS NULL AND expires_at > ?
		ORDER BY created_at ASC
	`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []helper.Quest
	for rows.Next() {
		var q helper.Quest
		var kindInt int
		var completedBy sql.NullString
		var completedAt sql.NullTime

		if err := rows.Scan(
			&q.ID, &q.Title, &kindInt, &q.TargetID, &q.TargetName, &q.RequiredCount,
			&q.RewardItemID, &q.IsRare, &q.IsGuild, &q.ExpiresAt, &completedAt, &completedBy, &q.CreatedAt,
		); err != nil {
			return nil, err
		}

		q.Kind = helper.QuestKind(kindInt)
		if completedAt.Valid {
			q.CompletedAt = &completedAt.Time
		}
		if completedBy.Valid {
			q.CompletedBy = completedBy.String
		}
		list = append(list, q)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return list, nil
}
