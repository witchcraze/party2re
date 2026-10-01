package database

import (
	"context"
	"time"
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
