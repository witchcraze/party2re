package database

import (
	"context"
	"database/sql"
	"errors"

	"github.com/witchcraze/party2re/internal/farm"
	"github.com/witchcraze/party2re/internal/id"
)

type FarmRepository struct {
	db *sql.DB
}

func NewFarmRepository(db *sql.DB) (*FarmRepository, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	return &FarmRepository{db: db}, nil
}

func (r *FarmRepository) GetPlots(ctx context.Context, characterID string) ([]farm.FarmPlot, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, character_id, plot_index, seed_type, status, planted_at, matures_at, wither_at, watered, fertilized, yield
		FROM farm_plots
		WHERE character_id = ?
		ORDER BY plot_index ASC
	`, characterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var plots []farm.FarmPlot
	for rows.Next() {
		var p farm.FarmPlot
		var plantedAt, maturesAt, witherAt sql.NullTime
		if err := rows.Scan(&p.ID, &p.CharacterID, &p.PlotIndex, &p.SeedType, &p.Status, &plantedAt, &maturesAt, &witherAt, &p.Watered, &p.Fertilized, &p.Yield); err != nil {
			return nil, err
		}
		if plantedAt.Valid {
			p.PlantedAt = &plantedAt.Time
		}
		if maturesAt.Valid {
			p.MaturesAt = &maturesAt.Time
		}
		if witherAt.Valid {
			p.WitherAt = &witherAt.Time
		}
		plots = append(plots, p)
	}
	return plots, rows.Err()
}

func (r *FarmRepository) GetPlot(ctx context.Context, characterID string, plotIndex int) (farm.FarmPlot, error) {
	var p farm.FarmPlot
	var plantedAt, maturesAt, witherAt sql.NullTime
	err := r.db.QueryRowContext(ctx, `
		SELECT id, character_id, plot_index, seed_type, status, planted_at, matures_at, wither_at, watered, fertilized, yield
		FROM farm_plots
		WHERE character_id = ? AND plot_index = ?
	`, characterID, plotIndex).Scan(&p.ID, &p.CharacterID, &p.PlotIndex, &p.SeedType, &p.Status, &plantedAt, &maturesAt, &witherAt, &p.Watered, &p.Fertilized, &p.Yield)
	if errors.Is(err, sql.ErrNoRows) {
		return farm.FarmPlot{}, farm.ErrPlotNotFound
	}
	if err != nil {
		return farm.FarmPlot{}, err
	}
	if plantedAt.Valid {
		p.PlantedAt = &plantedAt.Time
	}
	if maturesAt.Valid {
		p.MaturesAt = &maturesAt.Time
	}
	if witherAt.Valid {
		p.WitherAt = &witherAt.Time
	}
	return p, nil
}

func (r *FarmRepository) SavePlot(ctx context.Context, plot farm.FarmPlot) error {
	if plot.ID == "" {
		plot.ID = id.New()
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO farm_plots (id, character_id, plot_index, seed_type, status, planted_at, matures_at, wither_at, watered, fertilized, yield)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			seed_type = VALUES(seed_type),
			status = VALUES(status),
			planted_at = VALUES(planted_at),
			matures_at = VALUES(matures_at),
			wither_at = VALUES(wither_at),
			watered = VALUES(watered),
			fertilized = VALUES(fertilized),
			yield = VALUES(yield)
	`, plot.ID, plot.CharacterID, plot.PlotIndex, plot.SeedType, plot.Status, plot.PlantedAt, plot.MaturesAt, plot.WitherAt, plot.Watered, plot.Fertilized, plot.Yield)
	return err
}

func (r *FarmRepository) HarvestPlot(ctx context.Context, characterID string, plotIndex int, rewardGold int) (farm.FarmPlot, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return farm.FarmPlot{}, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	// 1. Reset plot to EMPTY
	_, err = tx.ExecContext(ctx, `
		UPDATE farm_plots
		SET status = 'EMPTY', seed_type = '', planted_at = NULL, matures_at = NULL, wither_at = NULL, watered = FALSE, fertilized = FALSE, yield = 1
		WHERE character_id = ? AND plot_index = ?
	`, characterID, plotIndex)
	if err != nil {
		return farm.FarmPlot{}, err
	}

	// 2. Add reward gold to character
	if rewardGold > 0 {
		if _, err := tx.ExecContext(ctx, `
			UPDATE characters
			SET money = money + ?
			WHERE id = ?
		`, rewardGold, characterID); err != nil {
			return farm.FarmPlot{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return farm.FarmPlot{}, err
	}

	return farm.FarmPlot{
		CharacterID: characterID,
		PlotIndex:   plotIndex,
		Status:      farm.PlotStatusEmpty,
		Yield:       1,
	}, nil
}

func (r *FarmRepository) ClearPlot(ctx context.Context, characterID string, plotIndex int) (farm.FarmPlot, error) {
	_, err := r.db.ExecContext(ctx, `
		UPDATE farm_plots
		SET status = 'EMPTY', seed_type = '', planted_at = NULL, matures_at = NULL, wither_at = NULL, watered = FALSE, fertilized = FALSE, yield = 1
		WHERE character_id = ? AND plot_index = ?
	`, characterID, plotIndex)
	if err != nil {
		return farm.FarmPlot{}, err
	}
	return farm.FarmPlot{
		CharacterID: characterID,
		PlotIndex:   plotIndex,
		Status:      farm.PlotStatusEmpty,
		Yield:       1,
	}, nil
}
