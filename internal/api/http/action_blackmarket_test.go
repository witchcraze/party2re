package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	nethttp "net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/blackmarket"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/playercontext"
)

var blackMarketInputs = []struct{ action, field, params string }{
	{"blackmarket_sacrifice", "item_instance_id", `{"item_instance_id":"owned-item"}`},
	{"blackmarket_trade", "prize_id", `{"prize_id":"bm_prize_087"}`},
}

type blackMarketGatewayFixture struct {
	*gatewayFixture
	BlackMarketService
	target string
}

func (f *blackMarketGatewayFixture) SacrificeItem(ctx context.Context, actor, itemID string) (*blackmarket.SacrificeResult, error) {
	f.target = itemID
	if _, err := f.execute(ctx, actor, 0); err != nil {
		return nil, err
	}
	return &blackmarket.SacrificeResult{CharacterID: actor, ItemInstanceID: itemID, RarePointsGained: 1, TotalRarePoints: 3, Message: "sacrificed"}, nil
}

func (f *blackMarketGatewayFixture) TradePrize(ctx context.Context, actor, prizeID string) (*blackmarket.TradeResult, error) {
	f.target = prizeID
	if _, err := f.execute(ctx, actor, 0); err != nil {
		return nil, err
	}
	return &blackmarket.TradeResult{CharacterID: actor, PrizeID: prizeID, DepotInstanceID: "delivered", Cost: 1, RemainingRare: 2, Message: "traded"}, nil
}

func (f *blackMarketGatewayFixture) router(t *testing.T, timers timer.Service, options ...Option) nethttp.Handler {
	t.Helper()
	pc := playercontext.NewService(f.gatewayFixture, f.gatewayFixture, timers)
	opts := []Option{WithBlackMarket(f), WithPlayerContext(pc), func(h *Handler) { h.homes = f.gatewayFixture }}
	h, err := NewHandler(f.gatewayFixture, f.gatewayFixture, &struct{ AdventureService }{}, &struct{ ShopService }{}, append(opts, options...)...)
	if err != nil {
		t.Fatal(err)
	}
	return h.Router()
}

func TestBlackMarketGatewayInputs(t *testing.T) {
	for _, input := range blackMarketInputs {
		for _, params := range []string{"", `null`, `[]`, `1`, `{}`, `{"character_id":"other"}`, `{"player_id":"other"}`, `{"extra":null}`,
			fmt.Sprintf(`{%q:null}`, input.field), fmt.Sprintf(`{%q:1}`, input.field), fmt.Sprintf(`{%q:"x","character_id":"other"}`, input.field)} {
			t.Run(input.action+params, func(t *testing.T) {
				f := &blackMarketGatewayFixture{gatewayFixture: newGatewayFixture(t)}
				status, got := gatewayRequest(t, f.router(t, timer.NewService(nil)), "hero", "session", "application/json", saleGatewayBody(input.action, params), nil)
				if status != 400 || f.executions != 0 || f.queryCalls != 0 {
					t.Fatalf("invalid input executed: %d %s", status, got)
				}
				assertGatewayError(t, got, "INVALID_ACTION_PARAMS")
			})
		}
	}
}

func TestBlackMarketGatewayOutcomes(t *testing.T) {
	for _, input := range blackMarketInputs {
		for _, refreshFailure := range []bool{false, true} {
			for _, rejected := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/refresh=%t/reject=%t", input.action, refreshFailure, rejected), func(t *testing.T) {
					f := &blackMarketGatewayFixture{gatewayFixture: newGatewayFixture(t)}
					f.expectedContext = context.WithValue(context.Background(), struct{}{}, "black market request")
					want := 200
					if rejected {
						f.executionErr, want = fmt.Errorf("wrapped: %w", blackmarket.ErrInsufficientRarePoints), 400
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
						assertGatewayError(t, got, "BLACKMARKET_INSUFFICIENT_RARE_POINTS")
					} else {
						var result struct {
							Message         string `json:"message"`
							ItemInstanceID  string `json:"item_instance_id"`
							PrizeID         string `json:"prize_id"`
							DepotInstanceID string `json:"depot_instance_id"`
						}
						if err := json.Unmarshal(got["result"], &result); err != nil {
							t.Fatal(err)
						}
						ok := result.Message == "sacrificed" && result.ItemInstanceID == "owned-item" && f.target == "owned-item"
						if input.action == "blackmarket_trade" {
							ok = result.Message == "traded" && result.PrizeID == "bm_prize_087" && result.DepotInstanceID == "delivered" && f.target == "bm_prize_087"
						}
						if string(got["success"]) != "true" || !ok {
							t.Fatalf("lost result facts: %s", got)
						}
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

func TestBlackMarketGatewayErrors(t *testing.T) {
	for _, tc := range []struct {
		err    error
		code   string
		status int
	}{
		{blackmarket.ErrAccessDenied, "BLACKMARKET_ACCESS_DENIED", 403},
		{blackmarket.ErrCharacterNotFound, "CHARACTER_NOT_FOUND", 404},
		{corecharacter.ErrNotFound, "CHARACTER_NOT_FOUND", 404},
		{blackmarket.ErrPrizeNotFound, "BLACKMARKET_PRIZE_NOT_FOUND", 404},
		{blackmarket.ErrUnownedItem, "BLACKMARKET_ITEM_NOT_OWNED", 400},
		{blackmarket.ErrNotSacrificeEligible, "BLACKMARKET_ITEM_NOT_ELIGIBLE", 400},
		{blackmarket.ErrInsufficientRarePoints, "BLACKMARKET_INSUFFICIENT_RARE_POINTS", 400},
		{blackmarket.ErrInsufficientURarePoints, "BLACKMARKET_INSUFFICIENT_U_RARE_POINTS", 400},
		{blackmarket.ErrDepotFull, "BLACKMARKET_DEPOT_FULL", 400},
		{blackmarket.ErrDepotNotConfigured, "EXECUTION_FAILED", 500},
		{errors.New("private storage failure"), "EXECUTION_FAILED", 500},
		{context.Canceled, "EXECUTION_FAILED", 500},
	} {
		t.Run(tc.err.Error(), func(t *testing.T) {
			f := &blackMarketGatewayFixture{gatewayFixture: newGatewayFixture(t)}
			f.executionErr = fmt.Errorf("private wrapped: %w", tc.err)
			status, got := gatewayRequest(t, f.router(t, timer.NewService(nil)), "hero", "session", "application/json", saleGatewayBody("blackmarket_trade", `{"prize_id":"p"}`), nil)
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

func TestBlackMarketGatewayGuardsAndDiscovery(t *testing.T) {
	for _, input := range blackMarketInputs {
		for _, condition := range []string{"anonymous", "foreign", "missing", "owner", "query", "sleep", "wake", "unfinished", "sleep read", "location"} {
			t.Run(input.action+condition, func(t *testing.T) {
				f := &blackMarketGatewayFixture{gatewayFixture: newGatewayFixture(t)}
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
		f := &blackMarketGatewayFixture{gatewayFixture: newGatewayFixture(t)}
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
		for _, option := range []Option{WithBlackMarket(nil), func(*Handler) {}} {
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
