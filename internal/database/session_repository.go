package database

import (
	"context"
	"database/sql"
	"errors"
	"time"

	coreplayer "github.com/witchcraze/party2re/internal/core/player"
)

type SessionRepository struct{ db *sql.DB }

func NewSessionRepository(db *sql.DB) (*SessionRepository, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	return &SessionRepository{db: db}, nil
}

func (r *SessionRepository) Save(ctx context.Context, value coreplayer.Session) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO player_sessions (id, player_id, created_at, expires_at, revoked_at) VALUES (?, ?, ?, ?, ?)`,
		value.ID, value.PlayerID, value.CreatedAt, value.ExpiresAt, value.RevokedAt)
	return err
}

func (r *SessionRepository) FindByID(ctx context.Context, id string) (coreplayer.Session, error) {
	var value coreplayer.Session
	var revoked sql.NullTime
	err := r.db.QueryRowContext(ctx, `SELECT id, player_id, created_at, expires_at, revoked_at FROM player_sessions WHERE id = ?`, id).
		Scan(&value.ID, &value.PlayerID, &value.CreatedAt, &value.ExpiresAt, &revoked)
	if errors.Is(err, sql.ErrNoRows) {
		return coreplayer.Session{}, coreplayer.ErrInvalidSession
	}
	if err != nil {
		return coreplayer.Session{}, err
	}
	if revoked.Valid {
		value.RevokedAt = &revoked.Time
	}
	return value, nil
}

func (r *SessionRepository) Revoke(ctx context.Context, id string, now time.Time) error {
	result, err := r.db.ExecContext(ctx, `UPDATE player_sessions SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL`, now.UTC(), id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return coreplayer.ErrInvalidSession
	}
	return nil
}
