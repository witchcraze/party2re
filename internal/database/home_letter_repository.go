package database

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/witchcraze/party2re/internal/home"
)

// CreateLetter persists a new player-to-player letter.
func (r *HomeRepository) CreateLetter(ctx context.Context, letter home.Letter) error {
	var readAt sql.NullTime
	if letter.ReadAt != nil {
		readAt = sql.NullTime{Time: letter.ReadAt.UTC(), Valid: true}
	}

	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		INSERT INTO character_letters (
			id, sender_character_id, sender_name, recipient_character_id, recipient_name,
			content, color, is_read, read_at, is_deleted_by_sender, is_deleted_by_recipient, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, letter.ID, letter.SenderCharacterID, letter.SenderName, letter.RecipientCharacterID, letter.RecipientName,
		letter.Content, letter.Color, letter.IsRead, readAt, letter.IsDeletedBySender, letter.IsDeletedByRecipient, letter.CreatedAt.UTC())
	return err
}

// GetLetterByID retrieves a single letter by ID.
func (r *HomeRepository) GetLetterByID(ctx context.Context, id string) (home.Letter, error) {
	var l home.Letter
	var readAt sql.NullTime

	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT id, sender_character_id, sender_name, recipient_character_id, recipient_name,
		       content, color, is_read, read_at, is_deleted_by_sender, is_deleted_by_recipient, created_at
		FROM character_letters
		WHERE id = ?
	`, id).Scan(&l.ID, &l.SenderCharacterID, &l.SenderName, &l.RecipientCharacterID, &l.RecipientName,
		&l.Content, &l.Color, &l.IsRead, &readAt, &l.IsDeletedBySender, &l.IsDeletedByRecipient, &l.CreatedAt)

	if errors.Is(err, sql.ErrNoRows) {
		return home.Letter{}, home.ErrLetterNotFound
	}
	if err != nil {
		return home.Letter{}, err
	}

	if readAt.Valid {
		t := readAt.Time
		l.ReadAt = &t
	}

	return l, nil
}

// ListInboxLetters retrieves received letters for a character, excluding ones deleted by recipient.
func (r *HomeRepository) ListInboxLetters(ctx context.Context, recipientID string, limit, offset int) ([]home.Letter, int, error) {
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	var total int
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM character_letters
		WHERE recipient_character_id = ? AND is_deleted_by_recipient = FALSE
	`, recipientID).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, `
		SELECT id, sender_character_id, sender_name, recipient_character_id, recipient_name,
		       content, color, is_read, read_at, is_deleted_by_sender, is_deleted_by_recipient, created_at
		FROM character_letters
		WHERE recipient_character_id = ? AND is_deleted_by_recipient = FALSE
		ORDER BY created_at DESC, id DESC
		LIMIT ? OFFSET ?
	`, recipientID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var letters []home.Letter
	for rows.Next() {
		var l home.Letter
		var readAt sql.NullTime
		if err := rows.Scan(&l.ID, &l.SenderCharacterID, &l.SenderName, &l.RecipientCharacterID, &l.RecipientName,
			&l.Content, &l.Color, &l.IsRead, &readAt, &l.IsDeletedBySender, &l.IsDeletedByRecipient, &l.CreatedAt); err != nil {
			return nil, 0, err
		}
		if readAt.Valid {
			t := readAt.Time
			l.ReadAt = &t
		}
		letters = append(letters, l)
	}

	return letters, total, rows.Err()
}

