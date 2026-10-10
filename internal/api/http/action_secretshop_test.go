package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/playercontext"
	"github.com/witchcraze/party2re/internal/secretshop"
)

type secretShopGatewayFixture struct {
	*shopSceneRepository
	SecretShopService
	service                     *secretshop.Service
	dep                         depot.Depot
	getCalls, requestedQuantity int
	requestedItem               string
	nameErr                     error
}

type secretShopGatewayDepot struct{ f *secretShopGatewayFixture }

func (r secretShopGatewayDepot) FindByCharacterIDForUpdate(ctx context.Context, id string) (depot.Depot, error) {
	r.f.checkContext(ctx)
	if id != "hero" {
		r.f.t.Fatal("wrong depot owner")
	}
	return r.f.dep, nil
}

func (r secretShopGatewayDepot) Save(ctx context.Context, dep depot.Depot) error {
	r.f.checkContext(ctx)
	r.f.writes++
	r.f.dep = dep
	return nil
}

type secretShopGatewayHelper struct{ *shopSceneRepository }

func (r secretShopGatewayHelper) GetActiveHelperItemIDs(ctx context.Context) ([]string, error) {
	return r.shopSceneRepository.GetActiveHelperItemIDs(ctx, time.Now())
}

func newSecretShopGatewayFixture(t *testing.T) *secretShopGatewayFixture {
	t.Helper()
	definitions, err := item.DefaultCatalog()
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := secretshop.LoadDefaultCatalog()
	if err != nil {
		t.Fatal(err)
	}
	r := &shopSceneRepository{gatewayFixture: newGatewayFixture(t), catalog: definitions, inv: coreinventory.Inventory{CharacterID: "hero"}}
	r.char.Name, r.char.JobLevel, r.char.Money = "テスト勇者", 7, 100000
	f := &secretShopGatewayFixture{shopSceneRepository: r, dep: depot.Depot{CharacterID: "hero", Capacity: 40}}
	f.service, err = secretshop.NewService(r, r, catalog, secretshop.WithHelperFilter(secretShopGatewayHelper{r}), secretshop.WithDepotRepository(secretShopGatewayDepot{f}), secretshop.WithItemDefinitionProvider(shopSceneDefinitions{r}))
	if err != nil {
		t.Fatal(err)
	}
	f.SecretShopService = f.service
	return f
}

func (f *secretShopGatewayFixture) Get(ctx context.Context, id string) (corecharacter.Character, error) {
	f.getCalls++
	if f.getCalls > 1 && f.nameErr != nil {
		return corecharacter.Character{}, f.nameErr
	}
	return f.gatewayFixture.Get(ctx, id)
}

func (f *secretShopGatewayFixture) PurchaseItem(ctx context.Context, id, itemID string, quantity int) (*secretshop.PurchaseResult, error) {
	f.checkContext(ctx)
	if id != "hero" {
		f.t.Fatal("wrong purchase actor")
	}
	f.executions++
	f.requestedItem, f.requestedQuantity = itemID, quantity
	var result *secretshop.PurchaseResult
	err := f.executionErr
	if err == nil {
		result, err = f.service.PurchaseItem(ctx, id, itemID, quantity)
	}
	if f.afterExecute != nil {
		f.afterExecute()
	}
	return result, err
}

func (f *secretShopGatewayFixture) router(timers playercontext.TimerReader) http.Handler {
	h, err := NewHandler(f.gatewayFixture, f, &struct{ AdventureService }{}, &struct{ ShopService }{},
		WithPlayerContext(playercontext.NewService(f.gatewayFixture, f.gatewayFixture, timers)), WithSecretShop(f), func(h *Handler) { h.homes = f.gatewayFixture })
	if err != nil {
		f.t.Fatal(err)
	}
	return h.Router()
}

func (f *secretShopGatewayFixture) request(router http.Handler, params string) (int, map[string]json.RawMessage) {
	body := `{"action":"secretshop_purchase"`
	if params != "" {
		body += `,"params":` + params
	}
	return gatewayRequest(f.t, router, "hero", "session", "application/json", body+`}`, f.expectedContext)
}

