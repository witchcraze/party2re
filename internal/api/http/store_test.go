package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apihttp "github.com/witchcraze/party2re/internal/api/http"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/store"
)

type stubStoreService struct {
	buildStoreFn      func(ctx context.Context, characterID, townID, houseStyle, storeName string) (*store.StoreCheckResult, error)
	getStoreFn        func(ctx context.Context, storeID string) (*store.StoreDetails, error)
	getTownStoresFn   func(ctx context.Context, townID string) ([]store.Store, error)
	checkStoreFn      func(ctx context.Context, characterID string) (*store.StoreCheckResult, error)
	listGoldItemFn    func(ctx context.Context, characterID, depotItemInstanceID string, price int) (*store.Sale, error)
	listBarterItemFn  func(ctx context.Context, characterID, depotItemInstanceID, wishItemName string) (*store.Sale, error)
	withdrawListingFn func(ctx context.Context, characterID, saleID string) error
	buyItemFn         func(ctx context.Context, buyerCharacterID, saleID string) error
	tradeItemFn       func(ctx context.Context, buyerCharacterID, saleID, customerDepotItemInstanceID string) error
	changeStoreNameFn func(ctx context.Context, characterID, newName string) error
	changeWallpaperFn func(ctx context.Context, characterID, wallpaper string) error
	addInteriorFn     func(ctx context.Context, characterID, furnitureID string) (*store.Interior, error)
	renameInteriorFn  func(ctx context.Context, characterID, interiorID, newName string) error
	cleanInteriorsFn  func(ctx context.Context, characterID string) error
}

func (s *stubStoreService) BuildStore(ctx context.Context, characterID, townID, houseStyle, storeName string) (*store.StoreCheckResult, error) {
	if s.buildStoreFn != nil {
		return s.buildStoreFn(ctx, characterID, townID, houseStyle, storeName)
	}
	return &store.StoreCheckResult{
		CharacterID: characterID,
		TownID:      townID,
		StoreName:   storeName,
	}, nil
}

func (s *stubStoreService) GetStore(ctx context.Context, storeID string) (*store.StoreDetails, error) {
	if s.getStoreFn != nil {
		return s.getStoreFn(ctx, storeID)
	}
	return &store.StoreDetails{
		Store: store.Store{
			ID:        storeID,
			StoreName: "MyStore",
			TownID:    "town1",
		},
		OwnerName: "OwnerHero",
	}, nil
}

func (s *stubStoreService) GetTownStores(ctx context.Context, townID string) ([]store.Store, error) {
	if s.getTownStoresFn != nil {
		return s.getTownStoresFn(ctx, townID)
	}
	return []store.Store{
		{ID: "st-1", TownID: townID, StoreName: "Store 1"},
	}, nil
}

func (s *stubStoreService) CheckStore(ctx context.Context, characterID string) (*store.StoreCheckResult, error) {
	if s.checkStoreFn != nil {
		return s.checkStoreFn(ctx, characterID)
	}
	return &store.StoreCheckResult{
		CharacterID: characterID,
		StoreName:   "MyStore",
	}, nil
}

func (s *stubStoreService) ListGoldItem(ctx context.Context, characterID, depotItemInstanceID string, price int) (*store.Sale, error) {
	if s.listGoldItemFn != nil {
		return s.listGoldItemFn(ctx, characterID, depotItemInstanceID, price)
	}
	return &store.Sale{
		ID:       "sale-1",
		StoreID:  "st-1",
		Price:    price,
		SaleType: store.SaleTypeGold,
	}, nil
}

func (s *stubStoreService) ListBarterItem(ctx context.Context, characterID, depotItemInstanceID, wishItemName string) (*store.Sale, error) {
	if s.listBarterItemFn != nil {
		return s.listBarterItemFn(ctx, characterID, depotItemInstanceID, wishItemName)
	}
	return &store.Sale{
		ID:           "sale-2",
		StoreID:      "st-1",
		WishItemName: wishItemName,
		SaleType:     store.SaleTypeBarter,
	}, nil
}

func (s *stubStoreService) WithdrawListing(ctx context.Context, characterID, saleID string) error {
	if s.withdrawListingFn != nil {
		return s.withdrawListingFn(ctx, characterID, saleID)
	}
	return nil
}

func (s *stubStoreService) BuyItem(ctx context.Context, buyerCharacterID, saleID string) error {
	if s.buyItemFn != nil {
		return s.buyItemFn(ctx, buyerCharacterID, saleID)
	}
	return nil
}

func (s *stubStoreService) TradeItem(ctx context.Context, buyerCharacterID, saleID, customerDepotItemInstanceID string) error {
	if s.tradeItemFn != nil {
		return s.tradeItemFn(ctx, buyerCharacterID, saleID, customerDepotItemInstanceID)
	}
	return nil
}

func (s *stubStoreService) ChangeStoreName(ctx context.Context, characterID, newName string) error {
	if s.changeStoreNameFn != nil {
		return s.changeStoreNameFn(ctx, characterID, newName)
	}
	return nil
}

