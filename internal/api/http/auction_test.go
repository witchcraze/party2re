package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	apihttp "github.com/witchcraze/party2re/internal/api/http"
	"github.com/witchcraze/party2re/internal/auction"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
)

type stubAuctionService struct {
	createListingFn func(ctx context.Context, sellerID, itemID, itemName, itemCategory string, enhancement int, startBid, buyoutPrice int, duration time.Duration) (auction.AuctionListing, error)
	getListingFn    func(ctx context.Context, listingID string) (auction.AuctionListing, error)
	listActiveFn    func(ctx context.Context, limit, offset int) ([]auction.AuctionListing, error)
	placeBidFn      func(ctx context.Context, listingID, bidderID string, bidAmount int) (auction.AuctionListing, error)
	buyoutFn        func(ctx context.Context, listingID, buyerID string) (auction.AuctionListing, error)
	cancelListingFn func(ctx context.Context, listingID, sellerID string) (auction.AuctionListing, error)
}

func (s *stubAuctionService) CreateListing(ctx context.Context, sellerID, itemID, itemName, itemCategory string, enhancement int, startBid, buyoutPrice int, duration time.Duration) (auction.AuctionListing, error) {
	if s.createListingFn != nil {
		return s.createListingFn(ctx, sellerID, itemID, itemName, itemCategory, enhancement, startBid, buyoutPrice, duration)
	}
	return auction.AuctionListing{ID: "auc-1", SellerCharacterID: sellerID, ItemID: itemID, ItemName: itemName, StartBid: startBid, BuyoutPrice: buyoutPrice, Status: auction.StatusActive}, nil
}
func (s *stubAuctionService) GetListing(ctx context.Context, listingID string) (auction.AuctionListing, error) {
	if s.getListingFn != nil {
		return s.getListingFn(ctx, listingID)
	}
	return auction.AuctionListing{ID: listingID, Status: auction.StatusActive}, nil
}
func (s *stubAuctionService) ListActive(ctx context.Context, limit, offset int) ([]auction.AuctionListing, error) {
	if s.listActiveFn != nil {
		return s.listActiveFn(ctx, limit, offset)
	}
	return []auction.AuctionListing{{ID: "auc-1", Status: auction.StatusActive}}, nil
}
func (s *stubAuctionService) PlaceBid(ctx context.Context, listingID, bidderID string, bidAmount int) (auction.AuctionListing, error) {
	if s.placeBidFn != nil {
		return s.placeBidFn(ctx, listingID, bidderID, bidAmount)
	}
	return auction.AuctionListing{ID: listingID, CurrentBid: bidAmount, HighestBidderID: &bidderID, Status: auction.StatusActive}, nil
}
func (s *stubAuctionService) Buyout(ctx context.Context, listingID, buyerID string) (auction.AuctionListing, error) {
	if s.buyoutFn != nil {
		return s.buyoutFn(ctx, listingID, buyerID)
	}
	return auction.AuctionListing{ID: listingID, HighestBidderID: &buyerID, Status: auction.StatusSold}, nil
}
func (s *stubAuctionService) CancelListing(ctx context.Context, listingID, sellerID string) (auction.AuctionListing, error) {
	if s.cancelListingFn != nil {
		return s.cancelListingFn(ctx, listingID, sellerID)
	}
	return auction.AuctionListing{ID: listingID, SellerCharacterID: sellerID, Status: auction.StatusCancelled}, nil
}

func TestAuctionEndpoints(t *testing.T) {
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
	aService := &stubAuctionService{}

	h := newTestHandler(
		t,
		pService,
		cService,
		&stubAdventureService{},
		&stubShopService{},
		apihttp.WithAuction(aService),
	)
	router := h.Router()

	t.Run("GET /auctions - success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/auctions?limit=10&offset=0", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
	})

	t.Run("GET /auctions/{id} - success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/auctions/auc-1", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
	})

	t.Run("POST /auctions - create success", func(t *testing.T) {
		req := jsonRequest(t, http.MethodPost, "/auctions", `{"seller_character_id":"c1","item_id":"i1","item_name":"Sword","item_category":"weapon","enhancement_level":0,"start_bid":100,"buyout_price":500,"duration_hours":24}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201 Created, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /auctions/{id}/bid - success", func(t *testing.T) {
		req := jsonRequest(t, http.MethodPost, "/auctions/auc-1/bid", `{"bidder_character_id":"c1","bid_amount":150}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /auctions/{id}/buyout - success", func(t *testing.T) {
		req := jsonRequest(t, http.MethodPost, "/auctions/auc-1/buyout", `{"buyer_character_id":"c1"}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /auctions/{id}/cancel - success", func(t *testing.T) {
		req := jsonRequest(t, http.MethodPost, "/auctions/auc-1/cancel", `{"seller_character_id":"c1"}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /auctions/{id}/cancel - forbidden for non-seller", func(t *testing.T) {
		hForbidden := newTestHandler(
			t,
			pService,
			cService,
			&stubAdventureService{},
			&stubShopService{},
			apihttp.WithAuction(&stubAuctionService{
				cancelListingFn: func(ctx context.Context, listingID, sellerID string) (auction.AuctionListing, error) {
					return auction.AuctionListing{}, auction.ErrUnauthorizedSeller
				},
			}),
		)
		req := jsonRequest(t, http.MethodPost, "/auctions/auc-1/cancel", `{"seller_character_id":"c1"}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		hForbidden.Router().ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected 403 Forbidden, got %d", rec.Code)
		}
	})
}
