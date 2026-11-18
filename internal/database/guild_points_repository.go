package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/witchcraze/party2re/internal/guild"
)

// DecayGuildPoints applies point decay across all guilds with positive points, then records the public guild list snapshot.
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
		return saveGuildListing(txCtx, executor, now)
	})
}

// saveGuildListing ranks guilds by points, then active member count; pending applicants are not listed.
func saveGuildListing(ctx context.Context, executor sqlContextExecutor, now time.Time) error {
	rows, err := executor.QueryContext(ctx, `
		SELECT g.id, g.name, g.mark, g.color, g.notice, g.points
		FROM guilds g
		ORDER BY g.points DESC,
			(SELECT COUNT(*) FROM guild_members m WHERE m.guild_id = g.id AND m.is_pending = FALSE) DESC,
			g.created_at, g.id
	`)
	if err != nil {
		return err
	}
	defer rows.Close()
	guilds := []guild.ListingEntry{}
	index := map[string]int{}
	for rows.Next() {
		var guildID string
		e := guild.ListingEntry{Members: []guild.ListingMember{}}
		if err := rows.Scan(&guildID, &e.Name, &e.Mark, &e.Color, &e.Message, &e.Points); err != nil {
			return err
		}
		index[guildID] = len(guilds)
		guilds = append(guilds, e)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	members, err := executor.QueryContext(ctx, `
		SELECT m.guild_id, c.name, m.title
		FROM guild_members m
		JOIN characters c ON c.id = m.character_id
		WHERE m.is_pending = FALSE
		ORDER BY m.joined_at, m.character_id
	`)
	if err != nil {
		return err
	}
	defer members.Close()
	for members.Next() {
		var guildID string
		var m guild.ListingMember
		if err := members.Scan(&guildID, &m.Name, &m.Title); err != nil {
			return err
		}
		if i, ok := index[guildID]; ok {
			guilds[i].Members = append(guilds[i].Members, m)
		}
	}
	if err := members.Err(); err != nil {
		return err
	}
	data, err := json.Marshal(guilds)
	if err != nil {
		return err
	}
	_, err = executor.ExecContext(ctx, `
		INSERT INTO guild_list_snapshot (id, guilds, generated_at) VALUES (1, ?, ?)
		ON DUPLICATE KEY UPDATE guilds = VALUES(guilds), generated_at = VALUES(generated_at)
	`, string(data), now)
	return err
}

// GetListing returns the last public guild list snapshot, or an empty list before the first one.
func (r *GuildRepository) GetListing(ctx context.Context) (guild.Listing, error) {
	var data string
	var generatedAt time.Time
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx,
		"SELECT guilds, generated_at FROM guild_list_snapshot WHERE id = 1").Scan(&data, &generatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return guild.Listing{Guilds: []guild.ListingEntry{}}, nil
	}
	if err != nil {
		return guild.Listing{}, err
	}
	listing := guild.Listing{GeneratedAt: &generatedAt}
	if err := json.Unmarshal([]byte(data), &listing.Guilds); err != nil {
		return guild.Listing{}, err
	}
	return listing, nil
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
