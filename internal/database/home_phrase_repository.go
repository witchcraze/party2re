package database

import (
	"context"
	"database/sql"
	"errors"

	"github.com/witchcraze/party2re/internal/home"
)

// AddCompanionPhrase adds a taught phrase for a character's companion.
func (r *HomeRepository) AddCompanionPhrase(ctx context.Context, phrase home.CompanionPhrase) error {
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		INSERT INTO character_companion_phrases (id, character_id, phrase, created_at)
		VALUES (?, ?, ?, ?)
	`, phrase.ID, phrase.CharacterID, phrase.Phrase, phrase.CreatedAt.UTC())
	return err
}

// DeleteCompanionPhrase removes a taught phrase belonging to a character.
func (r *HomeRepository) DeleteCompanionPhrase(ctx context.Context, id, characterID string) error {
	executor := ExecutorFromContext(ctx, r.db)
	res, err := executor.ExecContext(ctx, `
		DELETE FROM character_companion_phrases
		WHERE id = ? AND character_id = ?
	`, id, characterID)
	if err != nil {
		return err
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		var actualOwner string
		err = executor.QueryRowContext(ctx, `SELECT character_id FROM character_companion_phrases WHERE id = ?`, id).Scan(&actualOwner)
		if errors.Is(err, sql.ErrNoRows) {
			return home.ErrPhraseNotFound
		}
		if err != nil {
			return err
		}
		if actualOwner != characterID {
			return home.ErrForbidden
		}
	}
	return nil
}

// ListCompanionPhrases returns all taught phrases for a character's companion.
func (r *HomeRepository) ListCompanionPhrases(ctx context.Context, characterID string) ([]home.CompanionPhrase, error) {
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, `
		SELECT id, character_id, phrase, created_at
		FROM character_companion_phrases
		WHERE character_id = ?
		ORDER BY created_at ASC
	`, characterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var phrases []home.CompanionPhrase
	for rows.Next() {
		var p home.CompanionPhrase
		if err := rows.Scan(&p.ID, &p.CharacterID, &p.Phrase, &p.CreatedAt); err != nil {
			return nil, err
		}
		phrases = append(phrases, p)
	}

	return phrases, rows.Err()
}

// AddDeliveryNotice records an incoming delivery notice for a character.
func (r *HomeRepository) AddDeliveryNotice(ctx context.Context, notice home.DeliveryNotice) error {
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		INSERT INTO character_delivery_notices (id, character_id, notice_type, message, is_cleared, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, notice.ID, notice.CharacterID, notice.NoticeType, notice.Message, notice.IsCleared, notice.CreatedAt.UTC())
	return err
}

// ListDeliveryNotices retrieves delivery notices for a character, optionally filtering uncleared notices only.
func (r *HomeRepository) ListDeliveryNotices(ctx context.Context, characterID string, unclearedOnly bool) ([]home.DeliveryNotice, error) {
	query := `
		SELECT id, character_id, notice_type, message, is_cleared, created_at
		FROM character_delivery_notices
		WHERE character_id = ?
	`
	if unclearedOnly {
		query += ` AND is_cleared = FALSE`
	}
	query += ` ORDER BY created_at DESC LIMIT 50`

	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, query, characterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var notices []home.DeliveryNotice
	for rows.Next() {
		var n home.DeliveryNotice
		if err := rows.Scan(&n.ID, &n.CharacterID, &n.NoticeType, &n.Message, &n.IsCleared, &n.CreatedAt); err != nil {
			return nil, err
		}
		notices = append(notices, n)
	}

	return notices, rows.Err()
}

// ClearDeliveryNotices marks all active delivery notices as cleared for a character.
func (r *HomeRepository) ClearDeliveryNotices(ctx context.Context, characterID string) error {
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		UPDATE character_delivery_notices
		SET is_cleared = TRUE
		WHERE character_id = ? AND is_cleared = FALSE
	`, characterID)
	return err
}
