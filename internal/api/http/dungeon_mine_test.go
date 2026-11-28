package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	apihttp "github.com/witchcraze/party2re/internal/api/http"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/dungeon"
)

// The mine route passes the authenticated character and maps outcomes and
// admission errors.
func TestDungeonMineHTTP(t *testing.T) {
	for _, tc := range []struct {
		name    string
		res     dungeon.ExpeditionStepResult
		err     error
		status  int
		message string
	}{
		{"nothing", dungeon.ExpeditionStepResult{EventType: dungeon.EventMineEmpty}, nil, 200, "目ぼしいものは掘り出せなかった…"},
		{"depleted", dungeon.ExpeditionStepResult{EventType: dungeon.EventMineDepleted}, nil, 200, "もう掘り出せるものがなさそうだ…"},
		{"no site", dungeon.ExpeditionStepResult{}, dungeon.ErrNoMiningSite, 409, ""},
		{"enemy", dungeon.ExpeditionStepResult{}, dungeon.ErrCombatActive, 409, ""},
		{"fallen", dungeon.ExpeditionStepResult{}, dungeon.ErrActorFallen, 422, ""},
		{"no run", dungeon.ExpeditionStepResult{}, dungeon.ErrNoActiveExpedition, 404, ""},
		{"foreign", dungeon.ExpeditionStepResult{}, dungeon.ErrForbidden, 403, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var actor string
			ds := &stubDungeonService{mineFn: func(_ context.Context, id string) (dungeon.ExpeditionStepResult, error) {
				actor = id
				return tc.res, tc.err
			}}
			player := coreplayer.Player{ID: "p1"}
			h := newTestHandler(t, &stubPlayerService{authenticateFn: alwaysAuthPlayer(player)}, &stubCharacterService{getFn: func(context.Context, string) (corecharacter.Character, error) {
				return corecharacter.Character{ID: "c1", PlayerID: player.ID}, nil
			}}, &stubAdventureService{}, &stubShopService{}, apihttp.WithDungeon(ds))
			req := httptest.NewRequest(http.MethodPost, "/characters/c1/dungeons/mine", nil)
			req.Header.Set("Authorization", "Bearer valid-token")
			rec := httptest.NewRecorder()
			h.Router().ServeHTTP(rec, req)
			if actor != "c1" || rec.Code != tc.status {
				t.Fatalf("actor %q status %d body %s", actor, rec.Code, rec.Body.String())
			}
			if tc.status != 200 {
				return
			}
			var body struct {
				Result struct {
					EventType string `json:"event_type"`
					Message   string `json:"message"`
				} `json:"result"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Result.EventType != string(tc.res.EventType) || body.Result.Message != tc.message {
				t.Fatalf("body %s err=%v", rec.Body.String(), err)
			}
		})
	}
}
