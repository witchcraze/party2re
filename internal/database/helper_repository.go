package database

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/witchcraze/party2re/internal/helperquest"
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

func (r *HelperRepository) Save(ctx context.Context, q helperquest.Quest) error {
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
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

func (r *HelperRepository) FindByID(ctx context.Context, id string) (helperquest.Quest, error) {
	return r.findByID(ctx, id, false)
}

// FindByIDForUpdate locks the shared quest before character assets are acquired.
// The caller must hold the ambient transaction until completion or rollback.
func (r *HelperRepository) FindByIDForUpdate(ctx context.Context, id string) (helperquest.Quest, error) {
	return r.findByID(ctx, id, true)
}

func (r *HelperRepository) findByID(ctx context.Context, id string, forUpdate bool) (helperquest.Quest, error) {
	var q helperquest.Quest
	var kindInt int
	var completedBy sql.NullString
	var completedAt sql.NullTime

	query := `
		SELECT id, title, kind, target_id, target_name, required_count,
		       reward_item_id, is_rare, is_guild, expires_at, completed_at, completed_by, created_at
		FROM helper_quests
		WHERE id = ?
	`
	if forUpdate {
		query += " FOR UPDATE"
	}
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, query, id).Scan(
		&q.ID, &q.Title, &kindInt, &q.TargetID, &q.TargetName, &q.RequiredCount,
		&q.RewardItemID, &q.IsRare, &q.IsGuild, &q.ExpiresAt, &completedAt, &completedBy, &q.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return helperquest.Quest{}, helperquest.ErrQuestNotFound
	}
	if err != nil {
		return helperquest.Quest{}, err
	}

	q.Kind = helperquest.QuestKind(kindInt)
	if completedAt.Valid {
		q.CompletedAt = &completedAt.Time
	}
	if completedBy.Valid {
		q.CompletedBy = completedBy.String
	}
	return q, nil
}

func (r *HelperRepository) ListActive(ctx context.Context, now time.Time) ([]helperquest.Quest, error) {
	return r.list(ctx, "expires_at > ?", now)
}

func (r *HelperRepository) ListUncompleted(ctx context.Context) ([]helperquest.Quest, error) {
	return r.list(ctx, "TRUE")
}

// ListExpired reads uncompleted requests past their deadline (same boundary as
// helperquest.CompleteQuest). It takes no locks: a range FOR UPDATE would hold gap locks that
// deadlock with concurrent replacement inserts, so callers lock each row by id instead.
func (r *HelperRepository) ListExpired(ctx context.Context, now time.Time) ([]helperquest.Quest, error) {
	return r.list(ctx, "expires_at < ?", now)
}

// Delete removes an uncompleted request so that it can be exchanged for a replacement.
func (r *HelperRepository) Delete(ctx context.Context, id string) error {
	res, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `DELETE FROM helper_quests WHERE id = ? AND completed_at IS NULL`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return helperquest.ErrQuestNotFound
	}
	return nil
}

// list reads uncompleted requests matching a fixed predicate (never user input).
func (r *HelperRepository) list(ctx context.Context, predicate string, args ...any) ([]helperquest.Quest, error) {
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, `
		SELECT id, title, kind, target_id, target_name, required_count,
		       reward_item_id, is_rare, is_guild, expires_at, completed_at, completed_by, created_at
		FROM helper_quests
		WHERE completed_at IS NULL AND `+predicate+`
		ORDER BY created_at ASC, id ASC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []helperquest.Quest
	for rows.Next() {
		var q helperquest.Quest
		var kindInt int
		var completedBy sql.NullString
		var completedAt sql.NullTime

		if err := rows.Scan(
			&q.ID, &q.Title, &kindInt, &q.TargetID, &q.TargetName, &q.RequiredCount,
			&q.RewardItemID, &q.IsRare, &q.IsGuild, &q.ExpiresAt, &completedAt, &completedBy, &q.CreatedAt,
		); err != nil {
			return nil, err
		}

		q.Kind = helperquest.QuestKind(kindInt)
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
