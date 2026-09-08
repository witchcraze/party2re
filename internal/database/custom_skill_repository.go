package database

import (
	"context"
	"database/sql"
	"errors"

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
