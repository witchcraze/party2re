package database

import (
	"context"
	"database/sql"
	"errors"

	"github.com/witchcraze/party2re/internal/adventure"
)

func (r *AdventureRepository) GetRecordUnlocks(ctx context.Context, characterID string) (adventure.RecordUnlocks, error) {
	var u adventure.RecordUnlocks
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT try_mode, calm_mode, hard_mode, image, avatar
		FROM character_adventure_unlocks WHERE character_id = ?`, characterID).
		Scan(&u.Try, &u.Calm, &u.Hard, &u.Image, &u.Avatar)
	if errors.Is(err, sql.ErrNoRows) {
		return adventure.RecordUnlocks{}, nil
	}
	return u, err
}

func (r *AdventureRepository) SaveRecordUnlocks(ctx context.Context, characterID string, u adventure.RecordUnlocks) error {
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		INSERT INTO character_adventure_unlocks (character_id, try_mode, calm_mode, hard_mode, image, avatar)
		VALUES (?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE try_mode = VALUES(try_mode), calm_mode = VALUES(calm_mode),
			hard_mode = VALUES(hard_mode), image = VALUES(image), avatar = VALUES(avatar)`,
		characterID, u.Try, u.Calm, u.Hard, u.Image, u.Avatar)
	return err
}
