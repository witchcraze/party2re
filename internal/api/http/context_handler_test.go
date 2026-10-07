package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	apihttp "github.com/witchcraze/party2re/internal/api/http"
	"github.com/witchcraze/party2re/internal/character"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	corejob "github.com/witchcraze/party2re/internal/core/job"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/playercontext"
)

type contextReaders struct {
	char       corecharacter.Character
	actions    []scheduling.ScheduledAction
	err        error
	calls      int
	noAvatar   bool
	queryOwner string
	avatarURL  string
}

func (s *contextReaders) FindByID(ctx context.Context, id string) (corecharacter.Character, error) {
	s.calls++
	if id != s.char.ID {
		return corecharacter.Character{}, corecharacter.ErrNotFound
	}
	char := s.char
	if s.queryOwner != "" {
		char.PlayerID = s.queryOwner
	}
	return char, ctx.Err()
}

func (s *contextReaders) FindPendingByActorID(ctx context.Context, id string) ([]scheduling.ScheduledAction, error) {
	if id != s.char.ID {
		return nil, errors.New("wrong actor")
	}
	return s.actions, s.err
}

func contextRouter(t *testing.T, readers *contextReaders, timers timer.Service, profileErr error) http.Handler {
	t.Helper()
	chars := &stubCharacterService{
		getFn: func(ctx context.Context, id string) (corecharacter.Character, error) {
			if id == "missing" {
				return corecharacter.Character{}, corecharacter.ErrNotFound
			}
			if id == "other" {
				return corecharacter.Character{ID: id, PlayerID: "other-owner"}, nil
			}
			return readers.char, nil
		},
		getProfileFn: func(ctx context.Context, id string) (character.ProfileView, error) {
			avatar := "https://example.com/avatar.png"
			if readers.avatarURL != "" {
				avatar = readers.avatarURL
			}
			if readers.noAvatar {
				avatar = ""
			}
			return character.ProfileView{Character: readers.char, Profile: character.Profile{AvatarURL: avatar}}, profileErr
		},
	}
	players := &stubPlayerService{authenticateFn: func(ctx context.Context, session string) (coreplayer.Player, error) {
		if session != "session" {
			return coreplayer.Player{}, errors.New("invalid session")
		}
		return coreplayer.Player{ID: "player-1"}, nil
	}}
	return newTestHandler(t, players, chars, &stubAdventureService{}, &stubShopService{},
		apihttp.WithPlayerContext(playercontext.NewService(readers, readers, timers)),
		apihttp.WithHome(&mockHomeService{}), apihttp.WithRescue(&mockRescueService{}),
		apihttp.WithJob(&stubJobService{listDefinitionsFn: func() []corejob.Definition { return []corejob.Definition{{ID: "job-01", Name: "戦士"}} }})).Router()
}

