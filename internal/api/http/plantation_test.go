package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	apihttp "github.com/witchcraze/party2re/internal/api/http"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/plantation"
)

type stubPlantationService struct {
	getStatusFn func(ctx context.Context, characterID string) (plantation.StatusResponse, error)
	sowFn       func(ctx context.Context, characterID string, seedID string) (plantation.SowResult, error)
	fertilizeFn func(ctx context.Context, characterID string, fertilizerID string) (plantation.FertilizeResult, error)
	harvestFn   func(ctx context.Context, characterID string) (plantation.HarvestResult, error)
}

func (s *stubPlantationService) GetStatus(ctx context.Context, characterID string) (plantation.StatusResponse, error) {
	if s.getStatusFn != nil {
		return s.getStatusFn(ctx, characterID)
	}
	return plantation.StatusResponse{
		Status:      plantation.StatusNone,
		Seeds:       plantation.AllSeeds(),
		Fertilizers: plantation.AllFertilizers(),
		Dialogue:    "種をまいて何ができるかはお楽しみ〜♪",
	}, nil
}

func (s *stubPlantationService) Sow(ctx context.Context, characterID string, seedID string) (plantation.SowResult, error) {
	if s.sowFn != nil {
		return s.sowFn(ctx, characterID, seedID)
	}
	return plantation.SowResult{
		Plot: plantation.Plot{
			CharacterID: characterID,
			SeedID:      seedID,
			SownAt:      time.Now(),
			MaturesAt:   time.Now().Add(12 * time.Hour),
		},
		Message: "赤の種をまいたよ！",
	}, nil
}

func (s *stubPlantationService) Fertilize(ctx context.Context, characterID string, fertilizerID string) (plantation.FertilizeResult, error) {
	if s.fertilizeFn != nil {
		return s.fertilizeFn(ctx, characterID, fertilizerID)
	}
	return plantation.FertilizeResult{
		Plot: plantation.Plot{
			CharacterID:  characterID,
			SeedID:       "red",
			FertilizerID: &fertilizerID,
		},
		Message: "化学肥料をまくよ！",
	}, nil
}

func (s *stubPlantationService) Harvest(ctx context.Context, characterID string) (plantation.HarvestResult, error) {
	if s.harvestFn != nil {
		return s.harvestFn(ctx, characterID)
	}
	return plantation.HarvestResult{
		Withered: false,
		Yields: []plantation.HarvestYield{
			{ItemID: "item-001", ItemName: "薬草", Quantity: 1},
		},
		Message: "収穫したよ！<br>薬草を1個<br>倉庫に送っておいたよ",
	}, nil
}

func setupPlantationTestHandler(t *testing.T, plantationSvc *stubPlantationService) (*apihttp.Handler, string) {
	player := coreplayer.Player{ID: "p1", Username: "planter"}
	char := corecharacter.Character{ID: "c1", PlayerID: "p1", Name: "PlanterChar", Level: 10, Money: 1000}

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

	h := newTestHandler(
		t,
		pService,
		cService,
		&stubAdventureService{},
		&stubShopService{},
		apihttp.WithPlantation(plantationSvc),
	)
	return h, "c1"
}

func TestHTTP_GetCharacterPlantation(t *testing.T) {
	h, charID := setupPlantationTestHandler(t, &stubPlantationService{})
	router := h.Router()

	req := httptest.NewRequest(http.MethodGet, "/characters/"+charID+"/plantation", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}

	var res plantation.StatusResponse
	if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
		t.Fatalf("decode response error: %v", err)
	}
	if res.Status != plantation.StatusNone {
		t.Errorf("expected StatusNone, got %v", res.Status)
	}
	if len(res.Seeds) != 6 {
		t.Errorf("expected 6 seeds, got %d", len(res.Seeds))
	}
}

func TestHTTP_PlantationSow(t *testing.T) {
	h, charID := setupPlantationTestHandler(t, &stubPlantationService{})
	router := h.Router()

	body, _ := json.Marshal(map[string]string{"seed_id": "red"})
	req := httptest.NewRequest(http.MethodPost, "/characters/"+charID+"/plantation/sow", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer valid-token")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}

	var res plantation.SowResult
	if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
		t.Fatalf("decode response error: %v", err)
	}
	if res.Plot.SeedID != "red" {
		t.Errorf("expected seed red, got %s", res.Plot.SeedID)
	}
}

func TestHTTP_PlantationFertilize(t *testing.T) {
	h, charID := setupPlantationTestHandler(t, &stubPlantationService{})
	router := h.Router()

	body, _ := json.Marshal(map[string]string{"fertilizer_id": "chemical"})
	req := httptest.NewRequest(http.MethodPost, "/characters/"+charID+"/plantation/fertilize", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer valid-token")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}

	var res plantation.FertilizeResult
	if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
		t.Fatalf("decode response error: %v", err)
	}
	if *res.Plot.FertilizerID != "chemical" {
		t.Errorf("expected chemical fertilizer, got %v", res.Plot.FertilizerID)
	}
}

func TestHTTP_PlantationHarvest(t *testing.T) {
	h, charID := setupPlantationTestHandler(t, &stubPlantationService{})
	router := h.Router()

	req := httptest.NewRequest(http.MethodPost, "/characters/"+charID+"/plantation/harvest", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}

	var res plantation.HarvestResult
	if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
		t.Fatalf("decode response error: %v", err)
	}
	if res.Withered {
		t.Errorf("expected not withered")
	}
	if len(res.Yields) != 1 {
		t.Errorf("expected 1 yield, got %d", len(res.Yields))
	}
}

func TestHTTP_PlantationErrors(t *testing.T) {
	stub := &stubPlantationService{
		sowFn: func(_ context.Context, _ string, _ string) (plantation.SowResult, error) {
			return plantation.SowResult{}, plantation.ErrInsufficientGold
		},
		fertilizeFn: func(_ context.Context, _ string, _ string) (plantation.FertilizeResult, error) {
			return plantation.FertilizeResult{}, plantation.ErrMissingFertilizerItem
		},
		harvestFn: func(_ context.Context, _ string) (plantation.HarvestResult, error) {
			return plantation.HarvestResult{}, plantation.ErrCropNotMatured
		},
	}
	h, charID := setupPlantationTestHandler(t, stub)
	router := h.Router()

	// Sow with insufficient gold -> 422
	body, _ := json.Marshal(map[string]string{"seed_id": "gold"})
	req := httptest.NewRequest(http.MethodPost, "/characters/"+charID+"/plantation/sow", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer valid-token")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422 Unprocessable Entity, got %d", rec.Code)
	}

	// Fertilize with missing item -> 422
	body2, _ := json.Marshal(map[string]string{"fertilizer_id": "kupo_nut"})
	req2 := httptest.NewRequest(http.MethodPost, "/characters/"+charID+"/plantation/fertilize", bytes.NewReader(body2))
	req2.Header.Set("Authorization", "Bearer valid-token")
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422 Unprocessable Entity, got %d", rec2.Code)
	}

	// Harvest when not matured -> 422
	req3 := httptest.NewRequest(http.MethodPost, "/characters/"+charID+"/plantation/harvest", nil)
	req3.Header.Set("Authorization", "Bearer valid-token")
	rec3 := httptest.NewRecorder()
	router.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422 Unprocessable Entity, got %d", rec3.Code)
	}
}
