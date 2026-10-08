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

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/playercontext"
	"github.com/witchcraze/party2re/internal/shop"
)

type shopSceneRepository struct {
	*gatewayFixture
	inv                      coreinventory.Inventory
	active                   []string
	helperErr, definitionErr error
	catalog                  *item.Catalog
	writes                   int
}

func (r *shopSceneRepository) FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error) {
	return r.FindByID(ctx, id)
}
func (r *shopSceneRepository) Update(_ context.Context, char corecharacter.Character) error {
	r.writes++
	r.char = char
	return nil
}
func (r *shopSceneRepository) FindByCharacterID(ctx context.Context, id string) (coreinventory.Inventory, error) {
	r.checkContext(ctx)
	if id != "hero" {
		r.t.Fatal("wrong inventory owner")
	}
	return r.inv, nil
}
func (r *shopSceneRepository) FindByCharacterIDForUpdate(ctx context.Context, id string) (coreinventory.Inventory, error) {
	return r.FindByCharacterID(ctx, id)
}
func (r *shopSceneRepository) Save(_ context.Context, inv coreinventory.Inventory) error {
	r.writes++
	r.inv = inv
	return nil
}
func (r *shopSceneRepository) GetActiveHelperItemIDs(ctx context.Context, _ time.Time) ([]string, error) {
	r.checkContext(ctx)
	return r.active, r.helperErr
}

type shopSceneDefinitions struct{ repo *shopSceneRepository }

func (d shopSceneDefinitions) FindByID(id string) (item.Definition, error) {
	if d.repo.definitionErr != nil {
		return item.Definition{}, d.repo.definitionErr
	}
	return d.repo.catalog.FindByID(id)
}

func shopSceneRouter(t *testing.T, options ...Option) (*shopSceneRepository, *gatewayNavigationStore, http.Handler) {
	t.Helper()
	catalog, err := item.DefaultCatalog()
	if err != nil {
		t.Fatal(err)
	}
	f := &shopSceneRepository{gatewayFixture: newGatewayFixture(t), catalog: catalog, inv: coreinventory.Inventory{CharacterID: "hero"}}
	service, err := shop.NewService(f, f, shopSceneDefinitions{f}, shop.WithHelperProvider(f))
	if err != nil {
		t.Fatal(err)
	}
	store := &gatewayNavigationStore{}
	scenes := []playercontext.SceneDefinition{{ID: "town", Pageable: true}}
	for _, kind := range []shop.ShopType{shop.ShopTypeWeapon, shop.ShopTypeArmor, shop.ShopTypeItem, shop.ShopTypeAccessory} {
		scenes = append(scenes, playercontext.SceneDefinition{ID: "shop_" + string(kind), Parent: "town", SubjectKind: "item", Pageable: true,
			SubjectAvailable: func(ctx context.Context, actor, target string) (bool, error) {
				catalog, err := service.GetCatalog(ctx, kind, actor)
				return slices.ContainsFunc(catalog.Items, func(p shop.CatalogItem) bool { return p.ID == target }), err
			}})
	}
	pc := playercontext.NewService(f.gatewayFixture, f.gatewayFixture, timer.NewService(nil), playercontext.WithNavigation(store, scenes...))
	opts := []Option{WithPlayerContext(pc), func(h *Handler) { h.homes = f.gatewayFixture }}
	opts = append(opts, options...)
	h, err := NewHandler(f.gatewayFixture, f.gatewayFixture, &struct{ AdventureService }{}, service, opts...)
	if err != nil {
		t.Fatal(err)
	}
	return f, store, h.Router()
}

type shopSceneReadService struct {
	ShopService
	inspectErr error
	remove     bool
}

func (s *shopSceneReadService) InspectNPC(ctx context.Context, kind shop.ShopType, actor string) (shop.NPCInspectResult, error) {
	if s.inspectErr != nil {
		return shop.NPCInspectResult{}, s.inspectErr
	}
	return s.ShopService.InspectNPC(ctx, kind, actor)
}

func (s *shopSceneReadService) GetCatalog(ctx context.Context, kind shop.ShopType, actor string) (shop.ShopCatalog, error) {
	catalog, err := s.ShopService.GetCatalog(ctx, kind, actor)
	if s.remove {
		catalog.Items = nil
	}
	return catalog, err
}

