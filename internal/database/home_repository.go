package database

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/witchcraze/party2re/internal/home"
)

type HomeRepository struct {
	db *sql.DB
}

func NewHomeRepository(db *sql.DB) (*HomeRepository, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	return &HomeRepository{db: db}, nil
}

// GetHome retrieves home settings for a character or returns defaults if not yet created.
func (r *HomeRepository) GetHome(ctx context.Context, characterID string) (home.CharacterHome, error) {
	var h home.CharacterHome
	var lastVisited sql.NullTime

	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT character_id, theme, motto, companion_name, visitor_count, last_visited_at, updated_at
		FROM character_homes
		WHERE character_id = ?
	`, characterID).Scan(&h.CharacterID, &h.Theme, &h.Motto, &h.CompanionName, &h.VisitorCount, &lastVisited, &h.UpdatedAt)

	if errors.Is(err, sql.ErrNoRows) {
		return home.CharacterHome{
			CharacterID:   characterID,
			Theme:         home.DefaultTheme,
			Motto:         "",
			CompanionName: home.DefaultCompanionName,
			VisitorCount:  0,
			LastVisitedAt: nil,
			UpdatedAt:     time.Now().UTC(),
		}, nil
	}
	if err != nil {
		return home.CharacterHome{}, err
	}

	if lastVisited.Valid {
		t := lastVisited.Time
		h.LastVisitedAt = &t
	}

	return h, nil
}

// SaveHome inserts or updates home settings for a character.
func (r *HomeRepository) SaveHome(ctx context.Context, h home.CharacterHome) error {
	var lastVisited sql.NullTime
	if h.LastVisitedAt != nil {
		lastVisited = sql.NullTime{Time: h.LastVisitedAt.UTC(), Valid: true}
	}

	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		INSERT INTO character_homes (
			character_id, theme, motto, companion_name, visitor_count, last_visited_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			theme = VALUES(theme),
			motto = VALUES(motto),
			companion_name = VALUES(companion_name),
			visitor_count = VALUES(visitor_count),
			last_visited_at = VALUES(last_visited_at),
			updated_at = VALUES(updated_at)
	`, h.CharacterID, h.Theme, h.Motto, h.CompanionName, h.VisitorCount, lastVisited, h.UpdatedAt.UTC())
	return err
}

