package http_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	apihttp "github.com/witchcraze/party2re/internal/api/http"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/home"
)

func TestAuthHelpers(t *testing.T) {
	player := coreplayer.Player{ID: "player-1", Username: "hero"}
	char := corecharacter.Character{ID: "char-1", PlayerID: "player-1", Name: "Hero"}
	otherChar := corecharacter.Character{ID: "char-2", PlayerID: "player-2", Name: "Villain"}

	players := &stubPlayerService{
		authenticateFn: func(ctx context.Context, sessionID string) (coreplayer.Player, error) {
			if sessionID == "valid-session" {
				return player, nil
			}
			return coreplayer.Player{}, errors.New("invalid session")
		},
	}

	chars := &stubCharacterService{
		getFn: func(ctx context.Context, id string) (corecharacter.Character, error) {
			switch id {
			case "char-1":
				return char, nil
			case "char-2":
				return otherChar, nil
			case "error-char":
				return corecharacter.Character{}, errors.New("database connection failure")
			default:
				return corecharacter.Character{}, corecharacter.ErrNotFound
			}
		},
	}

	handler, err := apihttp.NewHandler(
		players,
		chars,
		&stubAdventureService{},
		&stubShopService{},
	)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}
	router := handler.Router()

	t.Run("GET /characters/{id} - missing session returns 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/characters/char-1", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rec.Code)
		}
	})

	t.Run("GET /characters/{id} - invalid session returns 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/characters/char-1", nil)
		req.Header.Set("Authorization", "Bearer bad-session")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rec.Code)
		}
	})

	t.Run("GET /characters/{id} - non-existent character returns 404", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/characters/unknown-char", nil)
		req.Header.Set("Authorization", "Bearer valid-session")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("expected 404, got %d", rec.Code)
		}
	})

	t.Run("GET /characters/{id} - database error returns 500", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/characters/error-char", nil)
		req.Header.Set("Authorization", "Bearer valid-session")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Errorf("expected 500, got %d", rec.Code)
		}
	})

	t.Run("GET /characters/{id} - character belonging to another player returns 403", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/characters/char-2", nil)
		req.Header.Set("Authorization", "Bearer valid-session")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Errorf("expected 403, got %d", rec.Code)
		}
	})

	t.Run("GET /characters/{id} - owned character returns 200", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/characters/char-1", nil)
		req.Header.Set("Authorization", "Bearer valid-session")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rec.Code)
		}
	})

	t.Run("POST /shop/purchase - invalid JSON body returns 400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/shop/purchase", bytes.NewReader([]byte("{invalid json")))
		req.Header.Set("Authorization", "Bearer valid-session")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", rec.Code)
		}
	})
}

