package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/bank"
	"github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/playercontext"
)

type bankSceneService struct {
	BankService
	service *bank.Service
	reads   int
	err     error
}

func (s *bankSceneService) GetState(ctx context.Context, actor string) (bank.State, error) {
	s.reads++
	if s.err != nil {
		return bank.State{}, s.err
	}
	return s.service.GetState(ctx, actor)
}

func bankSceneRouter(t *testing.T) (*bankGatewayFixture, *gatewayNavigationStore, *bankSceneService, http.Handler) {
	t.Helper()
	f := newBankGatewayFixture(t)
	store := &gatewayNavigationStore{selection: playercontext.Selection{Destination: "bank"}}
	reads := &bankSceneService{BankService: f, service: f.service}
	pc := playercontext.NewService(f.gatewayFixture, f.gatewayFixture, timer.NewService(nil), playercontext.WithNavigation(store,
		playercontext.SceneDefinition{ID: "town"}, playercontext.SceneDefinition{ID: "bank", Parent: "town"}))
	h, err := NewHandler(f.gatewayFixture, f.gatewayFixture, &struct{ AdventureService }{}, &struct{ ShopService }{},
		WithBank(reads), WithPlayerContext(pc), func(h *Handler) { h.homes = f.gatewayFixture })
	if err != nil {
		t.Fatal(err)
	}
	return f, store, reads, h.Router()
}

