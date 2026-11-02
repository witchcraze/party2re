package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/chapel"
	"github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/playercontext"
)

type chapelGatewayFixture struct {
	*gatewayFixture
	ChapelService
	blessing               chapel.CharacterBlessing
	blessingErr, statusErr error
}

func (f *chapelGatewayFixture) GetBlessing(ctx context.Context, actor string) (chapel.CharacterBlessing, error) {
	f.checkContext(ctx)
	if actor != f.char.ID {
		f.t.Fatal("wrong Chapel reader")
	}
	return f.blessing, f.blessingErr
}

func (f *chapelGatewayFixture) GetStatus(ctx context.Context, actor, name string) (chapel.ChapelStatus, error) {
	f.checkContext(ctx)
	if actor != f.char.ID || name != f.char.Name {
		f.t.Fatal("wrong Chapel status actor")
	}
	return chapel.ChapelStatus{AvailableBlessings: chapel.AvailableBlessings, ActiveBlessing: f.blessing.ActiveBlessing}, f.statusErr
}

func (f *chapelGatewayFixture) SelectBlessing(ctx context.Context, actor string, kind chapel.BlessingType) (chapel.CharacterBlessing, error) {
	f.checkContext(ctx)
	if actor != f.char.ID {
		f.t.Fatal("wrong Chapel mutation actor")
	}
	f.executions++
	if f.executionErr == nil {
		f.blessing.ActiveBlessing, f.blessing.PrayedAt = kind, time.Now().UTC()
	}
	if f.afterExecute != nil {
		f.afterExecute()
	}
	return f.blessing, f.executionErr
}

func chapelGatewayRouter(t *testing.T, configured bool) (*chapelGatewayFixture, *gatewayNavigationStore, timer.Service, http.Handler) {
	t.Helper()
	f := &chapelGatewayFixture{gatewayFixture: newGatewayFixture(t), blessing: chapel.CharacterBlessing{CharacterID: "hero", ActiveBlessing: chapel.BlessingNone}}
	f.char.Money = 0
	f.char.Name = "Hero"
	store := &gatewayNavigationStore{selection: playercontext.Selection{Destination: "chapel"}}
	timers := timer.NewService(nil)
	pc := playercontext.NewService(f.gatewayFixture, f.gatewayFixture, timers, playercontext.WithNavigation(store,
		playercontext.SceneDefinition{ID: "town"}, playercontext.SceneDefinition{ID: "chapel", Parent: "town"}, playercontext.SceneDefinition{ID: "bank", Parent: "town"}))
	var service ChapelService
	if configured {
		service = f
	}
	h, err := NewHandler(f.gatewayFixture, f.gatewayFixture, &struct{ AdventureService }{}, &struct{ ShopService }{},
		WithChapel(service), WithPlayerContext(pc), func(h *Handler) { h.homes = f.gatewayFixture })
	if err != nil {
		t.Fatal(err)
	}
	return f, store, timers, h.Router()
}

func TestChapelGatewayObservationAndPrayer(t *testing.T) {
	messages := map[chapel.BlessingType]string{
		chapel.BlessingGold:    "Heroは「お金がほしい」と祈るのですね…",
		chapel.BlessingExp:     "Heroは「強くなりたい」と祈るのですね…",
		chapel.BlessingMonster: "Heroは「モンスターと仲良くしたい」と祈るのですね…",
		chapel.BlessingDrop:    "Heroは「宝箱がほしい」と祈るのですね…",
		chapel.BlessingCasino:  "Heroは「コインがほしい」と祈るのですね…",
	}
	for _, input := range []string{"GOLD", "EXP", "MONSTER", "DROP", "CASINO", "1", "2", "3", "4", "5", "お金がほしい", "強くなりたい", "モンスターと仲良くしたい", "宝箱がほしい", "コインがほしい", "gold_wish", "exp_wish", "monster_wish", "treasure_wish", "casino_wish", " gold "} {
		t.Run(input, func(t *testing.T) {
			f, store, _, router := chapelGatewayRouter(t, true)
			status, observed := navigationGET(t, router)
			data := decodeShopScene[ChapelSceneData](t, observed)
			if status != 200 || data.Parent != "town" || data.Blessing.CharacterID != "hero" || len(data.Status.AvailableBlessings) != 5 || observed.Scene.Support.Observation != "details" {
				t.Fatalf("missing owned Chapel: %d %+v", status, observed)
			}
			idx := slices.IndexFunc(observed.AvailableActions, func(a ContextAction) bool { return a.Action == "chapel_pray" })
			if idx < 0 || !slices.Equal(observed.AvailableActions[idx].RequiredParams, []string{"blessing"}) || !strings.Contains(string(observed.AvailableActions[idx].ParamsSchema), `"additionalProperties":false`) {
				t.Fatalf("missing strict free prayer choice: %+v", observed.AvailableActions)
			}
			for i, info := range data.Status.AvailableBlessings {
				if info.Type != chapel.AvailableBlessings[i].Type {
					t.Fatalf("lost explicit blessing value: %+v", info)
				}
			}
			f.expectedContext = context.WithValue(context.Background(), struct{}{}, "prayer")
			body, err := json.Marshal(map[string]any{"action": "chapel_pray", "params": map[string]string{"blessing": input}})
			if err != nil {
				t.Fatal(err)
			}
			status, got := gatewayRequest(t, router, "hero", "session", "application/json", string(body), f.expectedContext)
			want, err := chapel.ParseBlessingType(input)
			if err != nil {
				t.Fatal(err)
			}
			var result chapelBlessingResponse
			if err := json.Unmarshal(got["result"], &result); err != nil {
				t.Fatal(err)
			}
			if status != 200 || f.executions != 1 || result.Blessing.ActiveBlessing != want || result.Blessing.CharacterID != "hero" || result.Message != messages[want] || f.char.Money != 0 || store.writes != 0 {
				t.Fatalf("incorrect free prayer: %d %s", status, got)
			}
			assertGatewayContext(t, got)
		})
	}
}

