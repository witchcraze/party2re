package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/witchcraze/party2re/internal/plantation"
)

// PlantationRepository provides MariaDB persistence for plantation plots.
type PlantationRepository struct {
	db *sql.DB
}

// NewPlantationRepository constructs a new PlantationRepository.
func NewPlantationRepository(db *sql.DB) (*PlantationRepository, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	return &PlantationRepository{db: db}, nil
}

// GetPlot retrieves the active plot for the specified character.
func (r *PlantationRepository) GetPlot(ctx context.Context, characterID string) (plantation.Plot, error) {
	executor := ExecutorFromContext(ctx, r.db)
	return r.scanPlot(ctx, executor, "SELECT character_id, seed_id, fertilizer_id, sown_at, matures_at, created_at, updated_at FROM plantation_plots WHERE character_id = ?", characterID)
}

// GetPlotForUpdate retrieves and locks the active plot for the specified character.
func (r *PlantationRepository) GetPlotForUpdate(ctx context.Context, characterID string) (plantation.Plot, error) {
	executor := ExecutorFromContext(ctx, r.db)
	return r.scanPlot(ctx, executor, "SELECT character_id, seed_id, fertilizer_id, sown_at, matures_at, created_at, updated_at FROM plantation_plots WHERE character_id = ? FOR UPDATE", characterID)
}

func (r *PlantationRepository) scanPlot(ctx context.Context, executor sqlContextExecutor, query string, characterID string) (plantation.Plot, error) {
	var (
		cID          string
		seedID       string
		fertilizerID sql.NullString
		sownAt       time.Time
		maturesAt    time.Time
		createdAt    time.Time
		updatedAt    time.Time
	)

	err := executor.QueryRowContext(ctx, query, characterID).Scan(
		&cID,
		&seedID,
		&fertilizerID,
		&sownAt,
		&maturesAt,
		&createdAt,
		&updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return plantation.Plot{}, plantation.ErrPlotNotFound
	}
	if err != nil {
		return plantation.Plot{}, fmt.Errorf("scan plantation plot: %w", err)
	}

	var fertPtr *string
	if fertilizerID.Valid {
		v := fertilizerID.String
		fertPtr = &v
	}

	return plantation.Plot{
		CharacterID:  cID,
		SeedID:       seedID,
		FertilizerID: fertPtr,
		SownAt:       sownAt,
		MaturesAt:    maturesAt,
		CreatedAt:    createdAt,
		UpdatedAt:    updatedAt,
	}, nil
}

// SavePlot creates or updates a plantation plot.
func (r *PlantationRepository) SavePlot(ctx context.Context, plot plantation.Plot) error {
	executor := ExecutorFromContext(ctx, r.db)
	query := `
		INSERT INTO plantation_plots (character_id, seed_id, fertilizer_id, sown_at, matures_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			seed_id = VALUES(seed_id),
			fertilizer_id = VALUES(fertilizer_id),
			sown_at = VALUES(sown_at),
			matures_at = VALUES(matures_at),
			updated_at = VALUES(updated_at)
	`
	var fertVal any
	if plot.FertilizerID != nil {
		fertVal = *plot.FertilizerID
	}

	now := time.Now()
	createdAt := plot.CreatedAt
	if createdAt.IsZero() {
		createdAt = now
	}
	updatedAt := plot.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = now
	}

	_, err := executor.ExecContext(
		ctx,
		query,
		plot.CharacterID,
		plot.SeedID,
		fertVal,
		plot.SownAt,
		plot.MaturesAt,
		createdAt,
		updatedAt,
	)
	if err != nil {
		return fmt.Errorf("save plantation plot: %w", err)
	}
	return nil
}

// DeletePlot removes a character's plantation plot.
func (r *PlantationRepository) DeletePlot(ctx context.Context, characterID string) error {
	executor := ExecutorFromContext(ctx, r.db)
	_, err := executor.ExecContext(ctx, "DELETE FROM plantation_plots WHERE character_id = ?", characterID)
	if err != nil {
		return fmt.Errorf("delete plantation plot: %w", err)
	}
	return nil
}
