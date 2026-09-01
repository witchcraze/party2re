package database

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/witchcraze/party2re/internal/contest"
	"github.com/witchcraze/party2re/internal/id"
)

type ContestRepository struct {
	db *sql.DB
}

func NewContestRepository(db *sql.DB) (*ContestRepository, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	return &ContestRepository{db: db}, nil
}

// Photos

func (r *ContestRepository) SavePhoto(ctx context.Context, p contest.Photo) error {
	if p.ID == "" {
		p.ID = id.New()
	}
	now := time.Now().UTC()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}
	if p.UpdatedAt.IsZero() {
		p.UpdatedAt = now
	}

	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		INSERT INTO character_photos (id, character_id, title, location, image_url, caption, metadata, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			title = VALUES(title),
			location = VALUES(location),
			image_url = VALUES(image_url),
			caption = VALUES(caption),
			metadata = VALUES(metadata),
			updated_at = VALUES(updated_at)
	`, p.ID, p.CharacterID, p.Title, p.Location, p.ImageURL, p.Caption, p.Metadata, p.CreatedAt, p.UpdatedAt)
	return err
}

func (r *ContestRepository) FindPhotoByID(ctx context.Context, id string) (contest.Photo, error) {
	row := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT id, character_id, title, location, image_url, caption, COALESCE(metadata, ''), created_at, updated_at
		FROM character_photos
		WHERE id = ?
	`, id)
	return r.scanPhoto(row)
}

func (r *ContestRepository) FindPhotoByIDForUpdate(ctx context.Context, id string) (contest.Photo, error) {
	row := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT id, character_id, title, location, image_url, caption, COALESCE(metadata, ''), created_at, updated_at
		FROM character_photos
		WHERE id = ?
		FOR UPDATE
	`, id)
	return r.scanPhoto(row)
}

func (r *ContestRepository) ListPhotosByCharacterID(ctx context.Context, characterID string) ([]contest.Photo, error) {
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, `
		SELECT id, character_id, title, location, image_url, caption, COALESCE(metadata, ''), created_at, updated_at
		FROM character_photos
		WHERE character_id = ?
		ORDER BY created_at DESC, id ASC
	`, characterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var photos []contest.Photo
	for rows.Next() {
		var p contest.Photo
		if err := rows.Scan(&p.ID, &p.CharacterID, &p.Title, &p.Location, &p.ImageURL, &p.Caption, &p.Metadata, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		photos = append(photos, p)
	}
	return photos, rows.Err()
}

func (r *ContestRepository) CountPhotosByCharacterID(ctx context.Context, characterID string) (int, error) {
	var count int
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM character_photos
		WHERE character_id = ?
	`, characterID).Scan(&count)
	return count, err
}

func (r *ContestRepository) DeletePhoto(ctx context.Context, id string) error {
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		DELETE FROM character_photos
		WHERE id = ?
	`, id)
	return err
}

func (r *ContestRepository) scanPhoto(row interface{ Scan(...any) error }) (contest.Photo, error) {
	var p contest.Photo
	err := row.Scan(&p.ID, &p.CharacterID, &p.Title, &p.Location, &p.ImageURL, &p.Caption, &p.Metadata, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contest.Photo{}, contest.ErrPhotoNotFound
		}
		return contest.Photo{}, err
	}
	return p, nil
}

// Rounds

func (r *ContestRepository) GetRoundByNumber(ctx context.Context, round int) (contest.ContestRound, error) {
	row := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT round, status, start_time, end_time, created_at, updated_at
		FROM contest_rounds
		WHERE round = ?
	`, round)
	return r.scanRound(row)
}

func (r *ContestRepository) GetRoundByNumberForUpdate(ctx context.Context, round int) (contest.ContestRound, error) {
	row := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT round, status, start_time, end_time, created_at, updated_at
		FROM contest_rounds
		WHERE round = ?
		FOR UPDATE
	`, round)
	return r.scanRound(row)
}

func (r *ContestRepository) GetActiveRound(ctx context.Context) (contest.ContestRound, error) {
	row := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT round, status, start_time, end_time, created_at, updated_at
		FROM contest_rounds
		WHERE status = 'active'
		ORDER BY round DESC
		LIMIT 1
	`)
	return r.scanRound(row)
}

func (r *ContestRepository) GetActiveRoundForUpdate(ctx context.Context) (contest.ContestRound, error) {
	row := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT round, status, start_time, end_time, created_at, updated_at
		FROM contest_rounds
		WHERE status = 'active'
		ORDER BY round DESC
		LIMIT 1
		FOR UPDATE
	`)
	return r.scanRound(row)
}

func (r *ContestRepository) GetPreparingRound(ctx context.Context) (contest.ContestRound, error) {
	row := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT round, status, start_time, end_time, created_at, updated_at
		FROM contest_rounds
		WHERE status = 'preparing'
		ORDER BY round DESC
		LIMIT 1
	`)
	return r.scanRound(row)
}

