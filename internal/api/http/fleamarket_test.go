package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	apihttp "github.com/witchcraze/party2re/internal/api/http"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/fleamarket"
)

type stubFleaMarketService struct {
	createListingFn        func(ctx context.Context, sellerCharacterID, itemInstanceOrDefID string, price int, now time.Time) (fleamarket.Listing, error)
	purchaseListingFn      func(ctx context.Context, buyerCharacterID, listingID string, now time.Time) (fleamarket.PurchaseResult, error)
	cancelListingFn        func(ctx context.Context, sellerCharacterID, listingID string) (fleamarket.Listing, error)
	listActiveListingsFn   func(ctx context.Context, limit, offset int) ([]fleamarket.Listing, int, error)
	getListingFn           func(ctx context.Context, listingID string) (fleamarket.Listing, error)
	getCharacterListingsFn func(ctx context.Context, characterID string) ([]fleamarket.Listing, error)
}

func (s *stubFleaMarketService) CreateListing(ctx context.Context, sellerCharacterID, itemInstanceOrDefID string, price int, now time.Time) (fleamarket.Listing, error) {
	if s.createListingFn != nil {
		return s.createListingFn(ctx, sellerCharacterID, itemInstanceOrDefID, price, now)
	}
	return fleamarket.Listing{
		ID:                "listing-1",
		SellerCharacterID: sellerCharacterID,
		SellerName:        "Hero",
		ItemID:            itemInstanceOrDefID,
		ItemName:          "銅の剣",
		ItemCategory:      "main-hand",
		Price:             price,
		Status:            fleamarket.StatusActive,
		CreatedAt:         now,
	}, nil
}

func (s *stubFleaMarketService) PurchaseListing(ctx context.Context, buyerCharacterID, listingID string, now time.Time) (fleamarket.PurchaseResult, error) {
	if s.purchaseListingFn != nil {
		return s.purchaseListingFn(ctx, buyerCharacterID, listingID, now)
	}
	buyerName := "BuyerHero"
	soldAt := now
	return fleamarket.PurchaseResult{
		Listing: fleamarket.Listing{
			ID:                listingID,
			SellerCharacterID: "char-seller",
			SellerName:        "SellerHero",
			ItemID:            "wea-sword",
			ItemName:          "銅の剣",
			ItemCategory:      "main-hand",
			Price:             300,
			Status:            fleamarket.StatusSold,
			BuyerCharacterID:  &buyerCharacterID,
			BuyerName:         &buyerName,
			CreatedAt:         now.Add(-time.Hour),
			SoldAt:            &soldAt,
		},
		BuyerGold:  700,
		SellerGold: 1300,
		ItemInstance: coreitem.Instance{
			ID:           "inst-new",
			DefinitionID: "wea-sword",
			Quantity:     1,
		},
	}, nil
}

func (s *stubFleaMarketService) CancelListing(ctx context.Context, sellerCharacterID, listingID string) (fleamarket.Listing, error) {
	if s.cancelListingFn != nil {
		return s.cancelListingFn(ctx, sellerCharacterID, listingID)
	}
	return fleamarket.Listing{
		ID:                listingID,
		SellerCharacterID: sellerCharacterID,
		SellerName:        "Hero",
		ItemID:            "wea-sword",
		ItemName:          "銅の剣",
		ItemCategory:      "main-hand",
		Price:             300,
		Status:            fleamarket.StatusCancelled,
	}, nil
}

func (s *stubFleaMarketService) ListActiveListings(ctx context.Context, limit, offset int) ([]fleamarket.Listing, int, error) {
	if s.listActiveListingsFn != nil {
		return s.listActiveListingsFn(ctx, limit, offset)
	}
	return []fleamarket.Listing{
		{
			ID:                "listing-1",
			SellerCharacterID: "char-seller-1",
			SellerName:        "Hero1",
			ItemID:            "wea-sword",
			ItemName:          "銅の剣",
			ItemCategory:      "main-hand",
			Price:             300,
			Status:            fleamarket.StatusActive,
		},
	}, 1, nil
}

