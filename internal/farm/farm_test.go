package farm_test

import (
	"context"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/farm"
)

type mockFarmRepo struct {
	getPlotsFn      func(ctx context.Context, charID string) ([]farm.FarmPlot, error)
	getPlotFn       func(ctx context.Context, charID string, plotIdx int) (farm.FarmPlot, error)
	plantSeedFn     func(ctx context.Context, charID string, plotIdx int, seedType farm.SeedType, seedDef farm.SeedDefinition, now time.Time) (farm.FarmPlot, error)
	waterPlotFn     func(ctx context.Context, charID string, plotIdx int, now time.Time) (farm.FarmPlot, error)
	fertilizePlotFn func(ctx context.Context, charID string, plotIdx int, now time.Time) (farm.FarmPlot, error)
	harvestPlotFn   func(ctx context.Context, charID string, plotIdx int, rewardGold int, rewardItemID string, yield int, now time.Time) (farm.FarmPlot, error)
	clearPlotFn     func(ctx context.Context, charID string, plotIdx int) (farm.FarmPlot, error)
}

func (m *mockFarmRepo) GetPlots(ctx context.Context, charID string) ([]farm.FarmPlot, error) {
	if m.getPlotsFn != nil {
		return m.getPlotsFn(ctx, charID)
	}
	return nil, nil
}

func (m *mockFarmRepo) GetPlot(ctx context.Context, charID string, plotIdx int) (farm.FarmPlot, error) {
	if m.getPlotFn != nil {
		return m.getPlotFn(ctx, charID, plotIdx)
	}
	return farm.FarmPlot{}, farm.ErrPlotNotFound
}

func (m *mockFarmRepo) PlantSeed(ctx context.Context, charID string, plotIdx int, seedType farm.SeedType, seedDef farm.SeedDefinition, now time.Time) (farm.FarmPlot, error) {
	if m.plantSeedFn != nil {
		return m.plantSeedFn(ctx, charID, plotIdx, seedType, seedDef, now)
	}
	matures := now.Add(seedDef.GrowthDuration)
	wither := matures.Add(seedDef.GraceDuration)
	return farm.FarmPlot{
		CharacterID: charID,
		PlotIndex:   plotIdx,
		SeedType:    seedType,
		Status:      farm.PlotStatusGrowing,
		PlantedAt:   &now,
		MaturesAt:   &matures,
		WitherAt:    &wither,
		Yield:       seedDef.BaseYield,
	}, nil
}

func (m *mockFarmRepo) WaterPlot(ctx context.Context, charID string, plotIdx int, now time.Time) (farm.FarmPlot, error) {
	if m.waterPlotFn != nil {
		return m.waterPlotFn(ctx, charID, plotIdx, now)
	}
	return farm.FarmPlot{
		CharacterID: charID,
		PlotIndex:   plotIdx,
		Status:      farm.PlotStatusGrowing,
		Watered:     true,
		Yield:       3,
	}, nil
}

func (m *mockFarmRepo) FertilizePlot(ctx context.Context, charID string, plotIdx int, now time.Time) (farm.FarmPlot, error) {
	if m.fertilizePlotFn != nil {
		return m.fertilizePlotFn(ctx, charID, plotIdx, now)
	}
	return farm.FarmPlot{
		CharacterID: charID,
		PlotIndex:   plotIdx,
		Status:      farm.PlotStatusGrowing,
		Fertilized:  true,
		Yield:       2,
	}, nil
}

func (m *mockFarmRepo) HarvestPlot(ctx context.Context, charID string, plotIdx int, rewardGold int, rewardItemID string, yield int, now time.Time) (farm.FarmPlot, error) {
	if m.harvestPlotFn != nil {
		return m.harvestPlotFn(ctx, charID, plotIdx, rewardGold, rewardItemID, yield, now)
	}
	return farm.FarmPlot{CharacterID: charID, PlotIndex: plotIdx, Status: farm.PlotStatusEmpty, Yield: 1}, nil
}