func TestShopSceneInspectFailureAndProductDisappearingDuringProjection(t *testing.T) {
	var reads *shopSceneReadService
	f, store, router := shopSceneRouter(t, func(h *Handler) {
		reads = &shopSceneReadService{ShopService: h.shops}
		h.shops = reads
	})
	store.selection = playercontext.Selection{Destination: "shop_weapon", Subject: playercontext.Subject{Kind: "item", ID: "weapon-01"}}
	reads.inspectErr = errors.New("required inspect failed")
	if status, _ := navigationGET(t, router); status != 500 {
		t.Fatalf("partial inspect observation: %d", status)
	}
	reads.inspectErr, reads.remove = nil, true
	status, response := navigationGET(t, router)
	if status != 200 || response.Scene.Kind != "selection_unavailable" || response.Scene.Subject == nil || response.Scene.Subject.ID != "weapon-01" || len(response.AvailableActions) != 2 || f.writes != 0 || store.writes != 0 {
		t.Fatalf("projection race: %d %+v", status, response)
	}
	reads.remove = false
	if status, response := navigationGET(t, router); status != 200 || response.Scene.Kind != "subject" {
		t.Fatalf("GET recovery: %d %+v", status, response)
	}
}

func decodeShopScene[T any](t *testing.T, response PlayerContextResponse) T {
	t.Helper()
	raw, err := json.Marshal(response.Scene.Data)
	if err != nil {
		t.Fatal(err)
	}
	var data T
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	return data
}

func TestShopScenesUseRealCatalogAndExplicitSelection(t *testing.T) {
	for _, kind := range []shop.ShopType{shop.ShopTypeWeapon, shop.ShopTypeArmor, shop.ShopTypeItem, shop.ShopTypeAccessory} {
		for _, level := range []int{0, 1, 3, 5, 7, 11, 12, 49, 50, 99, 100} {
			t.Run(fmt.Sprintf("%s/%d", kind, level), func(t *testing.T) {
				f, store, router := shopSceneRouter(t)
				f.char.JobLevel, f.char.Money = level, 0
				store.selection.Destination = "shop_" + string(kind)
				status, response := navigationGET(t, router)
				if status != 200 || response.Scene.Support.Observation != "details" {
					t.Fatalf("catalog: %d %+v", status, response)
				}
				data := decodeShopScene[ShopCatalogSceneData](t, response)
				ids, err := shop.GetSalesItemIDs(kind, level)
				if err != nil {
					t.Fatal(err)
				}
				slices.Sort(ids)
				if len(data.Items) != min(20, len(ids)) || data.Items == nil || data.ShopType != kind || data.Quantity.Minimum != 1 || data.Quantity.Maximum != shop.MaxTransactionQuantity {
					t.Fatalf("projection: %+v", data)
				}
				for i, p := range data.Items {
					def, err := f.catalog.FindByID(ids[i])
					if err != nil {
						t.Fatal(err)
					}
					multiplier := 2
					if kind == shop.ShopTypeAccessory {
						multiplier = 10
						if def.ID == "item-150" || def.ID == "item-151" {
							multiplier = 1000
						}
					}
					if p.ID != def.ID || p.BasePrice != def.Price || p.RetailPrice != def.Price*multiplier || p.Slot != def.Slot || p.PurchaseParams.ItemDefinitionID != p.ID || p.SelectParams != (playercontext.Subject{Kind: "item", ID: p.ID}) {
						t.Fatalf("product: %+v / %+v", p, def)
					}
				}
				if (data.NPC.SecretShopHint != "") != (kind == shop.ShopTypeItem) || data.NPC.Dialogue != shopInspectDialogue(kind) {
					t.Fatalf("NPC: %+v", data.NPC)
				}
				command := "shop_purchase"
				if kind == shop.ShopTypeAccessory {
					command = "shop_accessory_buy"
				}
				if !slices.ContainsFunc(response.Scene.Support.Actions, func(a SceneActionSupport) bool { return a.Action == command && !a.Connected }) || slices.ContainsFunc(response.AvailableActions, func(a ContextAction) bool { return a.Action == command }) {
					t.Fatalf("unconnected purchase: %+v", response.Scene.Support)
				}
				body, err := json.Marshal(map[string]any{"action": "scene_select", "params": data.Items[0].SelectParams})
				if err != nil {
					t.Fatal(err)
				}
				status, raw := gatewayRequest(t, router, "hero", "session", "application/json", string(body), nil)
				if status != 200 || string(raw["success"]) != "true" {
					t.Fatalf("select: %d %s", status, raw)
				}
				var refreshed PlayerContextResponse
				if err := json.Unmarshal(raw["context"], &refreshed); err != nil {
					t.Fatal(err)
				}
				status, observed := navigationGET(t, router)
				if status != 200 || !reflect.DeepEqual(refreshed.Scene, observed.Scene) {
					t.Fatalf("GET differs: %d %+v / %+v", status, refreshed.Scene, observed.Scene)
				}
				detail := decodeShopScene[ShopProductSceneData](t, observed)
				if observed.Scene.Kind != "subject" || detail.Product.ID != data.Items[0].ID || slices.ContainsFunc(observed.AvailableActions, func(a ContextAction) bool { return a.Action == "scene_page" || a.Action == "scene_select" }) || store.writes != 1 || f.writes != 0 {
					t.Fatalf("detail mutated or did not narrow: %+v", observed)
				}
			})
		}
	}
}

