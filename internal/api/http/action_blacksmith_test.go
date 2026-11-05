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

	"github.com/witchcraze/party2re/internal/blacksmith"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/playercontext"
)

var blacksmithInputs = []struct{ action, params string }{
	{"blacksmith_seal", `{"seal_id":2}`},
	{"blacksmith_name", `{"target":" ARM ","name":" 名剣 "}`},
	{"blacksmith_deposit", `{}`},
	{"blacksmith_withdraw", `{"slot":3}`},
}

type blacksmithGatewayFixture struct {
	*gatewayFixture
	BlacksmithService
	seal, slot   int
	target, name string
	activities   []playercontext.Activity
}

func (f *blacksmithGatewayFixture) ApplySeal(ctx context.Context, actor string, seal int) (blacksmith.Seal, error) {
	f.seal = seal
	_, err := f.execute(ctx, actor, 0)
	return blacksmith.Seals[1], err
}

func (f *blacksmithGatewayFixture) NameEquipment(ctx context.Context, actor, target, name string) error {
	f.target, f.name = target, name
	_, err := f.execute(ctx, actor, 0)
	return err
}

func (f *blacksmithGatewayFixture) DepositWeapon(ctx context.Context, actor string) (blacksmith.Deposit, error) {
	_, err := f.execute(ctx, actor, 0)
	return blacksmith.Deposit{ID: "stored", CharacterID: actor, Slot: 3, ItemDefinitionID: "weapon-01", SealID: 2, CustomName: "名剣"}, err
}

func (f *blacksmithGatewayFixture) WithdrawWeapon(ctx context.Context, actor string, slot int) error {
	f.slot = slot
	_, err := f.execute(ctx, actor, 0)
	return err
}

func (f *blacksmithGatewayFixture) router(t *testing.T, timers timer.Service, options ...Option) http.Handler {
	t.Helper()
	pc := playercontext.NewService(f.gatewayFixture, f.gatewayFixture, timers,
		playercontext.WithActivities(func(context.Context, string, string) ([]playercontext.Activity, error) { return f.activities, nil }))
	opts := []Option{WithBlacksmith(f), WithPlayerContext(pc), func(h *Handler) { h.homes = f.gatewayFixture }}
	h, err := NewHandler(f.gatewayFixture, f.gatewayFixture, &struct{ AdventureService }{}, &struct{ ShopService }{}, append(opts, options...)...)
	if err != nil {
		t.Fatal(err)
	}
	return h.Router()
}

func TestBlacksmithGatewayInputs(t *testing.T) {
	for _, tc := range blacksmithInputs {
		invalid := []string{`null`, `[]`, `1`, `{"character_id":"other"}`, `{"player_id":"other"}`, `{"extra":null}`}
		switch tc.action {
		case "blacksmith_seal":
			invalid = append(invalid, "", `{}`, `{"seal_id":null}`, `{"seal_id":"2"}`, `{"seal_id":1.5}`, `{"seal_id":9223372036854775808}`)
		case "blacksmith_name":
			invalid = append(invalid, "", `{}`, `{"target":"weapon"}`, `{"name":"x"}`, `{"target":null,"name":"x"}`, `{"target":"weapon","name":null}`, `{"target":1,"name":"x"}`, `{"target":"weapon","name":1}`)
		case "blacksmith_deposit":
			invalid = append(invalid, `{"slot":1}`)
		case "blacksmith_withdraw":
			invalid = append(invalid, "", `{}`, `{"slot":null}`, `{"slot":"1"}`, `{"slot":1.5}`, `{"slot":9223372036854775808}`)
		}
		for _, params := range invalid {
			t.Run(tc.action+params, func(t *testing.T) {
				f := &blacksmithGatewayFixture{gatewayFixture: newGatewayFixture(t)}
				status, got := gatewayRequest(t, f.router(t, timer.NewService(nil)), "hero", "session", "application/json", saleGatewayBody(tc.action, params), nil)
				if status != 400 || f.executions != 0 || f.queryCalls != 0 {
					t.Fatalf("invalid input executed: %d %s", status, got)
				}
				assertGatewayError(t, got, "INVALID_ACTION_PARAMS")
			})
		}
	}
}

