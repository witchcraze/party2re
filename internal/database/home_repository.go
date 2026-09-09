package database

import (
	"context"
	"database/sql"
	"errors"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
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
	var expiresAt sql.NullTime

	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT character_id, town_id, house_style, expires_at, companion_name, bgimg, updated_at
		FROM character_homes
		WHERE character_id = ?
	`, characterID).Scan(&h.CharacterID, &h.TownID, &h.HouseStyle, &expiresAt, &h.CompanionName, &h.BgImg, &h.UpdatedAt)

	if errors.Is(err, sql.ErrNoRows) {
		return home.CharacterHome{
			CharacterID:   characterID,
			TownID:        "",
			HouseStyle:    "",
			ExpiresAt:     nil,
			CompanionName: home.DefaultCompanionName,
			BgImg:         "",
			UpdatedAt:     time.Now().UTC(),
		}, nil
	}
	if err != nil {
		return home.CharacterHome{}, err
	}

	if expiresAt.Valid {
		t := expiresAt.Time
		h.ExpiresAt = &t
	}

	return h, nil
}

// SaveHome inserts or updates home settings for a character.
func (r *HomeRepository) SaveHome(ctx context.Context, h home.CharacterHome) error {
	var expiresAt sql.NullTime
	if h.ExpiresAt != nil {
		expiresAt = sql.NullTime{Time: h.ExpiresAt.UTC(), Valid: true}
	}

	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		INSERT INTO character_homes (
			character_id, town_id, house_style, expires_at, companion_name, bgimg, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			town_id = VALUES(town_id),
			house_style = VALUES(house_style),
			expires_at = VALUES(expires_at),
			companion_name = VALUES(companion_name),
			bgimg = VALUES(bgimg),
			updated_at = VALUES(updated_at)
	`, h.CharacterID, h.TownID, h.HouseStyle, expiresAt, h.CompanionName, h.BgImg, h.UpdatedAt.UTC())
	return err
}

// CountActiveTownHouses returns the count of active houses in the specified town.
func (r *HomeRepository) CountActiveTownHouses(ctx context.Context, townID string, now time.Time) (int, error) {
	var count int
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM character_homes
		WHERE town_id = ? AND expires_at > ?
	`, townID, now.UTC()).Scan(&count)
	return count, err
}

// FindActiveHomeByCharacterID retrieves active home details for a character if not expired.
func (r *HomeRepository) FindActiveHomeByCharacterID(ctx context.Context, characterID string, now time.Time) (home.CharacterHome, error) {
	var h home.CharacterHome
	var expiresAt sql.NullTime

	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT character_id, town_id, house_style, expires_at, companion_name, bgimg, updated_at
		FROM character_homes
		WHERE character_id = ? AND expires_at > ?
	`, characterID, now.UTC()).Scan(&h.CharacterID, &h.TownID, &h.HouseStyle, &expiresAt, &h.CompanionName, &h.BgImg, &h.UpdatedAt)

	if errors.Is(err, sql.ErrNoRows) {
		return home.CharacterHome{}, home.ErrHouseNotFound
	}
	if err != nil {
		return home.CharacterHome{}, err
	}
	if expiresAt.Valid {
		t := expiresAt.Time
		h.ExpiresAt = &t
	}
	return h, nil
}

// FindActiveHomeByCharacterName retrieves active home and owner by character name.
func (r *HomeRepository) FindActiveHomeByCharacterName(ctx context.Context, characterName string, now time.Time) (home.CharacterHome, corecharacter.Character, error) {
	var h home.CharacterHome
	var expiresAt sql.NullTime
	var charID, charName string

	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT c.id, c.name, h.town_id, h.house_style, h.expires_at, h.companion_name, h.bgimg, h.updated_at
		FROM characters c
		JOIN character_homes h ON c.id = h.character_id
		WHERE c.name = ? AND h.expires_at > ?
	`, characterName, now.UTC()).Scan(&charID, &charName, &h.TownID, &h.HouseStyle, &expiresAt, &h.CompanionName, &h.BgImg, &h.UpdatedAt)

	if errors.Is(err, sql.ErrNoRows) {
		return home.CharacterHome{}, corecharacter.Character{}, home.ErrHouseNotFound
	}
	if err != nil {
		return home.CharacterHome{}, corecharacter.Character{}, err
	}
	h.CharacterID = charID
	if expiresAt.Valid {
		t := expiresAt.Time
		h.ExpiresAt = &t
	}
	return h, corecharacter.Character{ID: charID, Name: charName}, nil
}

// ListActiveTownHouses returns all active houses in the specified town sorted by expiration ascending.
func (r *HomeRepository) ListActiveTownHouses(ctx context.Context, townID string, now time.Time) ([]home.CharacterHome, error) {
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, `
		SELECT character_id, town_id, house_style, expires_at, companion_name, bgimg, updated_at
		FROM character_homes
		WHERE town_id = ? AND expires_at > ?
		ORDER BY expires_at ASC
	`, townID, now.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var homes []home.CharacterHome
	for rows.Next() {
		var h home.CharacterHome
		var expiresAt sql.NullTime
		if err := rows.Scan(&h.CharacterID, &h.TownID, &h.HouseStyle, &expiresAt, &h.CompanionName, &h.BgImg, &h.UpdatedAt); err != nil {
			return nil, err
		}
		if expiresAt.Valid {
			t := expiresAt.Time
			h.ExpiresAt = &t
		}
		homes = append(homes, h)
	}
	return homes, rows.Err()
}
