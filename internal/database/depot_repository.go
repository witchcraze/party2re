package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

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
		SELECT character_id, ex_depot, capacity
		FROM character_depots
		WHERE character_id = ?
	`, characterID).Scan(&dep.CharacterID, &dep.ExDepot, &dep.Capacity)
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
		ORDER BY sort_position, id
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
		SELECT character_id, ex_depot, capacity
		FROM character_depots
		WHERE character_id = ? FOR UPDATE
	`, characterID).Scan(&dep.CharacterID, &dep.ExDepot, &dep.Capacity)
	if errors.Is(err, sql.ErrNoRows) {
		return depot.Depot{}, depot.ErrNotFound
	}
	if err != nil {
		return depot.Depot{}, err
	}

	// Full scans on sparse tables can lock another character's items.
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, `
		SELECT id, definition_id, quantity, enhancement_level
		FROM depot_items FORCE INDEX (idx_depot_items_character)
		WHERE character_id = ?
		ORDER BY sort_position, id FOR UPDATE
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
	seen := make(map[string]struct{}, len(value.Items))
	for _, instance := range value.Items {
		if _, ok := seen[instance.ID]; ok {
			return fmt.Errorf("duplicate item instance ID: %s", instance.ID)
		}
		seen[instance.ID] = struct{}{}
	}

	_, err := executor.ExecContext(ctx, `
		INSERT INTO character_depots (character_id, ex_depot, capacity)
		VALUES (?, ?, ?)
		ON DUPLICATE KEY UPDATE
			ex_depot = VALUES(ex_depot),
			capacity = VALUES(capacity)
	`, value.CharacterID, value.ExDepot, value.Capacity)
	if err != nil {
		return err
	}

	if len(value.Items) == 0 {
		_, err := executor.ExecContext(ctx, "DELETE depot_items FROM depot_items FORCE INDEX (idx_depot_items_character) WHERE character_id = ?", value.CharacterID)
		return err
	}

	currentIDs := make([]any, 0, len(value.Items)+1)
	currentIDs = append(currentIDs, value.CharacterID)
	placeholders := make([]string, len(value.Items))
	for idx, instance := range value.Items {
		currentIDs = append(currentIDs, instance.ID)
		placeholders[idx] = "?"
		if _, err := executor.ExecContext(ctx, `
			INSERT INTO depot_items (id, character_id, definition_id, quantity, enhancement_level, sort_position)
			VALUES (?, ?, ?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE
				character_id = IF(character_id = VALUES(character_id), character_id, NULL),
				definition_id = IF(character_id = VALUES(character_id), VALUES(definition_id), definition_id),
				quantity = IF(character_id = VALUES(character_id), VALUES(quantity), quantity),
				enhancement_level = IF(character_id = VALUES(character_id), VALUES(enhancement_level), enhancement_level),
				sort_position = IF(character_id = VALUES(character_id), VALUES(sort_position), sort_position)
		`, instance.ID, value.CharacterID, instance.DefinitionID, instance.Quantity, instance.EnhancementLevel, idx); err != nil {
			if strings.Contains(err.Error(), "character_id") && strings.Contains(err.Error(), "cannot be null") {
				return fmt.Errorf("item %s belongs to another character: %w", instance.ID, err)
			}
			return err
		}
	}

	query := fmt.Sprintf("DELETE depot_items FROM depot_items FORCE INDEX (idx_depot_items_character) WHERE character_id = ? AND id NOT IN (%s)", strings.Join(placeholders, ","))
	if _, err := executor.ExecContext(ctx, query, currentIDs...); err != nil {
		return err
	}
	return nil
}

func (r *DepotRepository) RunInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return RunInTx(ctx, r.db, fn)
}
