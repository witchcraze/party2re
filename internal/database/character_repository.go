package database

import (
	"context"
	"database/sql"
	"errors"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

type CharacterRepository struct {
	db *sql.DB
}

func NewCharacterRepository(db *sql.DB) (*CharacterRepository, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	return &CharacterRepository{db: db}, nil
}

func (r *CharacterRepository) Save(ctx context.Context, value corecharacter.Character) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO characters (id, name, level, experience)
		VALUES (?, ?, ?, ?)
	`, value.ID, value.Name, value.Level, value.Experience)
	return err
}

func (r *CharacterRepository) FindByID(ctx context.Context, id string) (corecharacter.Character, error) {
	var value corecharacter.Character
	err := r.db.QueryRowContext(ctx, `
		SELECT id, name, level, experience
		FROM characters
		WHERE id = ?
	`, id).Scan(&value.ID, &value.Name, &value.Level, &value.Experience)
	if errors.Is(err, sql.ErrNoRows) {
		return corecharacter.Character{}, corecharacter.ErrNotFound
	}
	if err != nil {
		return corecharacter.Character{}, err
	}
	return value, nil
}

func (r *CharacterRepository) Update(ctx context.Context, value corecharacter.Character) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE characters
		SET name = ?, level = ?, experience = ?
		WHERE id = ?
	`, value.Name, value.Level, value.Experience, value.ID)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err != nil {
		return err
	} else if affected == 0 {
		return corecharacter.ErrNotFound
	}
	return nil
}
