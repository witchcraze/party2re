package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	apihttp "github.com/witchcraze/party2re/internal/api/http"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/home"
)

func TestHomeSleepEndpoints(t *testing.T) {
	player := coreplayer.Player{ID: "player-1", Username: "user1"}
	char := corecharacter.Character{
		ID:       "char-1",
		PlayerID: "player-1",
		Name:     "Hero",
		Stats: corecharacter.Stats{
			HP: 10, MaxHP: 100,
			MP: 5, MaxMP: 50,
		},
		Tired: 80,
	}

	players := &stubPlayerService{
		authenticateFn: func(ctx context.Context, sessionID string) (coreplayer.Player, error) {
			if sessionID == "valid-session" {
				return player, nil
			}
			return coreplayer.Player{}, errorsNew("invalid session")
		},
	}
	characters := &stubCharacterService{
		getFn: func(ctx context.Context, id string) (corecharacter.Character, error) {
			if id == "char-1" {
				return char, nil
			}
			return corecharacter.Character{}, corecharacter.ErrNotFound
		},
	}

	t.Run("POST /characters/{id}/home/sleep - unauthorized", func(t *testing.T) {
		mockHome := &mockHomeService{}
		h := newTestHandler(t, players, characters, &stubAdventureService{}, &stubShopService{}, apihttp.WithHome(mockHome))
		router := h.Router()

		req := httptest.NewRequest(http.MethodPost, "/characters/char-1/home/sleep", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 Unauthorized, got %d", rec.Code)
		}
	})

	t.Run("POST /characters/{id}/home/sleep - success", func(t *testing.T) {
		mockHome := &mockHomeService{
			sleepFn: func(ctx context.Context, characterID, targetHomeID string) (home.SleepResult, error) {
				return home.SleepResult{
					Sleeping:         true,
					DurationSeconds:  60,
					RemainingSeconds: 60,
					HomeCharacterID:  characterID,
					Message:          "ベッドにもぐりこんだ！",
				}, nil
			},
		}
		h := newTestHandler(t, players, characters, &stubAdventureService{}, &stubShopService{}, apihttp.WithHome(mockHome))
		router := h.Router()

		req := httptest.NewRequest(http.MethodPost, "/characters/char-1/home/sleep", bytes.NewReader([]byte("{}")))
		req.Header.Set("Authorization", "Bearer valid-session")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}

		var resp map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp["sleeping"] != true || resp["duration_seconds"].(float64) != 60 {
			t.Fatalf("unexpected response: %+v", resp)
		}
	})

	t.Run("POST /characters/{id}/home/sleep - conflict when already sleeping", func(t *testing.T) {
		mockHome := &mockHomeService{
			sleepFn: func(ctx context.Context, characterID, targetHomeID string) (home.SleepResult, error) {
				return home.SleepResult{}, home.ErrAlreadySleeping
			},
		}
		h := newTestHandler(t, players, characters, &stubAdventureService{}, &stubShopService{}, apihttp.WithHome(mockHome))
		router := h.Router()

		req := httptest.NewRequest(http.MethodPost, "/characters/char-1/home/sleep", nil)
		req.Header.Set("Authorization", "Bearer valid-session")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusConflict {
			t.Fatalf("expected 409 Conflict, got %d", rec.Code)
		}
	})

	t.Run("POST /characters/{id}/home/sleep - house not found for homeless or expired target", func(t *testing.T) {
		mockHome := &mockHomeService{
			sleepFn: func(ctx context.Context, characterID, targetHomeID string) (home.SleepResult, error) {
				return home.SleepResult{}, home.ErrHouseNotFound
			},
		}
		h := newTestHandler(t, players, characters, &stubAdventureService{}, &stubShopService{}, apihttp.WithHome(mockHome))
		router := h.Router()

		req := httptest.NewRequest(http.MethodPost, "/characters/char-1/home/sleep", bytes.NewReader([]byte(`{"target_home_id":"c2"}`)))
		req.Header.Set("Authorization", "Bearer valid-session")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 Not Found, got %d", rec.Code)
		}
	})

	t.Run("GET /characters/{id}/home/sleep - status", func(t *testing.T) {
		mockHome := &mockHomeService{
			getSleepStatusFn: func(ctx context.Context, characterID string) (home.SleepStatus, error) {
				return home.SleepStatus{
					Sleeping:         true,
					RemainingSeconds: 45,
					CanWake:          false,
					Message:          "お休み中「Zzz...」 目覚めるまで 0分45秒",
				}, nil
			},
		}
		h := newTestHandler(t, players, characters, &stubAdventureService{}, &stubShopService{}, apihttp.WithHome(mockHome))
		router := h.Router()

		req := httptest.NewRequest(http.MethodGet, "/characters/char-1/home/sleep", nil)
		req.Header.Set("Authorization", "Bearer valid-session")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}

		var resp map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp["sleeping"] != true || resp["remaining_seconds"].(float64) != 45 {
			t.Fatalf("unexpected response: %+v", resp)
		}
	})

	t.Run("POST /characters/{id}/home/wake - conflict when still sleeping", func(t *testing.T) {
		mockHome := &mockHomeService{
			wakeFn: func(ctx context.Context, characterID string) (home.WakeResult, error) {
				return home.WakeResult{}, home.ErrStillSleeping
			},
		}
		h := newTestHandler(t, players, characters, &stubAdventureService{}, &stubShopService{}, apihttp.WithHome(mockHome))
		router := h.Router()

		req := httptest.NewRequest(http.MethodPost, "/characters/char-1/home/wake", nil)
		req.Header.Set("Authorization", "Bearer valid-session")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusConflict {
			t.Fatalf("expected 409 Conflict, got %d", rec.Code)
		}
	})

	t.Run("POST /characters/{id}/home/wake - success", func(t *testing.T) {
		awakenedChar := char
		awakenedChar.Stats.HP = 100
		awakenedChar.Stats.MP = 50
		awakenedChar.Tired = 0

		mockHome := &mockHomeService{
			wakeFn: func(ctx context.Context, characterID string) (home.WakeResult, error) {
				return home.WakeResult{
					Success:   true,
					Message:   "HeroのHP・MP・疲労度が回復した！",
					Character: awakenedChar,
				}, nil
			},
		}
		h := newTestHandler(t, players, characters, &stubAdventureService{}, &stubShopService{}, apihttp.WithHome(mockHome))
		router := h.Router()

		req := httptest.NewRequest(http.MethodPost, "/characters/char-1/home/wake", nil)
		req.Header.Set("Authorization", "Bearer valid-session")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}

		var resp map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp["success"] != true {
			t.Fatalf("expected success true, got %+v", resp)
		}
	})
}

func errorsNew(s string) error {
	return &testErr{s}
}

type testErr struct{ msg string }

func (e *testErr) Error() string { return e.msg }
