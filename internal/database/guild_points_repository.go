package database

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"strings"
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
		return saveGuildListing(txCtx, executor, guildIDs, now)
	})
}

type guildListingRow struct {
	id        string
	createdAt time.Time
	entry     guild.ListingEntry
}

// saveGuildListing ranks the processed guild cohort by points, then active member count from one roster read; pending applicants are not listed.
func saveGuildListing(ctx context.Context, executor sqlContextExecutor, guildIDs []string, now time.Time) error {
	guilds := []guild.ListingEntry{}
	if len(guildIDs) > 0 {
		placeholders := "?" + strings.Repeat(",?", len(guildIDs)-1)
		args := make([]any, len(guildIDs))
		for i, guildID := range guildIDs {
			args[i] = guildID
		}
		rows, err := executor.QueryContext(ctx, `
			SELECT id, name, mark, color, notice, points, created_at
			FROM guilds
			WHERE id IN (`+placeholders+`)
		`, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		listingRows := []guildListingRow{}
		index := map[string]int{}
		for rows.Next() {
			var r guildListingRow
			r.entry.Members = []guild.ListingMember{}
			if err := rows.Scan(&r.id, &r.entry.Name, &r.entry.Mark, &r.entry.Color, &r.entry.Message, &r.entry.Points, &r.createdAt); err != nil {
				return err
			}
			index[r.id] = len(listingRows)
			listingRows = append(listingRows, r)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if len(listingRows) > 0 {
			members, err := executor.QueryContext(ctx, `
				SELECT m.guild_id, c.name, m.title
				FROM guild_members m
				JOIN characters c ON c.id = m.character_id
				WHERE m.is_pending = FALSE AND m.guild_id IN (`+placeholders+`)
				ORDER BY m.joined_at, m.character_id
			`, args...)
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
					listingRows[i].entry.Members = append(listingRows[i].entry.Members, m)
				}
			}
			if err := members.Err(); err != nil {
				return err
			}
			slices.SortFunc(listingRows, func(a, b guildListingRow) int {
				if c := cmp.Compare(b.entry.Points, a.entry.Points); c != 0 {
					return c
				}
				if c := cmp.Compare(len(b.entry.Members), len(a.entry.Members)); c != 0 {
					return c
				}
				if c := a.createdAt.Compare(b.createdAt); c != 0 {
					return c
				}
				return cmp.Compare(a.id, b.id)
			})
			guilds = make([]guild.ListingEntry, len(listingRows))
			for i, r := range listingRows {
				guilds[i] = r.entry
			}
		}
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