func (m *mockFarmRepo) ClearPlot(ctx context.Context, charID string, plotIdx int) (farm.FarmPlot, error) {
	if m.clearPlotFn != nil {
		return m.clearPlotFn(ctx, charID, plotIdx)
	}
	return farm.FarmPlot{CharacterID: charID, PlotIndex: plotIdx, Status: farm.PlotStatusEmpty, Yield: 1}, nil
}

func TestFarmPlot_ComputeCurrentStatus(t *testing.T) {
	now := time.Now().UTC()
	planted := now.Add(-10 * time.Minute)
	matures := now.Add(5 * time.Minute)
	wither := now.Add(24 * time.Hour)

	plot := farm.FarmPlot{
		Status:    farm.PlotStatusGrowing,
		PlantedAt: &planted,
		MaturesAt: &matures,
		WitherAt:  &wither,
	}

	// 1. In progress (matures in future)
	if s := plot.ComputeCurrentStatus(now); s != farm.PlotStatusGrowing {
		t.Errorf("status = %v, want GROWING", s)
	}

	// 2. Mature (now is past maturesAt)
	if s := plot.ComputeCurrentStatus(matures.Add(time.Minute)); s != farm.PlotStatusMature {
		t.Errorf("status = %v, want MATURE", s)
	}

	// 3. Withered (now is past witherAt)
	if s := plot.ComputeCurrentStatus(wither.Add(time.Minute)); s != farm.PlotStatusWithered {
		t.Errorf("status = %v, want WITHERED", s)
	}
}

func TestFarmService_Lifecycle(t *testing.T) {
	ctx := context.Background()
	plotsMap := make(map[int]farm.FarmPlot)

	repo := &mockFarmRepo{
		getPlotFn: func(_ context.Context, charID string, idx int) (farm.FarmPlot, error) {
			p, ok := plotsMap[idx]
			if !ok {
				return farm.FarmPlot{}, farm.ErrPlotNotFound
			}
			return p, nil
		},
		plantSeedFn: func(_ context.Context, charID string, idx int, seedType farm.SeedType, seedDef farm.SeedDefinition, now time.Time) (farm.FarmPlot, error) {
			matures := now.Add(seedDef.GrowthDuration)
			wither := matures.Add(seedDef.GraceDuration)
			p := farm.FarmPlot{
				CharacterID: charID,
				PlotIndex:   idx,
				SeedType:    seedType,
				Status:      farm.PlotStatusGrowing,
				PlantedAt:   &now,
				MaturesAt:   &matures,
				WitherAt:    &wither,
				Yield:       seedDef.BaseYield,
			}
			plotsMap[idx] = p
			return p, nil
		},
		waterPlotFn: func(_ context.Context, charID string, idx int, now time.Time) (farm.FarmPlot, error) {
			p, ok := plotsMap[idx]
			if !ok {
				return farm.FarmPlot{}, farm.ErrPlotNotFound
			}
			if p.Watered {
				return farm.FarmPlot{}, farm.ErrAlreadyWatered
			}
			p.Watered = true
			p.Yield += 1
			plotsMap[idx] = p
			return p, nil
		},
		fertilizePlotFn: func(_ context.Context, charID string, idx int, now time.Time) (farm.FarmPlot, error) {
			p, ok := plotsMap[idx]
			if !ok {
				return farm.FarmPlot{}, farm.ErrPlotNotFound
			}
			if p.Fertilized {
				return farm.FarmPlot{}, farm.ErrAlreadyFertilized
			}
			p.Fertilized = true
			plotsMap[idx] = p
			return p, nil
		},
		harvestPlotFn: func(_ context.Context, charID string, idx int, rewardGold int, rewardItemID string, yield int, now time.Time) (farm.FarmPlot, error) {
			delete(plotsMap, idx)
			return farm.FarmPlot{CharacterID: charID, PlotIndex: idx, Status: farm.PlotStatusEmpty, Yield: 1}, nil
		},
	}

	svc, _ := farm.NewService(repo)

	// 1. Plant Herb Seed in Plot 0
	plot, err := svc.PlantSeed(ctx, "char1", 0, farm.SeedHerb)
	if err != nil {
		t.Fatalf("PlantSeed failed: %v", err)
	}
	if plot.Status != farm.PlotStatusGrowing || plot.Yield != 2 {
		t.Errorf("plot status=%v, yield=%d", plot.Status, plot.Yield)
	}

	// 2. Water plot (+1 yield)
	watered, err := svc.WaterPlot(ctx, "char1", 0)
	if err != nil {
		t.Fatalf("WaterPlot failed: %v", err)
	}
	if !watered.Watered || watered.Yield != 3 {
		t.Errorf("watered=%v, yield=%d", watered.Watered, watered.Yield)
	}

	// 3. Double water fails
	if _, err := svc.WaterPlot(ctx, "char1", 0); err != farm.ErrAlreadyWatered {
		t.Errorf("expected ErrAlreadyWatered, got %v", err)
	}

	// 4. Fertilize plot
	fertilized, err := svc.FertilizePlot(ctx, "char1", 0)
	if err != nil {
		t.Fatalf("FertilizePlot failed: %v", err)
	}
	if !fertilized.Fertilized {
		t.Error("expected Fertilized to be true")
	}

	// 5. Premature harvest fails
	if _, _, err := svc.HarvestPlot(ctx, "char1", 0); err != farm.ErrPlotNotMature {
		t.Errorf("expected ErrPlotNotMature, got %v", err)
	}

	// 6. Fast forward plot maturity
	past := time.Now().UTC().Add(-10 * time.Minute)
	fertilized.MaturesAt = &past
	plotsMap[0] = fertilized

	res, harvested, err := svc.HarvestPlot(ctx, "char1", 0)
	if err != nil {
		t.Fatalf("HarvestPlot failed: %v", err)
	}
	if res.Yield != 3 || res.RewardGold != 150 || harvested.Status != farm.PlotStatusEmpty {
		t.Errorf("harvest result: yield=%d, gold=%d, status=%v", res.Yield, res.RewardGold, harvested.Status)
	}
}