func TestChapelGatewayNavigationDoesNotPray(t *testing.T) {
	f, store, _, router := chapelGatewayRouter(t, true)
	store.selection.Destination = "town"
	status, observed := navigationGET(t, router)
	town := decodeShopScene[TownSceneData](t, observed)
	if status != 200 || !slices.ContainsFunc(town.Destinations, func(d SceneDestination) bool { return d.ID == "chapel" && d.Supported }) {
		t.Fatalf("Chapel is not discoverable from town: %d %+v", status, observed)
	}
	status, got := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"scene_enter","params":{"destination":"chapel"}}`, nil)
	if status != 200 || store.selection.Destination != "chapel" || store.writes != 1 || f.executions != 0 || f.blessing.ActiveBlessing != chapel.BlessingNone {
		t.Fatalf("navigation prayed: %d %s", status, got)
	}
	assertGatewayContext(t, got)
	status, got = gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"scene_back"}`, nil)
	if status != 200 || store.selection.Destination != "town" || store.writes != 2 || f.executions != 0 {
		t.Fatalf("back changed gameplay: %d %s", status, got)
	}
	if _, exists := defaultActionEndpoints()["chapel_bless"]; exists {
		t.Fatal("nonexistent bless route is still advertised")
	}
}

func TestChapelGatewayInputsAndGuards(t *testing.T) {
	for _, params := range []string{"", `null`, `[]`, `{}`, `{"blessing":null}`, `{"blessing":1}`, `{"blessing":"GOLD","character_id":"other"}`, `{"blessing":"GOLD","fee":0}`} {
		t.Run(params, func(t *testing.T) {
			f, _, _, router := chapelGatewayRouter(t, true)
			body := `{"action":"chapel_pray"`
			if params != "" {
				body += `,"params":` + params
			}
			status, got := gatewayRequest(t, router, "hero", "session", "application/json", body+`}`, nil)
			if status != 400 || f.executions != 0 || f.queryCalls != 0 {
				t.Fatalf("invalid params reached service: %d %s", status, got)
			}
			assertGatewayError(t, got, "INVALID_ACTION_PARAMS")
		})
	}
	for _, value := range []string{"NONE", "0", "", "revive"} {
		f, _, _, router := chapelGatewayRouter(t, true)
		status, got := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"chapel_pray","params":{"blessing":"`+value+`"}}`, nil)
		if status != 400 || f.executions != 0 {
			t.Fatalf("invalid blessing executed: %d %s", status, got)
		}
		assertGatewayError(t, got, "CHAPEL_INVALID_BLESSING")
	}
	for _, condition := range []string{"anonymous", "foreign", "wrong location", "sleep", "wake", "work", "unconfigured", "shared sleep", "shared wake", "sleep read error"} {
		t.Run(condition, func(t *testing.T) {
			f, store, timers, router := chapelGatewayRouter(t, condition != "unconfigured")
			actor, token, want := "hero", "session", 409
			switch condition {
			case "anonymous":
				token, want = "", 401
			case "foreign":
				actor, want = "other", 403
			case "wrong location":
				store.selection.Destination = "bank"
			case "sleep":
				if err := timers.SetLock(context.Background(), timer.CategorySleep, "hero", time.Hour); err != nil {
					t.Fatal(err)
				}
			case "wake":
				f.char.PendingWake = true
			case "work":
				f.actions = []scheduling.ScheduledAction{{ID: "work", State: scheduling.StateProcessing, ExecuteAt: time.Now().Add(-time.Hour)}}
			case "unconfigured":
				want = 501
			case "shared sleep":
				f.sleep.Sleeping = true
			case "shared wake":
				f.sleep.CanWake = true
			case "sleep read error":
				f.sleepErr, want = errors.New("sleep read failed"), 500
			}
			status, got := gatewayRequest(t, router, actor, token, "application/json", `{"action":"chapel_pray","params":{"blessing":"GOLD"}}`, nil)
			if status != want || f.executions != 0 || store.writes != 0 {
				t.Fatalf("guard bypass: %d %s", status, got)
			}
		})
	}
}

func TestChapelGatewayOutcomeAndReadRecovery(t *testing.T) {
	for _, rejected := range []bool{false, true} {
		for _, failure := range []string{"", "query", "blessing", "status"} {
			t.Run(fmt.Sprintf("%t/%s", rejected, failure), func(t *testing.T) {
				f, store, _, router := chapelGatewayRouter(t, true)
				want := 200
				if rejected {
					f.executionErr, want = fmt.Errorf("wrapped: %w", chapel.ErrAlreadyPrayed), 409
				}
				f.afterExecute = func() {
					f.char.Name = "Renamed after prayer"
					switch failure {
					case "query":
						f.queryErr = errors.New("private query")
					case "blessing":
						f.blessingErr = errors.New("private blessing")
					case "status":
						f.statusErr = errors.New("private status")
					}
				}
				status, got := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"chapel_pray","params":{"blessing":"DROP"}}`, nil)
				if status != want || f.executions != 1 {
					t.Fatalf("lost outcome: %d %s", status, got)
				}
				if rejected {
					assertGatewayError(t, got, "CHAPEL_ALREADY_PRAYED")
				} else if string(got["success"]) != "true" || !strings.Contains(string(got["result"]), `"active_blessing":"DROP"`) {
					t.Fatalf("lost blessing: %s", got)
				}
				if !rejected {
					var result chapelBlessingResponse
					if err := json.Unmarshal(got["result"], &result); err != nil {
						t.Fatal(err)
					}
					if result.Message != "Heroは「宝箱がほしい」と祈るのですね…" || f.characterCalls != 2 {
						t.Fatalf("lost confirmation or read name after prayer: %+v reads=%d", result, f.characterCalls)
					}
				}
				if failure == "" {
					assertGatewayContext(t, got)
				} else {
					assertLoopRefreshFailure(t, got)
				}
				f.queryErr, f.blessingErr, f.statusErr = nil, nil, nil
				status, observed := navigationGET(t, router)
				data := decodeShopScene[ChapelSceneData](t, observed)
				if status != 200 || f.executions != 1 || store.writes != 0 || data.Blessing.ActiveBlessing != f.blessing.ActiveBlessing {
					t.Fatalf("GET recovery: %d %+v", status, observed)
				}
			})
		}
	}
	for _, source := range []string{"blessing", "status"} {
		f, store, _, router := chapelGatewayRouter(t, true)
		if source == "blessing" {
			f.blessingErr = errors.New("private blessing")
		} else {
			f.statusErr = errors.New("private status")
		}
		status, observed := navigationGET(t, router)
		if status != 500 || observed.Scene.Data != nil || f.executions != 0 || store.writes != 0 {
			t.Fatalf("partial scene on read error: %d %+v", status, observed)
		}
	}
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{
		{chapel.ErrInvalidBlessing, 400, "CHAPEL_INVALID_BLESSING"},
		{chapel.ErrInvalidCharacterID, 400, "CHAPEL_INVALID_CHARACTER_ID"},
		{errors.New("private write failure"), 500, "EXECUTION_FAILED"},
		{context.Canceled, 500, "EXECUTION_FAILED"},
	} {
		f, _, _, router := chapelGatewayRouter(t, true)
		f.executionErr = fmt.Errorf("wrapped: %w", tc.err)
		status, got := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"chapel_pray","params":{"blessing":"GOLD"}}`, nil)
		if status != tc.status || f.executions != 1 {
			t.Fatalf("error mapping: %d %s", status, got)
		}
		assertGatewayError(t, got, tc.code)
		if status == 500 && (got["context"] != nil || strings.Contains(string(got["error"]), "private")) {
			t.Fatalf("unknown outcome refreshed/leaked: %s", got)
		}
	}
}
