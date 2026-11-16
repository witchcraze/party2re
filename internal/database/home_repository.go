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
	var expiresAt sql.NullTime

	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT character_id, town_id, house_style, expires_at, bgimg, updated_at
		FROM character_homes
		WHERE character_id = ?
	`, characterID).Scan(&h.CharacterID, &h.TownID, &h.HouseStyle, &expiresAt, &h.BgImg, &h.UpdatedAt)

	if errors.Is(err, sql.ErrNoRows) {
		return home.CharacterHome{
			CharacterID: characterID,
			TownID:      "",
			HouseStyle:  "",
			ExpiresAt:   nil,
			BgImg:       "",
			UpdatedAt:   time.Now().UTC(),
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
			character_id, town_id, house_style, expires_at, bgimg, updated_at
		) VALUES (?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			town_id = VALUES(town_id),
			house_style = VALUES(house_style),
			expires_at = VALUES(expires_at),
			bgimg = VALUES(bgimg),
			updated_at = VALUES(updated_at)
	`, h.CharacterID, h.TownID, h.HouseStyle, expiresAt, h.BgImg, h.UpdatedAt.UTC())
	return err
}

// LockTownHouseCapacityForUpdate takes the town's house capacity row lock, creating the row if absent.
func (r *HomeRepository) LockTownHouseCapacityForUpdate(ctx context.Context, townID string) error {
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		INSERT INTO house_town_capacity_lock (town_id) VALUES (?)
		ON DUPLICATE KEY UPDATE town_id = town_id
	`, townID)
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

// ListActiveTownHouses returns the active houses in a town with their owners' names, soonest expiry first.
func (r *HomeRepository) ListActiveTownHouses(ctx context.Context, townID string, now time.Time) ([]home.TownHouse, error) {
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, `
		SELECT h.character_id, c.name, h.town_id, h.house_style, h.expires_at
		FROM character_homes h
		JOIN characters c ON c.id = h.character_id
		WHERE h.town_id = ? AND h.expires_at > ?
		ORDER BY h.expires_at ASC, h.character_id ASC
	`, townID, now.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	houses := []home.TownHouse{}
	for rows.Next() {
		var h home.TownHouse
		if err := rows.Scan(&h.CharacterID, &h.OwnerName, &h.TownID, &h.HouseStyle, &h.ExpiresAt); err != nil {
			return nil, err
		}
		houses = append(houses, h)
	}
	return houses, rows.Err()
}