func TestAdminAuth(t *testing.T) {
	players := &stubPlayerService{}
	chars := &stubCharacterService{}
	advs := &stubAdventureService{}
	shops := &stubShopService{}
	notifs := &mockNotificationService{}

	t.Run("admin disabled when no key configured", func(t *testing.T) {
		h, err := apihttp.NewHandler(players, chars, advs, shops, apihttp.WithNotification(notifs))
		if err != nil {
			t.Fatalf("failed to create handler: %v", err)
		}
		router := h.Router()

		req := httptest.NewRequest(http.MethodPost, "/news", bytes.NewReader([]byte(`{"title":"T","content":"C","category":"system","author":"A"}`)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Admin-Key", "secret-key")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Errorf("expected 403 Forbidden when admin key unconfigured, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("admin key authorization", func(t *testing.T) {
		const adminKey = "super-secret-admin-key"
		h, err := apihttp.NewHandler(players, chars, advs, shops, apihttp.WithNotification(notifs), apihttp.WithAdminAPIKey(adminKey))
		if err != nil {
			t.Fatalf("failed to create handler: %v", err)
		}
		router := h.Router()

		// Missing credentials -> 401
		req := httptest.NewRequest(http.MethodPost, "/news", bytes.NewReader([]byte(`{"title":"T","content":"C","category":"system","author":"A"}`)))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 Unauthorized for missing admin credentials, got %d: %s", rec.Code, rec.Body.String())
		}

		// Invalid X-Admin-Key -> 403
		req = httptest.NewRequest(http.MethodPost, "/news", bytes.NewReader([]byte(`{"title":"T","content":"C","category":"system","author":"A"}`)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Admin-Key", "wrong-key")
		rec = httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("expected 403 Forbidden for wrong X-Admin-Key, got %d: %s", rec.Code, rec.Body.String())
		}

		// Invalid Bearer token -> 403
		req = httptest.NewRequest(http.MethodPost, "/news", bytes.NewReader([]byte(`{"title":"T","content":"C","category":"system","author":"A"}`)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer wrong-key")
		rec = httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("expected 403 Forbidden for wrong Bearer token, got %d: %s", rec.Code, rec.Body.String())
		}

		// Valid X-Admin-Key -> 201
		req = httptest.NewRequest(http.MethodPost, "/news", bytes.NewReader([]byte(`{"title":"T","content":"C","category":"system","author":"A"}`)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Admin-Key", adminKey)
		rec = httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Errorf("expected 201 Created for valid X-Admin-Key, got %d: %s", rec.Code, rec.Body.String())
		}

		// Valid Bearer token -> 201
		req = httptest.NewRequest(http.MethodPost, "/news", bytes.NewReader([]byte(`{"title":"T","content":"C","category":"system","author":"A"}`)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+adminKey)
		rec = httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Errorf("expected 201 Created for valid Bearer token, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

func TestGuardSleepingCharacter(t *testing.T) {
	player := coreplayer.Player{ID: "player-1", Username: "hero"}
	char := corecharacter.Character{ID: "char-1", PlayerID: "player-1", Name: "Hero"}

	players := &stubPlayerService{
		authenticateFn: func(ctx context.Context, sessionID string) (coreplayer.Player, error) {
			if sessionID == "valid-session" {
				return player, nil
			}
			return coreplayer.Player{}, errors.New("invalid session")
		},
	}
	chars := &stubCharacterService{
		getFn: func(ctx context.Context, id string) (corecharacter.Character, error) {
			if id == "char-1" {
				return char, nil
			}
			return corecharacter.Character{}, corecharacter.ErrNotFound
		},
	}

	t.Run("returns true when homes service is nil", func(t *testing.T) {
		h, err := apihttp.NewHandler(players, chars, &stubAdventureService{}, &stubShopService{})
		if err != nil {
			t.Fatalf("failed to create handler: %v", err)
		}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/adventures", bytes.NewReader([]byte(`{"character_id":"char-1","stage_id":"stage-1"}`)))
		req.Header.Set("Authorization", "Bearer valid-session")
		req.Header.Set("Content-Type", "application/json")
		h.Router().ServeHTTP(rec, req)
		if rec.Code == http.StatusConflict {
			t.Errorf("expected not conflict when homes is nil, got %d", rec.Code)
		}
	})

	t.Run("returns 409 Conflict when character is actively sleeping", func(t *testing.T) {
		mockHome := &mockHomeService{
			getSleepStatusFn: func(ctx context.Context, characterID string) (home.SleepStatus, error) {
				return home.SleepStatus{
					Sleeping:         true,
					RemainingSeconds: 300,
					Message:          "お休み中「Zzz...」 目覚めるまで 5分00秒",
				}, nil
			},
		}
		h, err := apihttp.NewHandler(players, chars, &stubAdventureService{}, &stubShopService{}, apihttp.WithHome(mockHome))
		if err != nil {
			t.Fatalf("failed to create handler: %v", err)
		}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/adventures", bytes.NewReader([]byte(`{"character_id":"char-1","stage_id":"stage-1"}`)))
		req.Header.Set("Authorization", "Bearer valid-session")
		req.Header.Set("Content-Type", "application/json")
		h.Router().ServeHTTP(rec, req)
		if rec.Code != http.StatusConflict {
			t.Errorf("expected 409 Conflict, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("returns 409 Conflict when character is ready to wake (CategoryAsleep locked)", func(t *testing.T) {
		mockHome := &mockHomeService{
			getSleepStatusFn: func(ctx context.Context, characterID string) (home.SleepStatus, error) {
				return home.SleepStatus{
					Sleeping:         false,
					CanWake:          true,
					RemainingSeconds: 0,
					Message:          "目を覚ます準備ができました",
				}, nil
			},
		}
		h, err := apihttp.NewHandler(players, chars, &stubAdventureService{}, &stubShopService{}, apihttp.WithHome(mockHome))
		if err != nil {
			t.Fatalf("failed to create handler: %v", err)
		}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/adventures", bytes.NewReader([]byte(`{"character_id":"char-1","stage_id":"stage-1"}`)))
		req.Header.Set("Authorization", "Bearer valid-session")
		req.Header.Set("Content-Type", "application/json")
		h.Router().ServeHTTP(rec, req)
		if rec.Code != http.StatusConflict {
			t.Errorf("expected 409 Conflict, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("allows action when character is awake", func(t *testing.T) {
		mockHome := &mockHomeService{
			getSleepStatusFn: func(ctx context.Context, characterID string) (home.SleepStatus, error) {
				return home.SleepStatus{
					Sleeping: false,
					CanWake:  false,
					Message:  "起きています",
				}, nil
			},
		}
		h, err := apihttp.NewHandler(players, chars, &stubAdventureService{}, &stubShopService{}, apihttp.WithHome(mockHome))
		if err != nil {
			t.Fatalf("failed to create handler: %v", err)
		}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/adventures", bytes.NewReader([]byte(`{"character_id":"char-1","stage_id":"stage-1"}`)))
		req.Header.Set("Authorization", "Bearer valid-session")
		req.Header.Set("Content-Type", "application/json")
		h.Router().ServeHTTP(rec, req)
		if rec.Code == http.StatusConflict {
			t.Errorf("expected awake character not to be blocked by conflict, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("shop batch purchase returns 409 Conflict when sleeping", func(t *testing.T) {
		mockHome := &mockHomeService{
			getSleepStatusFn: func(ctx context.Context, characterID string) (home.SleepStatus, error) {
				return home.SleepStatus{
					Sleeping:         true,
					RemainingSeconds: 300,
					Message:          "お休み中「Zzz...」 目覚めるまで 5分00秒",
				}, nil
			},
		}
		h, err := apihttp.NewHandler(players, chars, &stubAdventureService{}, &stubShopService{}, apihttp.WithHome(mockHome))
		if err != nil {
			t.Fatalf("failed to create handler: %v", err)
		}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/characters/char-1/shop/batch-purchase", bytes.NewReader([]byte(`{"shop_type":"item","items":[]}`)))
		req.Header.Set("Authorization", "Bearer valid-session")
		req.Header.Set("Content-Type", "application/json")
		h.Router().ServeHTTP(rec, req)
		if rec.Code != http.StatusConflict {
			t.Errorf("expected 409 Conflict for shop when sleeping, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("combat challenge start returns 409 Conflict when sleeping", func(t *testing.T) {
		mockHome := &mockHomeService{
			getSleepStatusFn: func(ctx context.Context, characterID string) (home.SleepStatus, error) {
				return home.SleepStatus{
					Sleeping:         true,
					RemainingSeconds: 300,
					Message:          "お休み中「Zzz...」 目覚めるまで 5分00秒",
				}, nil
			},
		}
		h, err := apihttp.NewHandler(players, chars, &stubAdventureService{}, &stubShopService{}, apihttp.WithHome(mockHome), apihttp.WithChallenge(&stubChallengeService{}))
		if err != nil {
			t.Fatalf("failed to create handler: %v", err)
		}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/characters/char-1/challenges/start", bytes.NewReader([]byte(`{"tier_id":"tier-1"}`)))
		req.Header.Set("Authorization", "Bearer valid-session")
		req.Header.Set("Content-Type", "application/json")
		h.Router().ServeHTTP(rec, req)
		if rec.Code != http.StatusConflict {
			t.Errorf("expected 409 Conflict for challenge when sleeping, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("pvp room create returns 409 Conflict when sleeping", func(t *testing.T) {
		mockHome := &mockHomeService{
			getSleepStatusFn: func(ctx context.Context, characterID string) (home.SleepStatus, error) {
				return home.SleepStatus{
					Sleeping:         true,
					RemainingSeconds: 300,
					Message:          "お休み中「Zzz...」 目覚めるまで 5分00秒",
				}, nil
			},
		}
		h, err := apihttp.NewHandler(players, chars, &stubAdventureService{}, &stubShopService{}, apihttp.WithHome(mockHome), apihttp.WithPvP(&stubColosseumPvPService{}))
		if err != nil {
			t.Fatalf("failed to create handler: %v", err)
		}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/characters/char-1/pvp/rooms", bytes.NewReader([]byte(`{"name":"Room"}`)))
		req.Header.Set("Authorization", "Bearer valid-session")
		req.Header.Set("Content-Type", "application/json")
		h.Router().ServeHTTP(rec, req)
		if rec.Code != http.StatusConflict {
			t.Errorf("expected 409 Conflict for pvp when sleeping, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("casino exchange returns 409 Conflict when sleeping", func(t *testing.T) {
		mockHome := &mockHomeService{
			getSleepStatusFn: func(ctx context.Context, characterID string) (home.SleepStatus, error) {
				return home.SleepStatus{
					Sleeping:         true,
					RemainingSeconds: 300,
					Message:          "お休み中「Zzz...」 目覚めるまで 5分00秒",
				}, nil
			},
		}
		h, err := apihttp.NewHandler(players, chars, &stubAdventureService{}, &stubShopService{}, apihttp.WithHome(mockHome), apihttp.WithCasino(&stubCasinoService{}))
		if err != nil {
			t.Fatalf("failed to create handler: %v", err)
		}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/characters/char-1/casino/exchange", bytes.NewReader([]byte(`{"coins":10}`)))
		req.Header.Set("Authorization", "Bearer valid-session")
		req.Header.Set("Content-Type", "application/json")
		h.Router().ServeHTTP(rec, req)
		if rec.Code != http.StatusConflict {
			t.Errorf("expected 409 Conflict for casino when sleeping, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("auction send returns 409 Conflict when sleeping", func(t *testing.T) {
		mockHome := &mockHomeService{
			getSleepStatusFn: func(ctx context.Context, characterID string) (home.SleepStatus, error) {
				return home.SleepStatus{
					Sleeping:         true,
					RemainingSeconds: 300,
					Message:          "お休み中「Zzz...」 目覚めるまで 5分00秒",
				}, nil
			},
		}
		h, err := apihttp.NewHandler(players, chars, &stubAdventureService{}, &stubShopService{}, apihttp.WithHome(mockHome), apihttp.WithAuction(&stubAuctionService{}))
		if err != nil {
			t.Fatalf("failed to create handler: %v", err)
		}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/characters/char-1/auction/send", bytes.NewReader([]byte(`{"target_character_id":"char-2","gold":100}`)))
		req.Header.Set("Authorization", "Bearer valid-session")
		req.Header.Set("Content-Type", "application/json")
		h.Router().ServeHTTP(rec, req)
		if rec.Code != http.StatusConflict {
			t.Errorf("expected 409 Conflict for auction when sleeping, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}
