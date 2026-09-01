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
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		INSERT INTO characters
			(id, player_id, name, job_id, gender, max_hp, max_mp, hp, mp, attack, defense, agility, money, level, experience, rebirth_count, small_medals, help_count, over_level, over_depot, over_monster, over_future, over_flea, over_store)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, value.ID, value.PlayerID, value.Name, value.JobID, value.Gender, value.Stats.MaxHP, value.Stats.MaxMP,
		value.Stats.HP, value.Stats.MP, value.Stats.Attack, value.Stats.Defense, value.Stats.Agility,
		value.Money, value.Level, value.Experience, value.RebirthCount, value.SmallMedals, value.HelpCount,
		value.OverLevel, value.OverDepot, value.OverMonster, value.OverFuture, value.OverFlea, value.OverStore)
	return err
}

func (r *CharacterRepository) FindByID(ctx context.Context, id string) (corecharacter.Character, error) {
	return r.findByIDWithQuery(ctx, id, `
		SELECT `+characterColumns+`
		FROM characters
		WHERE id = ?
	`)
}

func (r *CharacterRepository) FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error) {
	return r.findByIDWithQuery(ctx, id, `
		SELECT `+characterColumns+`
		FROM characters
		WHERE id = ? FOR UPDATE
	`)
}

func (r *CharacterRepository) findByIDWithQuery(ctx context.Context, id string, query string) (corecharacter.Character, error) {
	return scanCharacterRow(ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, query, id))
}

func (r *CharacterRepository) FindByPlayerID(ctx context.Context, playerID string) ([]corecharacter.Character, error) {
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, `
		SELECT `+characterColumns+`
		FROM characters
		WHERE player_id = ?
		ORDER BY created_at ASC
	`, playerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanCharacterRows(rows)
}

func (r *CharacterRepository) Update(ctx context.Context, value corecharacter.Character) error {
	return updateCharacter(ctx, ExecutorFromContext(ctx, r.db), value)
}
