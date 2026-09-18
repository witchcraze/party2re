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
	"github.com/witchcraze/party2re/internal/store"
)

type stubOracleShopService struct {
	getStatusFn           func(ctx context.Context, characterID string) (*store.OracleStatus, error)
	talkFn                func() string
	inspectFn             func(jobLevel int) store.OracleInspectResult
	rentCostumeFn         func(ctx context.Context, characterID string, itemNo int) (*store.CostumeRentalResult, error)
	returnCostumeFn       func(ctx context.Context, characterID string) error
	buyHomeWallpaperFn    func(ctx context.Context, characterID string, wallpaper string) (*store.HomeWallpaperResult, error)
	discoverBlackMarketFn func(jobLevel int) error
}

func (s *stubOracleShopService) GetOracleStatus(ctx context.Context, characterID string) (*store.OracleStatus, error) {
	if s.getStatusFn != nil {
		return s.getStatusFn(ctx, characterID)
	}
	return &store.OracleStatus{
		CharacterID:          characterID,
		LocationName:         store.OracleLocationName,
		NPCName:              store.OracleNPCName,
		BgImg:                store.OracleBgImg,
		AvailableCostumes:    store.AvailableCostumes(16),
		CanUnlockBlackMarket: true,
	}, nil
}

func (s *stubOracleShopService) OracleTalk() string {
	if s.talkFn != nil {
		return s.talkFn()
	}
	return store.OracleWords[0]
}

func (s *stubOracleShopService) OracleInspect(jobLevel int) store.OracleInspectResult {
	if s.inspectFn != nil {
		return s.inspectFn(jobLevel)
	}
	return store.OracleInspect(jobLevel)
}

func (s *stubOracleShopService) RentCostume(ctx context.Context, characterID string, itemNo int) (*store.CostumeRentalResult, error) {
	if s.rentCostumeFn != nil {
		return s.rentCostumeFn(ctx, characterID, itemNo)
	}
	return &store.CostumeRentalResult{
		ActiveCostume: store.ActiveCostume{
			CharacterID: characterID,
			ItemNo:      itemNo,
			ItemName:    "ピンクスカート",
			Icon:        "chr/001.gif",
			RentedAt:    time.Now().UTC(),
			ExpiresAt:   time.Now().UTC().Add(24 * time.Hour),
		},
		Message: "ピンクスカートの衣装をレンタルしたよん。次の日には返してもらうよ",
	}, nil
}

func (s *stubOracleShopService) ReturnCostume(ctx context.Context, characterID string) error {
	if s.returnCostumeFn != nil {
		return s.returnCostumeFn(ctx, characterID)
	}
	return nil
}

func (s *stubOracleShopService) BuyHomeWallpaper(ctx context.Context, characterID string, wallpaper string) (*store.HomeWallpaperResult, error) {
	if s.buyHomeWallpaperFn != nil {
		return s.buyHomeWallpaperFn(ctx, characterID, wallpaper)
	}
	return &store.HomeWallpaperResult{
		Wallpaper: wallpaper + ".gif",
		Message:   "ボブの家の壁紙を " + wallpaper + " に、張り替えておいたよん",
	}, nil
}

func (s *stubOracleShopService) DiscoverBlackMarket(jobLevel int) error {
	if s.discoverBlackMarketFn != nil {
		return s.discoverBlackMarketFn(jobLevel)
	}
	return store.DiscoverBlackMarket(jobLevel)
}