func TestSecretShopGatewayPurchaseAndGETRecovery(t *testing.T) {
	for _, quantity := range []int{1, 2, 99} {
		for _, failure := range []string{"", "query", "profile"} {
			t.Run(fmt.Sprintf("%d/%s", quantity, failure), func(t *testing.T) {
				f := newSecretShopGatewayFixture(t)
				f.expectedContext = context.WithValue(context.Background(), struct{}{}, "secret shop request")
				f.afterExecute = func() {
					if failure == "query" {
						f.queryErr = errors.New("private query failure")
					}
					if failure == "profile" {
						f.profileErr = errors.New("private profile failure")
					}
				}
				router := f.router(timer.NewService(nil))
				status, got := f.request(router, fmt.Sprintf(`{"item_id":"secret_item_herbal_root","quantity":%d}`, quantity))
				if status != 200 || string(got["success"]) != "true" || f.executions != 1 || f.requestedItem != "secret_item_herbal_root" || f.requestedQuantity != quantity {
					t.Fatalf("status=%d calls=%d: %s", status, f.executions, got)
				}
				var result secretShopPurchaseResponse
				if err := json.Unmarshal(got["result"], &result); err != nil {
					t.Fatal(err)
				}
				wantMessage := "薬草の根っこメェ〜。持ってけメェ〜"
				if quantity > 1 {
					wantMessage = "薬草の根っこはテスト勇者メェ〜の預かり所の方に投げましたメェ〜"
				}
				if result.CharacterID != "hero" || result.Quantity != quantity || result.TotalPrice != 750*quantity || result.RemainingGold != 100000-750*quantity || f.char.Money != result.RemainingGold || result.TransferredToDepot != (quantity > 1) || result.NPCMessage != wantMessage || result.InventoryInstanceID == "" {
					t.Fatalf("lost purchase facts/dialogue: %+v", result)
				}
				invCount := 0
				for _, inst := range f.inv.Items {
					if inst.DefinitionID == "item-010" {
						invCount += inst.Quantity
					}
				}
				if invCount+f.dep.Quantity("item-010") != quantity || (quantity == 1 && invCount != 1) || (quantity > 1 && invCount != 0) {
					t.Fatalf("wrong delivery: inventory=%+v depot=%+v", f.inv, f.dep)
				}
				if failure == "" {
					assertGatewayContext(t, got)
				} else if string(got["context"]) != "null" || !strings.Contains(string(got["context_error"]), "CONTEXT_REFRESH_FAILED") {
					t.Fatalf("lost refresh failure: %s", got)
				}
				f.queryErr, f.profileErr, f.expectedContext = nil, nil, nil
				if status, observation := navigationGET(t, router); status != 200 || observation.Character.Gold != result.RemainingGold || f.executions != 1 {
					t.Fatalf("GET recovery replayed or lost purchase: %d %+v", status, observation)
				}
			})
		}
	}
	t.Run("occupied inventory", func(t *testing.T) {
		f := newSecretShopGatewayFixture(t)
		f.inv.Items = []item.Instance{{ID: "held", DefinitionID: "item-015", Quantity: 1}}
		status, got := f.request(f.router(timer.NewService(nil)), `{"item_id":"secret_item_herbal_root","quantity":1}`)
		if status != 200 || f.dep.Quantity("item-010") != 1 || len(f.inv.Items) != 1 || !strings.Contains(string(got["result"]), `"transferred_to_depot":true`) || !strings.Contains(string(got["result"]), "預かり所") {
			t.Fatalf("occupied delivery: %d %s", status, got)
		}
	})
}

