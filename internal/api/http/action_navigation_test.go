package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/playercontext"
)

type gatewayNavigationStore struct {
	selection        playercontext.Selection
	writes           int
	loadErr, saveErr error
	afterSave        func()
}

func (s *gatewayNavigationStore) Load(ctx context.Context, _ string) (playercontext.Selection, error) {
	if err := ctx.Err(); err != nil {
		return playercontext.Selection{}, err
	}
	return s.selection, s.loadErr
}

func (s *gatewayNavigationStore) Save(ctx context.Context, actor string, n playercontext.Selection) error {
	if actor != "hero" {
		return errors.New("target replaced actor")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.saveErr != nil {
		return s.saveErr
	}
	s.writes++
	s.selection = n
	if s.afterSave != nil {
		s.afterSave()
	}
	return nil
}

func navigationRouter(t *testing.T, f *gatewayFixture, store *gatewayNavigationStore, available *bool, readErr *error) http.Handler {
	t.Helper()
	service := playercontext.NewService(f, f, timer.NewService(nil), playercontext.WithNavigation(store,
		playercontext.SceneDefinition{ID: "town", Pageable: true},
		playercontext.SceneDefinition{ID: "bank", Parent: "town"},
		playercontext.SceneDefinition{ID: "shop_weapon", Parent: "town", SubjectKind: "item", Pageable: true,
			SubjectAvailable: func(ctx context.Context, actor, target string) (bool, error) {
				f.checkContext(ctx)
				if actor != "hero" {
					t.Fatal("wrong viewer")
				}
				return *available && target == "weapon-01", *readErr
			}},
	))
	h, err := NewHandler(f, f, &struct{ AdventureService }{}, &struct{ ShopService }{}, WithPlayerContext(service), func(h *Handler) { h.homes = f })
	if err != nil {
		t.Fatal(err)
	}
	return h.Router()
}

func navigationGET(t *testing.T, router http.Handler) (int, PlayerContextResponse) {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/characters/hero/context", nil)
	r.Header.Set("Authorization", "Bearer session")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	var response PlayerContextResponse
	if w.Code == 200 {
		var slots map[string]json.RawMessage
		if err := json.Unmarshal(w.Body.Bytes(), &slots); err != nil {
			t.Fatal(err)
		}
		if len(slots) != 4 || string(slots["ongoing_actions"]) == "null" || string(slots["available_actions"]) == "null" {
			t.Fatal("lost four-slot arrays")
		}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
	}
	return w.Code, response
}

func TestNavigationGatewayAndGETShareSelection(t *testing.T) {
	f, store, available := newGatewayFixture(t), &gatewayNavigationStore{}, true
	var readErr error
	router := navigationRouter(t, f, store, &available, &readErr)
	if status, got := navigationGET(t, router); status != 200 || got.Scene.LocationID != "town" || store.writes != 0 {
		t.Fatalf("default: %d %+v", status, got)
	}
	for _, body := range []string{
		`{"action":"scene_enter","params":{"destination":"shop_weapon"}}`,
		`{"action":"scene_page","params":{"destination":"shop_weapon","offset":20}}`,
		`{"action":"scene_select","params":{"target_kind":"item","target_id":"weapon-01"}}`,
		`{"action":"scene_back"}`,
		`{"action":"scene_back","params":{}}`,
	} {
		status, raw := gatewayRequest(t, router, "hero", "session", "application/json", body, nil)
		if status != 200 || string(raw["success"]) != "true" {
			t.Fatalf("%s: %d %s", body, status, raw)
		}
		var refreshed PlayerContextResponse
		if err := json.Unmarshal(raw["context"], &refreshed); err != nil {
			t.Fatal(err)
		}
		status, observed := navigationGET(t, router)
		if status != 200 || !reflect.DeepEqual(observed.Scene, refreshed.Scene) || observed.Scene.LocationID != store.selection.Destination {
			t.Fatalf("GET differs: %+v / %+v", observed, refreshed)
		}
	}
	if store.writes != 5 || f.char.Money != 100 || f.char.Tired != 0 || len(f.actions) != 0 {
		t.Fatal("GET wrote or navigation changed gameplay")
	}
}

func TestNavigationGatewayRejectsSpoofedAndMalformedInput(t *testing.T) {
	for _, tc := range []struct {
		body, actor, token string
		status             int
	}{
		{`{"action":"scene_enter","params":{"destination":"bank"}}`, "hero", "", 401},
		{`{"action":"scene_enter","params":{"destination":"bank"}}`, "other", "session", 403},
		{`{"action":"scene_enter","params":{"destination":"bank"}}`, "missing", "session", 404},
		{`{"action":"scene_enter","params":{"destination":"invented"}}`, "hero", "session", 400},
		{`{"action":"scene_enter","params":{"destination":null}}`, "hero", "session", 400},
		{`{"action":"scene_enter","params":{"destination":"bank","character_id":"other"}}`, "hero", "session", 400},
		{`{"action":"scene_select","params":{"target_kind":"home","target_id":"other"}}`, "hero", "session", 400},
		{`{"action":"scene_select","params":{"target_kind":"item","target_id":"invented"}}`, "hero", "session", 404},
		{`{"action":"scene_select","params":{"target_kind":"item","target_id":1}}`, "hero", "session", 400},
		{`{"action":"scene_page","params":{"destination":"bank","offset":0}}`, "hero", "session", 400},
		{`{"action":"scene_page","params":{"destination":"shop_weapon","offset":-1}}`, "hero", "session", 400},
		{`{"action":"scene_page","params":{"destination":"shop_weapon","offset":0,"limit":0}}`, "hero", "session", 400},
		{`{"action":"scene_page","params":{"destination":"shop_weapon","offset":0,"Limit":0}}`, "hero", "session", 400},
		{`{"action":"scene_page","params":{"destination":"shop_weapon","offset":0,"limit":null}}`, "hero", "session", 400},
		{`{"action":"scene_page","params":{"destination":"shop_weapon","offset":0,"limit":101}}`, "hero", "session", 400},
		{`{"action":"scene_page","params":{"destination":"shop_weapon","offset":0,"cursor":"other-scene"}}`, "hero", "session", 400},
		{`{"action":"scene_page","params":{"destination":"shop_weapon","cursor":""}}`, "hero", "session", 400},
		{`{"action":"scene_page","params":{"destination":"shop_weapon"}}`, "hero", "session", 400},
		{`{"action":"scene_back","params":{"player_id":"other"}}`, "hero", "session", 400},
	} {
		t.Run(tc.body+tc.actor+tc.token, func(t *testing.T) {
			f, store, available := newGatewayFixture(t), &gatewayNavigationStore{selection: playercontext.Selection{Destination: "shop_weapon"}}, true
			var readErr error
			status, got := gatewayRequest(t, navigationRouter(t, f, store, &available, &readErr), tc.actor, tc.token, "application/json", tc.body, nil)
			if status != tc.status || store.writes != 0 {
				t.Fatalf("status %d, writes %d: %s", status, store.writes, got)
			}
		})
	}
}

func TestNavigationGatewayFailuresAndGETOnlyRecovery(t *testing.T) {
	for _, failure := range []string{"load", "save", "profile refresh", "store refresh", "required subject read", "sleep", "run", "canceled"} {
		t.Run(failure, func(t *testing.T) {
			f, store, available := newGatewayFixture(t), &gatewayNavigationStore{}, true
			var readErr error
			wantStatus, wantWrites := 500, 0
			ctx := context.Background()
			switch failure {
			case "load":
				store.loadErr = errors.New("secret storage failure")
			case "save":
				store.saveErr = errors.New("secret storage failure")
			case "profile refresh":
				store.afterSave = func() { f.profileErr = errors.New("secret profile failure") }
				wantStatus, wantWrites = 200, 1
			case "store refresh":
				store.afterSave = func() { store.loadErr = errors.New("secret storage failure") }
				wantStatus, wantWrites = 200, 1
			case "required subject read":
				store.selection = playercontext.Selection{Destination: "shop_weapon", Subject: playercontext.Subject{Kind: "item", ID: "weapon-01"}}
				readErr = errors.New("secret subject failure")
			case "sleep":
				f.sleep.CanWake = true
				wantStatus = 409
			case "run":
				f.actions = []scheduling.ScheduledAction{{ActorID: "hero", State: scheduling.StateProcessing}}
				wantStatus = 409
			case "canceled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			f.expectedContext = ctx
			router := navigationRouter(t, f, store, &available, &readErr)
			status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"scene_enter","params":{"destination":"bank"}}`, ctx)
			if status != wantStatus || store.writes != wantWrites {
				t.Fatalf("status %d writes %d: %s", status, store.writes, raw)
			}
			f.expectedContext = nil
			if wantWrites == 1 {
				if string(raw["context"]) != "null" || string(raw["success"]) != "true" {
					t.Fatalf("lost known success: %s", raw)
				}
				var detail ErrorDetail
				if err := json.Unmarshal(raw["context_error"], &detail); err != nil || detail.Code != "CONTEXT_REFRESH_FAILED" {
					t.Fatalf("refresh code: %+v %v", detail, err)
				}
				store.loadErr, f.profileErr = nil, nil
				status, got := navigationGET(t, router)
				if status != 200 || got.Scene.LocationID != "bank" || store.writes != 1 {
					t.Fatalf("GET recovery: %d %+v", status, got)
				}
			}
		})
	}
}

func TestNavigationUnavailableSubjectAndRequiredReadFailure(t *testing.T) {
	f, store, available := newGatewayFixture(t), &gatewayNavigationStore{selection: playercontext.Selection{Destination: "shop_weapon", Subject: playercontext.Subject{Kind: "item", ID: "weapon-01"}}}, false
	var readErr error
	router := navigationRouter(t, f, store, &available, &readErr)
	status, got := navigationGET(t, router)
	if status != 200 || got.Scene.Kind != "selection_unavailable" || got.Scene.Subject == nil || *got.Scene.Subject != store.selection.Subject || store.writes != 0 {
		t.Fatalf("unavailable: %d %+v", status, got)
	}
	readErr = errors.New("required subject store failed")
	if status, _ := navigationGET(t, router); status != 500 || store.writes != 0 {
		t.Fatalf("hidden error: %d", status)
	}
}

func TestNavigationKnownRejectionSurvivesFailedRefresh(t *testing.T) {
	f, store, available := newGatewayFixture(t), &gatewayNavigationStore{selection: playercontext.Selection{Destination: "bank"}}, true
	f.profileErr = errors.New("required profile read failed")
	var readErr error
	router := navigationRouter(t, f, store, &available, &readErr)
	status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"scene_enter","params":{"destination":"invented"}}`, nil)
	if status != 400 || string(raw["context"]) != "null" || store.writes != 0 {
		t.Fatalf("lost rejection: %d %s", status, raw)
	}
	assertGatewayError(t, raw, "INVALID_SELECTION")
	var detail ErrorDetail
	if err := json.Unmarshal(raw["context_error"], &detail); err != nil || detail.Code != "CONTEXT_REFRESH_FAILED" {
		t.Fatalf("refresh code: %+v %v", detail, err)
	}
	f.profileErr = nil
	if status, got := navigationGET(t, router); status != 200 || got.Scene.LocationID != store.selection.Destination || store.writes != 0 {
		t.Fatalf("GET recovery: %d %+v", status, got)
	}
}
