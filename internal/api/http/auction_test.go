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
	sendFn        func(ctx context.Context, req auction.SendRequest) (auction.SendResult, error)
	inspectFn     func(ctx context.Context, inspectorCharacterID, targetCharacterID string) (auction.InspectResult, error)
	inspectByName func(ctx context.Context, inspectorCharacterID, targetName string) (auction.InspectResult, error)
	getVenueFn    func() auction.VenueInfo
}

func (s *stubAuctionService) Send(ctx context.Context, req auction.SendRequest) (auction.SendResult, error) {
	if s.sendFn != nil {
		return s.sendFn(ctx, req)
	}
	return auction.SendResult{
		SenderCharacterID:   req.SenderCharacterID,
		TargetCharacterID:   req.TargetCharacterID,
		TargetCharacterName: req.TargetCharacterName,
		TransferredGold:     req.Gold,
		Message:             "送金完了",
	}, nil
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
		Title:    auction.VenueName,
		NPCName:  auction.NPCName,
		Dialogue: auction.DialogueWords,
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
		var info auction.VenueInfo
		decodeResponseBody(t, rec.Body.Bytes(), &info)
		if info.Title != "オークション会場" || info.NPCName != "@ワイルド" {
			t.Errorf("unexpected venue info: %+v", info)
		}
	})

	t.Run("POST /characters/{id}/auction/send - send gold success", func(t *testing.T) {
		req := jsonRequest(t, http.MethodPost, "/characters/c1/auction/send", `{"target_character_id":"c2","gold":500}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
		var res auction.SendResult
		decodeResponseBody(t, rec.Body.Bytes(), &res)
		if res.TransferredGold != 500 {
			t.Errorf("expected 500 transferred gold, got %d", res.TransferredGold)
		}
	})

	t.Run("POST /characters/{id}/auction/send - send item success", func(t *testing.T) {
		req := jsonRequest(t, http.MethodPost, "/characters/c1/auction/send", `{"target_character_id":"c2","slot":"weapon"}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /characters/{id}/auction/send - errors mapped to bad request", func(t *testing.T) {
		testCases := []struct {
			name string
			err  error
		}{
			{"cannot send to self", auction.ErrCannotSendToSelf},
			{"insufficient money", auction.ErrInsufficientMoney},
			{"depot full", auction.ErrDepotFull},
			{"taboo item", auction.ErrTabooItem},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				handler := newTestHandler(
					t,
					pService,
					cService,
					&stubAdventureService{},
					&stubShopService{},
					apihttp.WithAuction(&stubAuctionService{
						sendFn: func(_ context.Context, _ auction.SendRequest) (auction.SendResult, error) {
							return auction.SendResult{}, tc.err
						},
					}),
				)
				req := jsonRequest(t, http.MethodPost, "/characters/c1/auction/send", `{"target_character_id":"c2","gold":100}`)
				req.Header.Set("Authorization", "Bearer valid-token")
				rec := httptest.NewRecorder()
				handler.Router().ServeHTTP(rec, req)

				if rec.Code != http.StatusBadRequest {
					t.Errorf("expected 400 Bad Request, got %d", rec.Code)
				}
			})
		}
	})

	t.Run("POST /auction/send - legacy endpoint success", func(t *testing.T) {
		req := jsonRequest(t, http.MethodPost, "/auction/send", `{"sender_character_id":"c1","target_character_id":"c2","gold":200}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
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
