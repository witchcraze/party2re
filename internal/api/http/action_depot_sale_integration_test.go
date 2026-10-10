package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/witchcraze/party2re/internal/character"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/playercontext"
	"github.com/witchcraze/party2re/internal/testutil"
)

type saleGatewayCatalog struct {
	price int
	fail  bool
}

func (c saleGatewayCatalog) FindByID(id string) (item.Definition, error) {
	if c.fail && id == "second" {
		return item.Definition{}, errors.New("private catalog failure")
	}
	return item.Definition{ID: id, Price: c.price}, nil
}

type saleGatewayFailingCharacterSave struct{ *database.CharacterRepository }

func (saleGatewayFailingCharacterSave) Update(context.Context, corecharacter.Character) error {
	return errors.New("private character save failure")
}

func depotSaleDBRouter(t *testing.T, failure string, price int) (http.Handler, corecharacter.Character, *database.CharacterRepository, *database.DepotRepository, *gatewayNavigationStore, []item.Instance) {
	t.Helper()
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	chars, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	inventories, err := database.NewInventoryRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	depots, err := database.NewDepotRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	actor, err := database.CreateTestCharacterWithFunds(context.Background(), db, "Depot Gateway sale", 1000)
	if err != nil {
		t.Fatal(err)
	}
	items := make([]item.Instance, 3)
	for i, def := range []string{"first", "second", "survivor"} {
		items[i], err = item.NewInstance(def, []int{2, 1, 4}[i])
		if err != nil {
			t.Fatal(err)
		}
	}
	items[0].EnhancementLevel = 6
	if _, err := database.CreateTestDepot(context.Background(), db, actor.ID, 0, items); err != nil {
		t.Fatal(err)
	}
	var characters depot.CharacterRepository = chars
	if failure == "save" {
		characters = saleGatewayFailingCharacterSave{chars}
	}
	var options []depot.Option
	if failure != "provider" {
		options = append(options, depot.WithItemDefinitionProvider(saleGatewayCatalog{price, failure == "catalog"}))
	}
	svc, err := depot.NewServiceWithTransaction(depots, characters, inventories, depots, options...)
	if err != nil {
		t.Fatal(err)
	}
	characterService, err := character.NewService(chars)
	if err != nil {
		t.Fatal(err)
	}
	store := &gatewayNavigationStore{selection: playercontext.Selection{Destination: "depot"}}
	pc := playercontext.NewService(chars, emptyActionsReader{}, timer.NewService(nil), playercontext.WithNavigation(store,
		playercontext.SceneDefinition{ID: "town"}, playercontext.SceneDefinition{ID: "depot", Parent: "town", Pageable: true}))
	h, err := NewHandler(expansionDBPlayer{owner: actor.PlayerID}, characterService, &struct{ AdventureService }{}, &struct{ ShopService }{}, WithDepot(svc), WithPlayerContext(pc))
	if err != nil {
		t.Fatal(err)
	}
	return h.Router(), actor, chars, depots, store, items
}

