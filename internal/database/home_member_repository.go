package database

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/witchcraze/party2re/internal/god"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/monster"
)

// HomeMemberRepository manages home members and player home/avatar customizations.
type HomeMemberRepository struct {
	db *sql.DB
}

// NewHomeMemberRepository creates a new HomeMemberRepository.
func NewHomeMemberRepository(db *sql.DB) (*HomeMemberRepository, error) {
	if db == nil {
		return nil, errors.New("db cannot be nil")
	}
	return &HomeMemberRepository{db: db}, nil
}

// FindByCharacterID retrieves all home members for a character.
func (r *HomeMemberRepository) FindByCharacterID(ctx context.Context, characterID string) ([]god.HomeMember, error) {
	executor := ExecutorFromContext(ctx, r.db)
	rows, err := executor.QueryContext(ctx, `
		SELECT id, character_id, is_npc, name, icon, color, created_at
		FROM home_members
		WHERE character_id = ?
		ORDER BY created_at ASC, id ASC
	`, characterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []god.HomeMember
	for rows.Next() {
		var m god.HomeMember
		if err := rows.Scan(&m.ID, &m.CharacterID, &m.IsNPC, &m.Name, &m.Icon, &m.Color, &m.CreatedAt); err != nil {
			return nil, err
		}
		members = append(members, m)
	}
	return members, rows.Err()
}

// FindByIDForUpdate retrieves and locks a single home member by ID.
func (r *HomeMemberRepository) FindByIDForUpdate(ctx context.Context, id string) (god.HomeMember, error) {
	executor := ExecutorFromContext(ctx, r.db)
	row := executor.QueryRowContext(ctx, `
		SELECT id, character_id, is_npc, name, icon, color, created_at
		FROM home_members
		WHERE id = ?
		FOR UPDATE
	`, id)
	var m god.HomeMember
	if err := row.Scan(&m.ID, &m.CharacterID, &m.IsNPC, &m.Name, &m.Icon, &m.Color, &m.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return god.HomeMember{}, monster.ErrMonsterNotFound
		}
		return god.HomeMember{}, err
	}
	return m, nil
}

// Delete removes a home member by ID.
func (r *HomeMemberRepository) Delete(ctx context.Context, id string) error {
	executor := ExecutorFromContext(ctx, r.db)
	_, err := executor.ExecContext(ctx, `
		DELETE FROM home_members
		WHERE id = ?
	`, id)
	return err
}

// ListHomeCompanions retrieves all NPC companions for a character from home_members.
func (r *HomeMemberRepository) ListHomeCompanions(ctx context.Context, characterID string) ([]monster.HomeCompanion, error) {
	members, err := r.FindByCharacterID(ctx, characterID)
	if err != nil {
		return nil, err
	}
	var companions []monster.HomeCompanion
	for _, m := range members {
		if !m.IsNPC {
			continue
		}
		companions = append(companions, monster.HomeCompanion{
			ID:          m.ID,
			CharacterID: m.CharacterID,
			MonsterID:   m.Icon,
			CustomName:  m.Name,
			CreatedAt:   m.CreatedAt,
		})
	}
	return companions, nil
}

// FindHomeCompanionForUpdate retrieves and locks an NPC companion from home_members.
func (r *HomeMemberRepository) FindHomeCompanionForUpdate(ctx context.Context, id string) (monster.HomeCompanion, error) {
	m, err := r.FindByIDForUpdate(ctx, id)
	if err != nil {
		return monster.HomeCompanion{}, err
	}
	if !m.IsNPC {
		return monster.HomeCompanion{}, monster.ErrMonsterNotFound
	}
	return monster.HomeCompanion{
		ID:          m.ID,
		CharacterID: m.CharacterID,
		MonsterID:   m.Icon,
		CustomName:  m.Name,
		CreatedAt:   m.CreatedAt,
	}, nil
}

// DeleteHomeCompanion deletes an NPC companion from home_members.
func (r *HomeMemberRepository) DeleteHomeCompanion(ctx context.Context, id string) error {
	return r.Delete(ctx, id)
}

// AddMember adds a new resident or NPC companion to the character's home.
func (r *HomeMemberRepository) AddMember(ctx context.Context, member god.HomeMember) error {
	executor := ExecutorFromContext(ctx, r.db)
	if member.ID == "" {
		member.ID = id.New()
	}
	if member.CreatedAt.IsZero() {
		member.CreatedAt = time.Now().UTC()
	}
	_, err := executor.ExecContext(ctx, `
		INSERT INTO home_members (id, character_id, is_npc, name, icon, color, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, member.ID, member.CharacterID, member.IsNPC, member.Name, member.Icon, member.Color, member.CreatedAt)
	return err
}

// UpdateBgimg updates the background image of the character's home.
func (r *HomeMemberRepository) UpdateBgimg(ctx context.Context, characterID string, bgimg string) error {
	executor := ExecutorFromContext(ctx, r.db)
	_, err := executor.ExecContext(ctx, `
		UPDATE character_homes SET bgimg = ? WHERE character_id = ?
	`, bgimg, characterID)
	return err
}

// UpdateAvatar updates the character profile avatar URL.
func (r *HomeMemberRepository) UpdateAvatar(ctx context.Context, characterID string, avatarURL string) error {
	executor := ExecutorFromContext(ctx, r.db)
	_, err := executor.ExecContext(ctx, `
		INSERT INTO character_profiles (character_id, avatar_url)
		VALUES (?, ?)
		ON DUPLICATE KEY UPDATE avatar_url = VALUES(avatar_url)
	`, characterID, avatarURL)
	return err
}
