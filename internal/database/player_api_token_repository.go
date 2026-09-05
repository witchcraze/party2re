package database

import (
	"context"
	"database/sql"
	"errors"
	"time"

	coreplayer "github.com/witchcraze/party2re/internal/core/player"
)

var (
	ErrAPITokenNotFound  = errors.New("api token not found")
	ErrAPITokenForbidden = errors.New("forbidden: api token belongs to another player")
)

type PlayerAPITokenRepository struct {
	db *sql.DB
}

func NewPlayerAPITokenRepository(db *sql.DB) (*PlayerAPITokenRepository, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	return &PlayerAPITokenRepository{db: db}, nil
}

func (r *PlayerAPITokenRepository) Save(ctx context.Context, token coreplayer.APIToken) error {
	exec := ExecutorFromContext(ctx, r.db)
	_, err := exec.ExecContext(ctx,
		`INSERT INTO player_api_tokens (id, player_id, token_hash, name, created_at, expires_at) VALUES (?, ?, ?, ?, ?, ?)`,
		token.ID, token.PlayerID, token.TokenHash, token.Name, token.CreatedAt, token.ExpiresAt,
	)
	return err
}

func (r *PlayerAPITokenRepository) FindByTokenHash(ctx context.Context, tokenHash string) (coreplayer.APIToken, error) {
	exec := ExecutorFromContext(ctx, r.db)
	var t coreplayer.APIToken
	var lastUsedAt sql.NullTime
	var expiresAt sql.NullTime

	err := exec.QueryRowContext(ctx,
		`SELECT id, player_id, token_hash, name, created_at, last_used_at, expires_at FROM player_api_tokens WHERE token_hash = ?`,
		tokenHash,
	).Scan(&t.ID, &t.PlayerID, &t.TokenHash, &t.Name, &t.CreatedAt, &lastUsedAt, &expiresAt)

	if errors.Is(err, sql.ErrNoRows) {
		return coreplayer.APIToken{}, ErrAPITokenNotFound
	}
	if err != nil {
		return coreplayer.APIToken{}, err
	}

	if lastUsedAt.Valid {
		t.LastUsedAt = &lastUsedAt.Time
	}
	if expiresAt.Valid {
		t.ExpiresAt = &expiresAt.Time
	}

	return t, nil
}

func (r *PlayerAPITokenRepository) FindByPlayerID(ctx context.Context, playerID string) ([]coreplayer.APIToken, error) {
	exec := ExecutorFromContext(ctx, r.db)
	rows, err := exec.QueryContext(ctx,
		`SELECT id, player_id, token_hash, name, created_at, last_used_at, expires_at FROM player_api_tokens WHERE player_id = ? ORDER BY created_at DESC`,
		playerID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tokens []coreplayer.APIToken
	for rows.Next() {
		var t coreplayer.APIToken
		var lastUsedAt sql.NullTime
		var expiresAt sql.NullTime

		if err := rows.Scan(&t.ID, &t.PlayerID, &t.TokenHash, &t.Name, &t.CreatedAt, &lastUsedAt, &expiresAt); err != nil {
			return nil, err
		}
		if lastUsedAt.Valid {
			t.LastUsedAt = &lastUsedAt.Time
		}
		if expiresAt.Valid {
			t.ExpiresAt = &expiresAt.Time
		}
		tokens = append(tokens, t)
	}
	return tokens, rows.Err()
}

func (r *PlayerAPITokenRepository) TouchLastUsed(ctx context.Context, id string, lastUsed time.Time) error {
	exec := ExecutorFromContext(ctx, r.db)
	_, err := exec.ExecContext(ctx, `UPDATE player_api_tokens SET last_used_at = ? WHERE id = ?`, lastUsed.UTC(), id)
	return err
}

func (r *PlayerAPITokenRepository) Revoke(ctx context.Context, playerID, tokenID string) error {
	exec := ExecutorFromContext(ctx, r.db)

	var ownerID string
	err := exec.QueryRowContext(ctx, `SELECT player_id FROM player_api_tokens WHERE id = ?`, tokenID).Scan(&ownerID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrAPITokenNotFound
	}
	if err != nil {
		return err
	}
	if ownerID != playerID {
		return ErrAPITokenForbidden
	}

	res, err := exec.ExecContext(ctx, `DELETE FROM player_api_tokens WHERE id = ? AND player_id = ?`, tokenID, playerID)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrAPITokenNotFound
	}
	return nil
}

func (r *PlayerAPITokenRepository) DeleteByPlayerID(ctx context.Context, playerID string) error {
	exec := ExecutorFromContext(ctx, r.db)
	_, err := exec.ExecContext(ctx, `DELETE FROM player_api_tokens WHERE player_id = ?`, playerID)
	return err
}