func TestShopScenePagesFiltersAndMissingProducts(t *testing.T) {
	f, store, router := shopSceneRouter(t)
	f.char.JobLevel = 100
	store.selection.Destination = "shop_accessory"
	status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"scene_page","params":{"destination":"shop_accessory","offset":0,"limit":1}}`, nil)
	if status != 200 {
		t.Fatalf("page: %d %s", status, raw)
	}
	status, response := navigationGET(t, router)
	data := decodeShopScene[ShopCatalogSceneData](t, response)
	if status != 200 || len(data.Items) != 1 || data.Items[0].ID != "item-143" || data.Page.Next == nil || data.Page.Next.Offset != 1 || data.Page.Next.Limit != 1 {
		t.Fatalf("page: %+v", data)
	}
	index := slices.IndexFunc(response.AvailableActions, func(a ContextAction) bool { return a.Action == "scene_page" })
	if index < 0 || response.AvailableActions[index].ParamsTemplate["offset"] != float64(1) || response.AvailableActions[index].ParamsTemplate["limit"] != float64(1) {
		t.Fatalf("next template: %+v", response)
	}
	body, err := json.Marshal(map[string]any{"action": "scene_page", "params": data.Page.Next})
	if err != nil {
		t.Fatal(err)
	}
	if status, raw := gatewayRequest(t, router, "hero", "session", "application/json", string(body), nil); status != 200 {
		t.Fatalf("next: %d %s", status, raw)
	}
	_, response = navigationGET(t, router)
	if data := decodeShopScene[ShopCatalogSceneData](t, response); data.Items[0].ID != "item-144" {
		t.Fatalf("next: %+v", data)
	}
	for _, p := range []string{`{"destination":"shop_accessory","cursor":""}`, `{"destination":"shop_weapon","offset":0}`} {
		if status, _ := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"scene_page","params":`+p+`}`, nil); status != 400 {
			t.Fatalf("invalid page: %d", status)
		}
	}
	ids, err := shop.GetSalesItemIDs(shop.ShopTypeAccessory, 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, offset := range []int{len(ids) - 1, 1000000} {
		store.selection.Offset, store.selection.Limit = offset, 100
		status, response := navigationGET(t, router)
		data := decodeShopScene[ShopCatalogSceneData](t, response)
		if status != 200 || data.Items == nil || len(data.Items) != max(0, len(ids)-offset) || data.Page.Next != nil {
			t.Fatalf("end page: %+v", data)
		}
	}
	store.selection = playercontext.Selection{Destination: "shop_accessory"}
	f.active, err = shop.GetSalesItemIDs(shop.ShopTypeAccessory, 100)
	if err != nil {
		t.Fatal(err)
	}
	status, response = navigationGET(t, router)
	if data := decodeShopScene[ShopCatalogSceneData](t, response); status != 200 || data.Items == nil || len(data.Items) != 0 || data.Page.Next != nil {
		t.Fatalf("empty filtered list: %+v", response)
	}
	if status, _ := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"scene_select","params":{"target_kind":"item","target_id":"item-143"}}`, nil); status != 404 {
		t.Fatalf("excluded selectable: %d", status)
	}
	store.selection.Subject = playercontext.Subject{Kind: "item", ID: "item-143"}
	status, response = navigationGET(t, router)
	if status != 200 || response.Scene.Kind != "selection_unavailable" || store.writes != 2 || f.writes != 0 {
		t.Fatalf("unavailable fallback mutated state: %d %+v", status, response)
	}
	store.selection = playercontext.Selection{Destination: "shop_weapon"}
	if status, _ := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"scene_select","params":{"target_kind":"item","target_id":"item-143"}}`, nil); status != 404 {
		t.Fatalf("cross-shop selectable: %d", status)
	}
}

