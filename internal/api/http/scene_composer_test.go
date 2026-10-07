package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"testing"

	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/playercontext"
)

func TestSelectedScenesAndConnectedChoices(t *testing.T) {
	f, store, available := newGatewayFixture(t), &gatewayNavigationStore{}, true
	var readErr error
	router := navigationRouter(t, f, store, &available, &readErr)
	status, town := navigationGET(t, router)
	if status != 200 || town.Scene.Kind != "town" || town.Scene.Subject != nil {
		t.Fatalf("town: %d %+v", status, town.Scene)
	}
	if !slices.EqualFunc(town.AvailableActions, []string{"scene_enter", "adventure_start"}, func(a ContextAction, id string) bool { return a.Action == id }) {
		t.Fatalf("unconnected or irrelevant commands offered: %+v", town.AvailableActions)
	}
	var data TownSceneData
	raw, err := json.Marshal(town.Scene.Data)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	if len(data.Destinations) != 2 || data.Page.Limit != 20 || data.Page.Mode != "offset" || data.Page.Next != nil {
		t.Fatalf("town collection: %+v", data)
	}
	store.selection = playercontext.Selection{Destination: "shop_weapon", Subject: playercontext.Subject{Kind: "item", ID: "weapon-01"}}
	status, selected := navigationGET(t, router)
	if status != 200 || selected.Scene.Kind != "subject" || selected.Scene.LocationID != "shop_weapon" || selected.Scene.Subject == nil || *selected.Scene.Subject != store.selection.Subject {
		t.Fatalf("selected: %d %+v", status, selected.Scene)
	}
	if len(selected.AvailableActions) != 1 || selected.AvailableActions[0].Action != "scene_back" {
		t.Fatalf("detail choices: %+v", selected.AvailableActions)
	}
	available = false
	status, missing := navigationGET(t, router)
	if status != 200 || missing.Scene.Kind != "selection_unavailable" || store.writes != 0 {
		t.Fatalf("missing: %d %+v", status, missing.Scene)
	}
	readErr = errors.New("required read failed")
	if status, _ := navigationGET(t, router); status != 500 {
		t.Fatalf("required read: %d", status)
	}
	store.selection = playercontext.Selection{Destination: "retired_facility"}
	status, missing = navigationGET(t, router)
	if status != 200 || missing.Scene.Kind != "selection_unavailable" || len(missing.AvailableActions) != 1 || missing.AvailableActions[0].Action != "scene_enter" || store.writes != 0 {
		t.Fatalf("removed registration has no safe parent: %d %+v", status, missing)
	}
}

func TestSceneCollectionPagingAndEmptyArrays(t *testing.T) {
	rows := make([]SceneDestination, 101)
	for i := range rows {
		rows[i] = SceneDestination{ID: fmt.Sprintf("destination-%03d", i)}
	}
	for _, tc := range []struct {
		offset, limit, count int
		next                 bool
	}{
		{0, 0, 20, true}, {0, 100, 100, true}, {100, 100, 1, false}, {1000000, 20, 0, false},
	} {
		page := pageDestinations(rows, playercontext.Selection{Destination: "town", Offset: tc.offset, Limit: tc.limit})
		if page.Destinations == nil || len(page.Destinations) != tc.count || (page.Page.Next != nil) != tc.next {
			t.Fatalf("page: %+v", page)
		}
		if tc.next && (page.Page.Next.Destination != "town" || page.Page.Next.Offset != tc.offset+tc.count || page.Page.Next.Limit != page.Page.Limit) {
			t.Fatalf("next: %+v", page.Page.Next)
		}
	}
	if got := pageDestinations(nil, playercontext.Selection{}); got.Destinations == nil || got.Page.Next != nil {
		t.Fatalf("empty: %+v", got)
	}
}

