package database

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/witchcraze/party2re/internal/guild"
)

func (r *GuildRepository) AddMember(ctx context.Context, m guild.Member) (guild.Member, error) {
	if m.Title == "" && m.IsPending {
		m.Title = guild.DefaultTitlePending
	}
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		INSERT INTO guild_members (guild_id, character_id, role, title, is_pending, joined_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, m.GuildID, m.CharacterID, string(m.Role), m.Title, m.IsPending, m.JoinedAt)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") || strings.Contains(strings.ToLower(err.Error()), "primary") {
			return guild.Member{}, guild.ErrCharacterAlreadyInGuild
		}
		return guild.Member{}, err
	}
	return m, nil
}

func (r *GuildRepository) RemoveMember(ctx context.Context, guildID string, characterID string) error {
	return RunInTx(ctx, r.db, func(txCtx context.Context) error {
		res, err := ExecutorFromContext(txCtx, r.db).ExecContext(txCtx, `
			DELETE FROM guild_members
			WHERE guild_id = ? AND character_id = ?
		`, guildID, characterID)
		if err != nil {
			return err
		}
		rows, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if rows == 0 {
			return guild.ErrCharacterNotInGuild
		}
		return nil
	})
}

// GetGuildForUpdate retrieves a guild and its memberships with Rank-7 row locks in deterministic order.
func (r *GuildRepository) GetGuildForUpdate(ctx context.Context, guildID string) (guild.Guild, []guild.Member, error) {
	var g guild.Guild

	executor := ExecutorFromContext(ctx, r.db)
	err := executor.QueryRowContext(ctx, `
		SELECT id, name, leader_character_id, points, notice, color, mark, COALESCE(bgimg, ''), last_active_at, created_at, updated_at
		FROM guilds
		WHERE id = ?
		FOR UPDATE
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
		FOR UPDATE
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

func (r *GuildRepository) AssignCustomRole(ctx context.Context, guildID string, targetCharID string, title string) error {
	res, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		UPDATE guild_members
		SET title = ?
		WHERE guild_id = ? AND character_id = ?
	`, title, guildID, targetCharID)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return guild.ErrTargetNotMember
	}
	return nil
}

func (r *GuildRepository) ApproveMember(ctx context.Context, guildID string, characterID string, title string) error {
	if title == "" {
		title = ""
	}
	res, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		UPDATE guild_members
		SET is_pending = FALSE, title = ?
		WHERE guild_id = ? AND character_id = ? AND is_pending = TRUE
	`, title, guildID, characterID)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return guild.ErrMemberNotPending
	}
	return nil
}

func (r *GuildRepository) IsColorTaken(ctx context.Context, color string, excludeGuildID string) (bool, error) {
	var count int
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM guilds
		WHERE color = ? AND id != ?
	`, color, excludeGuildID).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *GuildRepository) UpdateNotice(ctx context.Context, guildID string, notice string) error {
	now := time.Now().UTC()
	res, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		UPDATE guilds
		SET notice = ?, last_active_at = ?, updated_at = ?
		WHERE id = ?
	`, notice, now, now, guildID)
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
	return nil
}

func (r *GuildRepository) UpdateColor(ctx context.Context, guildID string, color string) error {
	now := time.Now().UTC()
	res, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		UPDATE guilds
		SET color = ?, last_active_at = ?, updated_at = ?
		WHERE id = ?
	`, color, now, now, guildID)
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
	return nil
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
