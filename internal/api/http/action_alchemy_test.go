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

	"github.com/witchcraze/party2re/internal/alchemy"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/playercontext"
)

const alchemySynthesizeParams = `{"recipe_id":"recipe-001"}`

type alchemyGatewayFixture struct {
	*gatewayFixture
	AlchemyService
	target string
}

func (f *alchemyGatewayFixture) Synthesize(ctx context.Context, actor, recipeID string) (alchemy.SynthesisResult, error) {
	f.target = recipeID
	if _, err := f.execute(ctx, actor, 0); err != nil {
		return alchemy.SynthesisResult{}, err
	}
	return alchemy.SynthesisResult{Recipe: alchemy.RecipeView{ID: recipeID, Ingredients: []alchemy.Ingredient{}}, State: alchemy.StateOngoing, Message: "started"}, nil
}

func (f *alchemyGatewayFixture) router(t *testing.T, timers timer.Service, options ...Option) nethttp.Handler {
	t.Helper()
	pc := playercontext.NewService(f.gatewayFixture, f.gatewayFixture, timers)
	opts := []Option{WithAlchemy(f), WithPlayerContext(pc), func(h *Handler) { h.homes = f.gatewayFixture }}
	h, err := NewHandler(f.gatewayFixture, f.gatewayFixture, &struct{ AdventureService }{}, &struct{ ShopService }{}, append(opts, options...)...)
	if err != nil {
		t.Fatal(err)
	}
	return h.Router()
}

func TestAlchemyGatewayInputs(t *testing.T) {
	for _, params := range []string{"", `null`, `[]`, `1`, `{}`, `{"character_id":"other"}`, `{"extra":null}`, `{"recipe_id":null}`, `{"recipe_id":1}`, `{"recipe_id":"x","character_id":"other"}`} {
		t.Run(params, func(t *testing.T) {
			f := &alchemyGatewayFixture{gatewayFixture: newGatewayFixture(t)}
			status, got := gatewayRequest(t, f.router(t, timer.NewService(nil)), "hero", "session", "application/json", saleGatewayBody("alchemy_synthesize", params), nil)
			if status != 400 || f.executions != 0 || f.queryCalls != 0 {
				t.Fatalf("invalid input executed: %d %s", status, got)
			}
			assertGatewayError(t, got, "INVALID_ACTION_PARAMS")
		})
	}
}

