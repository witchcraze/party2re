package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/playercontext"
	"github.com/witchcraze/party2re/internal/secretshop"
)

type secretShopSceneService struct {
	SecretShopService
	reads  int
	err    error
	remove bool
}

func (*secretShopSceneService) Talk(context.Context, string) (string, error) {
	return "", errors.New("GET must not execute NPC talk")
}

func (*secretShopSceneService) Inspect(context.Context, string) (string, error) {
	return "", errors.New("GET must not execute NPC inspect")
}

func (*secretShopSceneService) PuffPuff(context.Context, string) (*secretshop.PuffPuffResult, error) {
	return nil, errors.New("GET must not execute NPC puff-puff")
}

func (s *secretShopSceneService) GetShopStatus(ctx context.Context, actor string) (*secretshop.ShopStatus, error) {
	s.reads++
	if s.err != nil {
		return nil, s.err
	}
	status, err := s.SecretShopService.GetShopStatus(ctx, actor)
	if err == nil && s.remove {
		status.Items = nil
	}
	return status, err
}

func secretShopSceneRouter(t *testing.T, options ...func(*playercontext.Service)) (*secretShopGatewayFixture, *gatewayNavigationStore, *secretShopSceneService, http.Handler) {
	t.Helper()
	f := newSecretShopGatewayFixture(t)
	store := &gatewayNavigationStore{}
	reads := &secretShopSceneService{SecretShopService: f}
	pc := playercontext.NewService(f.gatewayFixture, f.gatewayFixture, timer.NewService(nil), playercontext.WithNavigation(store,
		playercontext.SceneDefinition{ID: "town", Pageable: true},
		playercontext.SceneDefinition{ID: "secretshop", Parent: "town", SubjectKind: "item", Pageable: true, CanEnter: secretshop.CheckEligibility,
			SubjectAvailable: func(ctx context.Context, actor, target string) (bool, error) {
				status, err := f.service.GetShopStatus(ctx, actor)
				if err != nil {
					return false, err
				}
				return slices.ContainsFunc(status.Items, func(p secretshop.Item) bool { return p.ID == target }), nil
			}},
	))
	for _, option := range options {
		option(pc)
	}
	h, err := NewHandler(f.gatewayFixture, f, &struct{ AdventureService }{}, &struct{ ShopService }{}, WithSecretShop(reads), WithPlayerContext(pc), func(h *Handler) { h.homes = f.gatewayFixture })
	if err != nil {
		t.Fatal(err)
	}
	return f, store, reads, h.Router()
}

