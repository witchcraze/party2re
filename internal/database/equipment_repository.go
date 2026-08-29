package database

import (
	"context"
	"database/sql"
	"errors"

	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
	"github.com/witchcraze/party2re/internal/core/item"
)

var ErrEquipmentNotFound = errors.New("equipment not found")

type EquipmentRepository struct{ db *sql.DB }

func NewEquipmentRepository(db *sql.DB) (*EquipmentRepository, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	return &EquipmentRepository{db: db}, nil
}

func (r *EquipmentRepository) Save(ctx context.Context, value coreequipment.Equipment) error {
	return RunInTx(ctx, r.db, func(txCtx context.Context) error {
		executor := ExecutorFromContext(txCtx, r.db)
		if _, err := executor.ExecContext(txCtx, "DELETE FROM equipment_slots WHERE character_id = ?", value.CharacterID); err != nil {
			return err
		}
		for slot, instanceID := range value.Slots {
			if _, err := executor.ExecContext(txCtx, `
				INSERT INTO equipment_slots (character_id, slot, instance_id)
				VALUES (?, ?, ?)
			`, value.CharacterID, string(slot), instanceID); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *EquipmentRepository) FindByCharacterID(ctx context.Context, characterID string) (coreequipment.Equipment, error) {
	value, err := coreequipment.New(characterID)
	if err != nil {
		return coreequipment.Equipment{}, err
	}
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, `
		SELECT slot, instance_id
		FROM equipment_slots
		WHERE character_id = ?
	`, characterID)
	if err != nil {
		return coreequipment.Equipment{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var slot, instanceID string
		if err := rows.Scan(&slot, &instanceID); err != nil {
			return coreequipment.Equipment{}, err
		}
		value.Slots[item.Slot(slot)] = instanceID
	}
	if err := rows.Err(); err != nil {
		return coreequipment.Equipment{}, err
	}
	return value, nil
}
