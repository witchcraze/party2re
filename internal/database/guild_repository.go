package database

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/guild"
)

type GuildRepository struct {
	db *sql.DB
}

func NewGuildRepository(db *sql.DB) (*GuildRepository, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	return &GuildRepository{db: db}, nil
}

func (r *GuildRepository) CreateGuild(ctx context.Context, g guild.Guild, creator guild.Member, fee int) (guild.Guild, guild.Member, corecharacter.Character, error) {
	var createdGuild guild.Guild
	var createdMember guild.Member
	var updatedChar corecharacter.Character

	err := RunInTx(ctx, r.db, func(txCtx context.Context) error {
		executor := ExecutorFromContext(txCtx, r.db)

		// 1. Deduct fee from character wallet (if fee > 0)
		if fee > 0 {
			res, err := executor.ExecContext(txCtx, `
				UPDATE characters
				SET money = money - ?
				WHERE id = ? AND money >= ?
			`, fee, creator.CharacterID, fee)
			if err != nil {
				return err
			}
			rows, err := res.RowsAffected()
			if err != nil {
				return err
			}
			if rows == 0 {
				return guild.ErrInsufficientFunds
			}
		} else {
			var charID string
			err := executor.QueryRowContext(txCtx, `SELECT id FROM characters WHERE id = ? FOR UPDATE`, creator.CharacterID).Scan(&charID)
			if errors.Is(err, sql.ErrNoRows) {
				return guild.ErrCharacterNotFound
			}
			if err != nil {
				return err
			}
		}

		// 2. Insert guild record
		guildColor := g.Color
		if guildColor == "" {
			guildColor = guild.DefaultColor
		}
		mark := g.Mark
		if mark == "" {
			mark = guild.DefaultMark
		}
		lastActive := g.LastActiveAt
		if lastActive.IsZero() {
			lastActive = g.CreatedAt
		}
		_, err := executor.ExecContext(txCtx, `
			INSERT INTO guilds (id, name, leader_character_id, points, notice, color, mark, bgimg, last_active_at, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, g.ID, g.Name, g.LeaderCharacterID, g.Points, g.Notice, guildColor, mark, g.Bgimg, lastActive, g.CreatedAt, g.UpdatedAt)
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "duplicate") || strings.Contains(strings.ToLower(err.Error()), "unique") {
				return guild.ErrGuildNameTaken
			}
			return err
		}

		// 3. Insert creator as leader
		creator.GuildID = g.ID
		if creator.Title == "" {
			creator.Title = guild.DefaultTitleLeader
		}
		creator.IsPending = false
		_, err = executor.ExecContext(txCtx, `
			INSERT INTO guild_members (guild_id, character_id, role, title, is_pending, joined_at)
			VALUES (?, ?, ?, ?, ?, ?)
		`, creator.GuildID, creator.CharacterID, string(creator.Role), creator.Title, creator.IsPending, creator.JoinedAt)
		if err != nil {
			return err
		}

		// 4. Fetch updated character
		char, err := scanCharacterRow(executor.QueryRowContext(txCtx, `
			SELECT `+characterColumns+`
			FROM characters
			WHERE id = ?
		`, creator.CharacterID))
		if err != nil {
			return err
		}

		createdGuild = g
		createdGuild.Color = guildColor
		createdGuild.Mark = mark
		createdGuild.LastActiveAt = lastActive
		createdMember = creator
		updatedChar = char
		return nil
	})
	if err != nil {
		return guild.Guild{}, guild.Member{}, corecharacter.Character{}, err
	}

	return createdGuild, createdMember, updatedChar, nil
}

func (r *GuildRepository) GetGuild(ctx context.Context, guildID string) (guild.Guild, []guild.Member, error) {
	var g guild.Guild
	executor := ExecutorFromContext(ctx, r.db)
	err := executor.QueryRowContext(ctx, `
		SELECT id, name, leader_character_id, points, notice, color, mark, COALESCE(bgimg, ''), last_active_at, created_at, updated_at
		FROM guilds
		WHERE id = ?
	`, guildID).Scan(
		&g.ID, &g.Name, &g.LeaderCharacterID, &g.Points,
		&g.Notice, &g.Color, &g.Mark, &g.Bgimg, &g.LastActiveAt, &g.CreatedAt, &g.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return guild.Guild{}, nil, guild.ErrGuildNotFound
	}
	if err != nil {
		return guild.Guild{}, nil, err
	}

	rows, err := executor.QueryContext(ctx, `
		SELECT guild_id, character_id, role, title, is_pending, joined_at
		FROM guild_members
		WHERE guild_id = ?
		ORDER BY joined_at ASC, character_id ASC
	`, guildID)
	if err != nil {
		return guild.Guild{}, nil, err
	}
	defer rows.Close()

	var members []guild.Member
	for rows.Next() {
		var m guild.Member
		var roleStr string
		if err := rows.Scan(&m.GuildID, &m.CharacterID, &roleStr, &m.Title, &m.IsPending, &m.JoinedAt); err != nil {
			return guild.Guild{}, nil, err
		}
		m.Role = guild.Role(roleStr)
		members = append(members, m)
	}
	if err := rows.Err(); err != nil {
		return guild.Guild{}, nil, err
	}

	return g, members, nil
}

func (r *GuildRepository) GetGuildByCharacter(ctx context.Context, characterID string) (guild.Guild, guild.Member, error) {
	var g guild.Guild
	var m guild.Member
	var roleStr string

	executor := ExecutorFromContext(ctx, r.db)
	err := executor.QueryRowContext(ctx, `
		SELECT g.id, g.name, g.leader_character_id, g.points, g.notice, g.color, g.mark, COALESCE(g.bgimg, ''), g.last_active_at, g.created_at, g.updated_at,
		       gm.guild_id, gm.character_id, gm.role, gm.title, gm.is_pending, gm.joined_at
		FROM guild_members gm
		JOIN guilds g ON gm.guild_id = g.id
		WHERE gm.character_id = ?
	`, characterID).Scan(
		&g.ID, &g.Name, &g.LeaderCharacterID, &g.Points, &g.Notice, &g.Color, &g.Mark, &g.Bgimg, &g.LastActiveAt, &g.CreatedAt, &g.UpdatedAt,
		&m.GuildID, &m.CharacterID, &roleStr, &m.Title, &m.IsPending, &m.JoinedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return guild.Guild{}, guild.Member{}, guild.ErrCharacterNotInGuild
	}
	if err != nil {
		return guild.Guild{}, guild.Member{}, err
	}
	m.Role = guild.Role(roleStr)

	return g, m, nil
}

func (r *GuildRepository) ListGuilds(ctx context.Context, offset, limit int) ([]guild.Guild, error) {
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, `
		SELECT id, name, leader_character_id, points, notice, color, mark, COALESCE(bgimg, ''), last_active_at, created_at, updated_at
		FROM guilds
		ORDER BY points DESC, created_at ASC
		LIMIT ? OFFSET ?
	`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var guilds []guild.Guild
	for rows.Next() {
		var g guild.Guild
		if err := rows.Scan(
			&g.ID, &g.Name, &g.LeaderCharacterID, &g.Points,
			&g.Notice, &g.Color, &g.Mark, &g.Bgimg, &g.LastActiveAt, &g.CreatedAt, &g.UpdatedAt,
		); err != nil {
			return nil, err
		}
		guilds = append(guilds, g)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return guilds, nil
}

func (r *GuildRepository) ListInactiveGuilds(ctx context.Context, cutoff time.Time, limit int) ([]guild.Guild, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, `
		SELECT id, name, leader_character_id, points, notice, color, mark, COALESCE(bgimg, ''), last_active_at, created_at, updated_at
		FROM guilds
		WHERE last_active_at < ?
		ORDER BY last_active_at ASC
		LIMIT ?
	`, cutoff, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var guilds []guild.Guild
	for rows.Next() {
		var g guild.Guild
		if err := rows.Scan(
			&g.ID, &g.Name, &g.LeaderCharacterID, &g.Points,
			&g.Notice, &g.Color, &g.Mark, &g.Bgimg, &g.LastActiveAt, &g.CreatedAt, &g.UpdatedAt,
		); err != nil {
			return nil, err
		}
		guilds = append(guilds, g)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return guilds, nil
}

func (r *GuildRepository) TouchActive(ctx context.Context, guildID string) error {
	now := time.Now().UTC()
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		UPDATE guilds
		SET last_active_at = ?
		WHERE id = ?
	`, now, guildID)
	return err
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

func (r *GuildRepository) TransferLeadership(ctx context.Context, guildID string, oldLeaderID, newLeaderID string) error {
	return RunInTx(ctx, r.db, func(txCtx context.Context) error {
		executor := ExecutorFromContext(txCtx, r.db)
		now := time.Now().UTC()
		if _, err := executor.ExecContext(txCtx, `
			UPDATE guilds
			SET leader_character_id = ?, last_active_at = ?, updated_at = ?
			WHERE id = ?
		`, newLeaderID, now, now, guildID); err != nil {
			return err
		}

		if _, err := executor.ExecContext(txCtx, `
			UPDATE guild_members
			SET role = ?, title = ?
			WHERE guild_id = ? AND character_id = ?
		`, string(guild.RoleMember), "", guildID, oldLeaderID); err != nil {
			return err
		}

		if _, err := executor.ExecContext(txCtx, `
			UPDATE guild_members
			SET role = ?, title = ?, is_pending = FALSE
			WHERE guild_id = ? AND character_id = ?
		`, string(guild.RoleLeader), guild.DefaultTitleLeader, guildID, newLeaderID); err != nil {
			return err
		}

		return nil
	})
}

func (r *GuildRepository) UpdateMark(ctx context.Context, guildID string, mark string, fee int, leaderID string) (corecharacter.Character, error) {
	var updatedChar corecharacter.Character

	err := RunInTx(ctx, r.db, func(txCtx context.Context) error {
		executor := ExecutorFromContext(txCtx, r.db)

		// 1. Deduct fee from character wallet (Rank 2)
		if fee > 0 {
			res, err := executor.ExecContext(txCtx, `
				UPDATE characters
				SET money = money - ?
				WHERE id = ? AND money >= ?
			`, fee, leaderID, fee)
			if err != nil {
				return err
			}
			rows, err := res.RowsAffected()
			if err != nil {
				return err
			}
			if rows == 0 {
				return guild.ErrInsufficientFunds
			}
		}

		// 2. Update guild mark (Rank 7)
		now := time.Now().UTC()
		res, err := executor.ExecContext(txCtx, `
			UPDATE guilds
			SET mark = ?, last_active_at = ?, updated_at = ?
			WHERE id = ?
		`, mark, now, now, guildID)
		if err != nil {
			return err
		}
		rows, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if rows == 0 {
			return guild.ErrGuildNotFound
		}

		// 3. Scan updated character
		char, err := scanCharacterRow(executor.QueryRowContext(txCtx, `
			SELECT `+characterColumns+`
			FROM characters
			WHERE id = ?
		`, leaderID))
		if err != nil {
			return err
		}
		updatedChar = char
		return nil
	})
	if err != nil {
		return corecharacter.Character{}, err
	}
	return updatedChar, nil
}

func (r *GuildRepository) UpdateBgimg(ctx context.Context, guildID string, bgimg string) error {
	now := time.Now().UTC()
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		UPDATE guilds
		SET bgimg = ?, last_active_at = ?, updated_at = ?
		WHERE id = ?
	`, bgimg, now, now, guildID)
	return err
}

func (r *GuildRepository) UpdateWallpaper(ctx context.Context, guildID string, wallpaper string, fee int, leaderID string) (corecharacter.Character, error) {
	var updatedChar corecharacter.Character

	err := RunInTx(ctx, r.db, func(txCtx context.Context) error {
		executor := ExecutorFromContext(txCtx, r.db)

		// 1. Deduct fee from character wallet (Rank 2)
		if fee > 0 {
			res, err := executor.ExecContext(txCtx, `
				UPDATE characters
				SET money = money - ?
				WHERE id = ? AND money >= ?
			`, fee, leaderID, fee)
			if err != nil {
				return err
			}
			rows, err := res.RowsAffected()
			if err != nil {
				return err
			}
			if rows == 0 {
				return guild.ErrInsufficientFunds
			}
		}

		// 2. Update guild wallpaper (Rank 7)
		now := time.Now().UTC()
		res, err := executor.ExecContext(txCtx, `
			UPDATE guilds
			SET bgimg = ?, last_active_at = ?, updated_at = ?
			WHERE id = ?
		`, wallpaper, now, now, guildID)
		if err != nil {
			return err
		}
		rows, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if rows == 0 {
			return guild.ErrGuildNotFound
		}

		// 3. Scan updated character
		char, err := scanCharacterRow(executor.QueryRowContext(txCtx, `
			SELECT `+characterColumns+`
			FROM characters
			WHERE id = ?
		`, leaderID))
		if err != nil {
			return err
		}
		updatedChar = char
		return nil
	})
	if err != nil {
		return corecharacter.Character{}, err
	}
	return updatedChar, nil
}

func (r *GuildRepository) DisbandGuild(ctx context.Context, guildID string) error {
	return RunInTx(ctx, r.db, func(txCtx context.Context) error {
		executor := ExecutorFromContext(txCtx, r.db)
		if _, err := executor.ExecContext(txCtx, `DELETE FROM guild_members WHERE guild_id = ?`, guildID); err != nil {
			return err
		}
		if _, err := executor.ExecContext(txCtx, `DELETE FROM guilds WHERE id = ?`, guildID); err != nil {
			return err
		}
		return nil
	})
}