func TestSecretShopGatewayValidationAndRejectionRecovery(t *testing.T) {
	for _, tc := range []struct {
		name, params, code string
		status             int
		setup              func(*secretShopGatewayFixture)
	}{
		{"zero", `{"item_id":"secret_item_herbal_root","quantity":0}`, "SECRETSHOP_INVALID_QUANTITY", 400, nil},
		{"negative", `{"item_id":"secret_item_herbal_root","quantity":-1}`, "SECRETSHOP_INVALID_QUANTITY", 400, nil},
		{"too many", `{"item_id":"secret_item_herbal_root","quantity":100}`, "SECRETSHOP_INVALID_QUANTITY", 400, nil},
		{"missing item", `{"item_id":"missing","quantity":1}`, "SECRETSHOP_ITEM_NOT_FOUND", 404, nil},
		{"access", `{"item_id":"secret_item_herbal_root","quantity":1}`, "SECRETSHOP_ACCESS_DENIED", 403, func(f *secretShopGatewayFixture) { f.char.JobLevel = 6 }},
		{"funds", `{"item_id":"secret_item_herbal_root","quantity":1}`, "SECRETSHOP_INSUFFICIENT_FUNDS", 400, func(f *secretShopGatewayFixture) { f.char.Money = 749 }},
		{"helper", `{"item_id":"secret_item_herbal_root","quantity":1}`, "SECRETSHOP_ITEM_UNAVAILABLE", 409, func(f *secretShopGatewayFixture) { f.active = []string{"item-010"} }},
		{"full depot", `{"item_id":"secret_item_herbal_root","quantity":2}`, "SECRETSHOP_DEPOT_FULL", 400, func(f *secretShopGatewayFixture) {
			for i := 0; i < 40; i++ {
				f.dep.Items = append(f.dep.Items, item.Instance{ID: fmt.Sprint(i), DefinitionID: "weapon-01", Quantity: 1})
			}
		}},
	} {
		for _, failRefresh := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%t", tc.name, failRefresh), func(t *testing.T) {
				f := newSecretShopGatewayFixture(t)
				if tc.setup != nil {
					tc.setup(f)
				}
				before := f.char.Money
				if failRefresh {
					f.afterExecute = func() { f.profileErr = errors.New("private profile failure") }
				}
				router := f.router(timer.NewService(nil))
				status, got := f.request(router, tc.params)
				if status != tc.status || f.executions != 1 || f.writes != 0 || f.char.Money != before {
					t.Fatalf("status=%d writes=%d: %s", status, f.writes, got)
				}
				assertGatewayError(t, got, tc.code)
				if failRefresh {
					if string(got["context"]) != "null" || !strings.Contains(string(got["context_error"]), "CONTEXT_REFRESH_FAILED") {
						t.Fatalf("lost rejection: %s", got)
					}
				} else {
					assertGatewayContext(t, got)
				}
				f.profileErr = nil
				if status, _ := navigationGET(t, router); status != 200 || f.executions != 1 || f.writes != 0 {
					t.Fatal("GET recovery executed purchase")
				}
			})
		}
	}
}

func TestSecretShopGatewayInvalidParamsAndOwnership(t *testing.T) {
	for _, params := range []string{"", `null`, `{}`, `[]`, `{"item_id":"secret_item_herbal_root"}`, `{"quantity":1}`, `{"item_id":null,"quantity":1}`, `{"item_id":1,"quantity":1}`, `{"item_id":"x","quantity":null}`, `{"item_id":"x","quantity":"1"}`, `{"item_id":"x","quantity":1.5}`, `{"item_id":"x","quantity":9223372036854775808}`, `{"item_id":"x","quantity":1,"character_id":"other"}`, `{"item_id":"x","quantity":1,"player_id":"other"}`, `{"item_id":"x","quantity":1,"unknown":1}`} {
		t.Run(params, func(t *testing.T) {
			f := newSecretShopGatewayFixture(t)
			status, got := f.request(f.router(timer.NewService(nil)), params)
			if status != 400 || f.executions != 0 || f.queryCalls != 0 || f.writes != 0 {
				t.Fatalf("status=%d: %s", status, got)
			}
			assertGatewayError(t, got, "INVALID_ACTION_PARAMS")
		})
	}
	for _, tc := range []struct {
		path, token string
		status      int
	}{{"hero", "", 401}, {"other", "session", 403}, {"missing", "session", 404}} {
		f := newSecretShopGatewayFixture(t)
		status, got := gatewayRequest(t, f.router(timer.NewService(nil)), tc.path, tc.token, "application/json", `{"action":"secretshop_purchase","params":{"item_id":"secret_item_herbal_root","quantity":1}}`, nil)
		if status != tc.status || f.executions != 0 || f.queryCalls != 0 {
			t.Fatalf("status=%d: %s", status, got)
		}
	}
}

