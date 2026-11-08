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
	"github.com/witchcraze/party2re/internal/fleamarket"
	"github.com/witchcraze/party2re/internal/playercontext"
)

var fleaMarketInputs = []struct{ action, field, params string }{
	{"fleamarket_list", "item_id", `{"item_id":"owned-item","price":300}`},
	{"fleamarket_purchase", "listing_id", `{"listing_id":"listing-1"}`},
	{"fleamarket_cancel", "listing_id", `{"listing_id":"listing-1"}`},
}

type fleaMarketGatewayFixture struct {
	*gatewayFixture
	FleaMarketService
	target string
	price  int
}

func (f *fleaMarketGatewayFixture) CreateListing(ctx context.Context, actor, itemID string, price int, _ time.Time) (fleamarket.Listing, error) {
	f.target, f.price = itemID, price
	if _, err := f.execute(ctx, actor, 0); err != nil {
		return fleamarket.Listing{}, err
	}
	return fleamarket.Listing{ID: "created", SellerCharacterID: actor, ItemID: itemID, Price: price, Status: fleamarket.StatusActive}, nil
}

func (f *fleaMarketGatewayFixture) PurchaseListing(ctx context.Context, actor, listingID string, _ time.Time) (fleamarket.PurchaseResult, error) {
	f.target = listingID
	if _, err := f.execute(ctx, actor, 0); err != nil {
		return fleamarket.PurchaseResult{}, err
	}
	return fleamarket.PurchaseResult{Listing: fleamarket.Listing{ID: listingID, Status: fleamarket.StatusSold}, BuyerGold: 700}, nil
}

func (f *fleaMarketGatewayFixture) CancelListing(ctx context.Context, actor, listingID string) (fleamarket.Listing, error) {
	f.target = listingID
	if _, err := f.execute(ctx, actor, 0); err != nil {
		return fleamarket.Listing{}, err
	}
	return fleamarket.Listing{ID: listingID, Status: fleamarket.StatusCancelled}, nil
}

func (f *fleaMarketGatewayFixture) router(t *testing.T, timers timer.Service, options ...Option) nethttp.Handler {
	t.Helper()
	pc := playercontext.NewService(f.gatewayFixture, f.gatewayFixture, timers)
	opts := []Option{WithFleaMarket(f), WithPlayerContext(pc), func(h *Handler) { h.homes = f.gatewayFixture }}
	h, err := NewHandler(f.gatewayFixture, f.gatewayFixture, &struct{ AdventureService }{}, &struct{ ShopService }{}, append(opts, options...)...)
	if err != nil {
		t.Fatal(err)
	}
	return h.Router()
}

func TestFleaMarketGatewayInputs(t *testing.T) {
	cases := map[string][]string{}
	for _, input := range fleaMarketInputs {
		cases[input.action] = []string{"", `null`, `[]`, `1`, `{}`, `{"character_id":"other"}`, `{"extra":null}`,
			fmt.Sprintf(`{%q:null}`, input.field), fmt.Sprintf(`{%q:1}`, input.field)}
	}
	cases["fleamarket_list"] = append(cases["fleamarket_list"], `{"item_id":"x"}`, `{"price":1}`, `{"item_id":"x","price":null}`, `{"item_id":"x","price":"1"}`, `{"item_id":"x","price":1.5}`, `{"item_id":"x","price":1,"seller_character_id":"other"}`)
	for action, inputs := range cases {
		for _, params := range inputs {
			t.Run(action+params, func(t *testing.T) {
				f := &fleaMarketGatewayFixture{gatewayFixture: newGatewayFixture(t)}
				status, got := gatewayRequest(t, f.router(t, timer.NewService(nil)), "hero", "session", "application/json", saleGatewayBody(action, params), nil)
				if status != 400 || f.executions != 0 || f.queryCalls != 0 {
					t.Fatalf("invalid input executed: %d %s", status, got)
				}
				assertGatewayError(t, got, "INVALID_ACTION_PARAMS")
			})
		}
	}
}

