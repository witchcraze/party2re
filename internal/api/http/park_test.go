package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	apihttp "github.com/witchcraze/party2re/internal/api/http"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/pagination"
	"github.com/witchcraze/party2re/internal/park"
)

type mockParkService struct {
	postFn             func(ctx context.Context, charID, content, color, recipient string) (park.Post, error)
	getPostsFn         func(ctx context.Context, limit, offset int) ([]park.Post, int, error)
	getPostsByCursorFn func(ctx context.Context, limit int, cursor string) (pagination.CursorPage[park.Post], error)
	talkFn             func(ctx context.Context, charID string) (string, error)
	divinateFn         func(ctx context.Context, charID string) (park.DivinationResult, error)
	inspectLineFn      func() string
}

func (m *mockParkService) PostMessage(ctx context.Context, charID, content, color, recipient string) (park.Post, error) {
	if m.postFn != nil {
		return m.postFn(ctx, charID, content, color, recipient)
	}
	return park.Post{
		ID:            "post-1",
		CharacterID:   charID,
		CharacterName: "TestHero",
		Content:       content,
		Color:         color,
		RecipientName: recipient,
		CreatedAt:     time.Now().UTC(),
	}, nil
}

func (m *mockParkService) GetRecentPosts(ctx context.Context, limit, offset int) ([]park.Post, int, error) {
	if m.getPostsFn != nil {
		return m.getPostsFn(ctx, limit, offset)
	}
	return []park.Post{
		{
			ID:            "post-1",
			CharacterID:   "char-1",
			CharacterName: "TestHero",
			Content:       "Hello Park",
			CreatedAt:     time.Now().UTC(),
		},
	}, 1, nil
}

func (m *mockParkService) GetRecentPostsByCursor(ctx context.Context, limit int, cursor string) (pagination.CursorPage[park.Post], error) {
	if m.getPostsByCursorFn != nil {
		return m.getPostsByCursorFn(ctx, limit, cursor)
	}
	posts := []park.Post{
		{
			ID:            "post-1",
			CharacterID:   "char-1",
			CharacterName: "TestHero",
			Content:       "Hello Cursor Park",
			CreatedAt:     time.Now().UTC(),
		},
	}
	return pagination.NewCursorPage(posts, "next-tok", cursor, limit, true), nil
}

func (m *mockParkService) TalkToNPC(ctx context.Context, charID string) (string, error) {
	if m.talkFn != nil {
		return m.talkFn(ctx, charID)
	}
	return "今日はいい天気ですね〜", nil
}

func (m *mockParkService) Divinate(ctx context.Context, charID string) (park.DivinationResult, error) {
	if m.divinateFn != nil {
		return m.divinateFn(ctx, charID)
	}
	return park.DivinationResult{
		Fortune:    "大吉",
		LuckyColor: "赤",
		Message:    "今日の運勢は大吉です♪",
	}, nil
}

func (m *mockParkService) InspectNPC() string {
	if m.inspectLineFn != nil {
		return m.inspectLineFn()
	}
	return "何かお探しですか？"
}

