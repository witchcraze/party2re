package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/fleamarket"
	"github.com/witchcraze/party2re/internal/playercontext"
)

type fleaMarketSceneService struct {
	FleaMarketService
	listings        []fleamarket.Listing
	owned           []fleamarket.Listing
	listErr, ownErr error
	mutations       int
	limit, offset   int
}

func (s *fleaMarketSceneService) ListActiveListings(_ context.Context, limit, offset int) ([]fleamarket.Listing, int, error) {
	s.limit, s.offset = limit, offset
	if s.listErr != nil {
		return nil, 0, s.listErr
	}
	start := min(offset, len(s.listings))
	return s.listings[start:min(start+limit, len(s.listings))], len(s.listings), nil
}

func (s *fleaMarketSceneService) GetActiveCharacterListings(_ context.Context, actor string) ([]fleamarket.Listing, error) {
	if actor != "hero" {
		return nil, errors.New("wrong flea market owner")
	}
	return s.owned, s.ownErr
}

func (s *fleaMarketSceneService) PurchaseListing(_ context.Context, _, _ string, _ time.Time) (fleamarket.PurchaseResult, error) {
	s.mutations++
	return fleamarket.PurchaseResult{}, nil
}

func (s *fleaMarketSceneService) CancelListing(_ context.Context, actor, listingID string) (fleamarket.Listing, error) {
	s.mutations++
	var cancelled fleamarket.Listing
	s.owned = slices.DeleteFunc(s.owned, func(l fleamarket.Listing) bool {
		if l.ID == listingID && l.SellerCharacterID == actor {
			cancelled = l
			cancelled.Status = fleamarket.StatusCancelled
			return true
		}
		return false
	})
	s.listings = slices.DeleteFunc(s.listings, func(l fleamarket.Listing) bool {
		return l.ID == listingID
	})
	return cancelled, nil
}

func fleaMarketSceneRouter(t *testing.T, service FleaMarketService, selection playercontext.Selection) (*gatewayNavigationStore, http.Handler) {
	t.Helper()
	f := newGatewayFixture(t)
	store := &gatewayNavigationStore{selection: selection}
	pc := playercontext.NewService(f, f, timer.NewService(nil), playercontext.WithNavigation(store,
		playercontext.SceneDefinition{ID: "town", Pageable: true},
		playercontext.SceneDefinition{ID: "fleamarket", Parent: "town", Pageable: true}))
	options := []Option{WithPlayerContext(pc), func(h *Handler) { h.homes = f }}
	if service != nil {
		options = append(options, WithFleaMarket(service))
	}
	h, err := NewHandler(f, f, &struct{ AdventureService }{}, &struct{ ShopService }{}, options...)
	if err != nil {
		t.Fatal(err)
	}
	return store, h.Router()
}

func fleaMarketListings(n int) []fleamarket.Listing {
	rows := make([]fleamarket.Listing, n)
	for i := range rows {
		rows[i] = fleamarket.Listing{ID: fmt.Sprintf("l%d", i), SellerCharacterID: "peer", SellerName: "Peer", ItemID: "item-001", ItemName: "薬草", EnhancementLevel: 2, Price: 10, Status: fleamarket.StatusActive}
	}
	return rows
}

func TestFleaMarketSceneObservesWithoutMutation(t *testing.T) {
	service := &fleaMarketSceneService{listings: fleaMarketListings(2), owned: []fleamarket.Listing{{ID: "mine", SellerCharacterID: "hero", Status: fleamarket.StatusActive}}}
	store, router := fleaMarketSceneRouter(t, service, playercontext.Selection{Destination: "fleamarket"})
	status, observation := navigationGET(t, router)
	if status != 200 || observation.Scene.LocationID != "fleamarket" || observation.Scene.Kind != "facility" || observation.Scene.Title != "フリーマーケット" || observation.Scene.Support.Observation != "details" {
		t.Fatalf("projection: %d %+v", status, observation)
	}
	data := decodeShopScene[FleaMarketSceneData](t, observation)
	if data.Parent != "town" || len(data.Listings) != 2 || data.Listings[0].EnhancementLevel != 2 || data.ListingCount != 2 || len(data.OwnedListings) != 1 || data.OwnedListings[0].ID != "mine" || data.Page.Next != nil || service.limit != 20 {
		t.Fatalf("lost facts: %+v", data)
	}
	for _, action := range []string{"fleamarket_list", "fleamarket_purchase", "fleamarket_cancel", "scene_back"} {
		if !slices.ContainsFunc(observation.AvailableActions, func(a ContextAction) bool { return a.Action == action }) {
			t.Fatalf("missing choice %s: %+v", action, observation.AvailableActions)
		}
	}
	if service.mutations != 0 || store.writes != 0 {
		t.Fatalf("GET wrote: %d %d", service.mutations, store.writes)
	}
}