func TestSecretShopSceneQualifiedNavigationAndExplicitPurchase(t *testing.T) {
	f, store, reads, router := secretShopSceneRouter(t)
	f.helperErr = errors.New("town must not read catalog")
	f.char.JobLevel = 6
	status, observation := navigationGET(t, router)
	if status != 200 || len(decodeShopScene[TownSceneData](t, observation).Destinations) != 0 || reads.reads != 0 {
		t.Fatalf("unqualified town: %d %+v", status, observation)
	}
	enter := observation.AvailableActions[slices.IndexFunc(observation.AvailableActions, func(a ContextAction) bool { return a.Action == "scene_enter" })]
	var enterSchema struct {
		Properties map[string]struct{ Enum []string }
	}
	if err := json.Unmarshal(enter.ParamsSchema, &enterSchema); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(enterSchema.Properties["destination"].Enum, "secretshop") {
		t.Fatal("unqualified entry is advertised by the input schema")
	}
	status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"scene_enter","params":{"destination":"secretshop"}}`, nil)
	if status != 403 || store.writes != 0 {
		t.Fatalf("entry bypass: %d %s", status, raw)
	}
	assertGatewayError(t, raw, "SCENE_ACCESS_DENIED")
	f.char.JobLevel, f.helperErr = 7, nil
	status, observation = navigationGET(t, router)
	data := decodeShopScene[TownSceneData](t, observation)
	if status != 200 || len(data.Destinations) != 1 || data.Destinations[0].ID != "secretshop" || !data.Destinations[0].Supported || data.Destinations[0].EnterParams.Destination != "secretshop" || reads.reads != 0 {
		t.Fatalf("qualified town: %+v", data)
	}
	enter = observation.AvailableActions[slices.IndexFunc(observation.AvailableActions, func(a ContextAction) bool { return a.Action == "scene_enter" })]
	if err := json.Unmarshal(enter.ParamsSchema, &enterSchema); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(enterSchema.Properties["destination"].Enum, "secretshop") {
		t.Fatal("qualified entry is missing from the input schema")
	}
	status, raw = gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"scene_enter","params":{"destination":"secretshop"}}`, nil)
	if status != 200 || string(raw["success"]) != "true" || store.writes != 1 {
		t.Fatalf("entry: %d %s", status, raw)
	}
	status, observation = navigationGET(t, router)
	catalog := decodeShopScene[SecretShopCatalogSceneData](t, observation)
	want, err := f.service.GetShopStatus(context.Background(), "hero")
	if err != nil {
		t.Fatal(err)
	}
	slices.SortFunc(want.Items, func(a, b secretshop.Item) int { return strings.Compare(a.ID, b.ID) })
	if status != 200 || observation.Scene.Kind != "facility" || observation.Scene.Support.Observation != "details" || catalog.Title != want.LocationName || catalog.NPCName != want.NPCName || !catalog.IsEligible || catalog.Parent != "town" || catalog.Quantity != (ShopQuantityBounds{Minimum: 1, Maximum: 99}) || len(catalog.Items) != 8 {
		t.Fatalf("catalog: %d %+v", status, catalog)
	}
	for i, p := range catalog.Items {
		if !reflect.DeepEqual(p.Item, want.Items[i]) || p.PurchaseParams.ItemID != p.ID || p.SelectParams != (playercontext.Subject{Kind: "item", ID: p.ID}) {
			t.Fatalf("product: %+v / %+v", p, want.Items[i])
		}
	}
	if status, again := navigationGET(t, router); status != 200 || !reflect.DeepEqual(again.Scene, observation.Scene) || store.writes != 1 || f.writes != 0 || f.executions != 0 {
		t.Fatal("GET changed facts or assets")
	}
	status, raw = gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"scene_select","params":{"target_kind":"item","target_id":"secret_item_herbal_root"}}`, nil)
	if status != 200 {
		t.Fatalf("select: %d %s", status, raw)
	}
	var refreshed PlayerContextResponse
	if err := json.Unmarshal(raw["context"], &refreshed); err != nil {
		t.Fatal(err)
	}
	product := decodeShopScene[SecretShopProductSceneData](t, refreshed).Product
	index := slices.IndexFunc(refreshed.AvailableActions, func(a ContextAction) bool { return a.Action == "secretshop_purchase" })
	if product.ID != "secret_item_herbal_root" || index < 0 || refreshed.AvailableActions[index].ParamsTemplate["item_id"] != product.ID || refreshed.AvailableActions[index].ParamsSchema == nil {
		t.Fatalf("selected command: %+v", refreshed)
	}
	if _, exists := refreshed.AvailableActions[index].ParamsTemplate["quantity"]; exists {
		t.Fatal("quantity was silently bound")
	}
	var purchaseSchema struct {
		Required   []string
		Properties map[string]struct{ Const any }
	}
	if err := json.Unmarshal(refreshed.AvailableActions[index].ParamsSchema, &purchaseSchema); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(purchaseSchema.Required, []string{"item_id", "quantity"}) || purchaseSchema.Properties["item_id"].Const != product.ID || purchaseSchema.Properties["quantity"].Const != nil {
		t.Fatalf("purchase schema: %+v", purchaseSchema)
	}
	// An explicit purchase must retain its target after another client changes selection.
	store.selection.Subject = playercontext.Subject{Kind: "item", ID: "secret_item_magic_mirror"}
	status, raw = f.request(router, fmt.Sprintf(`{"item_id":%q,"quantity":1}`, product.PurchaseParams.ItemID))
	if status != 200 || string(raw["success"]) != "true" || f.executions != 1 || len(f.inv.Items) != 1 || f.inv.Items[0].DefinitionID != "item-010" || f.char.Money != 100000-product.Price || store.writes != 2 {
		t.Fatalf("explicit purchase redirected: %d %s", status, raw)
	}
}

func TestSecretShopScenePagesFiltersAndUnavailableSelection(t *testing.T) {
	f, store, reads, router := secretShopSceneRouter(t)
	store.selection.Destination = "secretshop"
	status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"scene_page","params":{"destination":"secretshop","offset":0,"limit":1}}`, nil)
	if status != 200 {
		t.Fatalf("page: %d %s", status, raw)
	}
	status, observation := navigationGET(t, router)
	data := decodeShopScene[SecretShopCatalogSceneData](t, observation)
	if status != 200 || len(data.Items) != 1 || data.Page.Next == nil || data.Page.Next.Offset != 1 || data.Page.Next.Limit != 1 {
		t.Fatalf("page: %+v", data)
	}
	i := slices.IndexFunc(observation.AvailableActions, func(a ContextAction) bool { return a.Action == "scene_page" })
	if i < 0 || observation.AvailableActions[i].ParamsTemplate["offset"] != float64(1) {
		t.Fatalf("page template: %+v", observation)
	}
	first := data.Items[0].ID
	for _, offset := range []int{1, 7, 1000000} {
		store.selection.Offset = offset
		status, observation = navigationGET(t, router)
		data = decodeShopScene[SecretShopCatalogSceneData](t, observation)
		if status != 200 || data.Items == nil || len(data.Items) != min(1, max(0, 8-offset)) || (offset == 1 && data.Items[0].ID <= first) || (offset >= 7 && data.Page.Next != nil) {
			t.Fatalf("end page: %+v", data)
		}
	}
	store.selection = playercontext.Selection{Destination: "secretshop"}
	want, err := f.service.GetShopStatus(context.Background(), "hero")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range want.Items {
		f.active = append(f.active, p.ItemDefinitionID)
	}
	status, observation = navigationGET(t, router)
	if data = decodeShopScene[SecretShopCatalogSceneData](t, observation); status != 200 || data.Items == nil || len(data.Items) != 0 || data.Page.Next != nil {
		t.Fatalf("empty filtered page: %+v", data)
	}
	status, raw = gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"scene_select","params":{"target_kind":"item","target_id":"secret_item_herbal_root"}}`, nil)
	if status != 404 || store.writes != 1 {
		t.Fatalf("filtered selection: %d %s", status, raw)
	}
	store.selection.Subject = playercontext.Subject{Kind: "item", ID: "secret_item_herbal_root"}
	status, observation = navigationGET(t, router)
	if status != 200 || observation.Scene.Kind != "selection_unavailable" {
		t.Fatalf("filtered saved selection: %+v", observation)
	}
	f.active, reads.remove = nil, true
	status, observation = navigationGET(t, router)
	if status != 200 || observation.Scene.Kind != "selection_unavailable" {
		t.Fatalf("projection race: %+v", observation)
	}
	reads.remove, f.char.JobLevel = false, 6
	f.helperErr = errors.New("disallowed scene must not read helpers")
	before := reads.reads
	status, observation = navigationGET(t, router)
	if status != 200 || observation.Scene.Kind != "selection_unavailable" || reads.reads != before {
		t.Fatalf("qualification loss: %d %+v", status, observation)
	}
	for _, command := range []string{
		`{"action":"scene_select","params":{"target_kind":"item","target_id":"secret_item_herbal_root"}}`,
		`{"action":"scene_page","params":{"destination":"secretshop","offset":0}}`,
	} {
		status, raw = gatewayRequest(t, router, "hero", "session", "application/json", command, nil)
		if status != 403 || store.writes != 1 || reads.reads != before {
			t.Fatalf("qualification bypass: %d %s", status, raw)
		}
		assertGatewayError(t, raw, "SCENE_ACCESS_DENIED")
	}
	for range 2 {
		status, raw = gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"scene_back","params":{}}`, nil)
		if status != 200 || string(raw["success"]) != "true" {
			t.Fatalf("safe back: %d %s", status, raw)
		}
	}
	if store.selection.Destination != "town" || f.writes != 0 || f.executions != 0 {
		t.Fatal("observation changed gameplay state")
	}
}