func TestParkEndpoints(t *testing.T) {
	player := coreplayer.Player{ID: "player-1", Username: "user1"}
	char := corecharacter.Character{ID: "char-1", PlayerID: "player-1", Name: "TestHero"}
	otherChar := corecharacter.Character{ID: "char-2", PlayerID: "other-player", Name: "OtherHero"}

	players := &stubPlayerService{
		authenticateFn: func(ctx context.Context, sessionID string) (coreplayer.Player, error) {
			if sessionID == "valid-session" {
				return player, nil
			}
			return coreplayer.Player{}, errors.New("unauthorized")
		},
	}
	chars := &stubCharacterService{
		getFn: func(ctx context.Context, id string) (corecharacter.Character, error) {
			if id == "char-1" {
				return char, nil
			}
			if id == "char-2" {
				return otherChar, nil
			}
			return corecharacter.Character{}, corecharacter.ErrNotFound
		},
	}
	adv := &stubAdventureService{}
	shopSvc := &stubShopService{}
	parkSvc := &mockParkService{}

	handler, err := apihttp.NewHandler(
		players,
		chars,
		adv,
		shopSvc,
		apihttp.WithPark(parkSvc),
	)
	if err != nil {
		t.Fatal(err)
	}

	router := handler.Router()

	t.Run("GET /park/posts", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/park/posts?limit=10&offset=0", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		var body map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		if body["total"].(float64) != 1 {
			t.Errorf("expected total 1, got %v", body["total"])
		}
		if body["limit"].(float64) != 10 {
			t.Errorf("expected limit 10, got %v", body["limit"])
		}
		if body["offset"].(float64) != 0 {
			t.Errorf("expected offset 0, got %v", body["offset"])
		}
		items, ok := body["items"].([]any)
		if !ok || len(items) != 1 {
			t.Errorf("expected 1 item, got %v", body["items"])
		}
	})

	t.Run("GET /park/posts - with cursor pagination", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/park/posts?cursor=tok123&limit=5", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		var body map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		if body["next_cursor"] != "next-tok" {
			t.Errorf("expected next_cursor 'next-tok', got %v", body["next_cursor"])
		}
		if body["has_more"] != true {
			t.Errorf("expected has_more true, got %v", body["has_more"])
		}
		if body["limit"].(float64) != 5 {
			t.Errorf("expected limit 5, got %v", body["limit"])
		}
		items, ok := body["items"].([]any)
		if !ok || len(items) != 1 {
			t.Errorf("expected 1 item, got %v", body["items"])
		}
	})

	t.Run("POST /park/posts - unauthorized", func(t *testing.T) {
		payload := []byte(`{"character_id":"char-1","content":"hello"}`)
		req := httptest.NewRequest(http.MethodPost, "/park/posts", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rec.Code)
		}
	})

	t.Run("POST /park/posts - forbidden character ownership", func(t *testing.T) {
		payload := []byte(`{"character_id":"char-2","content":"hello"}`)
		req := httptest.NewRequest(http.MethodPost, "/park/posts", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer valid-session")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected 403, got %d", rec.Code)
		}
	})

	t.Run("POST /park/posts - rate limited", func(t *testing.T) {
		rateLimitedPark := &mockParkService{
			postFn: func(ctx context.Context, charID, content, color, recipient string) (park.Post, error) {
				return park.Post{}, park.ErrRateLimited
			},
		}
		h, _ := apihttp.NewHandler(players, chars, adv, shopSvc, apihttp.WithPark(rateLimitedPark))
		r := h.Router()

		payload := []byte(`{"character_id":"char-1","content":"hello"}`)
		req := httptest.NewRequest(http.MethodPost, "/park/posts", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer valid-session")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusTooManyRequests {
			t.Fatalf("expected 429, got %d", rec.Code)
		}
	})

	t.Run("POST /park/posts - success", func(t *testing.T) {
		payload := []byte(`{"character_id":"char-1","content":"hello!","color":"#ff0000","recipient_name":"ボブ"}`)
		req := httptest.NewRequest(http.MethodPost, "/park/posts", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer valid-session")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d", rec.Code)
		}
	})

	t.Run("POST /park/npc/talk - success", func(t *testing.T) {
		payload := []byte(`{"character_id":"char-1"}`)
		req := httptest.NewRequest(http.MethodPost, "/park/npc/talk", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer valid-session")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
	})

	t.Run("POST /park/npc/divinate - success", func(t *testing.T) {
		payload := []byte(`{"character_id":"char-1"}`)
		req := httptest.NewRequest(http.MethodPost, "/park/npc/divinate", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer valid-session")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
	})

	t.Run("GET /park/npc/inspect - success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/park/npc/inspect", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
	})
}
