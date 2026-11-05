package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/auction"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/playercontext"
)

type auctionGatewayFixture struct {
	*gatewayFixture
	AuctionService
	request auction.SendRequest
	result  auction.SendResult
}

func (f *auctionGatewayFixture) Send(ctx context.Context, req auction.SendRequest) (auction.SendResult, error) {
	f.checkContext(ctx)
	f.executions++
	f.request = req
	if f.afterExecute != nil {
		f.afterExecute()
	}
	return f.result, f.executionErr
}

func auctionGatewayRouter(t *testing.T) (*auctionGatewayFixture, timer.Service, http.Handler) {
	t.Helper()
	f := &auctionGatewayFixture{gatewayFixture: newGatewayFixture(t), result: auction.SendResult{
		SenderCharacterID: "hero", TargetCharacterID: "peer", TargetCharacterName: "相手", TransferredGold: 7, Message: "7 Gを 相手 に送りました",
	}}
	timers := timer.NewService(nil)
	h, err := NewHandler(f.gatewayFixture, f.gatewayFixture, &struct{ AdventureService }{}, &struct{ ShopService }{},
		WithAuction(f), WithPlayerContext(playercontext.NewService(f.gatewayFixture, f.gatewayFixture, timers)), func(h *Handler) { h.homes = f.gatewayFixture })
	if err != nil {
		t.Fatal(err)
	}
	return f, timers, h.Router()
}

func TestAuctionGatewaySendAndRecovery(t *testing.T) {
	for _, tc := range []struct {
		params string
		want   auction.SendRequest
	}{
		{`{"target_character_id":"peer","gold":7}`, auction.SendRequest{TargetCharacterID: "peer", Gold: 7}},
		{`{"target_character_name":"相手","slot":"weapon"}`, auction.SendRequest{TargetCharacterName: "相手", Slot: "weapon"}},
		{`{"target_character_id":"peer","instance_id":"held"}`, auction.SendRequest{TargetCharacterID: "peer", InstanceID: "held"}},
		{`{"target_character_id":"peer","target_character_name":"ignored","gold":7,"slot":"armor","instance_id":"held"}`, auction.SendRequest{TargetCharacterID: "peer", TargetCharacterName: "ignored", Gold: 7, Slot: "armor", InstanceID: "held"}},
	} {
		for _, failure := range []string{"", "query", "profile"} {
			t.Run(tc.params+failure, func(t *testing.T) {
				f, _, router := auctionGatewayRouter(t)
				f.expectedContext = context.WithValue(context.Background(), struct{}{}, "auction request")
				if tc.want.Gold == 0 {
					f.result.TransferredGold = 0
					f.result.TransferredItem = &auction.TransferredItemInfo{InstanceID: "delivered", DefinitionID: "weapon-01", ItemName: "武器", EnhancementLevel: 6}
					f.result.Message = "武器を相手に送りました"
				}
				f.afterExecute = func() {
					if failure == "query" {
						f.queryErr = errors.New("private query")
					}
					if failure == "profile" {
						f.profileErr = errors.New("private profile")
					}
				}
				status, got := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"auction_send","params":`+tc.params+`}`, f.expectedContext)
				want := tc.want
				want.SenderCharacterID = "hero"
				var result auction.SendResult
				if err := json.Unmarshal(got["result"], &result); err != nil {
					t.Fatal(err)
				}
				if status != 200 || f.executions != 1 || !reflect.DeepEqual(f.request, want) || !reflect.DeepEqual(result, f.result) {
					t.Fatalf("status=%d request=%+v result=%+v: %s", status, f.request, result, got)
				}
				if failure == "" {
					assertGatewayContext(t, got)
				} else {
					assertLoopRefreshFailure(t, got)
				}
				f.queryErr, f.profileErr, f.expectedContext = nil, nil, nil
				if status, _ := navigationGET(t, router); status != 200 || f.executions != 1 {
					t.Fatalf("GET recovery: %d calls=%d", status, f.executions)
				}
			})
		}
	}
}

func TestAuctionGatewayStrictParams(t *testing.T) {
	for _, params := range []string{`null`, `[]`, `1`, `{"sender_character_id":"other"}`, `{"character_id":"other"}`, `{"player_id":"other"}`, `{"extra":1}`, `{"gold":"7"}`, `{"gold":1.5}`, `{"gold":9223372036854775808}`, `{"gold":null}`, `{"slot":null}`, `{"instance_id":null}`, `{"target_character_id":null}`, `{"target_character_name":null}`, `{"target_character_id":7}`} {
		t.Run(params, func(t *testing.T) {
			f, _, router := auctionGatewayRouter(t)
			status, got := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"auction_send","params":`+params+`}`, nil)
			if status != 400 || f.executions != 0 || f.queryCalls != 0 {
				t.Fatalf("status=%d calls=%d: %s", status, f.executions, got)
			}
			assertGatewayError(t, got, "INVALID_ACTION_PARAMS")
		})
	}
}

