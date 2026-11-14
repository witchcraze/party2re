package database

import (
	"context"
	"database/sql"
	"errors"

	"github.com/witchcraze/party2re/internal/character"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

// CharacterMemoryRepository stores character trajectory entries.
type CharacterMemoryRepository struct {
	db *sql.DB
}

func NewCharacterMemoryRepository(db *sql.DB) (*CharacterMemoryRepository, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	return &CharacterMemoryRepository{db: db}, nil
}

// AppendMemory snapshots the character and its active guild as the ambient
// transaction sees them. An unknown character is corecharacter.ErrNotFound.
func (r *CharacterMemoryRepository) AppendMemory(ctx context.Context, characterID string, kind corecharacter.MemoryKind, content string) error {
	// shortcut: rows beyond character.MaxMemories are kept but never shown; prune here if the table grows large.
	res, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		INSERT INTO character_memories (character_id, kind, content, name, guild_name, color, level, job_id, gender, sp, created_at)
		SELECT c.id, ?, ?, c.name, COALESCE(g.name, ''), c.color, c.level, c.job_id, c.gender, c.sp, UTC_TIMESTAMP(6)
		FROM characters c
		LEFT JOIN guild_members gm ON gm.character_id = c.id AND gm.is_pending = FALSE
		LEFT JOIN guilds g ON g.id = gm.guild_id
		WHERE c.id = ?
		LIMIT 1
	`, string(kind), content, characterID)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return corecharacter.ErrNotFound
	}
	return nil
}

func (r *CharacterMemoryRepository) ListMemories(ctx context.Context, characterID string, limit int) ([]character.Memory, error) {
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, `
		SELECT kind, content, name, guild_name, color, level, job_id, gender, sp, created_at
		FROM character_memories
		WHERE character_id = ?
		ORDER BY id DESC
		LIMIT ?
	`, characterID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []character.Memory{}
	for rows.Next() {
		var m character.Memory
		if err := rows.Scan(&m.Kind, &m.Content, &m.Name, &m.GuildName, &m.Color, &m.Level, &m.JobID, &m.Gender, &m.SP, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