func (r *ContestRepository) GetPreparingRoundForUpdate(ctx context.Context) (contest.ContestRound, error) {
	row := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT round, status, start_time, end_time, created_at, updated_at
		FROM contest_rounds
		WHERE status = 'preparing'
		ORDER BY round DESC
		LIMIT 1
		FOR UPDATE
	`)
	return r.scanRound(row)
}

func (r *ContestRepository) GetLatestSettledRound(ctx context.Context) (contest.ContestRound, error) {
	row := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT round, status, start_time, end_time, created_at, updated_at
		FROM contest_rounds
		WHERE status = 'settled'
		ORDER BY round DESC
		LIMIT 1
	`)
	return r.scanRound(row)
}

func (r *ContestRepository) SaveRound(ctx context.Context, cr contest.ContestRound) error {
	now := time.Now().UTC()
	if cr.CreatedAt.IsZero() {
		cr.CreatedAt = now
	}
	if cr.UpdatedAt.IsZero() {
		cr.UpdatedAt = now
	}

	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		INSERT INTO contest_rounds (round, status, start_time, end_time, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			status = VALUES(status),
			start_time = VALUES(start_time),
			end_time = VALUES(end_time),
			updated_at = VALUES(updated_at)
	`, cr.Round, cr.Status, cr.StartTime, cr.EndTime, cr.CreatedAt, cr.UpdatedAt)
	return err
}

func (r *ContestRepository) scanRound(row interface{ Scan(...any) error }) (contest.ContestRound, error) {
	var cr contest.ContestRound
	err := row.Scan(&cr.Round, &cr.Status, &cr.StartTime, &cr.EndTime, &cr.CreatedAt, &cr.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contest.ContestRound{}, contest.ErrContestNotFound
		}
		return contest.ContestRound{}, err
	}
	return cr, nil
}

// Entries

func (r *ContestRepository) SaveEntry(ctx context.Context, e contest.ContestEntry) error {
	if e.ID == "" {
		e.ID = id.New()
	}
	now := time.Now().UTC()
	if e.CreatedAt.IsZero() {
		e.CreatedAt = now
	}
	if e.UpdatedAt.IsZero() {
		e.UpdatedAt = now
	}

	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		INSERT INTO contest_entries (id, round, character_id, character_name, guild_name, title, photo_id, image_url, caption, votes, ranking, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			character_name = VALUES(character_name),
			guild_name = VALUES(guild_name),
			title = VALUES(title),
			photo_id = VALUES(photo_id),
			image_url = VALUES(image_url),
			caption = VALUES(caption),
			votes = VALUES(votes),
			ranking = VALUES(ranking),
			updated_at = VALUES(updated_at)
	`, e.ID, e.Round, e.CharacterID, e.CharacterName, e.GuildName, e.Title, e.PhotoID, e.ImageURL, e.Caption, e.Votes, e.Ranking, e.CreatedAt, e.UpdatedAt)
	return err
}

func (r *ContestRepository) FindEntryByID(ctx context.Context, id string) (contest.ContestEntry, error) {
	row := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT id, round, character_id, character_name, guild_name, title, photo_id, image_url, caption, votes, ranking, created_at, updated_at
		FROM contest_entries
		WHERE id = ?
	`, id)
	return r.scanEntry(row)
}

func (r *ContestRepository) FindEntryByIDForUpdate(ctx context.Context, id string) (contest.ContestEntry, error) {
	row := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT id, round, character_id, character_name, guild_name, title, photo_id, image_url, caption, votes, ranking, created_at, updated_at
		FROM contest_entries
		WHERE id = ?
		FOR UPDATE
	`, id)
	return r.scanEntry(row)
}

func (r *ContestRepository) FindEntryByRoundAndCharacter(ctx context.Context, round int, characterID string) (contest.ContestEntry, error) {
	row := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT id, round, character_id, character_name, guild_name, title, photo_id, image_url, caption, votes, ranking, created_at, updated_at
		FROM contest_entries
		WHERE round = ? AND character_id = ?
	`, round, characterID)
	return r.scanEntry(row)
}

func (r *ContestRepository) FindEntryByRoundAndTitle(ctx context.Context, round int, title string) (contest.ContestEntry, error) {
	row := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT id, round, character_id, character_name, guild_name, title, photo_id, image_url, caption, votes, ranking, created_at, updated_at
		FROM contest_entries
		WHERE round = ? AND title = ?
	`, round, title)
	return r.scanEntry(row)
}

