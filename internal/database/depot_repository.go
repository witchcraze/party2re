package database

import (
	"context"
	"database/sql"
	"errors"

	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
)

type DepotRepository struct {
	db *sql.DB
}

func NewDepotRepository(db *sql.DB) (*DepotRepository, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	return &DepotRepository{db: db}, nil
}

func (r *DepotRepository) FindByCharacterID(ctx context.Context, characterID string) (depot.Depot, error) {
	var dep depot.Depot
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT character_id, gold, capacity
		FROM character_depots
		WHERE character_id = ?
	`, characterID).Scan(&dep.CharacterID, &dep.Gold, &dep.Capacity)
	if errors.Is(err, sql.ErrNoRows) {
		return depot.Depot{}, depot.ErrNotFound
	}
	if err != nil {
		return depot.Depot{}, err
	}

	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, `
		SELECT id, definition_id, quantity, enhancement_level
		FROM depot_items
		WHERE character_id = ?
		ORDER BY id
	`, characterID)
	if err != nil {
		return depot.Depot{}, err
	}
	defer rows.Close()

	items := make([]item.Instance, 0)
	for rows.Next() {
		var instance item.Instance
		if err := rows.Scan(&instance.ID, &instance.DefinitionID, &instance.Quantity, &instance.EnhancementLevel); err != nil {
			return depot.Depot{}, err
		}
		items = append(items, instance)
	}
	if err := rows.Err(); err != nil {
		return depot.Depot{}, err
	}

	dep.Items = items
	return dep, nil
}

func (r *DepotRepository) FindByCharacterIDForUpdate(ctx context.Context, characterID string) (depot.Depot, error) {
	var dep depot.Depot
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT character_id, gold, capacity
		FROM character_depots
		WHERE character_id = ? FOR UPDATE
	`, characterID).Scan(&dep.CharacterID, &dep.Gold, &dep.Capacity)
	if errors.Is(err, sql.ErrNoRows) {
		return depot.Depot{}, depot.ErrNotFound
	}
	if err != nil {
		return depot.Depot{}, err
	}

	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, `
		SELECT id, definition_id, quantity, enhancement_level
		FROM depot_items
		WHERE character_id = ?
		ORDER BY id FOR UPDATE
	`, characterID)
	if err != nil {
		return depot.Depot{}, err
	}
	defer rows.Close()

	items := make([]item.Instance, 0)
	for rows.Next() {
		var instance item.Instance
		if err := rows.Scan(&instance.ID, &instance.DefinitionID, &instance.Quantity, &instance.EnhancementLevel); err != nil {
			return depot.Depot{}, err
		}
		items = append(items, instance)
	}
	if err := rows.Err(); err != nil {
		return depot.Depot{}, err
	}

	dep.Items = items
	return dep, nil
}

func (r *DepotRepository) Save(ctx context.Context, value depot.Depot) error {
	return RunInTx(ctx, r.db, func(txCtx context.Context) error {
		return saveDepotTx(txCtx, ExecutorFromContext(txCtx, r.db), value)
	})
}

func saveDepotTx(ctx context.Context, executor sqlContextExecutor, value depot.Depot) error {
	_, err := executor.ExecContext(ctx, `
		INSERT INTO character_depots (character_id, gold, capacity)
		VALUES (?, ?, ?)
		ON DUPLICATE KEY UPDATE
			gold = VALUES(gold),
			capacity = VALUES(capacity)
	`, value.CharacterID, value.Gold, value.Capacity)
	if err != nil {
		return err
	}

	if _, err := executor.ExecContext(ctx, "DELETE FROM depot_items WHERE character_id = ?", value.CharacterID); err != nil {
		return err
	}

	for _, instance := range value.Items {
		if _, err := executor.ExecContext(ctx, `
			INSERT INTO depot_items (id, character_id, definition_id, quantity, enhancement_level)
			VALUES (?, ?, ?, ?, ?)
		`, instance.ID, value.CharacterID, instance.DefinitionID, instance.Quantity, instance.EnhancementLevel); err != nil {
			return err
		}
	}
	return nil
}

func (r *DepotRepository) RunInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return RunInTx(ctx, r.db, fn)
}
