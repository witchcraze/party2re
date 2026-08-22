package database

import (
	"context"
	"database/sql"
	"errors"

	"github.com/witchcraze/party2re/internal/core/item"
)

var ErrItemDefinitionNotFound = errors.New("item definition not found")

type ItemDefinitionRepository struct {
	db *sql.DB
}

func NewItemDefinitionRepository(db *sql.DB) (*ItemDefinitionRepository, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	return &ItemDefinitionRepository{db: db}, nil
}

func (r *ItemDefinitionRepository) Save(ctx context.Context, value item.Definition) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO item_definitions (id, name)
		VALUES (?, ?)
		ON DUPLICATE KEY UPDATE name = VALUES(name)
	`, value.ID, value.Name)
	return err
}

func (r *ItemDefinitionRepository) FindByID(ctx context.Context, id string) (item.Definition, error) {
	var value item.Definition
	err := r.db.QueryRowContext(ctx, `
		SELECT id, name
		FROM item_definitions
		WHERE id = ?
	`, id).Scan(&value.ID, &value.Name)
	if errors.Is(err, sql.ErrNoRows) {
		return item.Definition{}, ErrItemDefinitionNotFound
	}
	if err != nil {
		return item.Definition{}, err
	}
	return value, nil
}
