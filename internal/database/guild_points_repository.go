package database

import (
	"context"
	"errors"
	"time"

	"github.com/witchcraze/party2re/internal/guild"
)

// DecayGuildPoints applies point decay across all guilds with positive points.
// Uses FLOOR(points * factor) to match authentic legacy Party2 behavior (login.cgi:448: int($gpoint * 0.8)).
func (r *GuildRepository) DecayGuildPoints(ctx context.Context, factor float64) error {
	return RunInTx(ctx, r.db, func(txCtx context.Context) error {
		executor := ExecutorFromContext(txCtx, r.db)
		now := time.Now().UTC()
		_, err := executor.ExecContext(txCtx, `
			UPDATE guilds
			SET points = FLOOR(points * ?), updated_at = ?
			WHERE points > 0
		`, factor, now)
		return err
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
