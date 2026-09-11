package database

import (
	"context"
	"database/sql"
	"errors"
	"github.com/witchcraze/party2re/internal/blackmarket"
)

type BlackMarketRepository struct {
	db *sql.DB
}

func NewBlackMarketRepository(db *sql.DB) (*BlackMarketRepository, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	return &BlackMarketRepository{db: db}, nil
}

func (r *BlackMarketRepository) GetCharacterPoints(ctx context.Context, characterID string) (blackmarket.CharacterPoints, error) {
	return r.getCharacterPointsWithQuery(ctx, `
		SELECT character_id, rare_points, u_rare_points
		FROM blackmarket_character_points
		WHERE character_id = ?
	`, characterID)
}

func (r *BlackMarketRepository) GetCharacterPointsForUpdate(ctx context.Context, characterID string) (blackmarket.CharacterPoints, error) {
	return r.getCharacterPointsWithQuery(ctx, `
		SELECT character_id, rare_points, u_rare_points
		FROM blackmarket_character_points
		WHERE character_id = ?
		FOR UPDATE
	`, characterID)
}

func (r *BlackMarketRepository) getCharacterPointsWithQuery(ctx context.Context, query string, characterID string) (blackmarket.CharacterPoints, error) {
	var pts blackmarket.CharacterPoints
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, query, characterID).Scan(
		&pts.CharacterID, &pts.RarePoints, &pts.URarePoints,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return blackmarket.CharacterPoints{CharacterID: characterID, RarePoints: 0, URarePoints: 0}, nil
	}
	if err != nil {
		return blackmarket.CharacterPoints{}, err
	}
	return pts, nil
}

func (r *BlackMarketRepository) SaveCharacterPoints(ctx context.Context, points blackmarket.CharacterPoints) error {
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		INSERT INTO blackmarket_character_points (character_id, rare_points, u_rare_points)
		VALUES (?, ?, ?)
		ON DUPLICATE KEY UPDATE
			rare_points = VALUES(rare_points),
			u_rare_points = VALUES(u_rare_points)
	`, points.CharacterID, points.RarePoints, points.URarePoints)
	return err
}
