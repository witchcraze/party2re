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
)

func TestAdminListPlayers(t *testing.T) {
	adminKey := "test-admin-secret"
	now := time.Now().UTC()
	bannedTime := now.Add(-10 * time.Minute)

	mockPlayers := []coreplayer.Player{
		{
			ID:        "p1",
			Username:  "alice",
			CreatedAt: now.Add(-1 * time.Hour),
			UpdatedAt: now.Add(-30 * time.Minute),
			LastIP:    "192.168.1.1",
		},
		{
			ID:        "p2",
			Username:  "bob",
			CreatedAt: now.Add(-2 * time.Hour),
			UpdatedAt: now.Add(-10 * time.Minute),
			LastIP:    "192.168.1.2",
			BannedAt:  &bannedTime,
		},
	}

	var capturedSort string
	pService := &stubPlayerService{
		listPlayersFn: func(ctx context.Context, sort string) ([]coreplayer.Player, error) {
			capturedSort = sort
			return mockPlayers, nil
		},
	}

	h, err := apihttp.NewHandler(
		pService,
		&stubCharacterService{},
		&stubAdventureService{},
		&stubShopService{},
		apihttp.WithAdminAPIKey(adminKey),
	)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}
	router := h.Router()

	t.Run("401 Unauthorized without admin key", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin/players", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 Unauthorized, got %d", rec.Code)
		}
	})

	t.Run("403 Forbidden with invalid admin key", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin/players", nil)
		req.Header.Set("X-Admin-Key", "wrong-key")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected 403 Forbidden, got %d", rec.Code)
		}
	})

	t.Run("200 OK with X-Admin-Key and sort param", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin/players?sort=updated_at", nil)
		req.Header.Set("X-Admin-Key", adminKey)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
		if capturedSort != "updated_at" {
			t.Errorf("expected captured sort %q, got %q", "updated_at", capturedSort)
		}

		var body struct {
			Players []struct {
				ID        string     `json:"id"`
				Username  string     `json:"username"`
				CreatedAt time.Time  `json:"created_at"`
				UpdatedAt time.Time  `json:"updated_at"`
				LastIP    string     `json:"last_ip"`
				BannedAt  *time.Time `json:"banned_at"`
				IsBanned  bool       `json:"is_banned"`
			} `json:"players"`
			Total int `json:"total"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("failed to parse response JSON: %v", err)
		}
		if body.Total != 2 || len(body.Players) != 2 {
			t.Fatalf("expected 2 players, got %d (total: %d)", len(body.Players), body.Total)
		}
		if body.Players[0].IsBanned || body.Players[0].BannedAt != nil {
			t.Errorf("expected player 1 not banned, got banned")
		}
		if !body.Players[1].IsBanned || body.Players[1].BannedAt == nil {
			t.Errorf("expected player 2 banned, got not banned")
		}
	})

	t.Run("200 OK with Bearer token header", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin/players", nil)
		req.Header.Set("Authorization", "Bearer "+adminKey)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
	})

	t.Run("500 InternalServerError when service fails", func(t *testing.T) {
		pService.listPlayersFn = func(ctx context.Context, sort string) ([]coreplayer.Player, error) {
			return nil, errors.New("database connection lost")
		}
		req := httptest.NewRequest(http.MethodGet, "/admin/players", nil)
		req.Header.Set("X-Admin-Key", adminKey)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500 InternalServerError, got %d", rec.Code)
		}
	})
}

func TestAdminBanPlayer(t *testing.T) {
	adminKey := "test-admin-secret"
	var bannedPlayerID string
	pService := &stubPlayerService{
		banPlayerFn: func(ctx context.Context, playerID string) error {
			if playerID == "unknown" {
				return coreplayer.ErrPlayerNotFound
			}
			if playerID == "err" {
				return errors.New("internal disk error")
			}
			bannedPlayerID = playerID
			return nil
		},
	}

	h, err := apihttp.NewHandler(
		pService,
		&stubCharacterService{},
		&stubAdventureService{},
		&stubShopService{},
		apihttp.WithAdminAPIKey(adminKey),
	)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}
	router := h.Router()

	t.Run("401 Unauthorized without admin key", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/admin/players/p123/ban", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 Unauthorized, got %d", rec.Code)
		}
	})

	t.Run("403 Forbidden with invalid admin key", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/admin/players/p123/ban", nil)
		req.Header.Set("X-Admin-Key", "wrong-key")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected 403 Forbidden, got %d", rec.Code)
		}
	})

	t.Run("200 OK on successful ban", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/admin/players/p123/ban", nil)
		req.Header.Set("X-Admin-Key", adminKey)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
		if bannedPlayerID != "p123" {
			t.Errorf("expected bannedPlayerID p123, got %q", bannedPlayerID)
		}
		var resp struct {
			Message  string `json:"message"`
			PlayerID string `json:"player_id"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to parse JSON: %v", err)
		}
		if resp.PlayerID != "p123" {
			t.Errorf("expected response player_id p123, got %q", resp.PlayerID)
		}
	})

	t.Run("404 NotFound for nonexistent player", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/admin/players/unknown/ban", nil)
		req.Header.Set("X-Admin-Key", adminKey)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 NotFound, got %d", rec.Code)
		}
	})

	t.Run("500 InternalServerError on unexpected error", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/admin/players/err/ban", nil)
		req.Header.Set("X-Admin-Key", adminKey)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500 InternalServerError, got %d", rec.Code)
		}
	})
}

