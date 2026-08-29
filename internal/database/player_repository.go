package database

import (
	"context"
	"database/sql"
	"errors"

	coreplayer "github.com/witchcraze/party2re/internal/core/player"
)

var ErrPlayerNotFound = errors.New("player not found")

type PlayerRepository struct{ db *sql.DB }

func NewPlayerRepository(db *sql.DB) (*PlayerRepository, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	return &PlayerRepository{db: db}, nil
}

func (r *PlayerRepository) Save(ctx context.Context, value coreplayer.Player) error {
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `INSERT INTO players (id, username, password_hash, created_at) VALUES (?, ?, ?, ?)`,
		value.ID, value.Username, value.PasswordHash, value.CreatedAt)
	return err
}

func (r *PlayerRepository) FindByUsername(ctx context.Context, username string) (coreplayer.Player, error) {
	var value coreplayer.Player
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `SELECT id, username, password_hash, created_at FROM players WHERE username = ?`, username).
		Scan(&value.ID, &value.Username, &value.PasswordHash, &value.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return coreplayer.Player{}, ErrPlayerNotFound
	}
	return value, err
}

func (r *PlayerRepository) FindByID(ctx context.Context, id string) (coreplayer.Player, error) {
	var value coreplayer.Player
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `SELECT id, username, password_hash, created_at FROM players WHERE id = ?`, id).
		Scan(&value.ID, &value.Username, &value.PasswordHash, &value.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return coreplayer.Player{}, ErrPlayerNotFound
	}
	return value, err
}