func TestAuctionGatewayRejections(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{
		{auction.ErrCannotSendToSelf, 400, "AUCTION_SELF_TRANSFER"},
		{auction.ErrInvalidSendTarget, 400, "AUCTION_INVALID_TARGET"},
		{auction.ErrNothingToSend, 400, "AUCTION_NOTHING_TO_SEND"},
		{auction.ErrInvalidSendAmount, 400, "AUCTION_INVALID_AMOUNT"},
		{auction.ErrInsufficientMoney, 400, "AUCTION_INSUFFICIENT_FUNDS"},
		{auction.ErrDepotFull, 400, "AUCTION_DEPOT_FULL"},
		{auction.ErrTabooItem, 400, "AUCTION_TABOO_ITEM"},
		{auction.ErrItemNotEquipped, 400, "AUCTION_ITEM_NOT_EQUIPPED"},
		{auction.ErrInvalidSlot, 400, "AUCTION_INVALID_SLOT"},
		{auction.ErrTargetNotFound, 404, "AUCTION_TARGET_NOT_FOUND"},
		{auction.ErrItemNotFound, 404, "AUCTION_ITEM_NOT_FOUND"},
		{errors.New("private storage failure"), 500, "EXECUTION_FAILED"},
		{context.Canceled, 500, "EXECUTION_FAILED"},
	} {
		for _, failRefresh := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%v/%t", tc.code, tc.err, failRefresh), func(t *testing.T) {
				f, _, router := auctionGatewayRouter(t)
				f.executionErr = fmt.Errorf("wrapped: %w", tc.err)
				if failRefresh {
					f.afterExecute = func() { f.profileErr = errors.New("private profile") }
				}
				status, got := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"auction_send","params":{}}`, nil)
				if status != tc.status || f.executions != 1 {
					t.Fatalf("status=%d: %s", status, got)
				}
				assertGatewayError(t, got, tc.code)
				if strings.Contains(fmt.Sprint(got), "private") {
					t.Fatalf("private error disclosed: %s", got)
				}
				if tc.status == 500 {
					if _, ok := got["context"]; ok {
						t.Fatalf("unknown outcome refreshed: %s", got)
					}
				} else if failRefresh {
					assertLoopRefreshFailure(t, got)
				} else {
					assertGatewayContext(t, got)
				}
			})
		}
	}
}

func TestAuctionGatewayAdmission(t *testing.T) {
	for _, condition := range []string{"auth", "owner", "sleep", "wake", "shared sleep", "shared wake", "work", "overdue", "processing", "query error", "sleep error", "unconfigured"} {
		t.Run(condition, func(t *testing.T) {
			f, timers, router := auctionGatewayRouter(t)
			actor, token, want, code := "hero", "session", 409, "ACTION_UNAVAILABLE"
			switch condition {
			case "auth":
				token, want = "", 401
			case "owner":
				actor, want = "other", 403
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
			case "work", "overdue", "processing":
				a := scheduling.ScheduledAction{ID: "work", State: scheduling.StatePending, ExecuteAt: time.Now().Add(time.Hour)}
				if condition != "work" {
					a.ExecuteAt = time.Now().Add(-time.Hour)
				}
				if condition == "processing" {
					a.State = scheduling.StateProcessing
				}
				f.actions = []scheduling.ScheduledAction{a}
			case "query error":
				f.queryErr, want, code = errors.New("private query"), 500, "ACTION_PREFLIGHT_FAILED"
			case "sleep error":
				f.sleepErr, want, code = errors.New("private sleep"), 500, "ACTION_PREFLIGHT_FAILED"
			case "unconfigured":
				h, err := NewHandler(f.gatewayFixture, f.gatewayFixture, &struct{ AdventureService }{}, &struct{ ShopService }{}, WithAuction(nil))
				if err != nil {
					t.Fatal(err)
				}
				router, want, code = h.Router(), 501, "ACTION_NOT_IMPLEMENTED"
			}
			status, got := gatewayRequest(t, router, actor, token, "application/json", `{"action":"auction_send","params":{"target_character_id":"peer","gold":7}}`, nil)
			if status != want || f.executions != 0 {
				t.Fatalf("status=%d calls=%d: %s", status, f.executions, got)
			}
			if condition != "auth" && condition != "owner" {
				assertGatewayError(t, got, code)
			}
		})
	}
}

func TestAuctionSendRetiredRoutes(t *testing.T) {
	f, _, router := auctionGatewayRouter(t)
	for _, path := range []string{"/characters/hero/auction/send", "/auction/send"} {
		r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"sender_character_id":"hero","target_character_id":"peer","gold":7}`))
		r.Header.Set("Authorization", "Bearer session")
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != 404 || f.executions != 0 {
			t.Fatalf("%s: status=%d calls=%d", path, w.Code, f.executions)
		}
	}
}

