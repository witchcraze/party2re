package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	coreplayer "github.com/witchcraze/party2re/internal/core/player"
)

var ErrPlayerNotFound = coreplayer.ErrPlayerNotFound

const playerSelectColumns = `id, username, password_hash, banned_at, created_at, updated_at, last_ip`

func scanPlayer(scanner interface{ Scan(dest ...any) error }) (coreplayer.Player, error) {
	var value coreplayer.Player
	var bannedAt sql.NullTime
	err := scanner.Scan(
		&value.ID,
		&value.Username,
		&value.PasswordHash,
		&bannedAt,
		&value.CreatedAt,
		&value.UpdatedAt,
		&value.LastIP,
	)
	if err != nil {
		return coreplayer.Player{}, err
	}
	if bannedAt.Valid {
		t := bannedAt.Time.UTC()
		value.BannedAt = &t
	}
	return value, nil
}

type PlayerRepository struct{ db *sql.DB }

func NewPlayerRepository(db *sql.DB) (*PlayerRepository, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	return &PlayerRepository{db: db}, nil
}

func (r *PlayerRepository) Save(ctx context.Context, value coreplayer.Player) error {
	var bannedVal any
	if value.BannedAt != nil {
		bannedVal = value.BannedAt.UTC()
	}
	updatedAt := value.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = value.CreatedAt
	}
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx,
		`INSERT INTO players (id, username, password_hash, banned_at, created_at, updated_at, last_ip) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		value.ID, value.Username, value.PasswordHash, bannedVal, value.CreatedAt, updatedAt, value.LastIP)
	return err
}

func (r *PlayerRepository) FindByUsername(ctx context.Context, username string) (coreplayer.Player, error) {
	row := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx,
		fmt.Sprintf(`SELECT %s FROM players WHERE username = ?`, playerSelectColumns), username)
	value, err := scanPlayer(row)
	if errors.Is(err, sql.ErrNoRows) {
		return coreplayer.Player{}, ErrPlayerNotFound
	}
	return value, err
}

func (r *PlayerRepository) FindByID(ctx context.Context, id string) (coreplayer.Player, error) {
	row := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx,
		fmt.Sprintf(`SELECT %s FROM players WHERE id = ?`, playerSelectColumns), id)
	value, err := scanPlayer(row)
	if errors.Is(err, sql.ErrNoRows) {
		return coreplayer.Player{}, ErrPlayerNotFound
	}
	return value, err
}

func (r *PlayerRepository) List(ctx context.Context, sort string) ([]coreplayer.Player, error) {
	var orderBy string
	switch strings.ToLower(strings.TrimSpace(sort)) {
	case "name":
		orderBy = "username ASC"
	case "ldate", "updated_at":
		orderBy = "updated_at DESC, created_at DESC"
	case "addr":
		orderBy = "last_ip ASC, username ASC"
	default:
		orderBy = "last_ip ASC, username ASC" // legacy admin.cgi default is addr
	}

	query := fmt.Sprintf(`SELECT %s FROM players ORDER BY %s`, playerSelectColumns, orderBy)
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var players []coreplayer.Player
	for rows.Next() {
		p, err := scanPlayer(rows)
		if err != nil {
			return nil, err
		}
		players = append(players, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return players, nil
}

func (r *PlayerRepository) UpdateBannedAt(ctx context.Context, id string, bannedAt *time.Time) error {
	var bannedVal any
	if bannedAt != nil {
		bannedVal = bannedAt.UTC()
	}
	res, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx,
		`UPDATE players SET banned_at = ?, updated_at = UTC_TIMESTAMP(6) WHERE id = ?`,
		bannedVal, id)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrPlayerNotFound
	}
	return nil
}

func (r *PlayerRepository) UpdateLastLogin(ctx context.Context, id string, ip string, at time.Time) error {
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx,
		`UPDATE players SET last_ip = ?, updated_at = ? WHERE id = ?`,
		ip, at.UTC(), id)
	return err
}

func (r *PlayerRepository) Delete(ctx context.Context, id string) error {
	exec := ExecutorFromContext(ctx, r.db)

	queries := []string{
		`DELETE FROM player_notifications WHERE player_id = ?`,
		`DELETE FROM player_api_tokens WHERE player_id = ?`,
		`DELETE FROM players WHERE id = ?`,
	}

	for _, q := range queries {
		if strings.Count(q, "?") == 2 {
			if _, err := exec.ExecContext(ctx, q, id, id); err != nil {
				return err
			}
		} else {
			if _, err := exec.ExecContext(ctx, q, id); err != nil {
				return err
			}
		}
	}

	return nil
}
