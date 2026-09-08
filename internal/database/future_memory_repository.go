package database

import (
	"context"
	"database/sql"
	"errors"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

type FutureMemoryRepository struct {
	db *sql.DB
}

func NewFutureMemoryRepository(db *sql.DB) (*FutureMemoryRepository, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	return &FutureMemoryRepository{db: db}, nil
}

func (r *FutureMemoryRepository) Save(ctx context.Context, memory corecharacter.FutureMemory) error {
	executor := ExecutorFromContext(ctx, r.db)
	_, err := executor.ExecContext(ctx, `
		INSERT INTO character_future_memories (
			id, character_id, slot, job_id, old_job_id, level, experience,
			max_hp, max_mp, attack, defense, agility, gender, over_level, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			slot = VALUES(slot), job_id = VALUES(job_id), old_job_id = VALUES(old_job_id),
			level = VALUES(level), experience = VALUES(experience),
			max_hp = VALUES(max_hp), max_mp = VALUES(max_mp),
			attack = VALUES(attack), defense = VALUES(defense), agility = VALUES(agility),
			gender = VALUES(gender), over_level = VALUES(over_level)
	`,
		memory.ID, memory.CharacterID, 1, memory.JobID, memory.OldJobID, memory.Level, memory.Experience,
		memory.MaxHP, memory.MaxMP, memory.Attack, memory.Defense, memory.Agility, memory.Gender, memory.OverLevel, memory.CreatedAt,
	)
	return err
}

func (r *FutureMemoryRepository) FindByCharacterID(ctx context.Context, characterID string) ([]corecharacter.FutureMemory, error) {
	executor := ExecutorFromContext(ctx, r.db)
	rows, err := executor.QueryContext(ctx, `
		SELECT id, character_id, job_id, old_job_id, level, experience,
		       max_hp, max_mp, attack, defense, agility, gender, over_level, created_at
		FROM character_future_memories
		WHERE character_id = ?
		ORDER BY created_at ASC
	`, characterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var memories []corecharacter.FutureMemory
	for rows.Next() {
		var m corecharacter.FutureMemory
		if err := rows.Scan(
			&m.ID, &m.CharacterID, &m.JobID, &m.OldJobID, &m.Level, &m.Experience,
			&m.MaxHP, &m.MaxMP, &m.Attack, &m.Defense, &m.Agility, &m.Gender, &m.OverLevel, &m.CreatedAt,
		); err != nil {
			return nil, err
		}
		memories = append(memories, m)
	}
	return memories, rows.Err()
}

func (r *FutureMemoryRepository) Delete(ctx context.Context, characterID, memoryID string) error {
	executor := ExecutorFromContext(ctx, r.db)
	_, err := executor.ExecContext(ctx, `
		DELETE FROM character_future_memories
		WHERE character_id = ? AND id = ?
	`, characterID, memoryID)
	return err
}
