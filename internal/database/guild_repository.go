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
		_, err := executor.ExecContext(txCtx, `
			INSERT INTO guilds (id, name, leader_character_id, level, exp, gold, notice, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, g.ID, g.Name, g.LeaderCharacterID, g.Level, g.Exp, g.Gold, g.Notice, g.CreatedAt, g.UpdatedAt)
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "duplicate") || strings.Contains(strings.ToLower(err.Error()), "unique") {
				return guild.ErrGuildNameTaken
			}
			return err
		}

		// 3. Insert creator as leader
		creator.GuildID = g.ID
		_, err = executor.ExecContext(txCtx, `
			INSERT INTO guild_members (guild_id, character_id, role, joined_at, total_donated_gold)
			VALUES (?, ?, ?, ?, ?)
		`, creator.GuildID, creator.CharacterID, string(creator.Role), creator.JoinedAt, creator.TotalDonatedGold)
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
		SELECT id, name, leader_character_id, level, exp, gold, notice, created_at, updated_at
		FROM guilds
		WHERE id = ?
	`, guildID).Scan(
		&g.ID, &g.Name, &g.LeaderCharacterID, &g.Level,
		&g.Exp, &g.Gold, &g.Notice, &g.CreatedAt, &g.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return guild.Guild{}, nil, guild.ErrGuildNotFound
	}
	if err != nil {
		return guild.Guild{}, nil, err
	}

	rows, err := executor.QueryContext(ctx, `
		SELECT guild_id, character_id, role, joined_at, total_donated_gold
		FROM guild_members
		WHERE guild_id = ?
		ORDER BY joined_at ASC
	`, guildID)
	if err != nil {
		return guild.Guild{}, nil, err
	}
	defer rows.Close()

	var members []guild.Member
	for rows.Next() {
		var m guild.Member
		var roleStr string
		if err := rows.Scan(&m.GuildID, &m.CharacterID, &roleStr, &m.JoinedAt, &m.TotalDonatedGold); err != nil {
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
		SELECT g.id, g.name, g.leader_character_id, g.level, g.exp, g.gold, g.notice, g.created_at, g.updated_at,
		       gm.guild_id, gm.character_id, gm.role, gm.joined_at, gm.total_donated_gold
		FROM guild_members gm
		JOIN guilds g ON gm.guild_id = g.id
		WHERE gm.character_id = ?
	`, characterID).Scan(
		&g.ID, &g.Name, &g.LeaderCharacterID, &g.Level, &g.Exp, &g.Gold, &g.Notice, &g.CreatedAt, &g.UpdatedAt,
		&m.GuildID, &m.CharacterID, &roleStr, &m.JoinedAt, &m.TotalDonatedGold,
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
		SELECT id, name, leader_character_id, level, exp, gold, notice, created_at, updated_at
		FROM guilds
		ORDER BY level DESC, exp DESC, created_at ASC
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
			&g.ID, &g.Name, &g.LeaderCharacterID, &g.Level,
			&g.Exp, &g.Gold, &g.Notice, &g.CreatedAt, &g.UpdatedAt,
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

func (r *GuildRepository) AddMember(ctx context.Context, m guild.Member) (guild.Member, error) {
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		INSERT INTO guild_members (guild_id, character_id, role, joined_at, total_donated_gold)
		VALUES (?, ?, ?, ?, ?)
	`, m.GuildID, m.CharacterID, string(m.Role), m.JoinedAt, m.TotalDonatedGold)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") || strings.Contains(strings.ToLower(err.Error()), "primary") {
			return guild.Member{}, guild.ErrCharacterAlreadyInGuild
		}
		return guild.Member{}, err
	}
	return m, nil
}

func (r *GuildRepository) RemoveMember(ctx context.Context, guildID string, characterID string) error {
	res, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
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
}

