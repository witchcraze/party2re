package database

import (
	"context"
	"database/sql"
	"errors"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

// characterColumns lists all standard columns of the characters table in canonical order.
const characterColumns = "id, player_id, name, job_id, gender, max_hp, max_mp, hp, mp, attack, defense, agility, money, level, experience, rebirth_count, small_medals, help_count, over_level, over_depot, over_monster, over_future, over_flea, over_store"

// rowScanner abstracts *sql.Row, *sql.Rows, or any scanner implementation.
type rowScanner interface {
	Scan(dest ...any) error
}

// scanCharacterRow scans a single database row into a corecharacter.Character struct.
func scanCharacterRow(scanner rowScanner) (corecharacter.Character, error) {
	var value corecharacter.Character
	err := scanner.Scan(
		&value.ID,
		&value.PlayerID,
		&value.Name,
		&value.JobID,
		&value.Gender,
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
		&value.RebirthCount,
		&value.SmallMedals,
		&value.HelpCount,
		&value.OverLevel,
		&value.OverDepot,
		&value.OverMonster,
		&value.OverFuture,
		&value.OverFlea,
		&value.OverStore,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return corecharacter.Character{}, corecharacter.ErrNotFound
	}
	if err != nil {
		return corecharacter.Character{}, err
	}
	return value, nil
}

// scanCharacterRows scans multiple database rows into a slice of corecharacter.Character.
func scanCharacterRows(rows *sql.Rows) ([]corecharacter.Character, error) {
	var characters []corecharacter.Character
	for rows.Next() {
		char, err := scanCharacterRow(rows)
		if err != nil {
			return nil, err
		}
		characters = append(characters, char)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return characters, nil
}

// executeCharacterUpdate runs the standard UPDATE characters query with full state persistence.
func executeCharacterUpdate(ctx context.Context, executor sqlContextExecutor, value corecharacter.Character) (int64, error) {
	result, err := executor.ExecContext(ctx, `
		UPDATE characters
		SET name = ?, job_id = ?, gender = ?, max_hp = ?, max_mp = ?, hp = ?, mp = ?,
			attack = ?, defense = ?, agility = ?, money = ?, level = ?, experience = ?, rebirth_count = ?, small_medals = ?, help_count = ?,
			over_level = ?, over_depot = ?, over_monster = ?, over_future = ?, over_flea = ?, over_store = ?
		WHERE id = ?
	`, value.Name, value.JobID, value.Gender, value.Stats.MaxHP, value.Stats.MaxMP, value.Stats.HP,
		value.Stats.MP, value.Stats.Attack, value.Stats.Defense, value.Stats.Agility, value.Money,
		value.Level, value.Experience, value.RebirthCount, value.SmallMedals, value.HelpCount,
		value.OverLevel, value.OverDepot, value.OverMonster, value.OverFuture, value.OverFlea, value.OverStore, value.ID)
	if err != nil {
		return 0, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	return affected, nil
}

// updateCharacter updates character fields in the database, returning corecharacter.ErrNotFound if no row matched.
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

// updateCharacterAtomically updates character fields, succeeding even if MySQL reports 0 affected rows because values were unchanged.
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