func TestShopSceneRequiredFailuresAndGETOnlyRecovery(t *testing.T) {
	for _, fail := range []string{"helper", "definition", "character"} {
		t.Run(fail, func(t *testing.T) {
			f, store, router := shopSceneRouter(t)
			setError := func(err error) {
				switch fail {
				case "helper":
					f.helperErr = err
				case "definition":
					f.definitionErr = err
				case "character":
					f.characterErr = err
				}
			}
			store.selection.Destination = "shop_item"
			setError(errors.New("required source failed"))
			if status, _ := navigationGET(t, router); status != 500 {
				t.Fatalf("partial observation: %d", status)
			}
			setError(nil)
			store.selection.Destination = "town"
			store.afterSave = func() { setError(errors.New("refresh failed")) }
			status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"scene_enter","params":{"destination":"shop_item"}}`, nil)
			if status != 200 || string(raw["success"]) != "true" || string(raw["context"]) != "null" || store.writes != 1 || f.writes != 0 {
				t.Fatalf("lost known outcome: %d %s", status, raw)
			}
			setError(nil)
			if status, response := navigationGET(t, router); status != 200 || response.Scene.Support.Observation != "details" || store.writes != 1 || f.writes != 0 {
				t.Fatalf("GET recovery: %d %+v", status, response)
			}
		})
	}
}

func TestShopSceneActorOwnershipAndExplicitPurchase(t *testing.T) {
	f, store, router := shopSceneRouter(t)
	store.selection = playercontext.Selection{Destination: "shop_weapon", Subject: playercontext.Subject{Kind: "item", ID: "weapon-01"}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f.expectedContext = ctx
	r := httptest.NewRequest(http.MethodGet, "/api/v1/characters/hero/context", nil).WithContext(ctx)
	r.Header.Set("Authorization", "Bearer session")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("request context: %d %s", w.Code, w.Body.String())
	}
	f.expectedContext = nil
	for _, tc := range []struct {
		actor, token string
		status       int
	}{{"hero", "", 401}, {"other", "session", 403}, {"missing", "session", 404}} {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/characters/"+tc.actor+"/context", nil)
		r.Header.Set("Authorization", "Bearer "+tc.token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != tc.status || f.writes != 0 || store.writes != 0 {
			t.Fatalf("ownership: %d %s", w.Code, w.Body.String())
		}
	}
	_, response := navigationGET(t, router)
	product := decodeShopScene[ShopProductSceneData](t, response).Product
	store.selection = playercontext.Selection{Destination: "shop_item", Subject: playercontext.Subject{Kind: "item", ID: "item-001"}}
	body, err := json.Marshal(map[string]any{"character_id": "hero", "item_definition_id": product.PurchaseParams.ItemDefinitionID, "quantity": 1})
	if err != nil {
		t.Fatal(err)
	}
	r = httptest.NewRequest(http.MethodPost, "/shop/purchase", strings.NewReader(string(body)))
	r.Header.Set("Authorization", "Bearer session")
	r.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, r)
	if w.Code != 200 || len(f.inv.Items) != 1 || f.inv.Items[0].DefinitionID != "weapon-01" || f.char.Money != 100-product.RetailPrice || store.writes != 0 {
		t.Fatalf("purchase redirected: %d %s %+v", w.Code, w.Body.String(), f.inv)
	}
}
