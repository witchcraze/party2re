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
	"github.com/witchcraze/party2re/internal/home"
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

func TestStoreHTTP_OwnerMutations_SleepGuardAndForbidden(t *testing.T) {
	player1 := coreplayer.Player{ID: "player-1", Username: "Tester"}
	player2 := coreplayer.Player{ID: "player-2", Username: "Intruder"}
	char1 := corecharacter.Character{
		ID:       "char-1",
		PlayerID: "player-1",
		Name:     "Hero",
		Money:    100000,
	}

	players := &stubPlayerService{
		authenticateFn: func(ctx context.Context, sessionID string) (coreplayer.Player, error) {
			switch sessionID {
			case "valid-session":
				return player1, nil
			case "other-session":
				return player2, nil
			default:
				return coreplayer.Player{}, coreplayer.ErrInvalidSession
			}
		},
	}

	characters := &stubCharacterService{
		getFn: func(ctx context.Context, id string) (corecharacter.Character, error) {
			if id == "char-1" {
				return char1, nil
			}
			return corecharacter.Character{}, corecharacter.ErrNotFound
		},
	}

	var serviceCalls int
	storeSvc := &stubStoreService{
		withdrawListingFn: func(ctx context.Context, characterID, saleID string) error {
			serviceCalls++
			return nil
		},
		changeStoreNameFn: func(ctx context.Context, characterID, newName string) error {
			serviceCalls++
			return nil
		},
		changeWallpaperFn: func(ctx context.Context, characterID, wallpaper string) error {
			serviceCalls++
			return nil
		},
		addInteriorFn: func(ctx context.Context, characterID, furnitureID string) (*store.Interior, error) {
			serviceCalls++
			return &store.Interior{ID: "int-1", FurnitureID: furnitureID}, nil
		},
		renameInteriorFn: func(ctx context.Context, characterID, interiorID, newName string) error {
			serviceCalls++
			return nil
		},
		cleanInteriorsFn: func(ctx context.Context, characterID string) error {
			serviceCalls++
			return nil
		},
	}

	var currentSleepStatus home.SleepStatus
	mockHome := &mockHomeService{
		getSleepStatusFn: func(ctx context.Context, characterID string) (home.SleepStatus, error) {
			return currentSleepStatus, nil
		},
	}

	handler := newTestHandler(
		t,
		players,
		characters,
		&stubAdventureService{},
		&stubShopService{},
		apihttp.WithHome(mockHome),
		apihttp.WithStore(storeSvc),
	)
	server := handler.Router()

	testMutations := []struct {
		name      string
		method    string
		path      string
		body      string
		awakeCode int
	}{
		{"withdraw_listing", http.MethodDelete, "/characters/char-1/store/listings/sale-1", "", http.StatusNoContent},
		{"change_name", http.MethodPost, "/characters/char-1/store/name", `{"store_name":"新看板"}`, http.StatusOK},
		{"change_wallpaper", http.MethodPost, "/characters/char-1/store/wallpaper", `{"wallpaper":"farm"}`, http.StatusOK},
		{"add_interior", http.MethodPost, "/characters/char-1/store/interiors", `{"furniture_id":"001"}`, http.StatusCreated},
		{"rename_interior", http.MethodPut, "/characters/char-1/store/interiors/int-1/name", `{"name":"デスク"}`, http.StatusOK},
		{"clean_interiors", http.MethodDelete, "/characters/char-1/store/interiors", "", http.StatusNoContent},
	}

	for _, mode := range []struct {
		desc   string
		status home.SleepStatus
	}{
		{"sleeping", home.SleepStatus{Sleeping: true, RemainingSeconds: 300}},
		{"can_wake", home.SleepStatus{CanWake: true, RemainingSeconds: 0}},
	} {
		t.Run(mode.desc+" blocks all owner mutations with 409", func(t *testing.T) {
			currentSleepStatus = mode.status
			for _, tc := range testMutations {
				t.Run(tc.name, func(t *testing.T) {
					beforeCalls := serviceCalls
					req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
					req.Header.Set("Authorization", "Bearer valid-session")
					if tc.body != "" {
						req.Header.Set("Content-Type", "application/json")
					}
					rec := httptest.NewRecorder()
					server.ServeHTTP(rec, req)

					if rec.Code != http.StatusConflict {
						t.Errorf("expected 409 Conflict, got %d: %s", rec.Code, rec.Body.String())
					}
					if serviceCalls != beforeCalls {
						t.Errorf("service method was invoked despite %s state", mode.desc)
					}
				})
			}
		})
	}

	t.Run("other-player access is rejected with 403 Forbidden", func(t *testing.T) {
		currentSleepStatus = home.SleepStatus{}
		for _, tc := range testMutations {
			t.Run(tc.name, func(t *testing.T) {
				beforeCalls := serviceCalls
				req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
				req.Header.Set("Authorization", "Bearer other-session")
				if tc.body != "" {
					req.Header.Set("Content-Type", "application/json")
				}
				rec := httptest.NewRecorder()
				server.ServeHTTP(rec, req)

				if rec.Code != http.StatusForbidden {
					t.Errorf("expected 403 Forbidden, got %d: %s", rec.Code, rec.Body.String())
				}
				if serviceCalls != beforeCalls {
					t.Errorf("service method was invoked for unauthorized character access")
				}
			})
		}
	})

	t.Run("awake character successfully executes mutations", func(t *testing.T) {
		currentSleepStatus = home.SleepStatus{}
		for _, tc := range testMutations {
			t.Run(tc.name, func(t *testing.T) {
				beforeCalls := serviceCalls
				req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
				req.Header.Set("Authorization", "Bearer valid-session")
				if tc.body != "" {
					req.Header.Set("Content-Type", "application/json")
				}
				rec := httptest.NewRecorder()
				server.ServeHTTP(rec, req)

				if rec.Code != tc.awakeCode {
					t.Errorf("expected %d, got %d: %s", tc.awakeCode, rec.Code, rec.Body.String())
				}
				if serviceCalls != beforeCalls+1 {
					t.Errorf("expected service to be called once, was called %d times", serviceCalls-beforeCalls)
				}
			})
		}
	})
}
