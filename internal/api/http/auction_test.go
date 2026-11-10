package http_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	apihttp "github.com/witchcraze/party2re/internal/api/http"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
)

func TestRetiredAuctionRoutesReturnNotFound(t *testing.T) {
	pService := &stubPlayerService{authenticateFn: alwaysAuthPlayer(coreplayer.Player{ID: "p1", Username: "hero"})}
	router := newTestHandler(t, pService, &stubCharacterService{}, &stubAdventureService{}, &stubShopService{}, apihttp.WithAuction(nil)).Router()
	for _, tc := range []struct{ method, path string }{
		// Venue talk and player inspection moved to npc_talk and player_inspect.
		{http.MethodGet, "/auction/hall"},
		{http.MethodGet, "/characters/c1/auction/inspect?target_character_id=c2"},
		{http.MethodGet, "/auctions"},
		{http.MethodPost, "/auctions/123/bid"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s %s: got %d, want 404", tc.method, tc.path, rec.Code)
		}
	}
}
