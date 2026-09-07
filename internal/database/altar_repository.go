package database

import (
	"context"
	"database/sql"
	"errors"

	"github.com/witchcraze/party2re/internal/altar"
)

type AltarRepository struct {
	db *sql.DB
}

func NewAltarRepository(db *sql.DB) (*AltarRepository, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	return &AltarRepository{db: db}, nil
}

func (r *AltarRepository) SaveAwakening(ctx context.Context, value altar.RamiaAwakening) error {
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		INSERT INTO altar_ramia_awakenings
			(id, character_id, character_name, awakened_at, expires_at)
		VALUES (?, ?, ?, ?, ?)
	`, value.ID, value.CharacterID, value.CharacterName, value.AwakenedAt, value.ExpiresAt)
	return err
}

func (r *AltarRepository) GetLatestAwakening(ctx context.Context) (*altar.RamiaAwakening, error) {
	var val altar.RamiaAwakening
	row := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT id, character_id, character_name, awakened_at, expires_at
		FROM altar_ramia_awakenings
		ORDER BY awakened_at DESC, id DESC
		LIMIT 1
	`)
	err := row.Scan(&val.ID, &val.CharacterID, &val.CharacterName, &val.AwakenedAt, &val.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &val, nil
}