func (r *ContestRepository) ListEntriesByRound(ctx context.Context, round int) ([]contest.ContestEntry, error) {
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, `
		SELECT id, round, character_id, character_name, guild_name, title, photo_id, image_url, caption, votes, ranking, created_at, updated_at
		FROM contest_entries
		WHERE round = ?
		ORDER BY votes DESC, created_at ASC, id ASC
	`, round)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []contest.ContestEntry
	for rows.Next() {
		var e contest.ContestEntry
		if err := rows.Scan(&e.ID, &e.Round, &e.CharacterID, &e.CharacterName, &e.GuildName, &e.Title, &e.PhotoID, &e.ImageURL, &e.Caption, &e.Votes, &e.Ranking, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func (r *ContestRepository) CountEntriesByRound(ctx context.Context, round int) (int, error) {
	var count int
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM contest_entries
		WHERE round = ?
	`, round).Scan(&count)
	return count, err
}

func (r *ContestRepository) scanEntry(row interface{ Scan(...any) error }) (contest.ContestEntry, error) {
	var e contest.ContestEntry
	err := row.Scan(&e.ID, &e.Round, &e.CharacterID, &e.CharacterName, &e.GuildName, &e.Title, &e.PhotoID, &e.ImageURL, &e.Caption, &e.Votes, &e.Ranking, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contest.ContestEntry{}, contest.ErrEntryNotFound
		}
		return contest.ContestEntry{}, err
	}
	return e, nil
}

// Votes

func (r *ContestRepository) SaveVote(ctx context.Context, v contest.ContestVote) error {
	if v.ID == "" {
		v.ID = id.New()
	}
	now := time.Now().UTC()
	if v.CreatedAt.IsZero() {
		v.CreatedAt = now
	}

	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		INSERT INTO contest_votes (id, round, entry_id, voter_character_id, voter_character_name, comment, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, v.ID, v.Round, v.EntryID, v.VoterCharacterID, v.VoterCharacterName, v.Comment, v.CreatedAt)
	return err
}

func (r *ContestRepository) HasVotedInRound(ctx context.Context, round int, voterCharacterID string) (bool, error) {
	var count int
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM contest_votes
		WHERE round = ? AND voter_character_id = ?
	`, round, voterCharacterID).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *ContestRepository) ListVotesByRound(ctx context.Context, round int) ([]contest.ContestVote, error) {
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, `
		SELECT id, round, entry_id, voter_character_id, voter_character_name, comment, created_at
		FROM contest_votes
		WHERE round = ?
		ORDER BY created_at ASC, id ASC
	`, round)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var votes []contest.ContestVote
	for rows.Next() {
		var v contest.ContestVote
		if err := rows.Scan(&v.ID, &v.Round, &v.EntryID, &v.VoterCharacterID, &v.VoterCharacterName, &v.Comment, &v.CreatedAt); err != nil {
			return nil, err
		}
		votes = append(votes, v)
	}
	return votes, rows.Err()
}

func (r *ContestRepository) ListVotesByEntryID(ctx context.Context, entryID string) ([]contest.ContestVote, error) {
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, `
		SELECT id, round, entry_id, voter_character_id, voter_character_name, comment, created_at
		FROM contest_votes
		WHERE entry_id = ?
		ORDER BY created_at ASC, id ASC
	`, entryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var votes []contest.ContestVote
	for rows.Next() {
		var v contest.ContestVote
		if err := rows.Scan(&v.ID, &v.Round, &v.EntryID, &v.VoterCharacterID, &v.VoterCharacterName, &v.Comment, &v.CreatedAt); err != nil {
			return nil, err
		}
		votes = append(votes, v)
	}
	return votes, rows.Err()
}

func (r *ContestRepository) IncrementEntryVotes(ctx context.Context, entryID string) error {
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		UPDATE contest_entries
		SET votes = votes + 1, updated_at = UTC_TIMESTAMP()
		WHERE id = ?
	`, entryID)
	return err
}

// Legends

func (r *ContestRepository) SaveLegend(ctx context.Context, l contest.ContestLegend) error {
	now := time.Now().UTC()
	if l.SettledAt.IsZero() {
		l.SettledAt = now
	}

	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		INSERT INTO contest_legends (round, entry_id, title, character_id, character_name, guild_name, votes, image_url, caption, settled_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			entry_id = VALUES(entry_id),
			title = VALUES(title),
			character_id = VALUES(character_id),
			character_name = VALUES(character_name),
			guild_name = VALUES(guild_name),
			votes = VALUES(votes),
			image_url = VALUES(image_url),
			caption = VALUES(caption),
			settled_at = VALUES(settled_at)
	`, l.Round, l.EntryID, l.Title, l.CharacterID, l.CharacterName, l.GuildName, l.Votes, l.ImageURL, l.Caption, l.SettledAt)
	return err
}

func (r *ContestRepository) ListLegends(ctx context.Context, limit, offset int) ([]contest.ContestLegend, error) {
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, `
		SELECT round, entry_id, title, character_id, character_name, guild_name, votes, image_url, caption, settled_at
		FROM contest_legends
		ORDER BY round DESC
		LIMIT ? OFFSET ?
	`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var legends []contest.ContestLegend
	for rows.Next() {
		var l contest.ContestLegend
		if err := rows.Scan(&l.Round, &l.EntryID, &l.Title, &l.CharacterID, &l.CharacterName, &l.GuildName, &l.Votes, &l.ImageURL, &l.Caption, &l.SettledAt); err != nil {
			return nil, err
		}
		legends = append(legends, l)
	}
	return legends, rows.Err()
}
