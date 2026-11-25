package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	apihttp "github.com/witchcraze/party2re/internal/api/http"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/dungeon"
)

// The listing is filtered by the viewer's job level; a gated start is 403 and
// a consumed once quota is 422.
func TestDungeonJobLevelAdmissionHTTP(t *testing.T) {
	startErr := dungeon.ErrJobLevelRequired
	ds := &stubDungeonService{
		startExpeditionFn: func(context.Context, string, string) (*dungeon.ActiveExpedition, error) {
			return nil, startErr
		},
		startPartyExpeditionFn: func(context.Context, string, []string, string, string) (*dungeon.ActiveExpedition, error) {
			return nil, startErr
		},
	}
	h := newTestHandler(t, &stubPlayerService{authenticateFn: alwaysAuthPlayer(coreplayer.Player{ID: "p1"})}, &stubCharacterService{getFn: func(context.Context, string) (corecharacter.Character, error) {
		return corecharacter.Character{ID: "c1", PlayerID: "p1", JobLevel: 6}, nil
	}}, &stubAdventureService{}, &stubShopService{}, apihttp.WithDungeon(ds))
	serve := func(method, path, body string) int {
		req := jsonRequest(t, method, path, body)
		req.Header.Set("Authorization", "Bearer token")
		rec := httptest.NewRecorder()
		h.Router().ServeHTTP(rec, req)
		return rec.Code
	}
	if code := serve(http.MethodGet, "/characters/c1/dungeons", ""); code != 200 || ds.listedJobLevel != 6 {
		t.Fatalf("list status=%d job level=%d", code, ds.listedJobLevel)
	}
	for _, path := range []string{"start", "party-start"} {
		if code := serve(http.MethodPost, "/characters/c1/dungeons/"+path, `{"dungeon_id":"native-map-3"}`); code != http.StatusForbidden {
			t.Fatalf("%s status=%d", path, code)
		}
	}
	startErr = dungeon.ErrOnceQuotaUsed
	for _, path := range []string{"start", "party-start"} {
		if code := serve(http.MethodPost, "/characters/c1/dungeons/"+path, `{"dungeon_id":"quota-map"}`); code != http.StatusUnprocessableEntity {
			t.Fatalf("%s once quota status=%d", path, code)
		}
	}
}
