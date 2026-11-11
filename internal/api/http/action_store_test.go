package http

import (
	"context"
	"errors"
	"fmt"
	nethttp "net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/playercontext"
	"github.com/witchcraze/party2re/internal/store"
)

// storeInputs lists each command with valid params and the service arguments they must reach.
var storeInputs = []struct{ action, params, args string }{
	{"store_build", `{"town_id":"town2","house_style":"005"}`, "town2 005"},
	{"store_list_gold", `{"item_id":"inst-1","price":300}`, "inst-1 300"},
	{"store_list_barter", `{"item_id":"inst-1","wish_item_name":"やくそう"}`, "inst-1 やくそう"},
	{"store_withdraw", `{"sale_id":"sale-1"}`, "sale-1"},
	{"store_buy", `{"sale_id":"sale-1"}`, "sale-1"},
	{"store_trade", `{"sale_id":"sale-1","item_id":"inst-2"}`, "sale-1 inst-2"},
	{"store_rename", `{"store_name":"新看板"}`, "新看板"},
	{"store_wallpaper", `{"wallpaper":"farm"}`, "farm"},
	{"store_interior_add", `{"furniture_id":"001"}`, "001"},
	{"store_interior_rename", `{"interior_id":"int-1","name":"机"}`, "int-1 机"},
	{"store_interior_clean", ``, ""},
}

type storeGatewayFixture struct {
	*gatewayFixture
	StoreService
	args string
}

func (f *storeGatewayFixture) run(ctx context.Context, actor string, args ...any) error {
	f.args = strings.TrimSpace(fmt.Sprintln(args...))
	_, err := f.execute(ctx, actor, 0)
	return err
}

func (f *storeGatewayFixture) BuildStore(ctx context.Context, actor, townID, style string) (*store.StoreCheckResult, error) {
	if err := f.run(ctx, actor, townID, style); err != nil {
		return nil, err
	}
	return &store.StoreCheckResult{CharacterID: actor, TownID: townID, StoreName: "@Heroの店@"}, nil
}

func (f *storeGatewayFixture) ListGoldItem(ctx context.Context, actor, item string, price int) (*store.Sale, error) {
	if err := f.run(ctx, actor, item, price); err != nil {
		return nil, err
	}
	return &store.Sale{ID: "sale-new", Price: price}, nil
}

func (f *storeGatewayFixture) ListBarterItem(ctx context.Context, actor, item, wish string) (*store.Sale, error) {
	if err := f.run(ctx, actor, item, wish); err != nil {
		return nil, err
	}
	return &store.Sale{ID: "sale-new", WishItemName: wish}, nil
}

func (f *storeGatewayFixture) WithdrawListing(ctx context.Context, actor, sale string) error {
	return f.run(ctx, actor, sale)
}

func (f *storeGatewayFixture) BuyItem(ctx context.Context, actor, sale string) error {
	return f.run(ctx, actor, sale)
}

func (f *storeGatewayFixture) TradeItem(ctx context.Context, actor, sale, item string) error {
	return f.run(ctx, actor, sale, item)
}

func (f *storeGatewayFixture) ChangeStoreName(ctx context.Context, actor, name string) error {
	return f.run(ctx, actor, name)
}

func (f *storeGatewayFixture) ChangeWallpaper(ctx context.Context, actor, wallpaper string) error {
	return f.run(ctx, actor, wallpaper)
}

func (f *storeGatewayFixture) AddInterior(ctx context.Context, actor, furniture string) (*store.Interior, error) {
	if err := f.run(ctx, actor, furniture); err != nil {
		return nil, err
	}
	return &store.Interior{ID: "int-new", FurnitureID: furniture}, nil
}

func (f *storeGatewayFixture) RenameInterior(ctx context.Context, actor, interior, name string) error {
	return f.run(ctx, actor, interior, name)
}

func (f *storeGatewayFixture) CleanInteriors(ctx context.Context, actor string) error {
	return f.run(ctx, actor)
}

func (f *storeGatewayFixture) router(t *testing.T, timers timer.Service, options ...Option) nethttp.Handler {
	t.Helper()
	pc := playercontext.NewService(f.gatewayFixture, f.gatewayFixture, timers)
	opts := []Option{WithStore(f), WithPlayerContext(pc), func(h *Handler) { h.homes = f.gatewayFixture }}
	h, err := NewHandler(f.gatewayFixture, f.gatewayFixture, &struct{ AdventureService }{}, &struct{ ShopService }{}, append(opts, options...)...)
	if err != nil {
		t.Fatal(err)
	}
	return h.Router()
}