func TestSecretShopGatewayGuards(t *testing.T) {
	for _, condition := range []string{"sleep", "wake", "shared sleep", "shared wake", "work", "overdue work", "processing", "query error", "sleep read error", "wake read error", "shared read error", "name read error"} {
		t.Run(condition, func(t *testing.T) {
			f := newSecretShopGatewayFixture(t)
			timers := timer.NewService(nil)
			reader := gatewayTimers{TimerReader: timers}
			want, code := 409, "ACTION_UNAVAILABLE"
			switch condition {
			case "sleep", "wake":
				category := timer.CategorySleep
				if condition == "wake" {
					category = timer.CategoryAsleep
				}
				if err := timers.SetLock(context.Background(), category, "hero", time.Hour); err != nil {
					t.Fatal(err)
				}
			case "shared sleep":
				f.sleep.Sleeping = true
			case "shared wake":
				f.sleep.CanWake = true
			case "work", "overdue work", "processing":
				a := scheduling.ScheduledAction{ID: "work", State: scheduling.StatePending, ExecuteAt: time.Now().Add(time.Hour)}
				if condition != "work" {
					a.ExecuteAt = time.Now().Add(-time.Hour)
				}
				if condition == "processing" {
					a.State = scheduling.StateProcessing
				}
				f.actions = []scheduling.ScheduledAction{a}
			case "query error":
				f.queryErr = errors.New("private query failure")
			case "sleep read error":
				reader.durationErr = errors.New("private timer failure")
			case "wake read error":
				reader.asleepErr = errors.New("private timer failure")
			case "shared read error":
				f.sleepErr = errors.New("private sleep failure")
			case "name read error":
				f.nameErr = errors.New("private name failure")
			}
			if strings.Contains(condition, "error") {
				want, code = 500, "ACTION_PREFLIGHT_FAILED"
			}
			if condition == "name read error" {
				code = "EXECUTION_FAILED"
			}
			status, got := f.request(f.router(reader), `{"item_id":"secret_item_herbal_root","quantity":1}`)
			if status != want || f.executions != 0 || f.writes != 0 || f.char.Money != 100000 {
				t.Fatalf("status=%d executions=%d: %s", status, f.executions, got)
			}
			assertGatewayError(t, got, code)
		})
	}
}

func TestSecretShopGatewayDependencyErrors(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{
		{secretshop.ErrCharacterNotFound, 404, "SECRETSHOP_CHARACTER_NOT_FOUND"},
		{secretshop.ErrPriceOverflow, 400, "SECRETSHOP_PRICE_OVERFLOW"},
		{secretshop.ErrDepotNotConfigured, 500, "EXECUTION_FAILED"},
		{errors.New("private unknown dependency"), 500, "EXECUTION_FAILED"},
	} {
		t.Run(tc.code+tc.err.Error(), func(t *testing.T) {
			f := newSecretShopGatewayFixture(t)
			f.executionErr = fmt.Errorf("wrapped: %w", tc.err)
			status, got := f.request(f.router(timer.NewService(nil)), `{"item_id":"secret_item_herbal_root","quantity":1}`)
			if status != tc.status || f.writes != 0 || f.executions != 1 {
				t.Fatalf("status=%d: %s", status, got)
			}
			assertGatewayError(t, got, tc.code)
			if tc.status == 500 {
				if _, exists := got["context"]; exists || strings.Contains(string(got["error"]), "private") {
					t.Fatalf("unknown outcome leaked: %s", got)
				}
			}
		})
	}
	t.Run("required helper read", func(t *testing.T) {
		f := newSecretShopGatewayFixture(t)
		f.helperErr = errors.New("private helper database")
		status, got := f.request(f.router(timer.NewService(nil)), `{"item_id":"secret_item_herbal_root","quantity":1}`)
		if status != 500 || f.writes != 0 || f.char.Money != 100000 {
			t.Fatalf("helper failed open: %d %s", status, got)
		}
		assertGatewayError(t, got, "EXECUTION_FAILED")
	})
}

func TestSecretShopGatewayUnconfigured(t *testing.T) {
	f := newGatewayFixture(t)
	h, err := NewHandler(f, f, &struct{ AdventureService }{}, &struct{ ShopService }{}, WithPlayerContext(playercontext.NewService(f, f, timer.NewService(nil))), WithSecretShop(nil))
	if err != nil {
		t.Fatal(err)
	}
	status, got := gatewayRequest(t, h.Router(), "hero", "session", "application/json", `{"action":"secretshop_purchase","params":{"item_id":"x","quantity":1}}`, nil)
	if status != 501 || f.queryCalls != 0 {
		t.Fatalf("status=%d: %s", status, got)
	}
	assertGatewayError(t, got, "ACTION_NOT_IMPLEMENTED")
}
