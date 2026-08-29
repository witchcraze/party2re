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
