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
	"github.com/witchcraze/party2re/internal/dungeon"
)

func TestDungeonCombatHTTP(t *testing.T) {
	for _, tc := range []struct {
		name, path, body, owner, token string
		domainErr                      error
		status                         int
		called                         bool
	}{
		{"blocked movement", "move", `{"direction":"north"}`, "p1", "token", dungeon.ErrCombatActive, 409, true},
		{"resume", "fight", "", "p1", "token", nil, 200, true},
		{"no encounter", "fight", "", "p1", "token", dungeon.ErrNoActiveCombat, 409, true},
		{"stale combat", "fight", "", "p1", "token", dungeon.ErrExpeditionStateChanged, 409, true},
		{"no expedition", "fight", "", "p1", "token", dungeon.ErrNoActiveExpedition, 404, true},
		{"nonleader", "fight", "", "p1", "token", dungeon.ErrForbidden, 403, true},
		{"foreign character", "fight", "", "p2", "token", nil, 403, false},
		{"anonymous", "fight", "", "p1", "", nil, 401, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			run := func(ctx context.Context, charID string) (dungeon.ExpeditionStepResult, error) {
				called = true
				if charID != "c1" || ctx.Err() != nil {
					t.Fatalf("bad actor/context: %s %v", charID, ctx.Err())
				}
				return dungeon.ExpeditionStepResult{EventType: dungeon.EventBattle}, tc.domainErr
			}
			ds := &stubDungeonService{fightFn: run, moveFn: func(ctx context.Context, c string, _ dungeon.Direction) (dungeon.ExpeditionStepResult, error) {
				return run(ctx, c)
			}}
			h := newTestHandler(t, &stubPlayerService{authenticateFn: alwaysAuthPlayer(coreplayer.Player{ID: "p1"})}, &stubCharacterService{getFn: func(context.Context, string) (corecharacter.Character, error) {
				return corecharacter.Character{ID: "c1", PlayerID: tc.owner}, nil
			}}, &stubAdventureService{}, &stubShopService{}, apihttp.WithDungeon(ds))
			req := jsonRequest(t, http.MethodPost, "/characters/c1/dungeons/"+tc.path, tc.body)
			if tc.token != "" {
				req.Header.Set("Authorization", "Bearer "+tc.token)
			}
			rec := httptest.NewRecorder()
			h.Router().ServeHTTP(rec, req)
			if rec.Code != tc.status || called != tc.called {
				t.Fatalf("status=%d called=%v body=%s", rec.Code, called, rec.Body.String())
			}
			if tc.domainErr == dungeon.ErrCombatActive && !strings.Contains(rec.Body.String(), "※敵を全て倒すまで、先に進むことはできません") {
				t.Fatalf("missing movement message: %s", rec.Body.String())
			}
		})
	}
}
