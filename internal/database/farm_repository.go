package database

import (
	"context"
	"database/sql"
	"errors"
	"time"

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

func (r *FarmRepository) PlantSeed(ctx context.Context, characterID string, plotIndex int, seedType farm.SeedType, seedDef farm.SeedDefinition, now time.Time) (farm.FarmPlot, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return farm.FarmPlot{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	// 1. Lock and inspect current plot if exists
	var existing farm.FarmPlot
	var plantedAt, maturesAt, witherAt sql.NullTime
	err = tx.QueryRowContext(ctx, `
		SELECT id, character_id, plot_index, seed_type, status, planted_at, matures_at, wither_at, watered, fertilized, yield
		FROM farm_plots
		WHERE character_id = ? AND plot_index = ?
		FOR UPDATE
	`, characterID, plotIndex).Scan(&existing.ID, &existing.CharacterID, &existing.PlotIndex, &existing.SeedType, &existing.Status, &plantedAt, &maturesAt, &witherAt, &existing.Watered, &existing.Fertilized, &existing.Yield)

	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return farm.FarmPlot{}, err
	}

	if err == nil {
		if plantedAt.Valid {
			existing.PlantedAt = &plantedAt.Time
		}
		if maturesAt.Valid {
			existing.MaturesAt = &maturesAt.Time
		}
		if witherAt.Valid {
			existing.WitherAt = &witherAt.Time
		}
		if existing.ComputeCurrentStatus(now) != farm.PlotStatusEmpty {
			return farm.FarmPlot{}, farm.ErrPlotNotEmpty
		}
	}

	// 2. Lock and consume 1 seed item from inventory
	var seedItemID string
	var seedQuantity int
	err = tx.QueryRowContext(ctx, `
		SELECT id, quantity
		FROM inventory_items
		WHERE character_id = ? AND definition_id = ?
		ORDER BY quantity DESC
		LIMIT 1
		FOR UPDATE
	`, characterID, string(seedType)).Scan(&seedItemID, &seedQuantity)
	if errors.Is(err, sql.ErrNoRows) || seedQuantity <= 0 {
		return farm.FarmPlot{}, farm.ErrInsufficientSeed
	}
	if err != nil {
		return farm.FarmPlot{}, err
	}

	if seedQuantity == 1 {
		if _, err := tx.ExecContext(ctx, "DELETE FROM inventory_items WHERE id = ?", seedItemID); err != nil {
			return farm.FarmPlot{}, err
		}
	} else {
		if _, err := tx.ExecContext(ctx, "UPDATE inventory_items SET quantity = quantity - 1 WHERE id = ?", seedItemID); err != nil {
			return farm.FarmPlot{}, err
		}
	}

	// 3. Insert or update plot state
	plotID := existing.ID
	if plotID == "" {
		plotID = id.New()
	}
	matures := now.Add(seedDef.GrowthDuration)
	wither := matures.Add(seedDef.GraceDuration)

	newPlot := farm.FarmPlot{
		ID:          plotID,
		CharacterID: characterID,
		PlotIndex:   plotIndex,
		SeedType:    seedType,
		Status:      farm.PlotStatusGrowing,
		PlantedAt:   &now,
		MaturesAt:   &matures,
		WitherAt:    &wither,
		Watered:     false,
		Fertilized:  false,
		Yield:       seedDef.BaseYield,
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO farm_plots (id, character_id, plot_index, seed_type, status, planted_at, matures_at, wither_at, watered, fertilized, yield)
		VALUES (?, ?, ?, ?, 'GROWING', ?, ?, ?, FALSE, FALSE, ?)
		ON DUPLICATE KEY UPDATE
			seed_type = VALUES(seed_type),
			status = 'GROWING',
			planted_at = VALUES(planted_at),
			matures_at = VALUES(matures_at),
			wither_at = VALUES(wither_at),
			watered = FALSE,
			fertilized = FALSE,
			yield = VALUES(yield)
	`, newPlot.ID, newPlot.CharacterID, newPlot.PlotIndex, newPlot.SeedType, newPlot.PlantedAt, newPlot.MaturesAt, newPlot.WitherAt, newPlot.Yield)
	if err != nil {
		return farm.FarmPlot{}, err
	}

	if err := tx.Commit(); err != nil {
		return farm.FarmPlot{}, err
	}
	committed = true
	return newPlot, nil
}

func (r *FarmRepository) WaterPlot(ctx context.Context, characterID string, plotIndex int, now time.Time) (farm.FarmPlot, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return farm.FarmPlot{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	var p farm.FarmPlot
	var plantedAt, maturesAt, witherAt sql.NullTime
	err = tx.QueryRowContext(ctx, `
		SELECT id, character_id, plot_index, seed_type, status, planted_at, matures_at, wither_at, watered, fertilized, yield
		FROM farm_plots
		WHERE character_id = ? AND plot_index = ?
		FOR UPDATE
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

	if p.ComputeCurrentStatus(now) != farm.PlotStatusGrowing {
		return farm.FarmPlot{}, farm.ErrPlotNotGrowing
	}
	if p.Watered {
		return farm.FarmPlot{}, farm.ErrAlreadyWatered
	}

	p.Watered = true
	p.Yield += 1
	p.Status = farm.PlotStatusGrowing

	_, err = tx.ExecContext(ctx, `
		UPDATE farm_plots
		SET watered = TRUE, yield = yield + 1
		WHERE character_id = ? AND plot_index = ?
	`, characterID, plotIndex)
	if err != nil {
		return farm.FarmPlot{}, err
	}

	if err := tx.Commit(); err != nil {
		return farm.FarmPlot{}, err
	}
	committed = true
	return p, nil
}

func (r *FarmRepository) FertilizePlot(ctx context.Context, characterID string, plotIndex int, now time.Time) (farm.FarmPlot, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return farm.FarmPlot{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	var p farm.FarmPlot
	var plantedAt, maturesAt, witherAt sql.NullTime
	err = tx.QueryRowContext(ctx, `
		SELECT id, character_id, plot_index, seed_type, status, planted_at, matures_at, wither_at, watered, fertilized, yield
		FROM farm_plots
		WHERE character_id = ? AND plot_index = ?
		FOR UPDATE
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

	if p.ComputeCurrentStatus(now) != farm.PlotStatusGrowing {
		return farm.FarmPlot{}, farm.ErrPlotNotGrowing
	}
	if p.Fertilized {
		return farm.FarmPlot{}, farm.ErrAlreadyFertilized
	}

	// Lock and consume 1 fertilizer item
	var fertItemID string
	var fertQuantity int
	err = tx.QueryRowContext(ctx, `
		SELECT id, quantity
		FROM inventory_items
		WHERE character_id = ? AND definition_id = ?
		ORDER BY quantity DESC
		LIMIT 1
		FOR UPDATE
	`, characterID, farm.FertilizerItemID).Scan(&fertItemID, &fertQuantity)
	if errors.Is(err, sql.ErrNoRows) || fertQuantity <= 0 {
		return farm.FarmPlot{}, farm.ErrInsufficientFertilizer
	}
	if err != nil {
		return farm.FarmPlot{}, err
	}

	if fertQuantity == 1 {
		if _, err := tx.ExecContext(ctx, "DELETE FROM inventory_items WHERE id = ?", fertItemID); err != nil {
			return farm.FarmPlot{}, err
		}
	} else {
		if _, err := tx.ExecContext(ctx, "UPDATE inventory_items SET quantity = quantity - 1 WHERE id = ?", fertItemID); err != nil {
			return farm.FarmPlot{}, err
		}
	}

	p.Fertilized = true
	if p.MaturesAt != nil && p.PlantedAt != nil {
		originalDuration := p.MaturesAt.Sub(*p.PlantedAt)
		halvedMaturesAt := p.PlantedAt.Add(originalDuration / 2)
		p.MaturesAt = &halvedMaturesAt
		if p.WitherAt != nil {
			halvedWitherAt := halvedMaturesAt.Add(farm.DefaultGraceDuration)
			p.WitherAt = &halvedWitherAt
		}
	}
	p.Status = p.ComputeCurrentStatus(now)

	_, err = tx.ExecContext(ctx, `
		UPDATE farm_plots
		SET fertilized = TRUE, matures_at = ?, wither_at = ?
		WHERE character_id = ? AND plot_index = ?
	`, p.MaturesAt, p.WitherAt, characterID, plotIndex)
	if err != nil {
		return farm.FarmPlot{}, err
	}

	if err := tx.Commit(); err != nil {
		return farm.FarmPlot{}, err
	}
	committed = true
	return p, nil
}

func (r *FarmRepository) HarvestPlot(ctx context.Context, characterID string, plotIndex int, rewardGold int, rewardItemID string, yield int, now time.Time) (farm.FarmPlot, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return farm.FarmPlot{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	var p farm.FarmPlot
	var plantedAt, maturesAt, witherAt sql.NullTime
	err = tx.QueryRowContext(ctx, `
		SELECT id, character_id, plot_index, seed_type, status, planted_at, matures_at, wither_at, watered, fertilized, yield
		FROM farm_plots
		WHERE character_id = ? AND plot_index = ?
		FOR UPDATE
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

	status := p.ComputeCurrentStatus(now)
	if status == farm.PlotStatusWithered {
		return farm.FarmPlot{}, farm.ErrPlotWithered
	}
	if status != farm.PlotStatusMature {
		return farm.FarmPlot{}, farm.ErrPlotNotMature
	}

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

	// 3. Add reward item to inventory if specified
	if rewardItemID != "" && yield > 0 {
		var existingItemID string
		err = tx.QueryRowContext(ctx, `
			SELECT id FROM inventory_items
			WHERE character_id = ? AND definition_id = ?
			LIMIT 1
			FOR UPDATE
		`, characterID, rewardItemID).Scan(&existingItemID)

		if err == nil {
			if _, err := tx.ExecContext(ctx, `
				UPDATE inventory_items
				SET quantity = quantity + ?
				WHERE id = ?
			`, yield, existingItemID); err != nil {
				return farm.FarmPlot{}, err
			}
		} else if errors.Is(err, sql.ErrNoRows) {
			newItemID := id.New()
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO inventory_items (id, character_id, definition_id, quantity, enhancement_level)
				VALUES (?, ?, ?, ?, 0)
			`, newItemID, characterID, rewardItemID, yield); err != nil {
				return farm.FarmPlot{}, err
			}
		} else {
			return farm.FarmPlot{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return farm.FarmPlot{}, err
	}
	committed = true

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
