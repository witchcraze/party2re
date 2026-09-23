package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	apihttp "github.com/witchcraze/party2re/internal/api/http"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/pagination"
	"github.com/witchcraze/party2re/internal/replay"
)

type stubReplayService struct {
	getReplayFn                   func(ctx context.Context, id string) (*replay.BattleReplay, error)
	getCharacterHistoryFn         func(ctx context.Context, characterID string, combatType string, limit int) ([]replay.ReplayHeader, error)
	getCharacterHistoryByCursorFn func(ctx context.Context, characterID string, combatType string, limit int, cursor string) (pagination.CursorPage[replay.ReplayHeader], error)
	getRecentReplaysFn            func(ctx context.Context, combatType string, limit int) ([]replay.ReplayHeader, error)
	getRecentReplaysByCursorFn    func(ctx context.Context, combatType string, limit int, cursor string) (pagination.CursorPage[replay.ReplayHeader], error)
}

func (s *stubReplayService) GetReplay(ctx context.Context, id string) (*replay.BattleReplay, error) {
	if s.getReplayFn != nil {
		return s.getReplayFn(ctx, id)
	}
	return nil, replay.ErrReplayNotFound
}

func (s *stubReplayService) GetCharacterHistory(ctx context.Context, characterID string, combatType string, limit int) ([]replay.ReplayHeader, error) {
	if s.getCharacterHistoryFn != nil {
		return s.getCharacterHistoryFn(ctx, characterID, combatType, limit)
	}
	return nil, nil
}

func (s *stubReplayService) GetCharacterHistoryByCursor(ctx context.Context, characterID string, combatType string, limit int, cursor string) (pagination.CursorPage[replay.ReplayHeader], error) {
	if s.getCharacterHistoryByCursorFn != nil {
		return s.getCharacterHistoryByCursorFn(ctx, characterID, combatType, limit, cursor)
	}
	return pagination.CursorPage[replay.ReplayHeader]{}, nil
}

func (s *stubReplayService) GetRecentReplays(ctx context.Context, combatType string, limit int) ([]replay.ReplayHeader, error) {
	if s.getRecentReplaysFn != nil {
		return s.getRecentReplaysFn(ctx, combatType, limit)
	}
	return nil, nil
}

func (s *stubReplayService) GetRecentReplaysByCursor(ctx context.Context, combatType string, limit int, cursor string) (pagination.CursorPage[replay.ReplayHeader], error) {
	if s.getRecentReplaysByCursorFn != nil {
		return s.getRecentReplaysByCursorFn(ctx, combatType, limit, cursor)
	}
	return pagination.CursorPage[replay.ReplayHeader]{}, nil
}