func fetchContext(t *testing.T, router http.Handler) apihttp.PlayerContextResponse {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/characters/hero/context", nil)
	r.Header.Set("Authorization", "Bearer session")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if len(raw) != 4 || string(raw["ongoing_actions"]) == "null" || string(raw["available_actions"]) == "null" {
		t.Fatalf("expected four slots with arrays: %s", w.Body.String())
	}
	var result apihttp.PlayerContextResponse
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestPlayerContextHTTP_States(t *testing.T) {
	for _, tc := range []struct {
		name            string
		hp, tired, gold int
	}{
		{"healthy", 80, 0, 500}, {"dead", 0, 0, 0}, {"exhausted", 80, 100, 0}, {"no gold", 80, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			readers := &contextReaders{char: corecharacter.Character{ID: "hero", PlayerID: "player-1", Name: "Hero", JobID: "job-01", Level: 15,
				Money: tc.gold, Tired: tc.tired, Stats: corecharacter.Stats{HP: tc.hp, MaxHP: 100, MP: 20, MaxMP: 30}}}
			readers.noAvatar = tc.name == "no gold"
			got := fetchContext(t, contextRouter(t, readers, timer.NewService(nil), nil))
			wantAvatar := "https://example.com/avatar.png"
			if readers.noAvatar {
				wantAvatar = ""
			}
			if got.Character.ID != "hero" || got.Character.HP != tc.hp || got.Character.Gold != tc.gold || got.Character.IsDead != (tc.hp <= 0) || got.Character.IconURL != wantAvatar || got.Character.JobName != "戦士" {
				t.Fatalf("incorrect snapshot: %+v", got.Character)
			}
			if got.Scene.LocationID != "town" || got.Scene.Title == "" || got.Scene.Dialogue == "" || got.Scene.Speaker == nil || !strings.HasPrefix(got.Scene.BgimgURL, "data:image/svg+xml,") {
				t.Fatalf("incomplete town scene: %+v", got.Scene)
			}
			if len(got.OngoingActions) != 0 {
				t.Fatalf("unexpected timers: %+v", got.OngoingActions)
			}
			ids := make([]string, 0, len(got.AvailableActions))
			for _, action := range got.AvailableActions {
				ids = append(ids, action.Action)
				if action.Style == "" || action.Label == "" || action.RequiredParams == nil || slices.Contains(action.RequiredParams, "character_id") {
					t.Fatalf("invalid metadata: %+v", action)
				}
				for _, def := range playercontext.DefaultCatalog {
					if def.ID == action.Action && !reflect.DeepEqual(action.RequiredParams, def.RequiredParams) {
						t.Fatalf("params for %s: %v", def.ID, action.RequiredParams)
					}
				}
			}
			want := playercontext.Evaluate(playercontext.Snapshot{Character: readers.char, LocationID: "town"})
			want = slices.DeleteFunc(want, func(id string) bool {
				return !slices.Contains([]string{"adventure_start", "home_sleep", "home_wake", "rescue_request"}, id)
			})
			if !reflect.DeepEqual(ids, want) {
				t.Fatalf("actions=%v want=%v", ids, want)
			}
			for _, a := range got.AvailableActions {
				if len(a.ParamsSchema) == 0 {
					t.Fatalf("missing strict schema: %+v", a)
				}
			}
		})
	}
}

func TestPlayerContextHTTP_MultipleTimers(t *testing.T) {
	now := time.Now().UTC()
	readers := &contextReaders{char: corecharacter.Character{ID: "hero", PlayerID: "player-1"}, actions: []scheduling.ScheduledAction{
		{ID: "future", ActionType: "activity:training_complete", State: scheduling.StatePending, ExecuteAt: now.Add(time.Hour)},
		{ID: "overdue", ActionType: "alchemy:complete", State: scheduling.StateProcessing, ExecuteAt: now.Add(-time.Hour)},
	}}
	got := fetchContext(t, contextRouter(t, readers, timer.NewService(nil), nil))
	if len(got.OngoingActions) != 2 {
		t.Fatalf("dropped timers: %+v", got.OngoingActions)
	}
	for _, a := range got.OngoingActions {
		if a.Label == "" || a.ActionType == "" {
			t.Fatalf("missing timer metadata: %+v", a)
		}
		if a.ID == "future" && (a.IsReady || a.RemainingSeconds <= 0) {
			t.Fatalf("future timer: %+v", a)
		}
		if a.ID == "overdue" && (!a.IsReady || a.RemainingSeconds != 0) {
			t.Fatalf("overdue timer: %+v", a)
		}
	}
	if len(got.AvailableActions) != 1 || got.AvailableActions[0].Action != "rescue_request" {
		t.Fatalf("unfinished work unblocked actions: %+v", got.AvailableActions)
	}
}