func TestTownNextInputsDriveSharedSceneComposition(t *testing.T) {
	f, store, available := newGatewayFixture(t), &gatewayNavigationStore{}, true
	var readErr error
	router := navigationRouter(t, f, store, &available, &readErr)
	status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"scene_page","params":{"destination":"town","offset":0,"limit":1}}`, nil)
	if status != 200 {
		t.Fatalf("page: %d %s", status, raw)
	}
	var response PlayerContextResponse
	if err := json.Unmarshal(raw["context"], &response); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(response.Scene.Data)
	if err != nil {
		t.Fatal(err)
	}
	var data TownSceneData
	if err := json.Unmarshal(encoded, &data); err != nil {
		t.Fatal(err)
	}
	if len(data.Destinations) != 1 || data.Destinations[0].ID != "bank" || data.Page.Next == nil || data.Page.Next.Offset != 1 {
		t.Fatalf("first page: %+v", data)
	}
	index := slices.IndexFunc(response.AvailableActions, func(a ContextAction) bool { return a.Action == "scene_page" })
	if index < 0 || response.AvailableActions[index].ParamsTemplate["destination"] != "town" || response.AvailableActions[index].ParamsTemplate["offset"] != float64(1) {
		t.Fatalf("next offer: %+v", response.AvailableActions)
	}
	body, err := json.Marshal(map[string]any{"action": "scene_page", "params": data.Page.Next})
	if err != nil {
		t.Fatal(err)
	}
	if status, raw := gatewayRequest(t, router, "hero", "session", "application/json", string(body), nil); status != 200 {
		t.Fatalf("next: %d %s", status, raw)
	}
	status, response = navigationGET(t, router)
	if status != 200 || store.writes != 2 {
		t.Fatalf("GET wrote: %d %d", status, store.writes)
	}
	encoded, err = json.Marshal(response.Scene.Data)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(encoded, &data); err != nil {
		t.Fatal(err)
	}
	if len(data.Destinations) != 1 || data.Destinations[0].ID != "shop_weapon" || data.Page.Next != nil {
		t.Fatalf("second page: %+v", data)
	}
}

func TestSceneAdapterFailureAndMissingConnection(t *testing.T) {
	f, store := newGatewayFixture(t), &gatewayNavigationStore{selection: playercontext.Selection{Destination: "bank"}}
	service := playercontext.NewService(f, f, timer.NewService(nil), playercontext.WithNavigation(store,
		playercontext.SceneDefinition{ID: "town"}, playercontext.SceneDefinition{ID: "bank", Parent: "town"}))
	h, err := NewHandler(f, f, &struct{ AdventureService }{}, &struct{ ShopService }{}, WithPlayerContext(service))
	if err != nil {
		t.Fatal(err)
	}
	status, response := navigationGET(t, h.Router())
	if status != 200 || len(response.AvailableActions) != 1 || response.AvailableActions[0].Action != "scene_back" {
		t.Fatalf("bank: %d %+v", status, response)
	}
	if !slices.ContainsFunc(response.Scene.Support.Actions, func(a SceneActionSupport) bool { return a.Action == "bank_deposit" && a.EntryEligible && !a.Connected }) {
		t.Fatalf("connection vs entry: %+v", response.Scene.Support)
	}
	adapter := h.sceneAdapters["bank"]
	adapter.read = nil
	h.sceneAdapters["bank"] = adapter
	if status, _ := navigationGET(t, h.Router()); status != http.StatusNotImplemented {
		t.Fatalf("unconfigured: %d", status)
	}
	adapter.read = func(context.Context, playercontext.Result) (any, error) {
		return nil, errors.New("required adapter failure")
	}
	h.sceneAdapters["bank"] = adapter
	if status, _ := navigationGET(t, h.Router()); status != 500 {
		t.Fatalf("read failure: %d", status)
	}
	store.selection.Destination = "town"
	store.afterSave = func() { adapter.read = nil; h.sceneAdapters["bank"] = adapter }
	status, raw := gatewayRequest(t, h.Router(), "hero", "session", "application/json", `{"action":"scene_enter","params":{"destination":"bank"}}`, nil)
	if status != 200 || string(raw["success"]) != "true" || string(raw["context"]) != "null" || store.writes != 1 {
		t.Fatalf("known result: %d %s", status, raw)
	}
	adapter.read = func(context.Context, playercontext.Result) (any, error) {
		return SelectionSceneData{Parent: "town"}, nil
	}
	h.sceneAdapters["bank"] = adapter
	if status, response := navigationGET(t, h.Router()); status != 200 || response.Scene.Kind != "facility" || store.writes != 1 {
		t.Fatalf("GET recovery: %d %+v", status, response)
	}
}

func TestHomeTemplateUsesExplicitActorAndDoesNotBindNavigation(t *testing.T) {
	f := newHomeGatewayFixture(t)
	f.expectedContext = nil
	store := &gatewayNavigationStore{selection: playercontext.Selection{Destination: "home"}}
	service := playercontext.NewService(f.gatewayFixture, f.gatewayFixture, f.timers, playercontext.WithNavigation(store,
		playercontext.SceneDefinition{ID: "town"}, playercontext.SceneDefinition{ID: "home", Parent: "town"}))
	h, err := NewHandler(f.gatewayFixture, f.gatewayFixture, &struct{ AdventureService }{}, &struct{ ShopService }{}, WithHome(f), WithPlayerContext(service))
	if err != nil {
		t.Fatal(err)
	}
	status, observed := navigationGET(t, h.Router())
	if status != 200 {
		t.Fatalf("status: %d", status)
	}
	action := observed.AvailableActions[slices.IndexFunc(observed.AvailableActions, func(a ContextAction) bool { return a.Action == "home_sleep" })]
	if action.ParamsTemplate["target_home_id"] != "hero" || len(action.RequiredParams) != 0 {
		t.Fatalf("full Home contract: %+v", action)
	}
	store.selection.Destination = "town"
	status, raw := gatewayRequest(t, h.Router(), "hero", "session", "application/json", `{"action":"home_sleep","params":{"target_home_id":"other"}}`, nil)
	if status != 200 || string(raw["success"]) != "true" || f.target != "other" || f.executions != 1 || store.writes != 0 {
		t.Fatalf("explicit target was redirected: %d %s", status, raw)
	}
}

func TestOfferedParamsReuseGatewaySchemaAndKeepCompleteInputs(t *testing.T) {
	h := &Handler{}
	a, err := h.contextAction("scene_page", map[string]any{"destination": "shop_weapon", "offset": 20, "limit": 20})
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Type                 string   `json:"type"`
		AdditionalProperties bool     `json:"additionalProperties"`
		Required             []string `json:"required"`
		Properties           map[string]struct {
			Const any `json:"const"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(a.ParamsSchema, &schema); err != nil {
		t.Fatal(err)
	}
	if schema.Type != "object" || schema.AdditionalProperties || !slices.Equal(a.RequiredParams, []string{"destination", "offset"}) || !slices.Equal(schema.Required, a.RequiredParams) || schema.Properties["destination"].Const != "shop_weapon" {
		t.Fatalf("schema/template: %+v / %+v", a, schema)
	}
}
