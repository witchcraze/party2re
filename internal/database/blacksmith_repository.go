package database

import (
	"context"
	"database/sql"
	"errors"

	"github.com/witchcraze/party2re/internal/blacksmith"
)

type BlacksmithRepository struct {
	db *sql.DB
}

func NewBlacksmithRepository(db *sql.DB) (*BlacksmithRepository, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	return &BlacksmithRepository{db: db}, nil
}

func (r *BlacksmithRepository) ListByCharacterID(ctx context.Context, characterID string) ([]blacksmith.Deposit, error) {
	return r.listByCharacterIDWithQuery(ctx, characterID, `
		SELECT id, character_id, slot, item_definition_id, seal_id, custom_name, created_at
		FROM blacksmith_deposits
		WHERE character_id = ?
		ORDER BY slot ASC
	`)
}

func (r *BlacksmithRepository) ListByCharacterIDForUpdate(ctx context.Context, characterID string) ([]blacksmith.Deposit, error) {
	return r.listByCharacterIDWithQuery(ctx, characterID, `
		SELECT id, character_id, slot, item_definition_id, seal_id, custom_name, created_at
		FROM blacksmith_deposits
		WHERE character_id = ?
		ORDER BY slot ASC
		FOR UPDATE
	`)
}

func (r *BlacksmithRepository) listByCharacterIDWithQuery(ctx context.Context, characterID string, query string) ([]blacksmith.Deposit, error) {
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, query, characterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deposits []blacksmith.Deposit
	for rows.Next() {
		var dep blacksmith.Deposit
		if err := rows.Scan(
			&dep.ID,
			&dep.CharacterID,
			&dep.Slot,
			&dep.ItemDefinitionID,
			&dep.SealID,
			&dep.CustomName,
			&dep.CreatedAt,
		); err != nil {
			return nil, err
		}
		deposits = append(deposits, dep)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return deposits, nil
}

func (r *BlacksmithRepository) Save(ctx context.Context, deposit blacksmith.Deposit) error {
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		INSERT INTO blacksmith_deposits
			(id, character_id, slot, item_definition_id, seal_id, custom_name, created_at)
		VALUES
			(?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			item_definition_id = VALUES(item_definition_id),
			seal_id = VALUES(seal_id),
			custom_name = VALUES(custom_name),
			character_id = VALUES(character_id),
			slot = VALUES(slot)
	`, deposit.ID, deposit.CharacterID, deposit.Slot, deposit.ItemDefinitionID, deposit.SealID, deposit.CustomName, deposit.CreatedAt)
	return err
}

func (r *BlacksmithRepository) Delete(ctx context.Context, characterID string, slot int) error {
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		DELETE FROM blacksmith_deposits
		WHERE character_id = ? AND slot = ?
	`, characterID, slot)
	return err
}