type auctionRealTestTxProvider struct{}

func (auctionRealTestTxProvider) RunInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

type auctionRealTestCharRepo struct {
	chars              map[string]corecharacter.Character
	faultByIDForUpdate map[string]error
	faultByName        error
	updateCalls        int
}

func newAuctionRealTestCharRepo() *auctionRealTestCharRepo {
	return &auctionRealTestCharRepo{
		chars:              make(map[string]corecharacter.Character),
		faultByIDForUpdate: make(map[string]error),
	}
}

func (r *auctionRealTestCharRepo) FindByID(_ context.Context, id string) (corecharacter.Character, error) {
	if c, ok := r.chars[id]; ok {
		return c, nil
	}
	return corecharacter.Character{}, corecharacter.ErrNotFound
}

func (r *auctionRealTestCharRepo) FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error) {
	if err, ok := r.faultByIDForUpdate[id]; ok && err != nil {
		return corecharacter.Character{}, err
	}
	return r.FindByID(ctx, id)
}

func (r *auctionRealTestCharRepo) FindByName(_ context.Context, name string) (corecharacter.Character, error) {
	if r.faultByName != nil {
		return corecharacter.Character{}, r.faultByName
	}
	for _, c := range r.chars {
		if c.Name == name {
			return c, nil
		}
	}
	return corecharacter.Character{}, corecharacter.ErrNotFound
}

func (r *auctionRealTestCharRepo) FindByNameForUpdate(ctx context.Context, name string) (corecharacter.Character, error) {
	return r.FindByName(ctx, name)
}

func (r *auctionRealTestCharRepo) Update(_ context.Context, char corecharacter.Character) error {
	r.updateCalls++
	r.chars[char.ID] = char
	return nil
}

type auctionRealTestEquipRepo struct {
	saveCalls int
}

func (r *auctionRealTestEquipRepo) FindByCharacterID(_ context.Context, id string) (coreequipment.Equipment, error) {
	return coreequipment.New(id)
}

func (r *auctionRealTestEquipRepo) Save(_ context.Context, _ coreequipment.Equipment) error {
	r.saveCalls++
	return nil
}

type auctionRealTestInvRepo struct {
	saveCalls int
}

func (r *auctionRealTestInvRepo) FindByCharacterID(_ context.Context, id string) (coreinventory.Inventory, error) {
	return coreinventory.New(id)
}

func (r *auctionRealTestInvRepo) FindByCharacterIDForUpdate(ctx context.Context, id string) (coreinventory.Inventory, error) {
	return r.FindByCharacterID(ctx, id)
}

func (r *auctionRealTestInvRepo) Save(_ context.Context, _ coreinventory.Inventory) error {
	r.saveCalls++
	return nil
}

type auctionRealTestDepotRepo struct {
	saveCalls int
}

func (r *auctionRealTestDepotRepo) FindByCharacterID(_ context.Context, id string) (depot.Depot, error) {
	return depot.Depot{CharacterID: id, Capacity: 50}, nil
}

func (r *auctionRealTestDepotRepo) FindByCharacterIDForUpdate(ctx context.Context, id string) (depot.Depot, error) {
	return r.FindByCharacterID(ctx, id)
}

func (r *auctionRealTestDepotRepo) Save(_ context.Context, _ depot.Depot) error {
	r.saveCalls++
	return nil
}