func TestFleaMarketSceneEmptyArraysAreNonNull(t *testing.T) {
	_, router := fleaMarketSceneRouter(t, &fleaMarketSceneService{}, playercontext.Selection{Destination: "fleamarket"})
	status, observation := navigationGET(t, router)
	raw, err := json.Marshal(observation.Scene.Data)
	if err != nil || status != 200 {
		t.Fatalf("%d %v", status, err)
	}
	var data map[string]json.RawMessage
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	if string(data["listings"]) != "[]" || string(data["owned_listings"]) != "[]" {
		t.Fatalf("null arrays: %s", raw)
	}
}

func TestFleaMarketScenePages(t *testing.T) {
	service := &fleaMarketSceneService{listings: fleaMarketListings(5)}
	_, router := fleaMarketSceneRouter(t, service, playercontext.Selection{Destination: "fleamarket", Offset: 2, Limit: 2})
	status, observation := navigationGET(t, router)
	data := decodeShopScene[FleaMarketSceneData](t, observation)
	if status != 200 || len(data.Listings) != 2 || data.Listings[0].ID != "l2" || data.ListingCount != 5 || data.Page.Next == nil || data.Page.Next.Offset != 4 || data.Page.Next.Limit != 2 {
		t.Fatalf("page: %d %+v", status, data)
	}
	if !slices.ContainsFunc(observation.AvailableActions, func(a ContextAction) bool { return a.Action == "scene_page" }) {
		t.Fatalf("missing page choice: %+v", observation.AvailableActions)
	}
	_, router = fleaMarketSceneRouter(t, service, playercontext.Selection{Destination: "fleamarket", Offset: 4, Limit: 2})
	_, observation = navigationGET(t, router)
	if data := decodeShopScene[FleaMarketSceneData](t, observation); data.Page.Next != nil || len(data.Listings) != 1 {
		t.Fatalf("last page: %+v", data)
	}
}

func TestFleaMarketSceneRequiredReadsFailWholeObservation(t *testing.T) {
	for name, service := range map[string]*fleaMarketSceneService{
		"listings": {listErr: errors.New("private list failure")},
		"owned":    {ownErr: errors.New("private owned failure")},
	} {
		t.Run(name, func(t *testing.T) {
			_, router := fleaMarketSceneRouter(t, service, playercontext.Selection{Destination: "fleamarket"})
			if status, _ := navigationGET(t, router); status != 500 {
				t.Fatalf("partial observation: %d", status)
			}
		})
	}
	_, router := fleaMarketSceneRouter(t, nil, playercontext.Selection{Destination: "fleamarket"})
	if status, _ := navigationGET(t, router); status != 501 {
		t.Fatalf("unconfigured: %d", status)
	}
}

func TestFleaMarketSceneCancelRefreshesOwnedListings(t *testing.T) {
	service := &fleaMarketSceneService{
		listings: []fleamarket.Listing{
			{ID: "mine-1", SellerCharacterID: "hero", Status: fleamarket.StatusActive},
			{ID: "mine-2", SellerCharacterID: "hero", Status: fleamarket.StatusActive},
		},
		owned: []fleamarket.Listing{
			{ID: "mine-1", SellerCharacterID: "hero", Status: fleamarket.StatusActive},
			{ID: "mine-2", SellerCharacterID: "hero", Status: fleamarket.StatusActive},
		},
	}
	_, router := fleaMarketSceneRouter(t, service, playercontext.Selection{Destination: "fleamarket"})
	status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"fleamarket_cancel","params":{"listing_id":"mine-1"}}`, nil)
	if status != 200 || string(raw["success"]) != "true" || service.mutations != 1 {
		t.Fatalf("cancel action: %d %s", status, raw)
	}
	var refreshed struct {
		Scene struct {
			Data FleaMarketSceneData `json:"data"`
		} `json:"scene"`
	}
	if err := json.Unmarshal(raw["context"], &refreshed); err != nil {
		t.Fatalf("decode refreshed context: %v (%s)", err, raw["context"])
	}
	if len(refreshed.Scene.Data.OwnedListings) != 1 || refreshed.Scene.Data.OwnedListings[0].ID != "mine-2" {
		t.Fatalf("expected cancelled listing excluded from owned_listings, got %+v", refreshed.Scene.Data.OwnedListings)
	}
}