func TestPlayerContextHTTP_SleepLifecycle(t *testing.T) {
	ctx := context.Background()
	readers := &contextReaders{char: corecharacter.Character{ID: "hero", PlayerID: "player-1"}}
	timers := timer.NewService(nil)
	for _, cat := range []string{timer.CategorySleep, timer.CategoryAsleep} {
		if err := timers.SetLock(ctx, cat, "hero", time.Hour); err != nil {
			t.Fatal(err)
		}
	}
	router := contextRouter(t, readers, timers, nil)
	got := fetchContext(t, router)
	if !got.Character.IsSleeping || len(got.OngoingActions) != 1 || got.OngoingActions[0].IsReady || got.OngoingActions[0].RemainingSeconds <= 0 {
		t.Fatalf("sleep: %+v", got)
	}
	if len(got.AvailableActions) != 1 || got.AvailableActions[0].Action != "rescue_request" {
		t.Fatalf("sleep actions: %+v", got.AvailableActions)
	}
	if err := timers.ReleaseLock(ctx, timer.CategorySleep, "hero"); err != nil {
		t.Fatal(err)
	}
	got = fetchContext(t, router)
	if !got.Character.IsSleeping || len(got.OngoingActions) != 1 || !got.OngoingActions[0].IsReady || got.OngoingActions[0].RemainingSeconds != 0 {
		t.Fatalf("wake recovery: %+v", got)
	}
	if len(got.AvailableActions) != 2 || got.AvailableActions[0].Action != "home_wake" {
		t.Fatalf("wake actions: %+v", got.AvailableActions)
	}
	if err := timers.ReleaseLock(ctx, timer.CategoryAsleep, "hero"); err != nil {
		t.Fatal(err)
	}
	if len(fetchContext(t, router).OngoingActions) != 0 {
		t.Fatal("stale sleep timer")
	}
}

func TestPlayerContextHTTP_AuthAndFailures(t *testing.T) {
	for _, tc := range []struct {
		name, id, token string
		err, profileErr error
		status          int
		queryCalls      int
	}{
		{name: "no session", id: "hero", status: 401},
		{name: "invalid session", id: "hero", token: "invalid", status: 401},
		{name: "other owner", id: "other", token: "session", status: 403},
		{name: "missing", id: "missing", token: "session", status: 404},
		{name: "read error", id: "hero", token: "session", err: errors.New("store unavailable"), status: 500, queryCalls: 1},
		{name: "profile error", id: "hero", token: "session", profileErr: errors.New("profile unavailable"), status: 500, queryCalls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			readers := &contextReaders{char: corecharacter.Character{ID: "hero", PlayerID: "player-1"}, err: tc.err}
			router := contextRouter(t, readers, timer.NewService(nil), tc.profileErr)
			r := httptest.NewRequest(http.MethodGet, "/api/v1/characters/"+tc.id+"/context", nil)
			if tc.token != "" {
				r.Header.Set("Authorization", "Bearer "+tc.token)
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, r)
			if w.Code != tc.status || readers.calls != tc.queryCalls {
				t.Fatalf("status=%d calls=%d body=%s", w.Code, readers.calls, w.Body.String())
			}
			if strings.Contains(w.Body.String(), "available_actions") {
				t.Fatal("partial observation returned on failure")
			}
		})
	}
}

type failingContextTimer struct{ timer.Service }

func TestPlayerContextHTTP_PreservesUploadSizedAvatar(t *testing.T) {
	avatar := "data:image/png;base64," + strings.Repeat("A", ((2*1024*1024+2)/3)*4)
	readers := &contextReaders{char: corecharacter.Character{ID: "hero", PlayerID: "player-1"}, avatarURL: avatar}
	got := fetchContext(t, contextRouter(t, readers, timer.NewService(nil), nil))
	if got.Character.IconURL != avatar {
		t.Fatal("avatar was truncated")
	}
}

func (s failingContextTimer) GetRemainingLock(context.Context, string, string) (time.Duration, error) {
	return 0, errors.New("timer unavailable")
}

func TestPlayerContextHTTP_TimerFailureAndChangedOwner(t *testing.T) {
	for _, tc := range []struct {
		name, owner string
		timers      timer.Service
		status      int
	}{
		{"timer failure", "", failingContextTimer{timer.NewService(nil)}, 500},
		{"owner changed after auth read", "other-owner", timer.NewService(nil), 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			readers := &contextReaders{char: corecharacter.Character{ID: "hero", PlayerID: "player-1"}, queryOwner: tc.owner}
			r := httptest.NewRequest(http.MethodGet, "/api/v1/characters/hero/context", nil)
			r.Header.Set("Authorization", "Bearer session")
			w := httptest.NewRecorder()
			contextRouter(t, readers, tc.timers, nil).ServeHTTP(w, r)
			if w.Code != tc.status || strings.Contains(w.Body.String(), "available_actions") {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
		})
	}
}