func TestBannedPlayer_LoginAndAuthenticate_Forbidden(t *testing.T) {
	pService := &stubPlayerService{
		loginFn: func(ctx context.Context, username, password string) (coreplayer.Session, error) {
			if username == "banned_user" {
				return coreplayer.Session{}, coreplayer.ErrPlayerBanned
			}
			return coreplayer.Session{ID: "sess-1", PlayerID: "p1", ExpiresAt: time.Now().Add(time.Hour)}, nil
		},
		authenticateFn: func(ctx context.Context, sessionID string) (coreplayer.Player, error) {
			if sessionID == "banned-session" {
				return coreplayer.Player{}, coreplayer.ErrPlayerBanned
			}
			return coreplayer.Player{ID: "p1", Username: "alice"}, nil
		},
	}

	h, err := apihttp.NewHandler(
		pService,
		&stubCharacterService{
			getFn: func(ctx context.Context, id string) (corecharacter.Character, error) {
				return corecharacter.Character{ID: id, PlayerID: "p1"}, nil
			},
		},
		&stubAdventureService{},
		&stubShopService{},
	)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}
	router := h.Router()

	t.Run("POST /sessions returns 403 Forbidden for banned player", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"username": "banned_user",
			"password": "password",
		})
		req := httptest.NewRequest(http.MethodPost, "/sessions", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected 403 Forbidden on banned login, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("Authenticated character endpoint returns 403 Forbidden for banned player session", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/characters/c1", nil)
		req.Header.Set("Authorization", "Bearer banned-session")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected 403 Forbidden on banned session auth, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

func TestAdmin_Regression_NonAdminRejected(t *testing.T) {
	h, err := apihttp.NewHandler(
		&stubPlayerService{},
		&stubCharacterService{},
		&stubAdventureService{},
		&stubShopService{},
		apihttp.WithAdminAPIKey("secret-key"),
	)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}
	router := h.Router()

	endpoints := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/admin/maintenance"},
		{http.MethodPut, "/admin/maintenance"},
		{http.MethodGet, "/admin/players"},
		{http.MethodPost, "/admin/players/p123/ban"},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path+" rejects without admin key", func(t *testing.T) {
			req := httptest.NewRequest(ep.method, ep.path, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401 Unauthorized for %s %s, got %d", ep.method, ep.path, rec.Code)
			}
		})

		t.Run(ep.method+" "+ep.path+" rejects with wrong admin key", func(t *testing.T) {
			req := httptest.NewRequest(ep.method, ep.path, nil)
			req.Header.Set("X-Admin-Key", "invalid-admin-key")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusForbidden {
				t.Fatalf("expected 403 Forbidden for %s %s, got %d", ep.method, ep.path, rec.Code)
			}
		})
	}
}