func TestAuctionGatewayRealServiceRegressions(t *testing.T) {
	catalog, err := coreitem.DefaultCatalog()
	if err != nil {
		t.Fatal(err)
	}

	setupRouter := func(t *testing.T) (*gatewayFixture, *auctionRealTestCharRepo, *auctionRealTestEquipRepo, *auctionRealTestInvRepo, *auctionRealTestDepotRepo, http.Handler) {
		t.Helper()
		gf := newGatewayFixture(t)
		charRepo := newAuctionRealTestCharRepo()
		charRepo.chars["hero"] = corecharacter.Character{ID: "hero", Name: "Hero", PlayerID: "owner", Money: 100}
		charRepo.chars["peer"] = corecharacter.Character{ID: "peer", Name: "Peer", PlayerID: "other", Money: 100}

		equipRepo := &auctionRealTestEquipRepo{}
		invRepo := &auctionRealTestInvRepo{}
		depotRepo := &auctionRealTestDepotRepo{}

		realService, err := auction.NewService(auctionRealTestTxProvider{}, charRepo, equipRepo, invRepo, depotRepo, catalog)
		if err != nil {
			t.Fatal(err)
		}

		timers := timer.NewService(nil)
		h, err := NewHandler(
			gf,
			gf,
			&struct{ AdventureService }{},
			&struct{ ShopService }{},
			WithAuction(realService),
			WithPlayerContext(playercontext.NewService(gf, gf, timers)),
			func(h *Handler) { h.homes = gf },
		)
		if err != nil {
			t.Fatal(err)
		}
		return gf, charRepo, equipRepo, invRepo, depotRepo, h.Router()
	}

	t.Run("first ordered lock read storage failure", func(t *testing.T) {
		_, charRepo, equipRepo, invRepo, depotRepo, router := setupRouter(t)
		charRepo.faultByIDForUpdate["hero"] = errors.New("private storage failure hero")

		status, got := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"auction_send","params":{"target_character_id":"peer","gold":7}}`, nil)
		if status != 500 {
			t.Fatalf("status=%d, want 500: %s", status, got)
		}
		assertGatewayError(t, got, "EXECUTION_FAILED")
		if _, ok := got["context"]; ok {
			t.Fatalf("unexpected context refresh on 500: %s", got)
		}
		if strings.Contains(fmt.Sprint(got), "private") {
			t.Fatalf("private storage failure leaked: %s", got)
		}
		if charRepo.updateCalls != 0 || equipRepo.saveCalls != 0 || invRepo.saveCalls != 0 || depotRepo.saveCalls != 0 {
			t.Fatalf("unexpected writes on prerequisite failure: updates=%d", charRepo.updateCalls)
		}
		if charRepo.chars["hero"].Money != 100 || charRepo.chars["peer"].Money != 100 {
			t.Fatalf("wallets mutated on failure: hero=%d peer=%d", charRepo.chars["hero"].Money, charRepo.chars["peer"].Money)
		}
	})

	t.Run("second ordered lock read storage failure", func(t *testing.T) {
		_, charRepo, equipRepo, invRepo, depotRepo, router := setupRouter(t)
		charRepo.faultByIDForUpdate["peer"] = errors.New("private storage failure peer")

		status, got := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"auction_send","params":{"target_character_id":"peer","gold":7}}`, nil)
		if status != 500 {
			t.Fatalf("status=%d, want 500: %s", status, got)
		}
		assertGatewayError(t, got, "EXECUTION_FAILED")
		if _, ok := got["context"]; ok {
			t.Fatalf("unexpected context refresh on 500: %s", got)
		}
		if strings.Contains(fmt.Sprint(got), "private") {
			t.Fatalf("private storage failure leaked: %s", got)
		}
		if charRepo.updateCalls != 0 || equipRepo.saveCalls != 0 || invRepo.saveCalls != 0 || depotRepo.saveCalls != 0 {
			t.Fatalf("unexpected writes on prerequisite failure: updates=%d", charRepo.updateCalls)
		}
		if charRepo.chars["hero"].Money != 100 || charRepo.chars["peer"].Money != 100 {
			t.Fatalf("wallets mutated on failure: hero=%d peer=%d", charRepo.chars["hero"].Money, charRepo.chars["peer"].Money)
		}
	})

	t.Run("target name lookup storage failure", func(t *testing.T) {
		_, charRepo, equipRepo, invRepo, depotRepo, router := setupRouter(t)
		charRepo.faultByName = errors.New("private storage failure name")

		status, got := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"auction_send","params":{"target_character_name":"Peer","gold":7}}`, nil)
		if status != 500 {
			t.Fatalf("status=%d, want 500: %s", status, got)
		}
		assertGatewayError(t, got, "EXECUTION_FAILED")
		if _, ok := got["context"]; ok {
			t.Fatalf("unexpected context refresh on 500: %s", got)
		}
		if strings.Contains(fmt.Sprint(got), "private") {
			t.Fatalf("private storage failure leaked: %s", got)
		}
		if charRepo.updateCalls != 0 || equipRepo.saveCalls != 0 || invRepo.saveCalls != 0 || depotRepo.saveCalls != 0 {
			t.Fatalf("unexpected writes on prerequisite failure: updates=%d", charRepo.updateCalls)
		}
		if charRepo.chars["hero"].Money != 100 || charRepo.chars["peer"].Money != 100 {
			t.Fatalf("wallets mutated on failure: hero=%d peer=%d", charRepo.chars["hero"].Money, charRepo.chars["peer"].Money)
		}
	})

	t.Run("genuine missing recipient by ID", func(t *testing.T) {
		_, charRepo, equipRepo, invRepo, depotRepo, router := setupRouter(t)

		status, got := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"auction_send","params":{"target_character_id":"missing","gold":7}}`, nil)
		if status != 404 {
			t.Fatalf("status=%d, want 404: %s", status, got)
		}
		assertGatewayError(t, got, "AUCTION_TARGET_NOT_FOUND")
		assertGatewayContext(t, got)
		if charRepo.updateCalls != 0 || equipRepo.saveCalls != 0 || invRepo.saveCalls != 0 || depotRepo.saveCalls != 0 {
			t.Fatalf("unexpected writes on missing target: updates=%d", charRepo.updateCalls)
		}
		if charRepo.chars["hero"].Money != 100 || charRepo.chars["peer"].Money != 100 {
			t.Fatalf("wallets mutated on missing target: hero=%d peer=%d", charRepo.chars["hero"].Money, charRepo.chars["peer"].Money)
		}
	})

	t.Run("genuine missing recipient by name", func(t *testing.T) {
		_, charRepo, equipRepo, invRepo, depotRepo, router := setupRouter(t)

		status, got := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"auction_send","params":{"target_character_name":"Missing","gold":7}}`, nil)
		if status != 404 {
			t.Fatalf("status=%d, want 404: %s", status, got)
		}
		assertGatewayError(t, got, "AUCTION_TARGET_NOT_FOUND")
		assertGatewayContext(t, got)
		if charRepo.updateCalls != 0 || equipRepo.saveCalls != 0 || invRepo.saveCalls != 0 || depotRepo.saveCalls != 0 {
			t.Fatalf("unexpected writes on missing target: updates=%d", charRepo.updateCalls)
		}
		if charRepo.chars["hero"].Money != 100 || charRepo.chars["peer"].Money != 100 {
			t.Fatalf("wallets mutated on missing target: hero=%d peer=%d", charRepo.chars["hero"].Money, charRepo.chars["peer"].Money)
		}
	})

	t.Run("healthy contrast", func(t *testing.T) {
		_, charRepo, _, _, _, router := setupRouter(t)

		status, got := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"auction_send","params":{"target_character_id":"peer","gold":7}}`, nil)
		if status != 200 {
			t.Fatalf("status=%d, want 200: %s", status, got)
		}
		assertGatewayContext(t, got)
		var result auction.SendResult
		if err := json.Unmarshal(got["result"], &result); err != nil {
			t.Fatal(err)
		}
		if result.TransferredGold != 7 {
			t.Fatalf("transferred gold=%d, want 7", result.TransferredGold)
		}
		if charRepo.updateCalls != 2 {
			t.Fatalf("expected 2 updates, got %d", charRepo.updateCalls)
		}
		if charRepo.chars["hero"].Money != 93 || charRepo.chars["peer"].Money != 107 {
			t.Fatalf("wallets not updated correctly: hero=%d peer=%d", charRepo.chars["hero"].Money, charRepo.chars["peer"].Money)
		}
	})

	t.Run("context cancellation during lock read", func(t *testing.T) {
		_, charRepo, equipRepo, invRepo, depotRepo, router := setupRouter(t)
		charRepo.faultByIDForUpdate["hero"] = context.Canceled

		status, got := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"auction_send","params":{"target_character_id":"peer","gold":7}}`, nil)
		if status != 500 {
			t.Fatalf("status=%d, want 500: %s", status, got)
		}
		assertGatewayError(t, got, "EXECUTION_FAILED")
		if _, ok := got["context"]; ok {
			t.Fatalf("unexpected context refresh on cancellation 500: %s", got)
		}
		if charRepo.updateCalls != 0 || equipRepo.saveCalls != 0 || invRepo.saveCalls != 0 || depotRepo.saveCalls != 0 {
			t.Fatalf("unexpected writes on cancellation: updates=%d", charRepo.updateCalls)
		}
	})
}
