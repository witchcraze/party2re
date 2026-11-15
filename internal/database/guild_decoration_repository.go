package database

import (
	"context"

	"github.com/witchcraze/party2re/internal/guild"
)

func (r *GuildRepository) AddDecoration(ctx context.Context, guildID string, d guild.Decoration) error {
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		INSERT INTO guild_decorations (id, guild_id, kind, icon, created_at)
		VALUES (?, ?, ?, ?, ?)
	`, d.ID, guildID, d.Kind, d.Icon, d.CreatedAt)
	return err
}

func (r *GuildRepository) ListDecorations(ctx context.Context, guildID string) ([]guild.Decoration, error) {
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, `
		SELECT id, kind, icon, created_at FROM guild_decorations
		WHERE guild_id = ? ORDER BY created_at, id
	`, guildID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	decorations := make([]guild.Decoration, 0)
	for rows.Next() {
		var d guild.Decoration
		if err := rows.Scan(&d.ID, &d.Kind, &d.Icon, &d.CreatedAt); err != nil {
			return nil, err
		}
		decorations = append(decorations, d)
	}
	return decorations, rows.Err()
}