func TestBankSceneOwnedStateAndSharedNavigation(t *testing.T) {
	for _, deposit := range []int64{0, 1000, bank.MaxDeposit} {
		t.Run(fmt.Sprint(deposit), func(t *testing.T) {
			f, store, reads, router := bankSceneRouter(t)
			f.char.Deposit = deposit
			status, observation := navigationGET(t, router)
			data := decodeShopScene[struct {
				Parent string `json:"parent"`
				bank.State
			}](t, observation)
			want, err := f.service.GetState(context.Background(), "hero")
			if err != nil {
				t.Fatal(err)
			}
			if status != 200 || observation.Scene.Kind != "facility" || observation.Scene.LocationID != "bank" || observation.Scene.Support.Observation != "details" || data.Parent != "town" || !reflect.DeepEqual(data.State, want) {
				t.Fatalf("Bank projection: %d %+v / %+v", status, observation, want)
			}
			for _, action := range []string{"bank_deposit", "bank_withdraw", "scene_back"} {
				if !slices.ContainsFunc(observation.AvailableActions, func(a ContextAction) bool { return a.Action == action }) {
					t.Fatalf("missing connected choice %s: %+v", action, observation.AvailableActions)
				}
			}
			if status, again := navigationGET(t, router); status != 200 || !reflect.DeepEqual(observation.Scene, again.Scene) || reads.reads != 2 || store.writes != 0 || f.executions != 0 || f.char.Deposit != deposit || f.char.Money != 100 || len(f.actions) != 0 {
				t.Fatalf("GET mutated or randomized state: %d %+v", status, again)
			}
			store.selection.Destination = "town"
			status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"scene_enter","params":{"destination":"bank"}}`, nil)
			var refreshed PlayerContextResponse
			if err := json.Unmarshal(raw["context"], &refreshed); err != nil {
				t.Fatal(err)
			}
			if status != 200 || string(raw["success"]) != "true" || !reflect.DeepEqual(refreshed.Scene, observation.Scene) || store.writes != 1 || f.executions != 0 {
				t.Fatalf("navigation refresh differs: %d %s", status, raw)
			}
		})
	}
}

func TestBankSceneCommandRefreshAndGETOnlyRecovery(t *testing.T) {
	for _, tc := range []struct {
		action, params, code string
		status, money        int
		deposit              int64
	}{
		{"bank_deposit", `{"amount":40}`, "", 200, 60, 1040},
		{"bank_withdraw", `{"amount":40}`, "", 200, 140, 960},
		{"bank_deposit", `{"amount":101}`, "BANK_INSUFFICIENT_FUNDS", 400, 100, 1000},
	} {
		for _, refreshFails := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%s/refreshFails=%t", tc.action, tc.code, refreshFails), func(t *testing.T) {
				f, store, reads, router := bankSceneRouter(t)
				if refreshFails {
					f.afterExecute = func() { reads.err = errors.New("private Bank read failure") }
				}
				status, raw := f.request(router, tc.action, tc.params)
				if status != tc.status || f.char.Money != tc.money || f.char.Deposit != tc.deposit || store.writes != 0 || f.executions != 1 {
					t.Fatalf("command: %d %s %+v", status, raw, f.char)
				}
				if tc.code != "" {
					assertGatewayError(t, raw, tc.code)
				} else if string(raw["success"]) != "true" || raw["result"] == nil {
					t.Fatalf("lost known success: %s", raw)
				}
				if refreshFails {
					var detail ErrorDetail
					if err := json.Unmarshal(raw["context_error"], &detail); err != nil || detail.Code != "CONTEXT_REFRESH_FAILED" || string(raw["context"]) != "null" {
						t.Fatalf("lost refresh failure: %s %v", raw, err)
					}
				} else {
					var refreshed PlayerContextResponse
					if err := json.Unmarshal(raw["context"], &refreshed); err != nil {
						t.Fatal(err)
					}
					data := decodeShopScene[bank.State](t, refreshed)
					if data.Money != tc.money || data.Deposit != tc.deposit {
						t.Fatalf("stale command refresh: %+v", refreshed)
					}
				}
				reads.err = nil
				status, observation := navigationGET(t, router)
				data := decodeShopScene[bank.State](t, observation)
				if status != 200 || data.Money != tc.money || data.Deposit != tc.deposit || f.executions != 1 || store.writes != 0 {
					t.Fatalf("GET recovery replayed or stale: %d %+v", status, observation)
				}
			})
		}
	}
}

func TestBankSceneRequiredReadOwnershipAndContext(t *testing.T) {
	f, store, reads, router := bankSceneRouter(t)
	reads.err = errors.New("private Bank read failure")
	if status, _ := navigationGET(t, router); status != 500 || store.writes != 0 || f.executions != 0 {
		t.Fatalf("partial required read: %d", status)
	}
	reads.err = nil
	ctx := context.WithValue(context.Background(), struct{}{}, "Bank observation")
	f.expectedContext = ctx
	r := httptest.NewRequest(http.MethodGet, "/api/v1/characters/hero/context", nil).WithContext(ctx)
	r.Header.Set("Authorization", "Bearer session")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("request context: %d %s", w.Code, w.Body.String())
	}
	f.expectedContext = nil
	before := reads.reads
	for _, tc := range []struct {
		actor, token string
		status       int
	}{{"hero", "", 401}, {"other", "session", 403}, {"missing", "session", 404}} {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/characters/"+tc.actor+"/context", nil)
		r.Header.Set("Authorization", "Bearer "+tc.token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != tc.status || reads.reads != before || store.writes != 0 || f.executions != 0 {
			t.Fatalf("unauthorized Bank read: %d %s", w.Code, w.Body.String())
		}
	}
}

func TestBankSceneActualActivitySkipsBankRead(t *testing.T) {
	for _, kind := range []string{"sleep", "work", "party"} {
		t.Run(kind, func(t *testing.T) {
			f := newBankGatewayFixture(t)
			store := &gatewayNavigationStore{selection: playercontext.Selection{Destination: "bank"}}
			reads := &bankSceneService{BankService: f, service: f.service, err: errors.New("Bank must not be read")}
			timers := timer.NewService(nil)
			activities := []playercontext.Activity{}
			switch kind {
			case "sleep":
				if err := timers.SetLock(context.Background(), timer.CategoryAsleep, "hero", time.Hour); err != nil {
					t.Fatal(err)
				}
			case "work":
				f.actions = []scheduling.ScheduledAction{{ID: "work", ActorID: "hero", State: scheduling.StateProcessing, ExecuteAt: time.Now().Add(time.Hour)}}
			case "party":
				activities = []playercontext.Activity{{Kind: "party", ID: "room", Role: "member", Phase: "in_progress", Actions: []string{}}}
			}
			pc := playercontext.NewService(f.gatewayFixture, f.gatewayFixture, timers,
				playercontext.WithNavigation(store, playercontext.SceneDefinition{ID: "town"}, playercontext.SceneDefinition{ID: "bank", Parent: "town"}),
				playercontext.WithActivities(func(context.Context, string, string) ([]playercontext.Activity, error) { return activities, nil }))
			h, err := NewHandler(f.gatewayFixture, f.gatewayFixture, &struct{ AdventureService }{}, &struct{ ShopService }{}, WithBank(reads), WithPlayerContext(pc))
			if err != nil {
				t.Fatal(err)
			}
			if status, observation := navigationGET(t, h.Router()); status != 200 || observation.Scene.Kind != "activity" || reads.reads != 0 || store.writes != 0 || f.executions != 0 || f.char.Money != 100 || f.char.Deposit != 1000 {
				t.Fatalf("saved Bank displaced activity: %d %+v", status, observation)
			}
		})
	}
}
