package database

import (
	"context"
	"database/sql"
	"errors"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
)

type ShopRepository struct {
	db *sql.DB
}

func NewShopRepository(db *sql.DB) (*ShopRepository, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	return &ShopRepository{db: db}, nil
}

func (r *ShopRepository) CommitTransaction(ctx context.Context, character corecharacter.Character, inventory coreinventory.Inventory) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if err := updateCharacterAtomically(ctx, tx, character); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, "DELETE FROM inventory_items WHERE character_id = ?", inventory.CharacterID); err != nil {
		return err
	}
	for _, instance := range inventory.Items {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO inventory_items (id, character_id, definition_id, quantity)
			VALUES (?, ?, ?, ?)
		`, instance.ID, inventory.CharacterID, instance.DefinitionID, instance.Quantity); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true
	return nil
}