func TestAlchemyGatewayOutcomes(t *testing.T) {
	for _, refreshFailure := range []bool{false, true} {
		for _, rejected := range []bool{false, true} {
			t.Run(fmt.Sprintf("refresh=%t/reject=%t", refreshFailure, rejected), func(t *testing.T) {
				f := &alchemyGatewayFixture{gatewayFixture: newGatewayFixture(t)}
				f.expectedContext = context.WithValue(context.Background(), struct{}{}, "alchemy request")
				want := 200
				if rejected {
					f.executionErr, want = fmt.Errorf("wrapped: %w", alchemy.ErrInsufficientMaterials), 422
				}
				if refreshFailure {
					f.afterExecute = func() { f.profileErr = errors.New("private profile failure") }
				}
				router := f.router(t, timer.NewService(nil))
				status, got := gatewayRequest(t, router, "hero", "session", "application/json", saleGatewayBody("alchemy_synthesize", alchemySynthesizeParams), f.expectedContext)
				if status != want || f.executions != 1 || f.target != "recipe-001" {
					t.Fatalf("lost outcome: %d %s", status, got)
				}
				if rejected {
					assertGatewayError(t, got, "ALCHEMY_INSUFFICIENT_MATERIALS")
				} else if string(got["success"]) != "true" || !strings.Contains(string(got["result"]), `"message":"started"`) {
					t.Fatalf("lost result facts: %s", got)
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

func TestAlchemyGatewayErrors(t *testing.T) {
	for _, tc := range []struct {
		err    error
		code   string
		status int
	}{
		{alchemy.ErrInvalidRecipeID, "ALCHEMY_INVALID_RECIPE", 400},
		{alchemy.ErrRecipeNotFound, "ALCHEMY_RECIPE_NOT_FOUND", 404},
		{corecharacter.ErrNotFound, "CHARACTER_NOT_FOUND", 404},
		{alchemy.ErrRecipeNotLearned, "ALCHEMY_RECIPE_NOT_LEARNED", 422},
		{alchemy.ErrInsufficientMaterials, "ALCHEMY_INSUFFICIENT_MATERIALS", 422},
		{alchemy.ErrSynthesisInProgress, "ALCHEMY_SYNTHESIS_IN_PROGRESS", 422},
		{errors.New("private storage failure"), "EXECUTION_FAILED", 500},
		{context.Canceled, "EXECUTION_FAILED", 500},
	} {
		t.Run(tc.err.Error(), func(t *testing.T) {
			f := &alchemyGatewayFixture{gatewayFixture: newGatewayFixture(t)}
			f.executionErr = fmt.Errorf("private wrapped: %w", tc.err)
			status, got := gatewayRequest(t, f.router(t, timer.NewService(nil)), "hero", "session", "application/json", saleGatewayBody("alchemy_synthesize", alchemySynthesizeParams), nil)
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

func TestAlchemyGatewayGuardsAndDiscovery(t *testing.T) {
	for _, condition := range []string{"anonymous", "foreign", "missing", "owner", "query", "sleep", "wake", "unfinished", "sleep read", "location"} {
		t.Run(condition, func(t *testing.T) {
			f := &alchemyGatewayFixture{gatewayFixture: newGatewayFixture(t)}
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
			status, got := gatewayRequest(t, f.router(t, timers, options...), path, token, "application/json", saleGatewayBody("alchemy_synthesize", alchemySynthesizeParams), nil)
			if status != want || f.executions != 0 {
				t.Fatalf("guard bypass: %d %s", status, got)
			}
		})
	}
	f := &alchemyGatewayFixture{gatewayFixture: newGatewayFixture(t)}
	status, observation := navigationGET(t, f.router(t, timer.NewService(nil)))
	if status != 200 || !slices.ContainsFunc(observation.AvailableActions, func(a ContextAction) bool {
		return a.Action == "alchemy_synthesize" && a.RequiredParams != nil && len(a.ParamsSchema) > 0
	}) || f.executions != 0 {
		t.Fatalf("missing discovery: %d %+v", status, observation)
	}
	resolved := NewActionURLResolver().Resolve("hero", "alchemy_synthesize", "", "")
	if resolved.Method != "POST" || resolved.URL != "/api/v1/characters/hero/actions" {
		t.Fatalf("obsolete resolver: %+v", resolved)
	}
	for _, option := range []Option{WithAlchemy(nil), func(*Handler) {}} {
		g := newGatewayFixture(t)
		h, err := NewHandler(g, g, &struct{ AdventureService }{}, &struct{ ShopService }{}, option, WithPlayerContext(playercontext.NewService(g, g, timer.NewService(nil))))
		if err != nil {
			t.Fatal(err)
		}
		status, got := gatewayRequest(t, h.Router(), "hero", "session", "application/json", saleGatewayBody("alchemy_synthesize", alchemySynthesizeParams), nil)
		if status != 501 || g.queryCalls != 0 {
			t.Fatalf("unconfigured: %d %s", status, got)
		}
	}
	if _, known := actionDefinition("alchemy_claim"); known {
		t.Fatal("delivery happens on entry; no separate claim command exists")
	}
}

func TestAlchemyRetiredRoutesAreUnreachable(t *testing.T) {
	f := &alchemyGatewayFixture{gatewayFixture: newGatewayFixture(t)}
	router := f.router(t, timer.NewService(nil))
	for _, route := range []struct{ method, path string }{
		{nethttp.MethodGet, "/characters/hero/alchemy"},
		{nethttp.MethodPost, "/characters/hero/alchemy/synthesize"},
		{nethttp.MethodPost, "/characters/hero/alchemy/claim"},
		{nethttp.MethodPost, "/characters/hero/alchemy/learn"},
	} {
		req := httptest.NewRequest(route.method, route.path, strings.NewReader(alchemySynthesizeParams))
		req.Header.Set("Authorization", "Bearer session")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != nethttp.StatusNotFound && rec.Code != nethttp.StatusMethodNotAllowed {
			t.Errorf("%s %s still reachable: %d", route.method, route.path, rec.Code)
		}
	}
	if f.executions != 0 {
		t.Fatal("retired route executed")
	}
}
