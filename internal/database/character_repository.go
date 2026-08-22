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

type sqlContextExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func NewCharacterRepository(db *sql.DB) (*CharacterRepository, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	return &CharacterRepository{db: db}, nil
}

func (r *CharacterRepository) Save(ctx context.Context, value corecharacter.Character) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO characters
			(id, name, job_id, gender, max_hp, max_mp, hp, mp, attack, defense, agility, money, level, experience)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, value.ID, value.Name, value.JobID, value.Gender, value.Stats.MaxHP, value.Stats.MaxMP,
		value.Stats.HP, value.Stats.MP, value.Stats.Attack, value.Stats.Defense, value.Stats.Agility,
		value.Money, value.Level, value.Experience)
	return err
}

func (r *CharacterRepository) FindByID(ctx context.Context, id string) (corecharacter.Character, error) {
	var value corecharacter.Character
	err := r.db.QueryRowContext(ctx, `
		SELECT id, name, job_id, gender, max_hp, max_mp, hp, mp, attack, defense, agility, money, level, experience
		FROM characters
		WHERE id = ?
	`, id).Scan(&value.ID, &value.Name, &value.JobID, &value.Gender, &value.Stats.MaxHP, &value.Stats.MaxMP,
		&value.Stats.HP, &value.Stats.MP, &value.Stats.Attack, &value.Stats.Defense, &value.Stats.Agility,
		&value.Money, &value.Level, &value.Experience)
	if errors.Is(err, sql.ErrNoRows) {
		return corecharacter.Character{}, corecharacter.ErrNotFound
	}
	if err != nil {
		return corecharacter.Character{}, err
	}
	return value, nil
}

func (r *CharacterRepository) Update(ctx context.Context, value corecharacter.Character) error {
	return updateCharacter(ctx, r.db, value)
}

func updateCharacter(ctx context.Context, executor sqlContextExecutor, value corecharacter.Character) error {
	affected, err := executeCharacterUpdate(ctx, executor, value)
	if err != nil {
		return err
	}
	if affected == 0 {
		return corecharacter.ErrNotFound
	}
	return nil
}

func updateCharacterAtomically(ctx context.Context, executor sqlContextExecutor, value corecharacter.Character) error {
	affected, err := executeCharacterUpdate(ctx, executor, value)
	if err != nil {
		return err
	}
	if affected != 0 {
		return nil
	}

	var id string
	if err := executor.QueryRowContext(ctx, "SELECT id FROM characters WHERE id = ?", value.ID).Scan(&id); errors.Is(err, sql.ErrNoRows) {
		return corecharacter.ErrNotFound
	} else if err != nil {
		return err
	}
	return nil
}

func executeCharacterUpdate(ctx context.Context, executor sqlContextExecutor, value corecharacter.Character) (int64, error) {
	result, err := executor.ExecContext(ctx, `
		UPDATE characters
		SET name = ?, job_id = ?, gender = ?, max_hp = ?, max_mp = ?, hp = ?, mp = ?,
			attack = ?, defense = ?, agility = ?, money = ?, level = ?, experience = ?
		WHERE id = ?
	`, value.Name, value.JobID, value.Gender, value.Stats.MaxHP, value.Stats.MaxMP, value.Stats.HP,
		value.Stats.MP, value.Stats.Attack, value.Stats.Defense, value.Stats.Agility, value.Money,
		value.Level, value.Experience, value.ID)
	if err != nil {
		return 0, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	return affected, nil
}