func TestStoreGatewayInputs(t *testing.T) {
	for _, input := range storeInputs {
		cases := []string{`[]`, `1`, `{"character_id":"other"}`, `{"extra":null}`}
		if input.params != "" {
			cases = append(cases, ``, `null`, `{}`)
			first := strings.SplitN(strings.TrimPrefix(input.params, `{"`), `"`, 2)[0]
			cases = append(cases, fmt.Sprintf(`{%q:null}`, first), fmt.Sprintf(`{%q:1}`, first))
		}
		for _, params := range cases {
			t.Run(input.action+params, func(t *testing.T) {
				f := &storeGatewayFixture{gatewayFixture: newGatewayFixture(t)}
				status, got := gatewayRequest(t, f.router(t, timer.NewService(nil)), "hero", "session", "application/json", saleGatewayBody(input.action, params), nil)
				if status != 400 || f.executions != 0 || f.queryCalls != 0 {
					t.Fatalf("invalid input executed: %d %s", status, got)
				}
				assertGatewayError(t, got, "INVALID_ACTION_PARAMS")
			})
		}
	}
}

func TestStoreGatewayOutcomes(t *testing.T) {
	for _, input := range storeInputs {
		for _, refreshFailure := range []bool{false, true} {
			for _, rejected := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/refresh=%t/reject=%t", input.action, refreshFailure, rejected), func(t *testing.T) {
					f := &storeGatewayFixture{gatewayFixture: newGatewayFixture(t)}
					f.expectedContext = context.WithValue(context.Background(), struct{}{}, "store request")
					want := 200
					if rejected {
						f.executionErr, want = fmt.Errorf("wrapped: %w", store.ErrStoreExpired), 409
					}
					if refreshFailure {
						f.afterExecute = func() { f.profileErr = errors.New("private profile failure") }
					}
					router := f.router(t, timer.NewService(nil))
					status, got := gatewayRequest(t, router, "hero", "session", "application/json", saleGatewayBody(input.action, input.params), f.expectedContext)
					if status != want || f.executions != 1 || f.args != input.args {
						t.Fatalf("lost outcome or explicit target: %d %q %s", status, f.args, got)
					}
					if rejected {
						assertGatewayError(t, got, "STORE_EXPIRED")
					} else if string(got["success"]) != "true" || string(got["result"]) == "null" {
						t.Fatalf("missing result: %s", got)
					}
					if refreshFailure {
						assertLoopRefreshFailure(t, got)
					} else {
						assertGatewayContext(t, got)
					}
					f.profileErr, f.expectedContext = nil, nil
					if status, _ := navigationGET(t, router); status != 200 || f.executions != 1 {
						t.Fatal("GET recovery failed or replayed command")
					}
				})
			}
		}
	}
}

func TestStoreGatewayErrors(t *testing.T) {
	cases := []struct {
		err    error
		code   string
		status int
	}{
		{corecharacter.ErrNotFound, "CHARACTER_NOT_FOUND", 404},
		{errors.New("private storage failure"), "EXECUTION_FAILED", 500},
		{context.Canceled, "EXECUTION_FAILED", 500},
	}
	for _, r := range storeRejections {
		cases = append(cases, struct {
			err    error
			code   string
			status int
		}{r.err, r.code, r.status})
	}
	for _, tc := range cases {
		t.Run(tc.err.Error(), func(t *testing.T) {
			f := &storeGatewayFixture{gatewayFixture: newGatewayFixture(t)}
			f.executionErr = fmt.Errorf("private wrapped: %w", tc.err)
			status, got := gatewayRequest(t, f.router(t, timer.NewService(nil)), "hero", "session", "application/json", saleGatewayBody("store_buy", `{"sale_id":"s"}`), nil)
			assertGatewayError(t, got, tc.code)
			if status != tc.status || f.executions != 1 || strings.Contains(fmt.Sprint(got), "private") {
				t.Fatalf("error mapping: %d %s", status, got)
			}
			if tc.status == 500 {
				if _, exists := got["context"]; exists || f.queryCalls != 1 {
					t.Fatalf("unknown outcome refreshed: %s", got)
				}
			} else {
				assertGatewayContext(t, got)
			}
		})
	}
}