func (s *stubStoreService) ChangeWallpaper(ctx context.Context, characterID, wallpaper string) error {
	if s.changeWallpaperFn != nil {
		return s.changeWallpaperFn(ctx, characterID, wallpaper)
	}
	return nil
}

func (s *stubStoreService) AddInterior(ctx context.Context, characterID, furnitureID string) (*store.Interior, error) {
	if s.addInteriorFn != nil {
		return s.addInteriorFn(ctx, characterID, furnitureID)
	}
	return &store.Interior{
		ID:          "int-1",
		FurnitureID: furnitureID,
	}, nil
}

func (s *stubStoreService) RenameInterior(ctx context.Context, characterID, interiorID, newName string) error {
	if s.renameInteriorFn != nil {
		return s.renameInteriorFn(ctx, characterID, interiorID, newName)
	}
	return nil
}

func (s *stubStoreService) CleanInteriors(ctx context.Context, characterID string) error {
	if s.cleanInteriorsFn != nil {
		return s.cleanInteriorsFn(ctx, characterID)
	}
	return nil
}

func TestStoreHTTP_Endpoints(t *testing.T) {
	players := &stubPlayerService{
		authenticateFn: func(ctx context.Context, sessionID string) (coreplayer.Player, error) {
			if sessionID == "valid-session" {
				return coreplayer.Player{ID: "player-1", Username: "Tester"}, nil
			}
			return coreplayer.Player{}, coreplayer.ErrInvalidSession
		},
	}

	characters := &stubCharacterService{
		getFn: func(ctx context.Context, id string) (corecharacter.Character, error) {
			if id == "char-1" {
				return corecharacter.Character{
					ID:       "char-1",
					PlayerID: "player-1",
					Name:     "Hero",
					Money:    100000,
				}, nil
			}
			return corecharacter.Character{}, corecharacter.ErrNotFound
		},
	}

	storeSvc := &stubStoreService{}

	handler := newTestHandler(
		t,
		players,
		characters,
		&stubAdventureService{},
		&stubShopService{},
		apihttp.WithStore(storeSvc),
	)

	server := handler.Router()

	t.Run("POST /towns/{town_id}/stores", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/towns/town1/stores", strings.NewReader(`{"character_id":"char-1","house_style":"001","store_name":"マイショップ"}`))
		req.Header.Set("Authorization", "Bearer valid-session")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201 Created, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("GET /towns/{town_id}/stores", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/towns/town1/stores", nil)
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("GET /stores/{store_id}", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/stores/st-1", nil)
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("GET /characters/{id}/store", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/characters/char-1/store", nil)
		req.Header.Set("Authorization", "Bearer valid-session")
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /characters/{id}/store/listings/gold", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/characters/char-1/store/listings/gold", strings.NewReader(`{"depot_item_instance_id":"inst-1","price":1000}`))
		req.Header.Set("Authorization", "Bearer valid-session")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201 Created, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /characters/{id}/store/listings/barter", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/characters/char-1/store/listings/barter", strings.NewReader(`{"depot_item_instance_id":"inst-1","wish_item_name":"やくそう"}`))
		req.Header.Set("Authorization", "Bearer valid-session")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201 Created, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("DELETE /characters/{id}/store/listings/{sale_id}", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/characters/char-1/store/listings/sale-1", nil)
		req.Header.Set("Authorization", "Bearer valid-session")
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Fatalf("expected 204 No Content, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /characters/{id}/store/sales/{sale_id}/buy", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/characters/char-1/store/sales/sale-1/buy", nil)
		req.Header.Set("Authorization", "Bearer valid-session")
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /characters/{id}/store/sales/{sale_id}/trade", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/characters/char-1/store/sales/sale-1/trade", strings.NewReader(`{"depot_item_instance_id":"inst-barter"}`))
		req.Header.Set("Authorization", "Bearer valid-session")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /characters/{id}/store/name", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/characters/char-1/store/name", strings.NewReader(`{"store_name":"新看板"}`))
		req.Header.Set("Authorization", "Bearer valid-session")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /characters/{id}/store/wallpaper", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/characters/char-1/store/wallpaper", strings.NewReader(`{"wallpaper":"farm"}`))
		req.Header.Set("Authorization", "Bearer valid-session")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /characters/{id}/store/interiors", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/characters/char-1/store/interiors", strings.NewReader(`{"furniture_id":"001"}`))
		req.Header.Set("Authorization", "Bearer valid-session")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201 Created, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("PUT /characters/{id}/store/interiors/{interior_id}/name", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/characters/char-1/store/interiors/int-1/name", strings.NewReader(`{"name":"お気に入りの机"}`))
		req.Header.Set("Authorization", "Bearer valid-session")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("DELETE /characters/{id}/store/interiors", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/characters/char-1/store/interiors", nil)
		req.Header.Set("Authorization", "Bearer valid-session")
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Fatalf("expected 204 No Content, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}
