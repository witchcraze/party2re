package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"testing"

	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/dungeon"
	"github.com/witchcraze/party2re/internal/playercontext"
)

type dungeonSceneFixture struct {
	DungeonService
	observeExpeditionFn func(context.Context, string) (*dungeon.ExpeditionObservation, error)
}

func (f *dungeonSceneFixture) ObserveExpedition(ctx context.Context, actor string) (*dungeon.ExpeditionObservation, error) {
	return f.observeExpeditionFn(ctx, actor)
}

func TestDungeonSceneReadAndRefresh(t *testing.T) {
	for _, tc := range []struct {
		name, role string
		status     dungeon.ExpeditionStatus
		controls   dungeon.ContinuationControls
		actions    []string
	}{
		{"exploring", "actor", dungeon.StatusExploring, dungeon.ContinuationControls{CanMove: true, CanEscape: true}, []string{"dungeon_move", "dungeon_escape"}},
		{"encounter", "actor", dungeon.StatusExploring, dungeon.ContinuationControls{CanFight: true, CanEscape: true}, []string{"dungeon_fight", "dungeon_escape"}},
		{"member", "member", dungeon.StatusExploring, dungeon.ContinuationControls{}, []string{}},
		{"escaped", "actor", dungeon.StatusEscaped, dungeon.ContinuationControls{CanFight: true, CanEscape: true}, []string{"dungeon_fight", "dungeon_escape"}},
		{"wiped_out", "actor", dungeon.StatusWipedOut, dungeon.ContinuationControls{CanFight: true, CanEscape: true}, []string{"dungeon_fight", "dungeon_escape"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newGatewayFixture(t)
			selection := &gatewayNavigationStore{selection: playercontext.Selection{Destination: "bank"}, loadErr: errors.New("lost selector")}
			fact := playercontext.Activity{Kind: "dungeon", ID: "run", Revision: 7, PartyID: "party", Role: tc.role, Phase: string(tc.status), Round: 2, Actions: tc.actions}
			view := dungeon.ExpeditionObservation{ID: "run", LeaderID: "hero", PartyID: "party", DungeonID: "dungeon-01", Revision: 7, Status: tc.status, CurrentFloor: 2, TurnsRemaining: 12, Directions: []dungeon.Direction{}, Controls: tc.controls}
			if tc.role == "member" {
				view.LeaderID = "leader"
			}
			if tc.status == dungeon.StatusExploring {
				view.Map = &dungeon.MapView{Radius: 1, Tiles: [][]string{{"■", "□", "■"}, {"□", "●", "□"}, {"■", "■", "■"}}}
			}
			if tc.controls.CanMove {
				view.Directions = []dungeon.Direction{dungeon.DirectionEast}
			}
			pc := playercontext.NewService(f, f, timer.NewService(nil), playercontext.WithNavigation(selection, playercontext.SceneDefinition{ID: "town"}, playercontext.SceneDefinition{ID: "bank", Parent: "town"}), playercontext.WithActivities(func(context.Context, string, string) ([]playercontext.Activity, error) {
				return []playercontext.Activity{fact}, nil
			}))
			var detailErr error
			reads := 0
			svc := &dungeonSceneFixture{observeExpeditionFn: func(ctx context.Context, actor string) (*dungeon.ExpeditionObservation, error) {
				f.checkContext(ctx)
				if actor != "hero" {
					t.Fatal("lost actor")
				}
				reads++
				cp := view
				return &cp, detailErr
			}}
			router := f.router("rescue_request", timer.NewService(nil), WithPlayerContext(pc), WithDungeon(svc))
			status, obs := navigationGET(t, router)
			if status != 200 {
				t.Fatalf("GET: %d %+v", status, obs)
			}
			data := decodeShopScene[ActivitySceneData](t, obs)
			if data.Dungeon == nil || !reflect.DeepEqual(*data.Dungeon, view) || !slices.Equal(data.Activities[0].Actions, tc.actions) || obs.Scene.LocationID != "dungeon" || selection.writes != 0 || f.executions != 0 {
				t.Fatalf("observation: %+v", obs)
			}
			for _, support := range obs.Scene.Support.Actions {
				if slices.Contains(tc.actions, support.Action) && support.Connected {
					t.Fatal("read connected dungeon mutation")
				}
			}
			req := httptest.NewRequest(http.MethodGet, "/api/v1/characters/other/context", nil)
			req.Header.Set("Authorization", "Bearer session")
			rec := httptest.NewRecorder()
			before := reads
			router.ServeHTTP(rec, req)
			if rec.Code != 403 || reads != before {
				t.Fatalf("unowned GET: %d reads=%d", rec.Code, reads-before)
			}
			status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"rescue_request","params":{"reason":"stuck"}}`, nil)
			var refreshed PlayerContextResponse
			if err := json.Unmarshal(raw["context"], &refreshed); err != nil {
				t.Fatal(err)
			}
			if status != 200 || !reflect.DeepEqual(obs, refreshed) || f.executions != 1 {
				t.Fatalf("refresh differs: %d %s", status, raw)
			}
			detailErr = errors.New("map source unavailable")
			if status, _ := navigationGET(t, router); status != 500 {
				t.Fatalf("partial map: %d", status)
			}
			status, raw = gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"rescue_request","params":{"reason":"stuck"}}`, nil)
			if status != 200 || string(raw["success"]) != "true" || string(raw["context"]) != "null" || raw["result"] == nil || f.executions != 2 {
				t.Fatalf("lost outcome: %d %s", status, raw)
			}
			var recovery ErrorDetail
			if err := json.Unmarshal(raw["context_error"], &recovery); err != nil || recovery.Code != "CONTEXT_REFRESH_FAILED" {
				t.Fatalf("recovery: %s %v", raw, err)
			}
			detailErr = nil
			if status, _ := navigationGET(t, router); status != 200 || f.executions != 2 {
				t.Fatalf("GET recovery replayed mutation: %d", status)
			}
			for _, change := range []string{"revision", "run", "phase", "role", "party", "floor", "controls", "ended"} {
				original := view
				switch change {
				case "revision":
					view.Revision++
				case "run":
					view.ID = "replacement"
				case "phase":
					view.Status = "changed"
				case "role":
					if view.LeaderID == "hero" {
						view.LeaderID = "other"
					} else {
						view.LeaderID = "hero"
					}
				case "party":
					view.PartyID = "other"
				case "floor":
					view.CurrentFloor++
				case "controls":
					view.Controls.CanEscape = !view.Controls.CanEscape
				case "ended":
					svc.observeExpeditionFn = func(context.Context, string) (*dungeon.ExpeditionObservation, error) { return nil, nil }
				}
				if status, _ := navigationGET(t, router); status != 500 {
					t.Fatalf("mixed %s: %d", change, status)
				}
				view = original
			}
		})
	}
}