func TestStoreGatewayGuardsAndDiscovery(t *testing.T) {
	for _, input := range storeInputs {
		for _, condition := range []string{"anonymous", "foreign", "missing", "owner", "query", "sleep", "wake", "unfinished", "sleep read", "location"} {
			t.Run(input.action+condition, func(t *testing.T) {
				f := &storeGatewayFixture{gatewayFixture: newGatewayFixture(t)}
				timers := timer.NewService(nil)
				path, token, want := "hero", "session", 409
				var options []Option
				switch condition {
				case "anonymous":
					token, want = "", 401
				case "foreign":
					path, want = "other", 403
				case "missing":
					path, want = "missing", 404
				case "owner":
					f.queryOwner, want = "other", 403
				case "query":
					f.queryErr, want = errors.New("read"), 500
				case "sleep":
					if err := timers.SetLock(context.Background(), timer.CategorySleep, "hero", time.Hour); err != nil {
						t.Fatal(err)
					}
				case "wake":
					f.char.PendingWake = true
				case "unfinished":
					f.actions = []scheduling.ScheduledAction{{ID: "work", ActorID: "hero", State: scheduling.StateProcessing, ActionType: "training"}}
				case "sleep read":
					f.sleepErr, want = errors.New("read"), 500
				case "location":
					options = append(options, WithPlayerContext(playercontext.NewService(f.gatewayFixture, f.gatewayFixture, timers, playercontext.WithNavigation(&gatewayNavigationStore{selection: playercontext.Selection{Destination: "bank"}}, playercontext.SceneDefinition{ID: "town"}, playercontext.SceneDefinition{ID: "bank", Parent: "town"}))))
				}
				status, got := gatewayRequest(t, f.router(t, timers, options...), path, token, "application/json", saleGatewayBody(input.action, input.params), nil)
				if status != want || f.executions != 0 {
					t.Fatalf("guard bypass: %d %s", status, got)
				}
			})
		}
		f := &storeGatewayFixture{gatewayFixture: newGatewayFixture(t)}
		status, observation := navigationGET(t, f.router(t, timer.NewService(nil)))
		if status != 200 || !slices.ContainsFunc(observation.AvailableActions, func(a ContextAction) bool {
			return a.Action == input.action && a.RequiredParams != nil && len(a.ParamsSchema) > 0
		}) || f.executions != 0 {
			t.Fatalf("missing discovery: %d %+v", status, observation)
		}
		resolved := NewActionURLResolver().Resolve("hero", input.action, "", "")
		if resolved.Method != "POST" || resolved.URL != "/api/v1/characters/hero/actions" {
			t.Fatalf("obsolete resolver: %+v", resolved)
		}
		for _, option := range []Option{WithStore(nil), func(*Handler) {}} {
			g := newGatewayFixture(t)
			h, err := NewHandler(g, g, &struct{ AdventureService }{}, &struct{ ShopService }{}, option, WithPlayerContext(playercontext.NewService(g, g, timer.NewService(nil))))
			if err != nil {
				t.Fatal(err)
			}
			status, got := gatewayRequest(t, h.Router(), "hero", "session", "application/json", saleGatewayBody(input.action, input.params), nil)
			if status != 501 || g.queryCalls != 0 {
				t.Fatalf("unconfigured: %d %s", status, got)
			}
		}
	}
}

func TestRetiredStoreRoutesDoNotReadOrExecute(t *testing.T) {
	f := &storeGatewayFixture{gatewayFixture: newGatewayFixture(t)}
	router := f.router(t, timer.NewService(nil))
	for _, route := range []struct{ method, path string }{
		{nethttp.MethodPost, "/towns/town1/stores"},
		{nethttp.MethodGet, "/towns/town1/stores"},
		{nethttp.MethodGet, "/stores/store-1"},
		{nethttp.MethodGet, "/characters/hero/store"},
		{nethttp.MethodPost, "/characters/hero/store/listings/gold"},
		{nethttp.MethodPost, "/characters/hero/store/listings/barter"},
		{nethttp.MethodDelete, "/characters/hero/store/listings/sale-1"},
		{nethttp.MethodPost, "/characters/hero/store/sales/sale-1/buy"},
		{nethttp.MethodPost, "/characters/hero/store/sales/sale-1/trade"},
		{nethttp.MethodPost, "/characters/hero/store/name"},
		{nethttp.MethodPost, "/characters/hero/store/wallpaper"},
		{nethttp.MethodPost, "/characters/hero/store/interiors"},
		{nethttp.MethodPut, "/characters/hero/store/interiors/int-1/name"},
		{nethttp.MethodDelete, "/characters/hero/store/interiors"},
	} {
		for _, token := range []string{"", "session"} {
			r := httptest.NewRequest(route.method, route.path, strings.NewReader(`{}`))
			r.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, r)
			if w.Code != nethttp.StatusNotFound || f.executions != 0 || f.queryCalls != 0 || f.characterCalls != 0 {
				t.Fatalf("retired %s %s: %d %s", route.method, route.path, w.Code, w.Body.String())
			}
		}
	}
}
