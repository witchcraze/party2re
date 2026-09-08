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

func (r *CustomSkillRepository) SaveCustomSkill(ctx context.Context, skill custom_skill.CustomSkill) error {
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		INSERT INTO character_custom_skills
			(character_id, skill_name, skill_comment, need_cmp, gem1, gem2, gem3, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			skill_name = VALUES(skill_name), skill_comment = VALUES(skill_comment),
			need_cmp = VALUES(need_cmp), gem1 = VALUES(gem1), gem2 = VALUES(gem2),
			gem3 = VALUES(gem3), updated_at = VALUES(updated_at)
	`, skill.CharacterID, skill.Name, skill.Comment, skill.CMP,
		skill.Gems[0], skill.Gems[1], skill.Gems[2], skill.UpdatedAt)
	return err
}

func (r *CustomSkillRepository) FindCustomSkill(ctx context.Context, characterID string) (*custom_skill.CustomSkill, error) {
	var skill custom_skill.CustomSkill
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT character_id, skill_name, skill_comment, need_cmp, gem1, gem2, gem3, updated_at
		FROM character_custom_skills WHERE character_id = ?
	`, characterID).Scan(&skill.CharacterID, &skill.Name, &skill.Comment, &skill.CMP,
		&skill.Gems[0], &skill.Gems[1], &skill.Gems[2], &skill.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &skill, nil
}
