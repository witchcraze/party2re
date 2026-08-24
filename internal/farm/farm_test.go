package farm_test

import (
	"context"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/farm"
)

type mockFarmRepo struct {
	getPlotsFn    func(ctx context.Context, charID string) ([]farm.FarmPlot, error)
	getPlotFn     func(ctx context.Context, charID string, plotIdx int) (farm.FarmPlot, error)
	savePlotFn    func(ctx context.Context, plot farm.FarmPlot) error
	harvestPlotFn func(ctx context.Context, charID string, plotIdx int, rewardGold int) (farm.FarmPlot, error)
	clearPlotFn   func(ctx context.Context, charID string, plotIdx int) (farm.FarmPlot, error)
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
func (m *mockFarmRepo) SavePlot(ctx context.Context, plot farm.FarmPlot) error {
	if m.savePlotFn != nil {
		return m.savePlotFn(ctx, plot)
	}
	return nil
}
func (m *mockFarmRepo) HarvestPlot(ctx context.Context, charID string, plotIdx int, rewardGold int) (farm.FarmPlot, error) {
	if m.harvestPlotFn != nil {
		return m.harvestPlotFn(ctx, charID, plotIdx, rewardGold)
	}
	return farm.FarmPlot{Status: farm.PlotStatusEmpty}, nil
}
func (m *mockFarmRepo) ClearPlot(ctx context.Context, charID string, plotIdx int) (farm.FarmPlot, error) {
	if m.clearPlotFn != nil {
		return m.clearPlotFn(ctx, charID, plotIdx)
	}
	return farm.FarmPlot{Status: farm.PlotStatusEmpty}, nil
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
		savePlotFn: func(_ context.Context, plot farm.FarmPlot) error {
			plotsMap[plot.PlotIndex] = plot
			return nil
		},
		harvestPlotFn: func(_ context.Context, charID string, idx int, rewardGold int) (farm.FarmPlot, error) {
			delete(plotsMap, idx)
			return farm.FarmPlot{CharacterID: charID, PlotIndex: idx, Status: farm.PlotStatusEmpty}, nil
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

	// 4. Fertilize plot (halves growth time)
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
