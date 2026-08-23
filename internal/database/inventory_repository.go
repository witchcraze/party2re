package database

import (
	"context"
	"database/sql"
	"errors"

	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
)

type InventoryRepository struct {
	db *sql.DB
}

func NewInventoryRepository(db *sql.DB) (*InventoryRepository, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	return &InventoryRepository{db: db}, nil
}

func (r *InventoryRepository) Save(ctx context.Context, value coreinventory.Inventory) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM inventory_items WHERE character_id = ?", value.CharacterID); err != nil {
		_ = tx.Rollback()
		return err
	}
	for _, instance := range value.Items {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO inventory_items (id, character_id, definition_id, quantity, enhancement_level)
			VALUES (?, ?, ?, ?, ?)
		`, instance.ID, value.CharacterID, instance.DefinitionID, instance.Quantity, instance.EnhancementLevel); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func (r *InventoryRepository) FindByCharacterID(ctx context.Context, characterID string) (coreinventory.Inventory, error) {
	value, err := coreinventory.New(characterID)
	if err != nil {
		return coreinventory.Inventory{}, err
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, definition_id, quantity, enhancement_level
		FROM inventory_items
		WHERE character_id = ?
		ORDER BY id
	`, characterID)
	if err != nil {
		return coreinventory.Inventory{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var instance item.Instance
		if err := rows.Scan(&instance.ID, &instance.DefinitionID, &instance.Quantity, &instance.EnhancementLevel); err != nil {
			return coreinventory.Inventory{}, err
		}
		if err := value.Add(instance); err != nil {
			return coreinventory.Inventory{}, err
		}
	}
	if err := rows.Err(); err != nil {
		return coreinventory.Inventory{}, err
	}
	return value, nil
}