// IncrementVisitorCount increments visitor count and updates last_visited_at.
func (r *HomeRepository) IncrementVisitorCount(ctx context.Context, characterID string, visitedAt time.Time) error {
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		INSERT INTO character_homes (
			character_id, theme, motto, companion_name, visitor_count, last_visited_at, updated_at
		) VALUES (?, ?, '', ?, 1, ?, ?)
		ON DUPLICATE KEY UPDATE
			visitor_count = visitor_count + 1,
			last_visited_at = VALUES(last_visited_at),
			updated_at = VALUES(updated_at)
	`, characterID, home.DefaultTheme, home.DefaultCompanionName, visitedAt.UTC(), visitedAt.UTC())
	return err
}

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

// ListInboxLetters retrieves letters received by a character that have not been deleted by the recipient.
func (r *HomeRepository) ListInboxLetters(ctx context.Context, recipientID string, limit, offset int) ([]home.Letter, int, error) {
	var total int
	executor := ExecutorFromContext(ctx, r.db)
	err := executor.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM character_letters
		WHERE recipient_character_id = ? AND is_deleted_by_recipient = FALSE
	`, recipientID).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := executor.QueryContext(ctx, `
		SELECT id, sender_character_id, sender_name, recipient_character_id, recipient_name,
		       content, color, is_read, read_at, is_deleted_by_sender, is_deleted_by_recipient, created_at
		FROM character_letters
		WHERE recipient_character_id = ? AND is_deleted_by_recipient = FALSE
		ORDER BY created_at DESC
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
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return letters, total, nil
}

// ListOutboxLetters retrieves letters sent by a character that have not been deleted by the sender.
func (r *HomeRepository) ListOutboxLetters(ctx context.Context, senderID string, limit, offset int) ([]home.Letter, int, error) {
	var total int
	executor := ExecutorFromContext(ctx, r.db)
	err := executor.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM character_letters
		WHERE sender_character_id = ? AND is_deleted_by_sender = FALSE
	`, senderID).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := executor.QueryContext(ctx, `
		SELECT id, sender_character_id, sender_name, recipient_character_id, recipient_name,
		       content, color, is_read, read_at, is_deleted_by_sender, is_deleted_by_recipient, created_at
		FROM character_letters
		WHERE sender_character_id = ? AND is_deleted_by_sender = FALSE
		ORDER BY created_at DESC
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
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return letters, total, nil
}

// GetUnreadLetterCount returns count of active unread letters for a recipient.
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
	var senderID, recipientID string
	var isDeletedSender, isDeletedRecipient bool
	executor := ExecutorFromContext(ctx, r.db)

	err := executor.QueryRowContext(ctx, `
		SELECT sender_character_id, recipient_character_id, is_deleted_by_sender, is_deleted_by_recipient
		FROM character_letters
		WHERE id = ?
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
		if isDeletedSender && !isDeletedRecipient {
			return home.ErrLetterNotFound
		}
		isDeletedSender = true
	}
	if characterID == recipientID {
		if isDeletedRecipient && !isDeletedSender {
			return home.ErrLetterNotFound
		}
		isDeletedRecipient = true
	}

	if isDeletedSender && isDeletedRecipient {
		_, err = executor.ExecContext(ctx, `DELETE FROM character_letters WHERE id = ?`, id)
		return err
	}

	_, err = executor.ExecContext(ctx, `
		UPDATE character_letters
		SET is_deleted_by_sender = ?, is_deleted_by_recipient = ?
		WHERE id = ?
	`, isDeletedSender, isDeletedRecipient, id)
	return err
}

// AddCompanionPhrase adds a taught phrase for a character's companion.
func (r *HomeRepository) AddCompanionPhrase(ctx context.Context, phrase home.CompanionPhrase) error {
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		INSERT INTO character_companion_phrases (id, character_id, phrase, created_at)
		VALUES (?, ?, ?, ?)
	`, phrase.ID, phrase.CharacterID, phrase.Phrase, phrase.CreatedAt.UTC())
	return err
}

// DeleteCompanionPhrase removes a taught phrase.
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
		return home.ErrPhraseNotFound
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
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return phrases, nil
}

// AddDeliveryNotice records a new delivery notice.
func (r *HomeRepository) AddDeliveryNotice(ctx context.Context, notice home.DeliveryNotice) error {
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		INSERT INTO character_delivery_notices (id, character_id, notice_type, message, is_cleared, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, notice.ID, notice.CharacterID, notice.NoticeType, notice.Message, notice.IsCleared, notice.CreatedAt.UTC())
	return err
}

// ListDeliveryNotices lists delivery notices for a character.
func (r *HomeRepository) ListDeliveryNotices(ctx context.Context, characterID string, unclearedOnly bool) ([]home.DeliveryNotice, error) {
	var query string
	if unclearedOnly {
		query = `
			SELECT id, character_id, notice_type, message, is_cleared, created_at
			FROM character_delivery_notices
			WHERE character_id = ? AND is_cleared = FALSE
			ORDER BY created_at DESC
		`
	} else {
		query = `
			SELECT id, character_id, notice_type, message, is_cleared, created_at
			FROM character_delivery_notices
			WHERE character_id = ?
			ORDER BY created_at DESC
		`
	}

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
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return notices, nil
}

// ClearDeliveryNotices marks all delivery notices as cleared.
func (r *HomeRepository) ClearDeliveryNotices(ctx context.Context, characterID string) error {
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		UPDATE character_delivery_notices
		SET is_cleared = TRUE
		WHERE character_id = ? AND is_cleared = FALSE
	`, characterID)
	return err
}
