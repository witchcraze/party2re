package http

import (
	"context"
	"encoding/json"
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

func depotOrderDBRouter(t *testing.T) (http.Handler, corecharacter.Character, *database.CharacterRepository, *database.DepotRepository, *gatewayNavigationStore, []item.Instance) {
	t.Helper()
	catalog, err := item.DefaultCatalog()
	if err != nil {
		t.Fatal(err)
	}
	router, actor, chars, depots, store, _ := depotSaleDBRouter(t, "", 0, depot.WithItemDefinitionProvider(catalog))
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
	router, actor, _, depots, store, items := depotOrderDBRouter(t)
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
	w := depotOrderRequest(router, "/characters/"+actor.ID+"/depot/sort", `{}`)
	var result depotResponse
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
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
	router, actor, chars, depots, store, items := depotOrderDBRouter(t)
	var sales atomic.Int64
	result := testutil.RunConcurrentStressTest(t, testutil.GetStressConfig(), func(worker, op int) error {
		path, body := "/characters/"+actor.ID+"/depot/sort", `{}`
		if (worker+op)%2 != 0 {
			path, body = "/api/v1/characters/"+actor.ID+"/actions", saleGatewayBody("depot_sell", `{"item_id":"`+items[4].ID+`"}`)
		}
		w := depotOrderRequest(router, path, body)
		if strings.HasSuffix(path, "/sort") {
			if w.Code != 200 {
				return fmt.Errorf("sort (including lock failure): %d %s", w.Code, w.Body.String())
			}
			return nil
		}
		var response struct {
			Success bool
			Error   ErrorDetail
			Result  sellDepotResponse
		}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			return err
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
