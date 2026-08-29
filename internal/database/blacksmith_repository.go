package database

import (
	"context"
	"database/sql"
	"errors"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
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

func (r *BlacksmithRepository) CommitEnhancement(ctx context.Context, character corecharacter.Character, inventory coreinventory.Inventory) error {
	return RunInTx(ctx, r.db, func(txCtx context.Context) error {
		executor := ExecutorFromContext(txCtx, r.db)

		if err := updateCharacterAtomically(txCtx, executor, character); err != nil {
			return err
		}

		if _, err := executor.ExecContext(txCtx, "DELETE FROM inventory_items WHERE character_id = ?", inventory.CharacterID); err != nil {
			return err
		}
		for _, instance := range inventory.Items {
			if _, err := executor.ExecContext(txCtx, `
				INSERT INTO inventory_items (id, character_id, definition_id, quantity, enhancement_level)
				VALUES (?, ?, ?, ?, ?)
			`, instance.ID, inventory.CharacterID, instance.DefinitionID, instance.Quantity, instance.EnhancementLevel); err != nil {
				return err
			}
		}

		return nil
	})
}