func (s *stubFleaMarketService) GetListing(ctx context.Context, listingID string) (fleamarket.Listing, error) {
	if s.getListingFn != nil {
		return s.getListingFn(ctx, listingID)
	}
	return fleamarket.Listing{
		ID:                listingID,
		SellerCharacterID: "char-seller-1",
		SellerName:        "Hero1",
		ItemID:            "wea-sword",
		ItemName:          "銅の剣",
		ItemCategory:      "main-hand",
		Price:             300,
		Status:            fleamarket.StatusActive,
	}, nil
}

func (s *stubFleaMarketService) GetCharacterListings(ctx context.Context, characterID string) ([]fleamarket.Listing, error) {
	if s.getCharacterListingsFn != nil {
		return s.getCharacterListingsFn(ctx, characterID)
	}
	return []fleamarket.Listing{
		{
			ID:                "listing-1",
			SellerCharacterID: characterID,
			SellerName:        "Hero1",
			ItemID:            "wea-sword",
			ItemName:          "銅の剣",
			ItemCategory:      "main-hand",
			Price:             300,
			Status:            fleamarket.StatusActive,
		},
	}, nil
}

func TestFleaMarketHTTP_Endpoints(t *testing.T) {
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
					Money:    1000,
				}, nil
			}
			return corecharacter.Character{}, corecharacter.ErrNotFound
		},
	}

	fleaService := &stubFleaMarketService{}

	handler := newTestHandler(
		t,
		players,
		characters,
		&stubAdventureService{},
		&stubShopService{},
		apihttp.WithFleaMarket(fleaService),
	)

	server := handler.Router()

	t.Run("GET /fleamarket/listings (Public)", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/fleamarket/listings?limit=10&offset=0", nil)
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"listings"`) || !strings.Contains(rec.Body.String(), `"total":1`) {
			t.Errorf("unexpected response body: %s", rec.Body.String())
		}
	})

	t.Run("GET /fleamarket/listings/{listing_id} (Public)", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/fleamarket/listings/listing-1", nil)
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"id":"listing-1"`) {
			t.Errorf("unexpected response body: %s", rec.Body.String())
		}
	})

	t.Run("GET /characters/{id}/fleamarket/listings (Authenticated)", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/characters/char-1/fleamarket/listings", nil)
		req.Header.Set("Authorization", "Bearer valid-session")
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"seller_character_id":"char-1"`) {
			t.Errorf("unexpected response body: %s", rec.Body.String())
		}
	})

	t.Run("POST /characters/{id}/fleamarket/listings (Create Listing)", func(t *testing.T) {
		body := `{"item_id":"wea-sword","price":350}`
		req := httptest.NewRequest(http.MethodPost, "/characters/char-1/fleamarket/listings", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer valid-session")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("expected status 201, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"price":350`) {
			t.Errorf("unexpected response body: %s", rec.Body.String())
		}
	})

	t.Run("POST /characters/{id}/fleamarket/listings/{listing_id}/purchase (Purchase Listing)", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/characters/char-1/fleamarket/listings/listing-1/purchase", nil)
		req.Header.Set("Authorization", "Bearer valid-session")
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"buyer_gold":700`) {
			t.Errorf("unexpected response body: %s", rec.Body.String())
		}
	})

	t.Run("DELETE /characters/{id}/fleamarket/listings/{listing_id} (Cancel Listing)", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/characters/char-1/fleamarket/listings/listing-1", nil)
		req.Header.Set("Authorization", "Bearer valid-session")
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"status":"cancelled"`) {
			t.Errorf("unexpected response body: %s", rec.Body.String())
		}
	})

	t.Run("Unauthorized without token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/characters/char-1/fleamarket/listings", nil)
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected status 401, got %d", rec.Code)
		}
	})
}
