package database

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/witchcraze/party2re/internal/character"
)

// PresenceRepository stores login messages, sampled-save clocks and public presence snapshots.
type PresenceRepository struct {
	db *sql.DB
}

func NewPresenceRepository(db *sql.DB) (*PresenceRepository, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	return &PresenceRepository{db: db}, nil
}

func (r *PresenceRepository) LockPresenceOwner(ctx context.Context, characterID string) (string, error) {
	var owner string
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx,
		`SELECT player_id FROM characters WHERE id = ? FOR UPDATE`, characterID).Scan(&owner)
	if errors.Is(err, sql.ErrNoRows) {
		return "", character.ErrNotFound
	}
	return owner, err
}

func (r *PresenceRepository) SetLoginMessage(ctx context.Context, characterID, message string) error {
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx,
		`UPDATE characters SET login_message = ? WHERE id = ?`, message, characterID)
	return err
}

func (r *PresenceRepository) AdvanceSampleClock(ctx context.Context, characterID string, now, window int64) (bool, error) {
	res, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		UPDATE characters SET presence_sampled_at = ?
		WHERE id = ? AND (presence_sampled_at IS NULL OR ? > presence_sampled_at + ?)`,
		now, characterID, now, window)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

func (r *PresenceRepository) PresenceIconSource(ctx context.Context, characterID string) (string, error) {
	var icon string
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx,
		`SELECT avatar_url FROM character_profiles WHERE character_id = ?`, characterID).Scan(&icon)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return icon, err
}

// PublishPresence replaces the character's snapshot of this kind; REPLACE assigns a
// new seq, preserving publication order for equal timestamps.
func (r *PresenceRepository) PublishPresence(ctx context.Context, characterID string, kind character.PresenceKind, at int64, iconURL string) error {
	res, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		REPLACE INTO character_presence_snapshots
			(character_id, kind, recorded_at, name, color, guild_id, guild_name, icon_url, aura_effect, login_message)
		SELECT c.id, ?, ?, c.name, c.color, COALESCE(g.id, ''), COALESCE(g.name, ''), ?, COALESCE(p.aura_effect, 0), c.login_message
		FROM characters c
		LEFT JOIN guild_members gm ON gm.character_id = c.id
		LEFT JOIN guilds g ON g.id = gm.guild_id
		LEFT JOIN character_profiles p ON p.character_id = c.id
		WHERE c.id = ?`, string(kind), at, iconURL, characterID)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return character.ErrNotFound
	}
	return nil
}

func (r *PresenceRepository) ObservePresence(ctx context.Context, now, window int64) ([]character.PresenceSnapshot, error) {
	var rows []character.PresenceSnapshot
	err := RunInTx(ctx, r.db, func(ctx context.Context) error {
		exec := ExecutorFromContext(ctx, r.db)
		if _, err := exec.ExecContext(ctx,
			`DELETE FROM character_presence_snapshots WHERE recorded_at < ?`, now-window); err != nil {
			return err
		}
		if err := r.deleteBanned(ctx, exec); err != nil {
			return err
		}
		// A valid entry hides that character's sample permanently, even if newer.
		// Records published after this instant are left alone.
		if _, err := exec.ExecContext(ctx, `
			DELETE s FROM character_presence_snapshots s
			JOIN character_presence_snapshots e ON e.character_id = s.character_id AND e.kind = 'entry'
			WHERE s.kind = 'sample' AND s.recorded_at <= ? AND e.recorded_at <= ?`, now, now); err != nil {
			return err
		}
		res, err := exec.QueryContext(ctx, `
			SELECT character_id, name, color, guild_id, guild_name, icon_url, aura_effect, login_message
			FROM character_presence_snapshots
			WHERE recorded_at <= ?
			ORDER BY kind = 'entry' DESC,
				CASE WHEN kind = 'entry' THEN -recorded_at ELSE recorded_at END,
				CASE WHEN kind = 'entry' THEN -seq ELSE seq END`, now)
		if err != nil {
			return err
		}
		defer res.Close() //lint:ignore error-swallow read-only cursor close; Err() below reports scan failures
		seen := map[string]bool{}
		for res.Next() {
			var s character.PresenceSnapshot
			if err := res.Scan(&s.CharacterID, &s.Name, &s.Color, &s.GuildID, &s.GuildName, &s.IconURL, &s.AuraEffect, &s.LoginMessage); err != nil {
				return err
			}
			// An entry recorded after now may leave a visible sample; never list a character twice.
			if !seen[s.CharacterID] {
				seen[s.CharacterID] = true
				rows = append(rows, s)
			}
		}
		return res.Err()
	})
	return rows, err
}

// deleteBanned reads banned owners without locking characters/players, then deletes
// only their snapshot rows, so observation never waits on a character lock held by a publisher.
func (r *PresenceRepository) deleteBanned(ctx context.Context, exec sqlContextExecutor) error {
	res, err := exec.QueryContext(ctx, `
		SELECT s.seq FROM character_presence_snapshots s
		JOIN characters c ON c.id = s.character_id
		JOIN players p ON p.id = c.player_id
		WHERE p.banned_at IS NOT NULL`)
	if err != nil {
		return err
	}
	var seqs []any
	for res.Next() {
		var seq int64
		if err := res.Scan(&seq); err != nil {
			res.Close() //lint:ignore error-swallow scan error is returned instead
			return err
		}
		seqs = append(seqs, seq)
	}
	if err := res.Close(); err != nil {
		return err
	}
	if err := res.Err(); err != nil || len(seqs) == 0 {
		return err
	}
	_, err = exec.ExecContext(ctx, `DELETE FROM character_presence_snapshots WHERE seq IN (?`+strings.Repeat(",?", len(seqs)-1)+`)`, seqs...)
	return err
}

func (r *PresenceRepository) GetPresenceState(ctx context.Context, characterID string) (character.PresenceState, error) {
	var state character.PresenceState
	var sampled sql.NullInt64
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx,
		`SELECT login_message, presence_sampled_at FROM characters WHERE id = ?`, characterID).Scan(&state.LoginMessage, &sampled)
	if errors.Is(err, sql.ErrNoRows) {
		return state, character.ErrNotFound
	}
	if sampled.Valid {
		state.SampledAt = time.Unix(sampled.Int64, 0).UTC()
	}
	return state, err
}
