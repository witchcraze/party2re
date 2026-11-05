package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/gemstore"
	"github.com/witchcraze/party2re/internal/playercontext"
)

var gemStoreInputs = []struct{ action, params string }{
	{"gemstore_sell", `{"item_id":"owned-gem"}`},
	{"gemstore_sort", `{}`},
}

type gemStoreGatewayFixture struct {
	*gatewayFixture
	GemStoreService
	itemID     string
	activities []playercontext.Activity
}

func (f *gemStoreGatewayFixture) SellGem(ctx context.Context, actor, itemID string) (gemstore.SellResult, error) {
	f.itemID = itemID
	_, err := f.execute(ctx, actor, 0)
	return gemstore.SellResult{Character: f.char, GemBox: gemstore.GemBox{CharacterID: actor, Capacity: 5}, Gem: gemstore.Gem{ID: "gem_atk_1"}, Payout: 250}, err
}

func (f *gemStoreGatewayFixture) SortGemBox(ctx context.Context, actor string) (gemstore.GemBox, error) {
	_, err := f.execute(ctx, actor, 0)
	return gemstore.GemBox{CharacterID: actor, Capacity: 5}, err
}

func (f *gemStoreGatewayFixture) router(t *testing.T, timers timer.Service, options ...Option) http.Handler {
	t.Helper()
	pc := playercontext.NewService(f.gatewayFixture, f.gatewayFixture, timers,
		playercontext.WithActivities(func(context.Context, string, string) ([]playercontext.Activity, error) { return f.activities, nil }))
	opts := []Option{WithGemStore(f), WithPlayerContext(pc), func(h *Handler) { h.homes = f.gatewayFixture }}
	h, err := NewHandler(f.gatewayFixture, f.gatewayFixture, &struct{ AdventureService }{}, &struct{ ShopService }{}, append(opts, options...)...)
	if err != nil {
		t.Fatal(err)
	}
	return h.Router()
}

func TestGemStoreGatewayInputs(t *testing.T) {
	for _, input := range gemStoreInputs {
		invalid := []string{`null`, `[]`, `1`, `{"character_id":"other"}`, `{"player_id":"other"}`, `{"extra":null}`}
		if input.action == "gemstore_sell" {
			invalid = append(invalid, "", `{}`, `{"item_id":null}`, `{"item_id":1}`)
		}
		for _, params := range invalid {
			t.Run(input.action+params, func(t *testing.T) {
				f := &gemStoreGatewayFixture{gatewayFixture: newGatewayFixture(t)}
				status, got := gatewayRequest(t, f.router(t, timer.NewService(nil)), "hero", "session", "application/json", saleGatewayBody(input.action, params), nil)
				if status != 400 || f.executions != 0 || f.queryCalls != 0 {
					t.Fatalf("invalid input executed: %d %s", status, got)
				}
				assertGatewayError(t, got, "INVALID_ACTION_PARAMS")
			})
		}
	}
}

func TestGemStoreGatewayOutcomes(t *testing.T) {
	for _, input := range gemStoreInputs {
		for _, refreshFailure := range []bool{false, true} {
			for _, rejected := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/refresh=%t/reject=%t", input.action, refreshFailure, rejected), func(t *testing.T) {
					f := &gemStoreGatewayFixture{gatewayFixture: newGatewayFixture(t)}
					f.expectedContext = context.WithValue(context.Background(), struct{}{}, "gem request")
					want := 200
					if rejected {
						f.executionErr, want = fmt.Errorf("wrapped: %w", gemstore.ErrItemNotOwned), 400
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
						assertGatewayError(t, got, "GEMSTORE_ITEM_NOT_OWNED")
					} else {
						var result struct {
							Payout  int
							Gem     gemstore.Gem
							GemBox  gemstore.GemBox `json:"gem_box"`
							Message string
						}
						if err := json.Unmarshal(got["result"], &result); err != nil {
							t.Fatal(err)
						}
						if string(got["success"]) != "true" || result.GemBox.CharacterID != "hero" || result.GemBox.Capacity != 5 {
							t.Fatalf("missing facts: %s", got)
						}
						if input.action == "gemstore_sell" && (f.itemID != "owned-gem" || result.Payout != 250 || result.Gem.ID != "gem_atk_1") {
							t.Fatalf("sale facts: %+v", result)
						}
						if input.action == "gemstore_sort" && result.Message != "お預かりしている宝石をせいとんしました" {
							t.Fatalf("sort message: %+v", result)
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

func TestGemStoreGatewayErrors(t *testing.T) {
	for _, tc := range []struct {
		err    error
		code   string
		status int
	}{
		{gemstore.ErrInvalidCharacterID, "GEMSTORE_INVALID_CHARACTER_ID", 400},
		{gemstore.ErrInvalidGemID, "GEMSTORE_INVALID_GEM_ID", 400},
		{gemstore.ErrItemNotOwned, "GEMSTORE_ITEM_NOT_OWNED", 400},
		{gemstore.ErrGemNotFound, "GEMSTORE_GEM_NOT_FOUND", 404},
		{gemstore.ErrGemBoxNotFound, "GEMSTORE_BOX_NOT_FOUND", 404},
		{corecharacter.ErrNotFound, "CHARACTER_NOT_FOUND", 404},
		{errors.New("private storage failure"), "EXECUTION_FAILED", 500},
		{context.Canceled, "EXECUTION_FAILED", 500},
	} {
		t.Run(tc.err.Error(), func(t *testing.T) {
			f := &gemStoreGatewayFixture{gatewayFixture: newGatewayFixture(t)}
			f.executionErr = fmt.Errorf("private wrapped: %w", tc.err)
			status, got := gatewayRequest(t, f.router(t, timer.NewService(nil)), "hero", "session", "application/json", saleGatewayBody("gemstore_sell", `{"item_id":""}`), nil)
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

func TestGemStoreGatewayGuardsAndDiscovery(t *testing.T) {
	for _, input := range gemStoreInputs {
		for _, condition := range []string{"anonymous", "foreign", "missing", "owner", "query", "sleep", "wake", "unfinished", "activity", "sleep read", "location"} {
			t.Run(input.action+condition, func(t *testing.T) {
				f := &gemStoreGatewayFixture{gatewayFixture: newGatewayFixture(t)}
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
				case "activity":
					f.activities = []playercontext.Activity{{Kind: "casino", ID: "room", Phase: "playing"}}
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
		f := &gemStoreGatewayFixture{gatewayFixture: newGatewayFixture(t)}
		f.char.Money = 0
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
		for _, option := range []Option{WithGemStore(nil), func(*Handler) {}} {
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

func TestGemStoreRetiredPosts(t *testing.T) {
	f := &gemStoreGatewayFixture{gatewayFixture: newGatewayFixture(t)}
	router := f.router(t, timer.NewService(nil))
	for _, path := range []string{"gemstore/sell", "gembox/sort"} {
		r := httptest.NewRequest(http.MethodPost, "/characters/hero/"+path, strings.NewReader(`{"item_id":"owned-gem"}`))
		r.Header.Set("Authorization", "Bearer session")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if (w.Code != 404 && w.Code != 405) || f.executions != 0 || f.queryCalls != 0 || f.characterCalls != 0 {
			t.Fatalf("retired POST reachable: %d %s", w.Code, w.Body.String())
		}
	}
}
