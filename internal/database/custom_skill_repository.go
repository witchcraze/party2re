package database

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/witchcraze/party2re/internal/custom_skill"
)

type CustomSkillRepository struct {
	db *sql.DB
}

func NewCustomSkillRepository(db *sql.DB) (*CustomSkillRepository, error) {
	if db == nil {
		return nil, errors.New("database connection is required")
	}
	return &CustomSkillRepository{db: db}, nil
}

func (r *CustomSkillRepository) SaveLoadout(ctx context.Context, loadout custom_skill.CharacterSkillLoadout) error {
	slotsJSON := custom_skill.EncodeJSON(loadout.Slots)
	query := `
		INSERT INTO character_custom_skills (
			character_id, max_slots, equipped_skills_json, updated_at
		) VALUES (?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			max_slots = VALUES(max_slots),
			equipped_skills_json = VALUES(equipped_skills_json),
			updated_at = VALUES(updated_at)
	`
	now := time.Now().UTC()
	if !loadout.UpdatedAt.IsZero() {
		now = loadout.UpdatedAt
	}
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, query, loadout.CharacterID, loadout.MaxSlots, slotsJSON, now)
	return err
}

func (r *CustomSkillRepository) FindLoadout(ctx context.Context, characterID string) (*custom_skill.CharacterSkillLoadout, error) {
	query := `
		SELECT character_id, max_slots, equipped_skills_json, updated_at
		FROM character_custom_skills
		WHERE character_id = ?
	`
	var (
		charID    string
		maxSlots  int
		slotsJSON string
		updatedAt time.Time
	)
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, query, characterID).Scan(
		&charID,
		&maxSlots,
		&slotsJSON,
		&updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	slots, _ := custom_skill.DecodeJSON[[]custom_skill.EquippedSkillSlot](slotsJSON)
	return &custom_skill.CharacterSkillLoadout{
		CharacterID: charID,
		MaxSlots:    maxSlots,
		Slots:       slots,
		UpdatedAt:   updatedAt,
	}, nil
}
