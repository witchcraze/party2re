package database

import (
	"context"
	"errors"
	"time"

	"github.com/witchcraze/party2re/internal/guild"
)

// DecayGuildPoints applies point decay across all guilds with positive points.
// Uses FLOOR(points * factor) to match authentic legacy Party2 behavior (login.cgi:448: int($gpoint * 0.8)).
// Transaction: RunInTx
// Lock Order: guilds(7, ascending ID)
func (r *GuildRepository) DecayGuildPoints(ctx context.Context, factor float64) error {
	return RunInTx(ctx, r.db, func(txCtx context.Context) error {
		executor := ExecutorFromContext(txCtx, r.db)
		// Read IDs without locking: a bulk update can lock through the points index.
		rows, err := executor.QueryContext(txCtx, "SELECT id FROM guilds ORDER BY id")
		if err != nil {
			return err
		}
		defer rows.Close()
		var guildIDs []string
		for rows.Next() {
			var guildID string
			if err := rows.Scan(&guildID); err != nil {
				return err
			}
			guildIDs = append(guildIDs, guildID)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		now := time.Now().UTC()
		for _, guildID := range guildIDs {
			// Keep zero balances locked; evaluate the timestamp before changing points.
			if _, err := executor.ExecContext(txCtx, `
				UPDATE guilds
				SET updated_at = IF(points > 0, ?, updated_at),
				    points = IF(points > 0, FLOOR(points * ?), points)
				WHERE id = ?
			`, now, factor, guildID); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *GuildRepository) AddPoints(ctx context.Context, guildID string, points int64) error {
	now := time.Now().UTC()
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		UPDATE guilds
		SET points = points + ?, updated_at = ?
		WHERE id = ?
	`, points, now, guildID)
	return err
}

func (r *GuildRepository) AddGuildPoints(ctx context.Context, characterID string, points int) error {
	g, _, err := r.GetGuildByCharacter(ctx, characterID)
	if err != nil {
		if errors.Is(err, guild.ErrCharacterNotInGuild) {
			return nil
		}
		return err
	}
	return r.AddPoints(ctx, g.ID, int64(points))
}
