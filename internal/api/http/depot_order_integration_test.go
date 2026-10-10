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
	"sync/atomic"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/testutil"
)

type sortGatewayFailingSave struct{ *database.DepotRepository }

func (r sortGatewayFailingSave) Save(ctx context.Context, value depot.Depot) error {
	if err := r.DepotRepository.Save(ctx, value); err != nil {
		return err
	}
	return errors.New("private post-save failure")
}

func depotOrderDBRouter(t *testing.T, failure string) (http.Handler, corecharacter.Character, *database.CharacterRepository, *database.DepotRepository, *gatewayNavigationStore, []item.Instance) {
	t.Helper()
	catalog, err := item.DefaultCatalog()
	if err != nil {
		t.Fatal(err)
	}
	router, actor, chars, depots, store, _ := depotSaleDBRouter(t, failure, 0, depot.WithItemDefinitionProvider(catalog))
	items := []item.Instance{
		{ID: actor.ID[:30] + "ff", DefinitionID: "weapon-01", Quantity: 1, EnhancementLevel: 6},
		{ID: actor.ID[:30] + "ee", DefinitionID: "weapon-02", Quantity: 1},
		{ID: actor.ID[:30] + "dd", DefinitionID: "armor-01", Quantity: 1, EnhancementLevel: 2},
		{ID: actor.ID[:30] + "cc", DefinitionID: "armor-02", Quantity: 1},
		{ID: actor.ID[:30] + "bb", DefinitionID: "item-001", Quantity: 8},
	}
	unsorted := slices.Clone(items)
	slices.Reverse(unsorted)
	if err := depots.Save(context.Background(), depot.Depot{CharacterID: actor.ID, Capacity: 5, Items: unsorted}); err != nil {
		t.Fatal(err)
	}
	return router, actor, chars, depots, store, items
}

func depotOrderRequest(router http.Handler, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer session")
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	return w
}

func TestDepotSortReloadAndContextPages(t *testing.T) {
	for _, transport := range []string{"REST", "Gateway"} {
		t.Run(transport, func(t *testing.T) { testDepotSortReloadAndContextPages(t, transport == "Gateway") })
	}
}

