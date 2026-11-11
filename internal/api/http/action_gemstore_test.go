package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/gemstore"
	"github.com/witchcraze/party2re/internal/playercontext"
)

var gemStoreInputs = []struct{ action, params string }{
	{"gemstore_buy", `{"gem_id":"gem_atk_1"}`},
	{"gemstore_sell", `{"item_id":"owned-gem"}`},
	{"gemstore_send", `{"recipient_character_id":"friend","item_id":"owned-gem"}`},
	{"gemstore_synthesize", `{"recipe_id":"recipe-1"}`},
	{"gemstore_appraise", `{"item_id":"orb"}`},
	{"gemstore_sort", `{}`},
}

// gemStoreRequiredFields lists each command's required string inputs.
var gemStoreRequiredFields = map[string][]string{
	"gemstore_buy":        {"gem_id"},
	"gemstore_sell":       {"item_id"},
	"gemstore_send":       {"recipient_character_id", "item_id"},
	"gemstore_synthesize": {"recipe_id"},
	"gemstore_appraise":   {"item_id"},
}

type gemStoreGatewayFixture struct {
	*gatewayFixture
	GemStoreService
	itemID     string
	inputs     []string
	activities []playercontext.Activity
}

func (f *gemStoreGatewayFixture) BuyGem(ctx context.Context, actor, gemID string) (gemstore.BuyResult, error) {
	f.inputs = []string{gemID}
	_, err := f.execute(ctx, actor, 0)
	return gemstore.BuyResult{Character: f.char, GemBox: gemstore.GemBox{CharacterID: actor, Capacity: 5}, Gem: gemstore.Gem{ID: "gem_atk_1"}, Cost: 300}, err
}

func (f *gemStoreGatewayFixture) SendGem(ctx context.Context, actor, recipientID, itemID string) (gemstore.SendResult, error) {
	f.inputs = []string{recipientID, itemID}
	_, err := f.execute(ctx, actor, 0)
	return gemstore.SendResult{SenderGemBox: gemstore.GemBox{CharacterID: actor, Capacity: 5}, Gem: gemstore.Gem{ID: "gem_atk_1"},
		SenderCharacter:    corecharacter.Character{ID: actor, PlayerID: "sender-player", Money: 12345},
		RecipientCharacter: corecharacter.Character{ID: recipientID, Name: "Friend", PlayerID: "private-player", Money: 987654, Deposit: 555777},
		RecipientGemBox:    gemstore.GemBox{CharacterID: recipientID, Capacity: 99, Items: []coreitem.Instance{{ID: "private-gem", DefinitionID: "gem_atk_9"}}}}, err
}

func (f *gemStoreGatewayFixture) SynthesizeGem(ctx context.Context, actor, recipeID string) (gemstore.SynthesizeResult, error) {
	f.inputs = []string{recipeID}
	_, err := f.execute(ctx, actor, 0)
	return gemstore.SynthesizeResult{GemBox: gemstore.GemBox{CharacterID: actor, Capacity: 5}, CreatedGem: gemstore.Gem{ID: "gem_atk_2"}, Recipe: gemstore.Recipe{ID: recipeID}}, err
}