func TestDungeonMapRetirementPreservesRemainingRoutes(t *testing.T) {
	for _, configured := range []bool{false, true} {
		f := newGatewayFixture(t)
		var svc DungeonService
		if configured {
			svc = &dungeonSceneFixture{observeExpeditionFn: func(context.Context, string) (*dungeon.ExpeditionObservation, error) {
				t.Fatal("retired route reached Dungeon")
				return nil, nil
			}}
		}
		router := f.router("rescue_request", timer.NewService(nil), WithDungeon(svc))
		for _, actor := range []struct{ id, token string }{{"hero", ""}, {"hero", "session"}, {"other", "session"}, {"missing", "session"}} {
			req := httptest.NewRequest(http.MethodGet, "/characters/"+actor.id+"/dungeons/map", nil)
			req.Header.Set("Authorization", "Bearer "+actor.token)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != 404 || f.executions != 0 || f.characterCalls != 0 || f.profileCalls != 0 {
				t.Fatalf("retired route: %d %s", rec.Code, rec.Body.String())
			}
		}
	}
	var spec struct {
		Paths      map[string]map[string]json.RawMessage
		Components struct{ Schemas map[string]json.RawMessage }
	}
	if err := json.Unmarshal(OpenAPISpec(), &spec); err != nil {
		t.Fatal(err)
	}
	if _, ok := spec.Paths["/characters/{id}/dungeons/map"]; ok {
		t.Fatal("retired map in spec")
	}
	if _, ok := spec.Components.Schemas["DungeonMapResponse"]; ok {
		t.Fatal("obsolete response schema")
	}
	if len(spec.Components.Schemas["DungeonExpeditionObservation"]) == 0 || len(spec.Components.Schemas["DungeonMapView"]) == 0 {
		t.Fatal("independent scene schemas missing")
	}
	for _, route := range []struct{ method, path string }{
		{"get", "/characters/{id}/dungeons"}, {"get", "/api/v1/characters/{id}/context"}, {"post", "/api/v1/characters/{id}/actions"},
		{"post", "/characters/{id}/dungeons/start"}, {"post", "/characters/{id}/dungeons/party-start"}, {"post", "/characters/{id}/dungeons/move"}, {"post", "/characters/{id}/dungeons/fight"}, {"post", "/characters/{id}/dungeons/escape"},
	} {
		if len(spec.Paths[route.path][route.method]) == 0 {
			t.Errorf("retained route missing: %s %s", route.method, route.path)
		}
	}
	for _, endpoint := range defaultActionEndpoints() {
		if endpoint.PathPattern == "/characters/%s/dungeons/map" {
			t.Fatal("retired resolver entry")
		}
	}
}