func TestOracleShopEndpoints(t *testing.T) {
	player := coreplayer.Player{ID: "p1", Username: "hero"}
	char := corecharacter.Character{ID: "c1", PlayerID: "p1", Name: "Hero", Level: 20, JobLevel: 16, Money: 50000}

	pService := &stubPlayerService{
		authenticateFn: alwaysAuthPlayer(player),
	}
	cService := &stubCharacterService{
		getFn: func(_ context.Context, id string) (corecharacter.Character, error) {
			if id == char.ID {
				return char, nil
			}
			return corecharacter.Character{}, corecharacter.ErrNotFound
		},
	}
	oracleSvc := &stubOracleShopService{}

	handler, err := apihttp.NewHandler(
		pService,
		cService,
		&stubAdventureService{},
		&stubShopService{},
		apihttp.WithOracleShop(oracleSvc),
	)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	server := httptest.NewServer(handler.Router())
	defer server.Close()

	// 1. GET /characters/{id}/oracle -> 200 OK
	t.Run("GetOracleStatus_Success", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, server.URL+"/characters/c1/oracle", nil)
		req.Header.Set("Authorization", "Bearer session-token")
		resp, err := server.Client().Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200 OK, got %d", resp.StatusCode)
		}

		var status store.OracleStatus
		if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if status.LocationName != store.OracleLocationName || status.NPCName != store.OracleNPCName {
			t.Errorf("unexpected status: %+v", status)
		}
	})

	// 2. POST /characters/{id}/oracle/talk -> 200 OK
	t.Run("OracleTalk_Success", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodPost, server.URL+"/characters/c1/oracle/talk", nil)
		req.Header.Set("Authorization", "Bearer session-token")
		resp, err := server.Client().Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200 OK, got %d", resp.StatusCode)
		}

		var talkResp map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&talkResp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if talkResp["location_name"] != store.OracleLocationName {
			t.Errorf("unexpected location_name: %v", talkResp["location_name"])
		}
	})

	// 3. POST /characters/{id}/oracle/inspect -> 200 OK
	t.Run("OracleInspect_Success", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodPost, server.URL+"/characters/c1/oracle/inspect", nil)
		req.Header.Set("Authorization", "Bearer session-token")
		resp, err := server.Client().Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200 OK, got %d", resp.StatusCode)
		}

		var inspectResp map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&inspectResp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if inspectResp["hint"] != store.OracleBlackMarketHint {
			t.Errorf("expected hint for job_lv 16, got: %v", inspectResp["hint"])
		}
	})

	// 4. POST /characters/{id}/oracle/rent -> 200 OK
	t.Run("OracleRentCostume_Success", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{"item_no": 44})
		req, _ := http.NewRequest(http.MethodPost, server.URL+"/characters/c1/oracle/rent", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer session-token")
		req.Header.Set("Content-Type", "application/json")
		resp, err := server.Client().Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200 OK, got %d", resp.StatusCode)
		}

		var rentResp store.CostumeRentalResult
		if err := json.NewDecoder(resp.Body).Decode(&rentResp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if rentResp.ActiveCostume.ItemNo != 44 {
			t.Errorf("expected item 44, got %d", rentResp.ActiveCostume.ItemNo)
		}
	})

	// 5. POST /characters/{id}/oracle/rent -> 400 Bad Request on invalid item
	t.Run("OracleRentCostume_NotAvailable", func(t *testing.T) {
		oracleSvc.rentCostumeFn = func(_ context.Context, _ string, _ int) (*store.CostumeRentalResult, error) {
			return nil, store.ErrCostumeNotAvailable
		}
		defer func() { oracleSvc.rentCostumeFn = nil }()

		body, _ := json.Marshal(map[string]interface{}{"item_no": 999})
		req, _ := http.NewRequest(http.MethodPost, server.URL+"/characters/c1/oracle/rent", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer session-token")
		req.Header.Set("Content-Type", "application/json")
		resp, err := server.Client().Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400 Bad Request, got %d", resp.StatusCode)
		}
	})

	// 6. POST /characters/{id}/oracle/return -> 200 OK
	t.Run("OracleReturnCostume_Success", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodPost, server.URL+"/characters/c1/oracle/return", nil)
		req.Header.Set("Authorization", "Bearer session-token")
		resp, err := server.Client().Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200 OK, got %d", resp.StatusCode)
		}
	})

	// 7. POST /characters/{id}/oracle/wallpaper -> 200 OK
	t.Run("OracleBuyWallpaper_Success", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{"wallpaper": "goods"})
		req, _ := http.NewRequest(http.MethodPost, server.URL+"/characters/c1/oracle/wallpaper", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer session-token")
		req.Header.Set("Content-Type", "application/json")
		resp, err := server.Client().Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200 OK, got %d", resp.StatusCode)
		}
	})

	// 8. POST /characters/{id}/oracle/blackmarket -> 200 OK when eligible
	t.Run("OracleBlackMarket_Success", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodPost, server.URL+"/characters/c1/oracle/blackmarket", nil)
		req.Header.Set("Authorization", "Bearer session-token")
		resp, err := server.Client().Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200 OK, got %d", resp.StatusCode)
		}
	})

	// 9. POST /characters/{id}/oracle/blackmarket -> 403 Forbidden when ineligible
	t.Run("OracleBlackMarket_Forbidden", func(t *testing.T) {
		oracleSvc.discoverBlackMarketFn = func(jobLevel int) error {
			return store.ErrBlackMarketNotDiscovered
		}
		defer func() { oracleSvc.discoverBlackMarketFn = nil }()

		req, _ := http.NewRequest(http.MethodPost, server.URL+"/characters/c1/oracle/blackmarket", nil)
		req.Header.Set("Authorization", "Bearer session-token")
		resp, err := server.Client().Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("expected 403 Forbidden, got %d", resp.StatusCode)
		}
	})
}