func (f *gemStoreGatewayFixture) AppraiseItem(ctx context.Context, actor, itemID string) (gemstore.AppraiseResult, error) {
	f.inputs = []string{itemID}
	_, err := f.execute(ctx, actor, 0)
	return gemstore.AppraiseResult{GemBox: gemstore.GemBox{CharacterID: actor, Capacity: 5}, IsGem: true, Message: "appraised"}, err
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
		if fields := gemStoreRequiredFields[input.action]; fields != nil {
			invalid = append(invalid, "", `{}`)
			for _, field := range fields {
				invalid = append(invalid, strings.Replace(input.params, `"`+field+`":"`, `"`+field+`":1,"x":"`, 1),
					strings.Replace(input.params, `"`+field+`":`, `"`+field+`":null,"x":`, 1))
			}
			if len(fields) > 1 {
				invalid = append(invalid, fmt.Sprintf(`{%q:"only"}`, fields[0]))
			}
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

func TestGemStoreGatewayMissingMaterialsNames(t *testing.T) {
	f := &gemStoreGatewayFixture{gatewayFixture: newGatewayFixture(t)}
	f.executionErr = fmt.Errorf("wrapped: %w", &gemstore.InsufficientMaterialsError{Recipe: gemstore.Recipe{
		Material1Name: "攻撃の天珠Ⅱ", Material2Name: "天馬のたづな",
	}})
	status, got := gatewayRequest(t, f.router(t, timer.NewService(nil)), "hero", "session", "application/json",
		saleGatewayBody("gemstore_synthesize", `{"recipe_id":"recipe_burst_sky"}`), nil)
	if status != 400 {
		t.Fatalf("status=%d: %s", status, got)
	}
	assertGatewayError(t, got, "GEMSTORE_INSUFFICIENT_MATERIALS")
	for _, name := range []string{"攻撃の天珠Ⅱ", "天馬のたづな"} {
		if !strings.Contains(string(got["error"]), name) {
			t.Fatalf("missing material name %q: %s", name, got)
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
							Payout      int
							Cost        int
							Gem         gemstore.Gem
							CreatedGem  gemstore.Gem    `json:"created_gem"`
							GemBox      gemstore.GemBox `json:"gem_box"`
							IsGem       bool            `json:"is_gem"`
							Message     string
							Recipient   string `json:"recipient_character_id"`
							RecipientNm string `json:"recipient_character_name"`
						}
						if err := json.Unmarshal(got["result"], &result); err != nil {
							t.Fatal(err)
						}
						box := result.GemBox
						if string(got["success"]) != "true" || box.CharacterID != "hero" || box.Capacity != 5 {
							t.Fatalf("missing facts: %s", got)
						}
						if want := map[string][]string{"gemstore_buy": {"gem_atk_1"}, "gemstore_send": {"friend", "owned-gem"}, "gemstore_synthesize": {"recipe-1"}, "gemstore_appraise": {"orb"}}[input.action]; want != nil && !slices.Equal(f.inputs, want) {
							t.Fatalf("explicit inputs lost: %v", f.inputs)
						}
						if input.action == "gemstore_send" {
							// Only the recipient's public identity may cross the boundary.
							var fields map[string]json.RawMessage
							if err := json.Unmarshal(got["result"], &fields); err != nil {
								t.Fatal(err)
							}
							keys := slices.Sorted(maps.Keys(fields))
							if !slices.Equal(keys, []string{"gem", "gem_box", "recipient_character_id", "recipient_character_name"}) || result.Recipient != "friend" || result.RecipientNm != "Friend" {
								t.Fatalf("send result fields: %s", got["result"])
							}
							for _, private := range []string{"private-player", "sender-player", "987654", "555777", "12345", "private-gem", "PlayerID"} {
								if strings.Contains(string(got["result"]), private) {
									t.Fatalf("send result leaked %q: %s", private, got["result"])
								}
							}
						}
						if (input.action == "gemstore_buy" && result.Cost != 300) || (input.action == "gemstore_send" && result.Gem.ID != "gem_atk_1") ||
							(input.action == "gemstore_synthesize" && result.CreatedGem.ID != "gem_atk_2") || (input.action == "gemstore_appraise" && (!result.IsGem || result.Message != "appraised")) {
							t.Fatalf("command facts: %s", got["result"])
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
		{gemstore.ErrInvalidRecipeID, "GEMSTORE_INVALID_RECIPE_ID", 400},
		{gemstore.ErrItemNotOwned, "GEMSTORE_ITEM_NOT_OWNED", 400},
		{gemstore.ErrGemNotForSale, "GEMSTORE_GEM_NOT_FOR_SALE", 400},
		{gemstore.ErrInsufficientFunds, "GEMSTORE_INSUFFICIENT_FUNDS", 400},
		{gemstore.ErrInsufficientMaterials, "GEMSTORE_INSUFFICIENT_MATERIALS", 400},
		{gemstore.ErrCannotSendToSelf, "GEMSTORE_SELF_TRANSFER", 400},
		{gemstore.ErrGemBoxFull, "GEMSTORE_BOX_FULL", 400},
		{gemstore.ErrRecipientGemBoxFull, "GEMSTORE_RECIPIENT_BOX_FULL", 400},
		{gemstore.ErrGemNotFound, "GEMSTORE_GEM_NOT_FOUND", 404},
		{gemstore.ErrRecipeNotFound, "GEMSTORE_RECIPE_NOT_FOUND", 404},
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
		if input.action != "gemstore_buy" {
			f.char.Money = 0 // the other commands are not gated on funds
		}
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

func TestGemStoreRetiredRoutes(t *testing.T) {
	f := &gemStoreGatewayFixture{gatewayFixture: newGatewayFixture(t)}
	router := f.router(t, timer.NewService(nil))
	for _, route := range []struct{ method, path string }{
		{http.MethodPost, "/characters/hero/gemstore/sell"}, {http.MethodPost, "/characters/hero/gembox/sort"},
		{http.MethodPost, "/characters/hero/gemstore/buy"}, {http.MethodPost, "/characters/hero/gemstore/send"},
		{http.MethodPost, "/characters/hero/gemstore/synthesize"}, {http.MethodPost, "/characters/hero/gemstore/appraise"},
		{http.MethodGet, "/characters/hero/gembox"}, {http.MethodGet, "/gemstore/catalog"},
		{http.MethodGet, "/gemstore/recipes"}, {http.MethodGet, "/gemstore/dialogue"},
	} {
		r := httptest.NewRequest(route.method, route.path, strings.NewReader(`{"item_id":"owned-gem"}`))
		r.Header.Set("Authorization", "Bearer session")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if (w.Code != 404 && w.Code != 405) || f.executions != 0 || f.queryCalls != 0 || f.characterCalls != 0 {
			t.Fatalf("retired route reachable: %s %s %d %s", route.method, route.path, w.Code, w.Body.String())
		}
	}
}
