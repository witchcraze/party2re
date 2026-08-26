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
			(id, player_id, name, job_id, gender, max_hp, max_mp, hp, mp, attack, defense, agility, money, level, experience, rebirth_count, small_medals)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, value.ID, value.PlayerID, value.Name, value.JobID, value.Gender, value.Stats.MaxHP, value.Stats.MaxMP,
		value.Stats.HP, value.Stats.MP, value.Stats.Attack, value.Stats.Defense, value.Stats.Agility,
		value.Money, value.Level, value.Experience, value.RebirthCount, value.SmallMedals)
	return err
}

func (r *CharacterRepository) FindByID(ctx context.Context, id string) (corecharacter.Character, error) {
	var value corecharacter.Character
	err := r.db.QueryRowContext(ctx, `
		SELECT id, player_id, name, job_id, gender, max_hp, max_mp, hp, mp, attack, defense, agility, money, level, experience, rebirth_count, small_medals
		FROM characters
		WHERE id = ?
	`, id).Scan(&value.ID, &value.PlayerID, &value.Name, &value.JobID, &value.Gender, &value.Stats.MaxHP, &value.Stats.MaxMP,
		&value.Stats.HP, &value.Stats.MP, &value.Stats.Attack, &value.Stats.Defense, &value.Stats.Agility,
		&value.Money, &value.Level, &value.Experience, &value.RebirthCount, &value.SmallMedals)
	if errors.Is(err, sql.ErrNoRows) {
		return corecharacter.Character{}, corecharacter.ErrNotFound
	}
	if err != nil {
		return corecharacter.Character{}, err
	}
	return value, nil
}

func (r *CharacterRepository) FindByPlayerID(ctx context.Context, playerID string) ([]corecharacter.Character, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, player_id, name, job_id, gender, max_hp, max_mp, hp, mp, attack, defense, agility, money, level, experience, rebirth_count, small_medals
		FROM characters
		WHERE player_id = ?
		ORDER BY created_at ASC
	`, playerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var characters []corecharacter.Character
	for rows.Next() {
		var value corecharacter.Character
		if err := rows.Scan(&value.ID, &value.PlayerID, &value.Name, &value.JobID, &value.Gender, &value.Stats.MaxHP, &value.Stats.MaxMP,
			&value.Stats.HP, &value.Stats.MP, &value.Stats.Attack, &value.Stats.Defense, &value.Stats.Agility,
			&value.Money, &value.Level, &value.Experience, &value.RebirthCount, &value.SmallMedals); err != nil {
			return nil, err
		}
		characters = append(characters, value)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return characters, nil
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
			attack = ?, defense = ?, agility = ?, money = ?, level = ?, experience = ?, rebirth_count = ?, small_medals = ?
		WHERE id = ?
	`, value.Name, value.JobID, value.Gender, value.Stats.MaxHP, value.Stats.MaxMP, value.Stats.HP,
		value.Stats.MP, value.Stats.Attack, value.Stats.Defense, value.Stats.Agility, value.Money,
		value.Level, value.Experience, value.RebirthCount, value.SmallMedals, value.ID)
	if err != nil {
		return 0, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	return affected, nil
}