func TestBlacksmithGatewayOutcomes(t *testing.T) {
	for _, tc := range blacksmithInputs {
		for _, refreshFailure := range []bool{false, true} {
			for _, rejected := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/refresh=%t/reject=%t", tc.action, refreshFailure, rejected), func(t *testing.T) {
					f := &blacksmithGatewayFixture{gatewayFixture: newGatewayFixture(t)}
					f.expectedContext = context.WithValue(context.Background(), struct{}{}, "blacksmith request")
					want := 200
					if rejected {
						f.executionErr, want = fmt.Errorf("wrapped: %w", blacksmith.ErrNoWeaponEquipped), 400
					}
					if refreshFailure {
						f.afterExecute = func() { f.profileErr = errors.New("private profile failure") }
					}
					router := f.router(t, timer.NewService(nil))
					status, got := gatewayRequest(t, router, "hero", "session", "application/json", saleGatewayBody(tc.action, tc.params), f.expectedContext)
					if status != want || f.executions != 1 {
						t.Fatalf("lost outcome: %d %s", status, got)
					}
					if rejected {
						assertGatewayError(t, got, "BLACKSMITH_NO_WEAPON")
					} else {
						var result map[string]json.RawMessage
						if err := json.Unmarshal(got["result"], &result); err != nil {
							t.Fatal(err)
						}
						if string(got["success"]) != "true" || len(result["message"]) == 0 {
							t.Fatalf("missing result: %s", got)
						}
						switch tc.action {
						case "blacksmith_seal":
							var seal blacksmith.Seal
							if err := json.Unmarshal(result["seal"], &seal); err != nil || seal.ID != 2 || seal.CrystalCost != 500 || f.seal != 2 {
								t.Fatalf("seal facts: %s", result)
							}
						case "blacksmith_name":
							if f.target != " ARM " || f.name != " 名剣 " || string(result["target"]) != `"armor"` || string(result["name"]) != `"名剣"` || string(result["message"]) != `"防具を名剣と名づけたぜ"` {
								t.Fatalf("name facts: %s", result)
							}
						case "blacksmith_deposit":
							var dep blacksmith.Deposit
							if err := json.Unmarshal(result["deposit"], &dep); err != nil || dep.Slot != 3 || dep.SealID != 2 || dep.CustomName != "名剣" || dep.CharacterID != "hero" {
								t.Fatalf("deposit facts: %s", result)
							}
						case "blacksmith_withdraw":
							if f.slot != 3 || string(result["slot"]) != "3" {
								t.Fatalf("withdraw facts: %s", result)
							}
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
	f := &blacksmithGatewayFixture{gatewayFixture: newGatewayFixture(t)}
	router := f.router(t, timer.NewService(nil))
	for _, body := range []string{`{"action":"blacksmith_deposit"}`, `{"action":"blacksmith_name","params":{"target":"weapon","name":""}}`} {
		status, got := gatewayRequest(t, router, "hero", "session", "application/json", body, nil)
		if status != 200 {
			t.Fatalf("empty input/reset failed: %d %s", status, got)
		}
	}
}

func TestBlacksmithGatewayErrors(t *testing.T) {
	for _, tc := range []struct {
		err    error
		code   string
		status int
	}{
		{blacksmith.ErrInvalidCharacterID, "BLACKSMITH_INVALID_CHARACTER_ID", 400},
		{blacksmith.ErrNoWeaponEquipped, "BLACKSMITH_NO_WEAPON", 400},
		{blacksmith.ErrNoArmorEquipped, "BLACKSMITH_NO_ARMOR", 400},
		{blacksmith.ErrInvalidSeal, "BLACKSMITH_INVALID_SEAL", 400},
		{blacksmith.ErrSealNotApplicable, "BLACKSMITH_SEAL_NOT_APPLICABLE", 400},
		{blacksmith.ErrInsufficientCrystals, "BLACKSMITH_INSUFFICIENT_CRYSTALS", 400},
		{blacksmith.ErrStorageFull, "BLACKSMITH_STORAGE_FULL", 400},
		{blacksmith.ErrDuplicateStoredName, "BLACKSMITH_DUPLICATE_STORED_NAME", 400},
		{blacksmith.ErrWeaponSlotOccupied, "BLACKSMITH_WEAPON_SLOT_OCCUPIED", 400},
		{blacksmith.ErrInvalidSlot, "BLACKSMITH_INVALID_SLOT", 400},
		{blacksmith.ErrInvalidTarget, "BLACKSMITH_INVALID_TARGET", 400},
		{blacksmith.ErrNameWhitespace, "BLACKSMITH_INVALID_NAME", 400},
		{blacksmith.ErrNameForbiddenChar, "BLACKSMITH_INVALID_NAME", 400},
		{blacksmith.ErrNameAtSymbol, "BLACKSMITH_INVALID_NAME", 400},
		{blacksmith.ErrNameTooLong, "BLACKSMITH_INVALID_NAME", 400},
		{blacksmith.ErrEmptyName, "BLACKSMITH_INVALID_NAME", 400},
		{coreinventory.ErrInventoryFull, "BLACKSMITH_INVENTORY_FULL", 400},
		{blacksmith.ErrDepositNotFound, "BLACKSMITH_DEPOSIT_NOT_FOUND", 404},
		{corecharacter.ErrNotFound, "CHARACTER_NOT_FOUND", 404},
		{errors.New("private storage failure"), "EXECUTION_FAILED", 500},
		{context.Canceled, "EXECUTION_FAILED", 500},
	} {
		t.Run(tc.err.Error(), func(t *testing.T) {
			f := &blacksmithGatewayFixture{gatewayFixture: newGatewayFixture(t)}
			f.executionErr = fmt.Errorf("private wrapped: %w", tc.err)
			status, got := gatewayRequest(t, f.router(t, timer.NewService(nil)), "hero", "session", "application/json", saleGatewayBody("blacksmith_seal", `{"seal_id":0}`), nil)
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

func TestBlacksmithGatewayGuardsAndDiscovery(t *testing.T) {
	for _, input := range blacksmithInputs {
		for _, condition := range []string{"anonymous", "foreign", "missing", "owner", "query", "sleep", "wake", "unfinished", "activity", "sleep read", "location"} {
			t.Run(input.action+condition, func(t *testing.T) {
				f := &blacksmithGatewayFixture{gatewayFixture: newGatewayFixture(t)}
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
					store := &gatewayNavigationStore{selection: playercontext.Selection{Destination: "bank"}}
					options = append(options, WithPlayerContext(playercontext.NewService(f.gatewayFixture, f.gatewayFixture, timers, playercontext.WithNavigation(store, playercontext.SceneDefinition{ID: "town"}, playercontext.SceneDefinition{ID: "bank", Parent: "town"}))))
				}
				status, got := gatewayRequest(t, f.router(t, timers, options...), path, token, "application/json", saleGatewayBody(input.action, input.params), nil)
				if status != want || f.executions != 0 {
					t.Fatalf("guard bypass: %d %s", status, got)
				}
			})
		}
		f := &blacksmithGatewayFixture{gatewayFixture: newGatewayFixture(t)}
		f.char.Money, f.char.Crystal = 0, 0
		status, observation := navigationGET(t, f.router(t, timer.NewService(nil)))
		index := slices.IndexFunc(observation.AvailableActions, func(a ContextAction) bool { return a.Action == input.action })
		if status != 200 || index < 0 || f.executions != 0 {
			t.Fatalf("missing discovery: %d %+v", status, observation)
		}
		action := observation.AvailableActions[index]
		var schema struct {
			Properties           map[string]json.RawMessage
			Required             []string
			AdditionalProperties bool
		}
		if err := json.Unmarshal(action.ParamsSchema, &schema); err != nil {
			t.Fatal(err)
		}
		if action.RequiredParams == nil || !slices.Equal(schema.Required, action.RequiredParams) || schema.AdditionalProperties || len(schema.Properties) != len(action.RequiredParams) {
			t.Fatalf("incorrect schema: %s", action.ParamsSchema)
		}
		resolved := NewActionURLResolver().Resolve("hero", input.action, "", "")
		if resolved.Method != "POST" || resolved.URL != "/api/v1/characters/hero/actions" {
			t.Fatalf("obsolete resolver: %+v", resolved)
		}
		for _, option := range []Option{WithBlacksmith(nil), func(*Handler) {}} {
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

func TestBlacksmithRetiredPosts(t *testing.T) {
	f := &blacksmithGatewayFixture{gatewayFixture: newGatewayFixture(t)}
	router := f.router(t, timer.NewService(nil))
	for _, suffix := range []string{"seal", "name", "storage/deposit", "storage/withdraw"} {
		r := httptest.NewRequest(http.MethodPost, "/characters/hero/blacksmith/"+suffix, strings.NewReader(`{"seal_id":1,"slot":1}`))
		r.Header.Set("Authorization", "Bearer session")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if (w.Code != 404 && w.Code != 405) || f.executions != 0 || f.queryCalls != 0 || f.characterCalls != 0 {
			t.Fatalf("retired POST reachable: %d %s", w.Code, w.Body.String())
		}
	}
}
