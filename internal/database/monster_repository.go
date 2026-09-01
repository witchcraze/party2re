package database

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/monster"
)

type MonsterRepository struct {
	db *sql.DB
}

func NewMonsterRepository(db *sql.DB) (*MonsterRepository, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	return &MonsterRepository{db: db}, nil
}

func (r *MonsterRepository) ListByCharacterID(ctx context.Context, characterID string) ([]monster.MonsterInstance, error) {
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, `
		SELECT id, character_id, monster_id, custom_name, location, created_at, updated_at
		FROM character_monsters
		WHERE character_id = ?
		ORDER BY created_at ASC, id ASC
	`, characterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanMonsters(rows)
}

func (r *MonsterRepository) ListByCharacterIDAndLocation(ctx context.Context, characterID, location string) ([]monster.MonsterInstance, error) {
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, `
		SELECT id, character_id, monster_id, custom_name, location, created_at, updated_at
		FROM character_monsters
		WHERE character_id = ? AND location = ?
		ORDER BY created_at ASC, id ASC
	`, characterID, location)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanMonsters(rows)
}

func (r *MonsterRepository) FindByID(ctx context.Context, id string) (monster.MonsterInstance, error) {
	row := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT id, character_id, monster_id, custom_name, location, created_at, updated_at
		FROM character_monsters
		WHERE id = ?
	`, id)
	return r.scanMonster(row)
}

func (r *MonsterRepository) FindByIDForUpdate(ctx context.Context, id string) (monster.MonsterInstance, error) {
	row := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT id, character_id, monster_id, custom_name, location, created_at, updated_at
		FROM character_monsters
		WHERE id = ?
		FOR UPDATE
	`, id)
	return r.scanMonster(row)
}

func (r *MonsterRepository) Save(ctx context.Context, inst monster.MonsterInstance) error {
	if inst.ID == "" {
		inst.ID = id.New()
	}
	now := time.Now().UTC()
	if inst.CreatedAt.IsZero() {
		inst.CreatedAt = now
	}
	if inst.UpdatedAt.IsZero() {
		inst.UpdatedAt = now
	}

	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		INSERT INTO character_monsters (id, character_id, monster_id, custom_name, location, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			character_id = VALUES(character_id),
			monster_id = VALUES(monster_id),
			custom_name = VALUES(custom_name),
			location = VALUES(location),
			updated_at = VALUES(updated_at)
	`, inst.ID, inst.CharacterID, inst.MonsterID, inst.CustomName, inst.Location, inst.CreatedAt, inst.UpdatedAt)
	return err
}

func (r *MonsterRepository) Delete(ctx context.Context, id string) error {
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		DELETE FROM character_monsters
		WHERE id = ?
	`, id)
	return err
}

func (r *MonsterRepository) CountByLocation(ctx context.Context, characterID, location string) (int, error) {
	var count int
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT COUNT(1)
		FROM character_monsters
		WHERE character_id = ? AND location = ?
	`, characterID, location).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

func (r *MonsterRepository) scanMonster(row scanner) (monster.MonsterInstance, error) {
	var inst monster.MonsterInstance
	err := row.Scan(
		&inst.ID,
		&inst.CharacterID,
		&inst.MonsterID,
		&inst.CustomName,
		&inst.Location,
		&inst.CreatedAt,
		&inst.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return monster.MonsterInstance{}, monster.ErrMonsterNotFound
	}
	if err != nil {
		return monster.MonsterInstance{}, err
	}
	return inst, nil
}

func (r *MonsterRepository) scanMonsters(rows *sql.Rows) ([]monster.MonsterInstance, error) {
	var list []monster.MonsterInstance
	for rows.Next() {
		inst, err := r.scanMonster(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, inst)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return list, nil
}
