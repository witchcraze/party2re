package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	apihttp "github.com/witchcraze/party2re/internal/api/http"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/farm"
)

type stubFarmService struct {
	getPlotsFn      func(ctx context.Context, characterID string) ([]farm.FarmPlot, error)
	plantSeedFn     func(ctx context.Context, characterID string, plotIndex int, seedType farm.SeedType) (farm.FarmPlot, error)
	waterPlotFn     func(ctx context.Context, characterID string, plotIndex int) (farm.FarmPlot, error)
	fertilizePlotFn func(ctx context.Context, characterID string, plotIndex int) (farm.FarmPlot, error)
	harvestPlotFn   func(ctx context.Context, characterID string, plotIndex int) (farm.HarvestResult, farm.FarmPlot, error)
	clearPlotFn     func(ctx context.Context, characterID string, plotIndex int) (farm.FarmPlot, error)
}

func (s *stubFarmService) GetPlots(ctx context.Context, characterID string) ([]farm.FarmPlot, error) {
	if s.getPlotsFn != nil {
		return s.getPlotsFn(ctx, characterID)
	}
	return nil, nil
}

func (s *stubFarmService) PlantSeed(ctx context.Context, characterID string, plotIndex int, seedType farm.SeedType) (farm.FarmPlot, error) {
	if s.plantSeedFn != nil {
		return s.plantSeedFn(ctx, characterID, plotIndex, seedType)
	}
	return farm.FarmPlot{CharacterID: characterID, PlotIndex: plotIndex, SeedType: seedType, Status: farm.PlotStatusGrowing}, nil
}

func (s *stubFarmService) WaterPlot(ctx context.Context, characterID string, plotIndex int) (farm.FarmPlot, error) {
	if s.waterPlotFn != nil {
		return s.waterPlotFn(ctx, characterID, plotIndex)
	}
	return farm.FarmPlot{CharacterID: characterID, PlotIndex: plotIndex, Watered: true}, nil
}

func (s *stubFarmService) FertilizePlot(ctx context.Context, characterID string, plotIndex int) (farm.FarmPlot, error) {
	if s.fertilizePlotFn != nil {
		return s.fertilizePlotFn(ctx, characterID, plotIndex)
	}
	return farm.FarmPlot{CharacterID: characterID, PlotIndex: plotIndex, Fertilized: true}, nil
}

func (s *stubFarmService) HarvestPlot(ctx context.Context, characterID string, plotIndex int) (farm.HarvestResult, farm.FarmPlot, error) {
	if s.harvestPlotFn != nil {
		return s.harvestPlotFn(ctx, characterID, plotIndex)
	}
	return farm.HarvestResult{Yield: 2}, farm.FarmPlot{CharacterID: characterID, PlotIndex: plotIndex, Status: farm.PlotStatusEmpty}, nil
}

func (s *stubFarmService) ClearPlot(ctx context.Context, characterID string, plotIndex int) (farm.FarmPlot, error) {
	if s.clearPlotFn != nil {
		return s.clearPlotFn(ctx, characterID, plotIndex)
	}
	return farm.FarmPlot{CharacterID: characterID, PlotIndex: plotIndex, Status: farm.PlotStatusEmpty}, nil
}

func TestFarmEndpoints(t *testing.T) {
	player := coreplayer.Player{ID: "p1", Username: "hero"}
	char := corecharacter.Character{ID: "c1", PlayerID: "p1", Name: "Hero"}

	pService := &stubPlayerService{
		authenticateFn: alwaysAuthPlayer(player),
	}
	cService := &stubCharacterService{
		getFn: func(_ context.Context, id string) (corecharacter.Character, error) {
			if id == "c1" {
				return char, nil
			}
			return corecharacter.Character{}, corecharacter.ErrNotFound
		},
	}
	fService := &stubFarmService{
		getPlotsFn: func(_ context.Context, characterID string) ([]farm.FarmPlot, error) {
			return []farm.FarmPlot{
				{PlotIndex: 0, Status: farm.PlotStatusEmpty},
			}, nil
		},
	}

	h := newTestHandler(
		t,
		pService,
		cService,
		&stubAdventureService{},
		&stubShopService{},
		apihttp.WithFarm(fService),
	)
	router := h.Router()

	t.Run("GET /characters/{id}/farm - success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/characters/c1/farm", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /characters/{id}/farm/plant - success", func(t *testing.T) {
		req := jsonRequest(t, http.MethodPost, "/characters/c1/farm/plant", `{"plot_index":0,"seed_type":"seed_herb"}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /characters/{id}/farm/water - success", func(t *testing.T) {
		req := jsonRequest(t, http.MethodPost, "/characters/c1/farm/water", `{"plot_index":0}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /characters/{id}/farm/fertilize - success", func(t *testing.T) {
		req := jsonRequest(t, http.MethodPost, "/characters/c1/farm/fertilize", `{"plot_index":0}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /characters/{id}/farm/harvest - success", func(t *testing.T) {
		req := jsonRequest(t, http.MethodPost, "/characters/c1/farm/harvest", `{"plot_index":0}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /characters/{id}/farm/clear - success", func(t *testing.T) {
		req := jsonRequest(t, http.MethodPost, "/characters/c1/farm/clear", `{"plot_index":0}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}