func (r *GuildRepository) TransferLeadership(ctx context.Context, guildID string, oldLeaderID, newLeaderID string) error {
	return RunInTx(ctx, r.db, func(txCtx context.Context) error {
		executor := ExecutorFromContext(txCtx, r.db)
		now := time.Now().UTC()
		if _, err := executor.ExecContext(txCtx, `
			UPDATE guilds
			SET leader_character_id = ?, updated_at = ?
			WHERE id = ?
		`, newLeaderID, now, guildID); err != nil {
			return err
		}

		if _, err := executor.ExecContext(txCtx, `
			UPDATE guild_members
			SET role = ?
			WHERE guild_id = ? AND character_id = ?
		`, string(guild.RoleOfficer), guildID, oldLeaderID); err != nil {
			return err
		}

		if _, err := executor.ExecContext(txCtx, `
			UPDATE guild_members
			SET role = ?
			WHERE guild_id = ? AND character_id = ?
		`, string(guild.RoleLeader), guildID, newLeaderID); err != nil {
			return err
		}

		return nil
	})
}

func (r *GuildRepository) UpdateMemberRole(ctx context.Context, guildID string, targetCharID string, newRole guild.Role) error {
	res, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		UPDATE guild_members
		SET role = ?
		WHERE guild_id = ? AND character_id = ?
	`, string(newRole), guildID, targetCharID)
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

func (r *GuildRepository) UpdateNotice(ctx context.Context, guildID string, notice string) error {
	now := time.Now().UTC()
	res, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		UPDATE guilds
		SET notice = ?, updated_at = ?
		WHERE id = ?
	`, notice, now, guildID)
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

func (r *GuildRepository) Donate(ctx context.Context, guildID string, characterID string, amount int) (guild.Guild, guild.Member, corecharacter.Character, error) {
	var g guild.Guild
	var m guild.Member
	var char corecharacter.Character

	err := RunInTx(ctx, r.db, func(txCtx context.Context) error {
		executor := ExecutorFromContext(txCtx, r.db)

		// 1. Deduct gold from character
		res, err := executor.ExecContext(txCtx, `
			UPDATE characters
			SET money = money - ?
			WHERE id = ? AND money >= ?
		`, amount, characterID, amount)
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

		// 2. Lock and retrieve current guild stats
		var currentExp int64
		var currentGold int64
		err = executor.QueryRowContext(txCtx, `
			SELECT exp, gold
			FROM guilds
			WHERE id = ? FOR UPDATE
		`, guildID).Scan(&currentExp, &currentGold)
		if errors.Is(err, sql.ErrNoRows) {
			return guild.ErrGuildNotFound
		}
		if err != nil {
			return err
		}

		newExp := currentExp + int64(amount)
		newLevel := guild.CalculateLevel(newExp)

		// 3. Update guild stats
		now := time.Now().UTC()
		if _, err := executor.ExecContext(txCtx, `
			UPDATE guilds
			SET gold = gold + ?, exp = ?, level = ?, updated_at = ?
			WHERE id = ?
		`, amount, newExp, newLevel, now, guildID); err != nil {
			return err
		}

		// 4. Update member donated gold
		if _, err := executor.ExecContext(txCtx, `
			UPDATE guild_members
			SET total_donated_gold = total_donated_gold + ?
			WHERE guild_id = ? AND character_id = ?
		`, amount, guildID, characterID); err != nil {
			return err
		}

		// 5. Scan updated values
		if err := executor.QueryRowContext(txCtx, `
			SELECT id, name, leader_character_id, level, exp, gold, notice, created_at, updated_at
			FROM guilds
			WHERE id = ?
		`, guildID).Scan(
			&g.ID, &g.Name, &g.LeaderCharacterID, &g.Level,
			&g.Exp, &g.Gold, &g.Notice, &g.CreatedAt, &g.UpdatedAt,
		); err != nil {
			return err
		}

		var roleStr string
		if err := executor.QueryRowContext(txCtx, `
			SELECT guild_id, character_id, role, joined_at, total_donated_gold
			FROM guild_members
			WHERE guild_id = ? AND character_id = ?
		`, guildID, characterID).Scan(
			&m.GuildID, &m.CharacterID, &roleStr, &m.JoinedAt, &m.TotalDonatedGold,
		); err != nil {
			return err
		}
		m.Role = guild.Role(roleStr)

		char, err = scanCharacterRow(executor.QueryRowContext(txCtx, `
			SELECT `+characterColumns+`
			FROM characters
			WHERE id = ?
		`, characterID))
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return guild.Guild{}, guild.Member{}, corecharacter.Character{}, err
	}

	return g, m, char, nil
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