func TestDepotSaleGatewayPersistence(t *testing.T) {
	for _, tc := range []struct {
		name, action, failure, code string
		price, status               int
	}{
		{"single", "depot_sell", "", "", 101, 200}, {"batch", "depot_sell_batch", "", "", 101, 200},
		{"zero price", "depot_sell", "", "", 0, 200}, {"batch zero price", "depot_sell_batch", "", "", 0, 200},
		{"single rollback", "depot_sell", "save", "EXECUTION_FAILED", 101, 500}, {"batch rollback", "depot_sell_batch", "save", "EXECUTION_FAILED", 101, 500},
		{"late catalog failure", "depot_sell_batch", "catalog", "EXECUTION_FAILED", 101, 500}, {"missing provider", "depot_sell", "provider", "EXECUTION_FAILED", 101, 500},
		{"missing single", "depot_sell", "missing", "DEPOT_ITEM_NOT_FOUND", 101, 404}, {"missing batch", "depot_sell_batch", "missing", "DEPOT_ITEM_NOT_FOUND", 101, 404},
		{"duplicate batch", "depot_sell_batch", "duplicate", "DEPOT_ITEM_NOT_FOUND", 101, 404},
		{"blank single", "depot_sell", "empty", "DEPOT_INVALID_ITEM_INSTANCE_ID", 101, 400}, {"empty batch", "depot_sell_batch", "empty", "DEPOT_EMPTY_ITEM_LIST", 101, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router, actor, chars, depots, store, items := depotSaleDBRouter(t, tc.failure, tc.price)
			ids := []string{items[0].ID}
			if tc.action == "depot_sell_batch" {
				ids = append(ids, items[1].ID)
			}
			switch tc.failure {
			case "missing":
				ids[len(ids)-1] = "missing"
			case "duplicate":
				ids[1] = ids[0]
			case "empty":
				if tc.action == "depot_sell" {
					ids[0] = ""
				} else {
					ids = []string{}
				}
			}
			var params any
			if tc.action == "depot_sell" {
				params = sellDepotItemRequest{ItemID: ids[0]}
			} else {
				params = sellDepotBatchRequest{ItemIDs: ids}
			}
			encoded, err := json.Marshal(params)
			if err != nil {
				t.Fatal(err)
			}
			before, err := depots.FindByCharacterID(context.Background(), actor.ID)
			if err != nil {
				t.Fatal(err)
			}
			status, got := gatewayRequest(t, router, actor.ID, "session", "application/json", saleGatewayBody(tc.action, string(encoded)), nil)
			if status != tc.status {
				t.Fatalf("status=%d: %s", status, got)
			}
			wantMoney := actor.Money
			if status == 200 {
				var result sellDepotResponse
				if err := json.Unmarshal(got["result"], &result); err != nil {
					t.Fatal(err)
				}
				earned := 100
				if tc.action == "depot_sell_batch" {
					earned = 150
				}
				if tc.price == 0 {
					earned = 0
				}
				wantMoney += earned
				if result.GoldEarned != earned || result.Depot.CharacterID != actor.ID || result.Depot.ItemCount != 3-len(ids) {
					t.Fatalf("lost sale result: %+v", result)
				}
			} else {
				assertGatewayError(t, got, tc.code)
				if status == 500 {
					if _, ok := got["context"]; ok || strings.Contains(fmt.Sprint(got), "private") {
						t.Fatalf("unknown failure exposed: %s", got)
					}
				}
			}
			if status != 500 {
				var observation PlayerContextResponse
				if err := json.Unmarshal(got["context"], &observation); err != nil {
					t.Fatal(err)
				}
				if observation.Character.ID != actor.ID || observation.Character.Gold != wantMoney || observation.Scene.LocationID != "depot" || observation.OngoingActions == nil || observation.AvailableActions == nil {
					t.Fatalf("wrong owned context: %+v", observation)
				}
			}
			wallet, err := chars.FindByID(context.Background(), actor.ID)
			if err != nil {
				t.Fatal(err)
			}
			after, err := depots.FindByCharacterID(context.Background(), actor.ID)
			if err != nil {
				t.Fatal(err)
			}
			if wallet.Money != wantMoney || store.writes != 0 {
				t.Fatalf("wrong wallet/selection: %d writes=%d", wallet.Money, store.writes)
			}
			if status != 200 {
				if !reflect.DeepEqual(before, after) {
					t.Fatalf("failed sale changed Depot: %+v -> %+v", before, after)
				}
			} else {
				remaining := map[string]item.Instance{}
				for _, stored := range after.Items {
					remaining[stored.ID] = stored
				}
				if len(remaining) != 3-len(ids) || !reflect.DeepEqual(remaining[items[2].ID], items[2]) {
					t.Fatal("unselected items changed")
				}
				for _, id := range ids {
					if _, exists := remaining[id]; exists {
						t.Fatal("sold item remains")
					}
				}
				if tc.action == "depot_sell" && !reflect.DeepEqual(remaining[items[1].ID], items[1]) {
					t.Fatal("single sale removed second item")
				}
			}
		})
	}
}

func TestDepotSaleGatewayConcurrentSales(t *testing.T) {
	router, actor, chars, depots, store, items := depotSaleDBRouter(t, "", 101)
	var sales, paid atomic.Int64
	result := testutil.RunConcurrentStressTest(t, testutil.GetStressConfig(), func(worker, op int) error {
		action := "depot_sell"
		var params any = sellDepotItemRequest{ItemID: items[0].ID}
		if (worker+op)%2 == 1 {
			action, params = "depot_sell_batch", sellDepotBatchRequest{ItemIDs: []string{items[0].ID, items[1].ID}}
		}
		encoded, err := json.Marshal(params)
		if err != nil {
			return err
		}
		r := httptest.NewRequest(http.MethodPost, "/api/v1/characters/"+actor.ID+"/actions", strings.NewReader(saleGatewayBody(action, string(encoded))))
		r.Header.Set("Authorization", "Bearer session")
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		var response struct {
			Success bool
			Result  sellDepotResponse
			Error   ErrorDetail
		}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			return err
		}
		if w.Code == 200 && response.Success {
			sales.Add(1)
			paid.Add(int64(response.Result.GoldEarned))
			return nil
		}
		if w.Code == 404 && !response.Success && response.Error.Code == "DEPOT_ITEM_NOT_FOUND" {
			return nil
		}
		return fmt.Errorf("unexpected sale (including transaction/lock failure): %d %s", w.Code, w.Body.String())
	})
	if result.Failures != 0 || sales.Load() != 1 || store.writes != 0 {
		t.Fatalf("results=%+v sales=%d writes=%d", result, sales.Load(), store.writes)
	}
	wallet, err := chars.FindByID(context.Background(), actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	storage, err := depots.FindByCharacterID(context.Background(), actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	remaining := map[string]item.Instance{}
	for _, stored := range storage.Items {
		remaining[stored.ID] = stored
	}
	if _, exists := remaining[items[0].ID]; exists {
		t.Fatal("sold item remains")
	}
	if !reflect.DeepEqual(remaining[items[2].ID], items[2]) {
		t.Fatal("unselected item changed")
	}
	if paid.Load() == 100 {
		if len(remaining) != 2 || !reflect.DeepEqual(remaining[items[1].ID], items[1]) {
			t.Fatal("single sale changed second item")
		}
	} else if paid.Load() == 150 {
		if len(remaining) != 1 {
			t.Fatal("batch sale left selected item")
		}
	} else {
		t.Fatalf("wrong sale payment=%d", paid.Load())
	}
	if wallet.Money != actor.Money+int(paid.Load()) {
		t.Fatalf("lost or duplicated gold: money=%d paid=%d", wallet.Money, paid.Load())
	}
}