func TestSecretShopSceneRefreshFailureAndGETRecovery(t *testing.T) {
	for _, rejection := range []bool{false, true} {
		t.Run(fmt.Sprint(rejection), func(t *testing.T) {
			f, store, reads, router := secretShopSceneRouter(t)
			store.selection.Destination = "secretshop"
			if rejection {
				f.char.Money = 749
			}
			before := f.char.Money
			f.afterExecute = func() { reads.err = errors.New("private SecretShop source failure") }
			status, raw := f.request(router, `{"item_id":"secret_item_herbal_root","quantity":1}`)
			wantStatus := 200
			if rejection {
				wantStatus = 400
				assertGatewayError(t, raw, "SECRETSHOP_INSUFFICIENT_FUNDS")
			} else if string(raw["success"]) != "true" || f.char.Money != before-750 {
				t.Fatalf("lost success: %s", raw)
			}
			if status != wantStatus || string(raw["context"]) != "null" || !strings.Contains(string(raw["context_error"]), "CONTEXT_REFRESH_FAILED") || f.executions != 1 || store.writes != 0 {
				t.Fatalf("refresh failure: %d %s", status, raw)
			}
			reads.err = nil
			if status, observed := navigationGET(t, router); status != 200 || observed.Scene.Support.Observation != "details" || observed.Character.Gold != f.char.Money || f.executions != 1 || store.writes != 0 {
				t.Fatalf("GET replayed: %d %+v", status, observed)
			}
		})
	}
	f, store, _, router := secretShopSceneRouter(t)
	store.selection.Destination, f.helperErr = "secretshop", errors.New("required helper failed")
	if status, _ := navigationGET(t, router); status != 500 || f.writes != 0 {
		t.Fatalf("partial required read: %d", status)
	}
	f.helperErr, store.selection.Destination = nil, "town"
	store.afterSave = func() { f.helperErr = errors.New("helper refresh failed") }
	status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"scene_enter","params":{"destination":"secretshop"}}`, nil)
	if status != 200 || string(raw["success"]) != "true" || string(raw["context"]) != "null" || store.writes != 1 {
		t.Fatalf("navigation outcome lost: %d %s", status, raw)
	}
	f.helperErr = nil
	if status, observed := navigationGET(t, router); status != 200 || observed.Scene.Support.Observation != "details" || store.writes != 1 || f.writes != 0 {
		t.Fatal("GET navigation recovery replayed")
	}
}

func TestSecretShopSceneOwnershipContextAndActivityPrecedence(t *testing.T) {
	f, store, reads, router := secretShopSceneRouter(t)
	store.selection.Destination = "secretshop"
	ctx := context.WithValue(context.Background(), struct{}{}, "SecretShop observation")
	f.expectedContext = ctx
	r := httptest.NewRequest(http.MethodGet, "/api/v1/characters/hero/context", nil).WithContext(ctx)
	r.Header.Set("Authorization", "Bearer session")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("request context: %d %s", w.Code, w.Body.String())
	}
	f.expectedContext = nil
	before := reads.reads
	for _, tc := range []struct {
		actor, token string
		status       int
	}{{"hero", "", 401}, {"other", "session", 403}, {"missing", "session", 404}} {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/characters/"+tc.actor+"/context", nil)
		r.Header.Set("Authorization", "Bearer "+tc.token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != tc.status || reads.reads != before || store.writes != 0 {
			t.Fatalf("unauthorized reads: %d %s", w.Code, w.Body.String())
		}
	}
	for _, kind := range []string{"sleep", "work", "party", "dungeon"} {
		t.Run(kind, func(t *testing.T) {
			f, store, reads, router := secretShopSceneRouter(t, playercontext.WithActivities(func(context.Context, string, string) ([]playercontext.Activity, error) {
				if kind == "party" || kind == "dungeon" {
					return []playercontext.Activity{{Kind: kind, ID: "active", Role: "member", Phase: "in_progress", Actions: []string{}}}, nil
				}
				return []playercontext.Activity{}, nil
			}))
			if kind == "sleep" {
				f.char.PendingWake = true
			}
			if kind == "work" {
				f.actions = []scheduling.ScheduledAction{{ID: "work", ActorID: "hero", State: scheduling.StateProcessing, ExecuteAt: time.Now().Add(time.Hour)}}
			}
			store.selection.Destination, f.helperErr = "secretshop", errors.New("must skip ordinary reads")
			if status, observed := navigationGET(t, router); status != 200 || observed.Scene.Kind != "activity" || reads.reads != 0 || store.writes != 0 || f.writes != 0 {
				t.Fatalf("activity overridden: %d %+v", status, observed)
			}
			if status, _ := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"scene_enter","params":{"destination":"secretshop"}}`, nil); status != 409 || store.writes != 0 {
				t.Fatalf("activity entry: %d", status)
			}
		})
	}
}
