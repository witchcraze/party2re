package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	apihttp "github.com/witchcraze/party2re/internal/api/http"
	"github.com/witchcraze/party2re/internal/auction"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
)

type stubAuctionService struct {
	inspectFn     func(ctx context.Context, inspectorCharacterID, targetCharacterID string) (auction.InspectResult, error)
	inspectByName func(ctx context.Context, inspectorCharacterID, targetName string) (auction.InspectResult, error)
	getVenueFn    func() auction.VenueInfo
}

func (*stubAuctionService) Send(context.Context, auction.SendRequest) (auction.SendResult, error) {
	panic("retained Auction reads must not send")
}

func (s *stubAuctionService) Inspect(ctx context.Context, inspectorCharacterID, targetCharacterID string) (auction.InspectResult, error) {
	if s.inspectFn != nil {
		return s.inspectFn(ctx, inspectorCharacterID, targetCharacterID)
	}
	return auction.InspectResult{
		CharacterID: targetCharacterID,
		Name:        "Bob",
		Level:       10,
		Money:       5000,
	}, nil
}

func (s *stubAuctionService) InspectByName(ctx context.Context, inspectorCharacterID, targetName string) (auction.InspectResult, error) {
	if s.inspectByName != nil {
		return s.inspectByName(ctx, inspectorCharacterID, targetName)
	}
	return auction.InspectResult{
		CharacterID: "c2",
		Name:        targetName,
		Level:       10,
		Money:       5000,
	}, nil
}

func (s *stubAuctionService) GetVenueInfo() auction.VenueInfo {
	if s.getVenueFn != nil {
		return s.getVenueFn()
	}
	return auction.VenueInfo{
		Title:   auction.VenueName,
		NPCName: auction.NPCName,
	}
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

	t.Run("GET /auction/hall - public venue info", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/auction/hall", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
		var info struct {
			Title    string   `json:"title"`
			NPCName  string   `json:"npc_name"`
			Dialogue []string `json:"dialogue"`
		}
		decodeResponseBody(t, rec.Body.Bytes(), &info)
		if info.Title != "オークション会場" || info.NPCName != "@ワイルド" {
			t.Errorf("unexpected venue info: %+v", info)
		}
		if len(info.Dialogue) != 3 {
			t.Errorf("expected 3 dialogue entries, got %d", len(info.Dialogue))
		}
	})

	t.Run("GET /characters/{id}/auction/inspect - inspect success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/characters/c1/auction/inspect?target_character_id=c2", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
		var res auction.InspectResult
		decodeResponseBody(t, rec.Body.Bytes(), &res)
		if res.CharacterID != "c2" || res.Name != "Bob" {
			t.Errorf("unexpected inspect result: %+v", res)
		}
	})

	t.Run("Obsolete fictional auction endpoints return 404", func(t *testing.T) {
		obsoleteRoutes := []struct {
			method string
			path   string
		}{
			{http.MethodGet, "/auctions"},
			{http.MethodGet, "/auctions/123"},
			{http.MethodPost, "/auctions"},
			{http.MethodPost, "/auctions/123/bid"},
			{http.MethodPost, "/auctions/123/buyout"},
			{http.MethodPost, "/auctions/123/cancel"},
		}

		for _, tc := range obsoleteRoutes {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			req.Header.Set("Authorization", "Bearer valid-token")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusNotFound {
				t.Errorf("expected 404 Not Found for obsolete route %s %s, got %d", tc.method, tc.path, rec.Code)
			}
		}
	})
}
