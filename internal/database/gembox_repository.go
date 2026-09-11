package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/gemstore"
)

type GemBoxRepository struct {
	db *sql.DB
}

func NewGemBoxRepository(db *sql.DB) (*GemBoxRepository, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	return &GemBoxRepository{db: db}, nil
}

func (r *GemBoxRepository) FindByCharacterID(ctx context.Context, characterID string) (gemstore.GemBox, error) {
	var box gemstore.GemBox
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT character_id, capacity
		FROM character_gem_boxes
		WHERE character_id = ?
	`, characterID).Scan(&box.CharacterID, &box.Capacity)
	if errors.Is(err, sql.ErrNoRows) {
		return gemstore.GemBox{}, gemstore.ErrGemBoxNotFound
	}
	if err != nil {
		return gemstore.GemBox{}, err
	}

	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, `
		SELECT id, definition_id
		FROM gem_box_items
		WHERE character_id = ?
		ORDER BY slot_index, id
	`, characterID)
	if err != nil {
		return gemstore.GemBox{}, err
	}
	defer rows.Close()

	items := make([]coreitem.Instance, 0)
	for rows.Next() {
		var inst coreitem.Instance
		if err := rows.Scan(&inst.ID, &inst.DefinitionID); err != nil {
			return gemstore.GemBox{}, err
		}
		inst.Quantity = 1
		items = append(items, inst)
	}
	if err := rows.Err(); err != nil {
		return gemstore.GemBox{}, err
	}

	box.Items = items
	return box, nil
}

func (r *GemBoxRepository) FindByCharacterIDForUpdate(ctx context.Context, characterID string) (gemstore.GemBox, error) {
	var box gemstore.GemBox
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT character_id, capacity
		FROM character_gem_boxes
		WHERE character_id = ? FOR UPDATE
	`, characterID).Scan(&box.CharacterID, &box.Capacity)
	if errors.Is(err, sql.ErrNoRows) {
		return gemstore.GemBox{}, gemstore.ErrGemBoxNotFound
	}
	if err != nil {
		return gemstore.GemBox{}, err
	}

	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, `
		SELECT id, definition_id
		FROM gem_box_items
		WHERE character_id = ?
		ORDER BY slot_index, id FOR UPDATE
	`, characterID)
	if err != nil {
		return gemstore.GemBox{}, err
	}
	defer rows.Close()

	items := make([]coreitem.Instance, 0)
	for rows.Next() {
		var inst coreitem.Instance
		if err := rows.Scan(&inst.ID, &inst.DefinitionID); err != nil {
			return gemstore.GemBox{}, err
		}
		inst.Quantity = 1
		items = append(items, inst)
	}
	if err := rows.Err(); err != nil {
		return gemstore.GemBox{}, err
	}

	box.Items = items
	return box, nil
}

func (r *GemBoxRepository) Save(ctx context.Context, value gemstore.GemBox) error {
	return RunInTx(ctx, r.db, func(txCtx context.Context) error {
		return saveGemBoxTx(txCtx, ExecutorFromContext(txCtx, r.db), value)
	})
}

func saveGemBoxTx(ctx context.Context, executor sqlContextExecutor, value gemstore.GemBox) error {
	_, err := executor.ExecContext(ctx, `
		INSERT INTO character_gem_boxes (character_id, capacity)
		VALUES (?, ?)
		ON DUPLICATE KEY UPDATE capacity = VALUES(capacity)
	`, value.CharacterID, value.Capacity)
	if err != nil {
		return err
	}

	if len(value.Items) == 0 {
		_, err = executor.ExecContext(ctx, "DELETE FROM gem_box_items WHERE character_id = ?", value.CharacterID)
		return err
	}

	ids := make([]any, len(value.Items))
	for i, item := range value.Items {
		ids[i] = item.ID
	}
	placeholders := strings.Repeat("?,", len(ids))
	placeholders = placeholders[:len(placeholders)-1]
	args := append([]any{value.CharacterID}, ids...)
	if _, err := executor.ExecContext(ctx, fmt.Sprintf("DELETE FROM gem_box_items WHERE character_id = ? AND id NOT IN (%s)", placeholders), args...); err != nil {
		return err
	}

	for idx, inst := range value.Items {
		if _, err := executor.ExecContext(ctx, `
			INSERT INTO gem_box_items (id, character_id, definition_id, slot_index)
			VALUES (?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE slot_index = VALUES(slot_index)
		`, inst.ID, value.CharacterID, inst.DefinitionID, idx); err != nil {
			return err
		}
	}
	return nil
}

func (r *GemBoxRepository) RunInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return RunInTx(ctx, r.db, fn)
}