func TestFarmService_PreconditionValidations(t *testing.T) {
	ctx := context.Background()
	repo := &mockFarmRepo{}
	svc, _ := farm.NewService(repo)

	// Invalid plot indices
	if _, err := svc.PlantSeed(ctx, "char1", -1, farm.SeedHerb); err != farm.ErrInvalidPlotIndex {
		t.Errorf("want ErrInvalidPlotIndex, got %v", err)
	}
	if _, err := svc.PlantSeed(ctx, "char1", 4, farm.SeedHerb); err != farm.ErrInvalidPlotIndex {
		t.Errorf("want ErrInvalidPlotIndex, got %v", err)
	}
	if _, err := svc.WaterPlot(ctx, "char1", 4); err != farm.ErrInvalidPlotIndex {
		t.Errorf("want ErrInvalidPlotIndex, got %v", err)
	}
	if _, err := svc.FertilizePlot(ctx, "char1", 4); err != farm.ErrInvalidPlotIndex {
		t.Errorf("want ErrInvalidPlotIndex, got %v", err)
	}
	if _, _, err := svc.HarvestPlot(ctx, "char1", 4); err != farm.ErrInvalidPlotIndex {
		t.Errorf("want ErrInvalidPlotIndex, got %v", err)
	}
	if _, err := svc.ClearPlot(ctx, "char1", 4); err != farm.ErrInvalidPlotIndex {
		t.Errorf("want ErrInvalidPlotIndex, got %v", err)
	}

	// Invalid seed type
	if _, err := svc.PlantSeed(ctx, "char1", 0, farm.SeedType("invalid_seed")); err != farm.ErrInvalidSeedType {
		t.Errorf("want ErrInvalidSeedType, got %v", err)
	}
}