func TestFleaMarketGatewayOutcomes(t *testing.T) {
	for _, input := range fleaMarketInputs {
		for _, refreshFailure := range []bool{false, true} {
			for _, rejected := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/refresh=%t/reject=%t", input.action, refreshFailure, rejected), func(t *testing.T) {
					f := &fleaMarketGatewayFixture{gatewayFixture: newGatewayFixture(t)}
					f.expectedContext = context.WithValue(context.Background(), struct{}{}, "flea market request")
					want := 200
					if rejected {
						f.executionErr, want = fmt.Errorf("wrapped: %w", fleamarket.ErrListingNotActive), 400
					}
					if refreshFailure {
						f.afterExecute = func() { f.profileErr = errors.New("private profile failure") }
					}
					router := f.router(t, timer.NewService(nil))
					status, got := gatewayRequest(t, router, "hero", "session", "application/json", saleGatewayBody(input.action, input.params), f.expectedContext)
					if status != want || f.executions != 1 {
						t.Fatalf("lost outcome: %d %s", status, got)
					}
					if rejected {
						assertGatewayError(t, got, "FLEAMARKET_LISTING_NOT_ACTIVE")
					} else if string(got["success"]) != "true" || (f.target != "owned-item" && f.target != "listing-1") || (input.action == "fleamarket_list" && f.price != 300) {
						t.Fatalf("lost explicit target: %+v %s", f, got)
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

func TestFleaMarketGatewayErrors(t *testing.T) {
	for _, tc := range []struct {
		err    error
		code   string
		status int
	}{
		{fleamarket.ErrCharacterNotFound, "CHARACTER_NOT_FOUND", 404},
		{corecharacter.ErrNotFound, "CHARACTER_NOT_FOUND", 404},
		{fleamarket.ErrListingNotFound, "FLEAMARKET_LISTING_NOT_FOUND", 404},
		{fleamarket.ErrUnauthorizedSeller, "FLEAMARKET_NOT_SELLER", 403},
		{fleamarket.ErrInvalidInput, "FLEAMARKET_INVALID_INPUT", 400},
		{fleamarket.ErrInvalidPrice, "FLEAMARKET_INVALID_PRICE", 400},
		{fleamarket.ErrListingNotActive, "FLEAMARKET_LISTING_NOT_ACTIVE", 400},
		{fleamarket.ErrCannotBuyOwnListing, "FLEAMARKET_OWN_LISTING", 400},
		{fleamarket.ErrInsufficientGold, "FLEAMARKET_INSUFFICIENT_GOLD", 400},
		{fleamarket.ErrMaxListingsReached, "FLEAMARKET_MAX_LISTINGS", 400},
		{fleamarket.ErrServerMaxListingsReached, "FLEAMARKET_SERVER_MAX_LISTINGS", 400},
		{fleamarket.ErrItemNotInDepot, "FLEAMARKET_ITEM_NOT_IN_DEPOT", 400},
		{errors.New("private storage failure"), "EXECUTION_FAILED", 500},
		{context.Canceled, "EXECUTION_FAILED", 500},
	} {
		t.Run(tc.err.Error(), func(t *testing.T) {
			f := &fleaMarketGatewayFixture{gatewayFixture: newGatewayFixture(t)}
			f.executionErr = fmt.Errorf("private wrapped: %w", tc.err)
			status, got := gatewayRequest(t, f.router(t, timer.NewService(nil)), "hero", "session", "application/json", saleGatewayBody("fleamarket_purchase", `{"listing_id":"l"}`), nil)
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

func TestFleaMarketGatewayGuardsAndDiscovery(t *testing.T) {
	for _, input := range fleaMarketInputs {
		for _, condition := range []string{"anonymous", "foreign", "missing", "owner", "query", "sleep", "wake", "unfinished", "sleep read", "location"} {
			t.Run(input.action+condition, func(t *testing.T) {
				f := &fleaMarketGatewayFixture{gatewayFixture: newGatewayFixture(t)}
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
		f := &fleaMarketGatewayFixture{gatewayFixture: newGatewayFixture(t)}
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
		for _, option := range []Option{WithFleaMarket(nil), func(*Handler) {}} {
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

func TestRetiredFleaMarketRoutesDoNotReadOrExecute(t *testing.T) {
	f := &fleaMarketGatewayFixture{gatewayFixture: newGatewayFixture(t)}
	router := f.router(t, timer.NewService(nil))
	for _, route := range []struct{ method, path string }{
		{nethttp.MethodGet, "/fleamarket/listings"},
		{nethttp.MethodGet, "/fleamarket/listings/listing-1"},
		{nethttp.MethodGet, "/characters/hero/fleamarket/listings"},
		{nethttp.MethodPost, "/characters/hero/fleamarket/listings"},
		{nethttp.MethodPost, "/characters/hero/fleamarket/listings/listing-1/purchase"},
		{nethttp.MethodDelete, "/characters/hero/fleamarket/listings/listing-1"},
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