func testDepotSortReloadAndContextPages(t *testing.T, gateway bool) {
	t.Helper()
	router, actor, _, depots, store, items := depotOrderDBRouter(t, "")
	ctx := context.Background()
	before, err := depots.FindByCharacterID(ctx, actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	unsorted := slices.Clone(items)
	slices.Reverse(unsorted)
	if !reflect.DeepEqual(before.Items, unsorted) {
		t.Fatal("Save must not implicitly sort")
	}
	path, body := "/characters/"+actor.ID+"/depot/sort", `{}`
	if gateway {
		path, body = "/api/v1/characters/"+actor.ID+"/actions", `{"action":"depot_sort","params":{}}`
	}
	w := depotOrderRequest(router, path, body)
	raw := w.Body.Bytes()
	if gateway {
		var outcome struct {
			Success bool
			Result  json.RawMessage
			Context PlayerContextResponse
		}
		if err := json.Unmarshal(raw, &outcome); err != nil {
			t.Fatal(err)
		}
		if !outcome.Success || outcome.Context.Character.ID != actor.ID || outcome.Context.Scene.LocationID != "depot" {
			t.Fatalf("Gateway sort outcome: %d %s", w.Code, raw)
		}
		data := decodeShopScene[DepotSceneData](t, outcome.Context)
		if len(data.Items) != len(items) || data.Items[0].ID != items[0].ID || data.Items[0].EnhancementLevel != items[0].EnhancementLevel {
			t.Fatalf("Gateway sort lost context facts: %+v", data)
		}
		raw = outcome.Result
	}
	var result depotResponse
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || !reflect.DeepEqual(result, toDepotResponse(depot.Depot{CharacterID: actor.ID, Capacity: 5, Items: items})) {
		t.Fatalf("sort result: %d %s", w.Code, w.Body.String())
	}
	for _, offset := range []int{0, 2, 4} {
		store.selection.Offset, store.selection.Limit = offset, 2
		r := httptest.NewRequest(http.MethodGet, "/api/v1/characters/"+actor.ID+"/context", nil)
		r.Header.Set("Authorization", "Bearer session")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		var observation PlayerContextResponse
		if err := json.Unmarshal(w.Body.Bytes(), &observation); err != nil {
			t.Fatal(err)
		}
		data := decodeShopScene[DepotSceneData](t, observation)
		want := items[offset:min(offset+2, len(items))]
		if w.Code != 200 || data.ItemCount != len(items) || len(data.Items) != len(want) || data.Page.Offset != offset {
			t.Fatalf("page: %d %+v", w.Code, data)
		}
		for i, row := range data.Items {
			if row.ID != want[i].ID || row.DefinitionID != want[i].DefinitionID || row.Quantity != want[i].Quantity || row.EnhancementLevel != want[i].EnhancementLevel {
				t.Fatalf("page lost order/facts: %+v want %+v", row, want[i])
			}
		}
	}
	after, err := depots.FindByCharacterID(ctx, actor.ID)
	if err != nil || !reflect.DeepEqual(after.Items, items) || store.writes != 0 {
		t.Fatalf("sort/reload/GET: %+v err=%v writes=%d", after.Items, err, store.writes)
	}
}

func TestDepotSortAndSaleConcurrency(t *testing.T) {
	router, actor, chars, depots, store, items := depotOrderDBRouter(t, "")
	var sales atomic.Int64
	result := testutil.RunConcurrentStressTest(t, testutil.GetStressConfig(), func(worker, op int) error {
		path, body := "/api/v1/characters/"+actor.ID+"/actions", `{"action":"depot_sort"}`
		sale := (worker+op)%2 != 0
		if sale {
			body = saleGatewayBody("depot_sell", `{"item_id":"`+items[4].ID+`"}`)
		}
		w := depotOrderRequest(router, path, body)
		var response struct {
			Success bool
			Error   ErrorDetail
			Result  sellDepotResponse
		}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			return err
		}
		if !sale {
			if w.Code != 200 || !response.Success {
				return fmt.Errorf("sort (including lock failure): %d %s", w.Code, w.Body.String())
			}
			return nil
		}
		if w.Code == 200 && response.Success && response.Result.GoldEarned == 120 {
			sales.Add(1)
			return nil
		}
		if w.Code == 404 && response.Error.Code == "DEPOT_ITEM_NOT_FOUND" {
			return nil
		}
		return fmt.Errorf("sale (including lock failure): %d %s", w.Code, w.Body.String())
	})
	if result.Failures != 0 || sales.Load() != 1 {
		t.Fatalf("results=%+v sales=%d", result, sales.Load())
	}
	storage, err := depots.FindByCharacterID(context.Background(), actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	wallet, err := chars.FindByID(context.Background(), actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(storage.Items, items[:4]) || wallet.Money != actor.Money+120 || store.writes != 0 {
		t.Fatalf("assets/order changed: %+v money=%d writes=%d", storage.Items, wallet.Money, store.writes)
	}
}

func TestDepotSortGatewayRollbackAndEmptyStorage(t *testing.T) {
	for _, failure := range []string{"depot-save", ""} {
		t.Run(failure, func(t *testing.T) {
			router, actor, chars, depots, store, _ := depotOrderDBRouter(t, failure)
			ctx := context.Background()
			before, err := depots.FindByCharacterID(ctx, actor.ID)
			if err != nil {
				t.Fatal(err)
			}
			if failure == "" {
				before.Items = []item.Instance{}
				if err := depots.Save(ctx, before); err != nil {
					t.Fatal(err)
				}
			}
			status, got := gatewayRequest(t, router, actor.ID, "session", "application/json", `{"action":"depot_sort"}`, nil)
			if failure != "" {
				if status != 500 || strings.Contains(fmt.Sprint(got), "private") {
					t.Fatalf("save failure: %d %s", status, got)
				}
				assertGatewayError(t, got, "EXECUTION_FAILED")
				if _, exists := got["context"]; exists {
					t.Fatal("unknown failure refreshed")
				}
			} else {
				var result depotResponse
				if err := json.Unmarshal(got["result"], &result); err != nil {
					t.Fatal(err)
				}
				if status != 200 || result.Items == nil || result.ItemCount != 0 {
					t.Fatalf("empty sort: %d %s", status, got)
				}
			}
			after, err := depots.FindByCharacterID(ctx, actor.ID)
			if err != nil {
				t.Fatal(err)
			}
			wallet, err := chars.FindByID(ctx, actor.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(after, before) || wallet.Money != actor.Money || store.writes != 0 {
				t.Fatalf("sort rollback/empty changed assets: %+v -> %+v money=%d writes=%d", before, after, wallet.Money, store.writes)
			}
		})
	}
}