// ListInboxLettersByCursor retrieves received letters using keyset pagination.
func (r *HomeRepository) ListInboxLettersByCursor(ctx context.Context, recipientID string, limit int, beforeTime time.Time, beforeID string) ([]home.Letter, error) {
	if limit <= 0 {
		limit = 20
	}

	executor := ExecutorFromContext(ctx, r.db)
	var rows *sql.Rows
	var err error

	if beforeTime.IsZero() {
		rows, err = executor.QueryContext(ctx, `
			SELECT id, sender_character_id, sender_name, recipient_character_id, recipient_name,
			       content, color, is_read, read_at, is_deleted_by_sender, is_deleted_by_recipient, created_at
			FROM character_letters
			WHERE recipient_character_id = ? AND is_deleted_by_recipient = FALSE
			ORDER BY created_at DESC, id DESC
			LIMIT ?
		`, recipientID, limit)
	} else {
		rows, err = executor.QueryContext(ctx, `
			SELECT id, sender_character_id, sender_name, recipient_character_id, recipient_name,
			       content, color, is_read, read_at, is_deleted_by_sender, is_deleted_by_recipient, created_at
			FROM character_letters
			WHERE recipient_character_id = ? AND is_deleted_by_recipient = FALSE
			  AND (created_at < ? OR (created_at = ? AND id < ?))
			ORDER BY created_at DESC, id DESC
			LIMIT ?
		`, recipientID, beforeTime.UTC(), beforeTime.UTC(), beforeID, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var letters []home.Letter
	for rows.Next() {
		var l home.Letter
		var readAt sql.NullTime
		if err := rows.Scan(&l.ID, &l.SenderCharacterID, &l.SenderName, &l.RecipientCharacterID, &l.RecipientName,
			&l.Content, &l.Color, &l.IsRead, &readAt, &l.IsDeletedBySender, &l.IsDeletedByRecipient, &l.CreatedAt); err != nil {
			return nil, err
		}
		if readAt.Valid {
			t := readAt.Time
			l.ReadAt = &t
		}
		letters = append(letters, l)
	}

	return letters, rows.Err()
}

// ListOutboxLetters retrieves sent letters for a character, excluding ones deleted by sender.
func (r *HomeRepository) ListOutboxLetters(ctx context.Context, senderID string, limit, offset int) ([]home.Letter, int, error) {
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	var total int
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM character_letters
		WHERE sender_character_id = ? AND is_deleted_by_sender = FALSE
	`, senderID).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, `
		SELECT id, sender_character_id, sender_name, recipient_character_id, recipient_name,
		       content, color, is_read, read_at, is_deleted_by_sender, is_deleted_by_recipient, created_at
		FROM character_letters
		WHERE sender_character_id = ? AND is_deleted_by_sender = FALSE
		ORDER BY created_at DESC, id DESC
		LIMIT ? OFFSET ?
	`, senderID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var letters []home.Letter
	for rows.Next() {
		var l home.Letter
		var readAt sql.NullTime
		if err := rows.Scan(&l.ID, &l.SenderCharacterID, &l.SenderName, &l.RecipientCharacterID, &l.RecipientName,
			&l.Content, &l.Color, &l.IsRead, &readAt, &l.IsDeletedBySender, &l.IsDeletedByRecipient, &l.CreatedAt); err != nil {
			return nil, 0, err
		}
		if readAt.Valid {
			t := readAt.Time
			l.ReadAt = &t
		}
		letters = append(letters, l)
	}

	return letters, total, rows.Err()
}

// ListOutboxLettersByCursor retrieves sent letters using keyset pagination.
func (r *HomeRepository) ListOutboxLettersByCursor(ctx context.Context, senderID string, limit int, beforeTime time.Time, beforeID string) ([]home.Letter, error) {
	if limit <= 0 {
		limit = 20
	}

	executor := ExecutorFromContext(ctx, r.db)
	var rows *sql.Rows
	var err error

	if beforeTime.IsZero() {
		rows, err = executor.QueryContext(ctx, `
			SELECT id, sender_character_id, sender_name, recipient_character_id, recipient_name,
			       content, color, is_read, read_at, is_deleted_by_sender, is_deleted_by_recipient, created_at
			FROM character_letters
			WHERE sender_character_id = ? AND is_deleted_by_sender = FALSE
			ORDER BY created_at DESC, id DESC
			LIMIT ?
		`, senderID, limit)
	} else {
		rows, err = executor.QueryContext(ctx, `
			SELECT id, sender_character_id, sender_name, recipient_character_id, recipient_name,
			       content, color, is_read, read_at, is_deleted_by_sender, is_deleted_by_recipient, created_at
			FROM character_letters
			WHERE sender_character_id = ? AND is_deleted_by_sender = FALSE
			  AND (created_at < ? OR (created_at = ? AND id < ?))
			ORDER BY created_at DESC, id DESC
			LIMIT ?
		`, senderID, beforeTime.UTC(), beforeTime.UTC(), beforeID, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var letters []home.Letter
	for rows.Next() {
		var l home.Letter
		var readAt sql.NullTime
		if err := rows.Scan(&l.ID, &l.SenderCharacterID, &l.SenderName, &l.RecipientCharacterID, &l.RecipientName,
			&l.Content, &l.Color, &l.IsRead, &readAt, &l.IsDeletedBySender, &l.IsDeletedByRecipient, &l.CreatedAt); err != nil {
			return nil, err
		}
		if readAt.Valid {
			t := readAt.Time
			l.ReadAt = &t
		}
		letters = append(letters, l)
	}

	return letters, rows.Err()
}

// GetUnreadLetterCount returns the number of unread received letters for a character.
func (r *HomeRepository) GetUnreadLetterCount(ctx context.Context, recipientID string) (int, error) {
	var count int
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM character_letters
		WHERE recipient_character_id = ? AND is_read = FALSE AND is_deleted_by_recipient = FALSE
	`, recipientID).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// MarkLetterAsRead marks a letter as read.
func (r *HomeRepository) MarkLetterAsRead(ctx context.Context, id, recipientID string, readAt time.Time) error {
	executor := ExecutorFromContext(ctx, r.db)
	res, err := executor.ExecContext(ctx, `
		UPDATE character_letters
		SET is_read = TRUE, read_at = ?
		WHERE id = ? AND recipient_character_id = ? AND is_deleted_by_recipient = FALSE
	`, readAt.UTC(), id, recipientID)
	if err != nil {
		return err
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		var actualRecipient string
		var isDeleted bool
		err = executor.QueryRowContext(ctx, `SELECT recipient_character_id, is_deleted_by_recipient FROM character_letters WHERE id = ?`, id).Scan(&actualRecipient, &isDeleted)
		if errors.Is(err, sql.ErrNoRows) || isDeleted {
			return home.ErrLetterNotFound
		}
		if err != nil {
			return err
		}
		if actualRecipient != recipientID {
			return home.ErrForbidden
		}
	}
	return nil
}

// DeleteLetter marks a letter as deleted for the given character (sender or recipient), and physically purges when deleted by both.
func (r *HomeRepository) DeleteLetter(ctx context.Context, id, characterID string) error {
	return RunInTx(ctx, r.db, func(txCtx context.Context) error {
		var senderID, recipientID string
		var isDeletedSender, isDeletedRecipient bool
		executor := ExecutorFromContext(txCtx, r.db)

		err := executor.QueryRowContext(txCtx, `
			SELECT sender_character_id, recipient_character_id, is_deleted_by_sender, is_deleted_by_recipient
			FROM character_letters
			WHERE id = ?
			FOR UPDATE
		`, id).Scan(&senderID, &recipientID, &isDeletedSender, &isDeletedRecipient)

		if errors.Is(err, sql.ErrNoRows) {
			return home.ErrLetterNotFound
		}
		if err != nil {
			return err
		}

		if characterID != senderID && characterID != recipientID {
			return home.ErrForbidden
		}

		if characterID == senderID {
			if isDeletedSender {
				return home.ErrLetterNotFound
			}
			isDeletedSender = true
		}
		if characterID == recipientID {
			if isDeletedRecipient {
				return home.ErrLetterNotFound
			}
			isDeletedRecipient = true
		}

		if isDeletedSender && isDeletedRecipient {
			_, err = executor.ExecContext(txCtx, `DELETE FROM character_letters WHERE id = ?`, id)
			return err
		}

		_, err = executor.ExecContext(txCtx, `
			UPDATE character_letters
			SET is_deleted_by_sender = ?, is_deleted_by_recipient = ?
			WHERE id = ?
		`, isDeletedSender, isDeletedRecipient, id)
		return err
	})
}