func TestReplayEndpoints(t *testing.T) {
	player := coreplayer.Player{ID: "p1", Username: "hero"}
	char := corecharacter.Character{ID: "c1", PlayerID: "p1", Name: "Hero"}

	pService := &stubPlayerService{
		authenticateFn: alwaysAuthPlayer(player),
	}
	cService := &stubCharacterService{
		getFn: func(_ context.Context, id string) (corecharacter.Character, error) {
			if id == "c1" {
				return char, nil
			}
			return corecharacter.Character{}, corecharacter.ErrNotFound
		},
	}

	testReplay := replay.BattleReplay{
		ID:            "rep-123",
		CombatType:    replay.CombatTypePvP,
		InitiatorID:   "c1",
		InitiatorName: "Hero",
		OpponentID:    "c2",
		OpponentName:  "Rival",
		Outcome:       corebattle.OutcomeWin,
		WinnerID:      "c1",
		LoserID:       "c2",
		TotalTurns:    3,
		InitialParticipants: []replay.ParticipantSnapshot{
			{ID: "c1", Name: "Hero", MaxHP: 100, Attack: 20, Defense: 10},
			{ID: "c2", Name: "Rival", MaxHP: 100, Attack: 18, Defense: 8},
		},
		TurnLogs: []corebattle.TurnLog{
			{Turn: 1, ActorID: "c1", TargetID: "c2", ActionName: "こうげき", DamageDealt: 40},
		},
		CreatedAt: time.Now().UTC(),
	}

	testHeaders := []replay.ReplayHeader{
		{
			ID:            "rep-123",
			CombatType:    replay.CombatTypePvP,
			InitiatorID:   "c1",
			InitiatorName: "Hero",
			OpponentID:    "c2",
			OpponentName:  "Rival",
			Outcome:       corebattle.OutcomeWin,
			WinnerID:      "c1",
			TotalTurns:    3,
			CreatedAt:     time.Now().UTC(),
		},
	}

	rService := &stubReplayService{
		getReplayFn: func(_ context.Context, id string) (*replay.BattleReplay, error) {
			if id == "rep-123" {
				return &testReplay, nil
			}
			if id == "err-500" {
				return nil, errors.New("db failure")
			}
			return nil, replay.ErrReplayNotFound
		},
		getCharacterHistoryFn: func(_ context.Context, characterID string, combatType string, limit int) ([]replay.ReplayHeader, error) {
			if characterID == "c1" {
				if combatType == "dungeon" {
					return []replay.ReplayHeader{}, nil
				}
				return testHeaders, nil
			}
			return nil, errors.New("unknown character")
		},
		getCharacterHistoryByCursorFn: func(_ context.Context, characterID string, combatType string, limit int, cursor string) (pagination.CursorPage[replay.ReplayHeader], error) {
			return pagination.NewCursorPage(testHeaders, "next-tok", cursor, limit, false), nil
		},
		getRecentReplaysFn: func(_ context.Context, combatType string, limit int) ([]replay.ReplayHeader, error) {
			if combatType == "dungeon" {
				return []replay.ReplayHeader{}, nil
			}
			return testHeaders, nil
		},
		getRecentReplaysByCursorFn: func(_ context.Context, combatType string, limit int, cursor string) (pagination.CursorPage[replay.ReplayHeader], error) {
			return pagination.NewCursorPage(testHeaders, "recent-next", cursor, limit, false), nil
		},
	}

	h := newTestHandler(
		t,
		pService,
		cService,
		&stubAdventureService{},
		&stubShopService{},
		apihttp.WithReplay(rService),
	)
	router := h.Router()

	t.Run("GET /replays/{id} - 200 OK", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/replays/rep-123", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}

		var result replay.BattleReplay
		if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if result.ID != "rep-123" || result.CombatType != "pvp" {
			t.Errorf("unexpected replay data: %+v", result)
		}
	})

	t.Run("GET /replays/{id} - 404 Not Found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/replays/nonexistent", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 Not Found, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("GET /replays/{id} - 500 Internal Server Error", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/replays/err-500", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500 Internal Server Error, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("GET /characters/{id}/replays - 200 OK default list", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/characters/c1/replays", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}

		var headers []replay.ReplayHeader
		if err := json.Unmarshal(rec.Body.Bytes(), &headers); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if len(headers) != 1 || headers[0].ID != "rep-123" {
			t.Errorf("unexpected headers: %+v", headers)
		}
	})

	t.Run("GET /characters/{id}/replays - 200 OK with combat_type filter", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/characters/c1/replays?combat_type=dungeon", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}

		var headers []replay.ReplayHeader
		if err := json.Unmarshal(rec.Body.Bytes(), &headers); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if len(headers) != 0 {
			t.Errorf("expected 0 headers, got %d", len(headers))
		}
	})

	t.Run("GET /characters/{id}/replays - 200 OK with cursor pagination", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/characters/c1/replays?cursor=prev-tok&limit=10", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}

		var page pagination.CursorPage[replay.ReplayHeader]
		if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if len(page.Items) != 1 || page.NextCursor != "next-tok" {
			t.Errorf("unexpected page: %+v", page)
		}
	})

	t.Run("GET /characters/{id}/replays - 401 Unauthorized without auth", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/characters/c1/replays", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 Unauthorized, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("GET /replays/recent - 200 OK default list", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/replays/recent", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}

		var headers []replay.ReplayHeader
		if err := json.Unmarshal(rec.Body.Bytes(), &headers); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if len(headers) != 1 || headers[0].ID != "rep-123" {
			t.Errorf("unexpected headers: %+v", headers)
		}
	})

	t.Run("GET /replays/recent - 200 OK with combat_type filter", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/replays/recent?combat_type=dungeon", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}

		var headers []replay.ReplayHeader
		if err := json.Unmarshal(rec.Body.Bytes(), &headers); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if len(headers) != 0 {
			t.Errorf("expected 0 headers, got %d", len(headers))
		}
	})

	t.Run("GET /replays/recent - 200 OK with cursor pagination", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/replays/recent?cursor=recent-cur&limit=5", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}

		var page pagination.CursorPage[replay.ReplayHeader]
		if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if len(page.Items) != 1 || page.NextCursor != "recent-next" {
			t.Errorf("unexpected page: %+v", page)
		}
	})

	t.Run("501 Not Implemented when service is unconfigured", func(t *testing.T) {
		hUnconfigured := newTestHandler(
			t,
			pService,
			cService,
			&stubAdventureService{},
			&stubShopService{},
		)
		unconfiguredRouter := hUnconfigured.Router()

		// GET /replays/{id}
		req := httptest.NewRequest(http.MethodGet, "/replays/rep-123", nil)
		rec := httptest.NewRecorder()
		unconfiguredRouter.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotImplemented {
			t.Errorf("expected 501, got %d", rec.Code)
		}

		// GET /replays/recent
		req = httptest.NewRequest(http.MethodGet, "/replays/recent", nil)
		rec = httptest.NewRecorder()
		unconfiguredRouter.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotImplemented {
			t.Errorf("expected 501, got %d", rec.Code)
		}

		// GET /characters/{id}/replays
		req = httptest.NewRequest(http.MethodGet, "/characters/c1/replays", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec = httptest.NewRecorder()
		unconfiguredRouter.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotImplemented {
			t.Errorf("expected 501, got %d", rec.Code)
		}
	})
}
