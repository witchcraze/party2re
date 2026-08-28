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
