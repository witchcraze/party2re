package database

import (
	"context"
	"database/sql"
	"errors"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
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
	err := r.db.QueryRowContext(ctx, `
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

	rows, err := r.db.QueryContext(ctx, `
		SELECT id, definition_id, quantity
		FROM depot_items
		WHERE character_id = ?
		ORDER BY created_at ASC
	`, characterID)
	if err != nil {
		return depot.Depot{}, err
	}
	defer rows.Close()

	items := make([]item.Instance, 0)
	for rows.Next() {
		var instance item.Instance
		if err := rows.Scan(&instance.ID, &instance.DefinitionID, &instance.Quantity); err != nil {
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

	if err := saveDepotTx(ctx, tx, value); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true
	return nil
}

func saveDepotTx(ctx context.Context, tx *sql.Tx, value depot.Depot) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO character_depots (character_id, gold, capacity)
		VALUES (?, ?, ?)
		ON DUPLICATE KEY UPDATE
			gold = VALUES(gold),
			capacity = VALUES(capacity)
	`, value.CharacterID, value.Gold, value.Capacity)
	if err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, "DELETE FROM depot_items WHERE character_id = ?", value.CharacterID); err != nil {
		return err
	}

	for _, instance := range value.Items {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO depot_items (id, character_id, definition_id, quantity)
			VALUES (?, ?, ?, ?)
		`, instance.ID, value.CharacterID, instance.DefinitionID, instance.Quantity); err != nil {
			return err
		}
	}
	return nil
}

type sqlDepotTx struct {
	tx *sql.Tx
}

func (t *sqlDepotTx) GetCharacter(ctx context.Context, characterID string) (corecharacter.Character, error) {
	var value corecharacter.Character
	var gender, jobID string
	err := t.tx.QueryRowContext(ctx, `
		SELECT id, name, job_id, gender, max_hp, max_mp, hp, mp, attack, defense, agility, money, level, experience
		FROM characters
		WHERE id = ?
	`, characterID).Scan(
		&value.ID,
		&value.Name,
		&jobID,
		&gender,
		&value.Stats.MaxHP,
		&value.Stats.MaxMP,
		&value.Stats.HP,
		&value.Stats.MP,
		&value.Stats.Attack,
		&value.Stats.Defense,
		&value.Stats.Agility,
		&value.Money,
		&value.Level,
		&value.Experience,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return corecharacter.Character{}, corecharacter.ErrNotFound
	}
	if err != nil {
		return corecharacter.Character{}, err
	}
	value.JobID = jobID
	value.Gender = gender
	return value, nil
}

func (t *sqlDepotTx) SaveCharacter(ctx context.Context, character corecharacter.Character) error {
	return updateCharacterAtomically(ctx, t.tx, character)
}

func (t *sqlDepotTx) GetInventory(ctx context.Context, characterID string) (coreinventory.Inventory, error) {
	var count int
	err := t.tx.QueryRowContext(ctx, "SELECT COUNT(1) FROM characters WHERE id = ?", characterID).Scan(&count)
	if err != nil {
		return coreinventory.Inventory{}, err
	}
	if count == 0 {
		return coreinventory.Inventory{}, corecharacter.ErrNotFound
	}

	rows, err := t.tx.QueryContext(ctx, `
		SELECT id, definition_id, quantity
		FROM inventory_items
		WHERE character_id = ?
		ORDER BY created_at ASC
	`, characterID)
	if err != nil {
		return coreinventory.Inventory{}, err
	}
	defer rows.Close()

	items := make([]item.Instance, 0)
	for rows.Next() {
		var instance item.Instance
		if err := rows.Scan(&instance.ID, &instance.DefinitionID, &instance.Quantity); err != nil {
			return coreinventory.Inventory{}, err
		}
		items = append(items, instance)
	}
	if err := rows.Err(); err != nil {
		return coreinventory.Inventory{}, err
	}

	return coreinventory.Inventory{
		CharacterID: characterID,
		Items:       items,
	}, nil
}

func (t *sqlDepotTx) SaveInventory(ctx context.Context, inventory coreinventory.Inventory) error {
	if _, err := t.tx.ExecContext(ctx, "DELETE FROM inventory_items WHERE character_id = ?", inventory.CharacterID); err != nil {
		return err
	}
	for _, instance := range inventory.Items {
		if _, err := t.tx.ExecContext(ctx, `
			INSERT INTO inventory_items (id, character_id, definition_id, quantity)
			VALUES (?, ?, ?, ?)
		`, instance.ID, inventory.CharacterID, instance.DefinitionID, instance.Quantity); err != nil {
			return err
		}
	}
	return nil
}

func (t *sqlDepotTx) GetDepot(ctx context.Context, characterID string) (depot.Depot, error) {
	var dep depot.Depot
	err := t.tx.QueryRowContext(ctx, `
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

	rows, err := t.tx.QueryContext(ctx, `
		SELECT id, definition_id, quantity
		FROM depot_items
		WHERE character_id = ?
		ORDER BY created_at ASC
	`, characterID)
	if err != nil {
		return depot.Depot{}, err
	}
	defer rows.Close()

	items := make([]item.Instance, 0)
	for rows.Next() {
		var instance item.Instance
		if err := rows.Scan(&instance.ID, &instance.DefinitionID, &instance.Quantity); err != nil {
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

func (t *sqlDepotTx) SaveDepot(ctx context.Context, dep depot.Depot) error {
	return saveDepotTx(ctx, t.tx, dep)
}

func (r *DepotRepository) Execute(ctx context.Context, fn func(ctx context.Context, tx depot.Tx) error) error {
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

	wrapped := &sqlDepotTx{tx: tx}
	if err := fn(ctx, wrapped); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true
	return nil
}
