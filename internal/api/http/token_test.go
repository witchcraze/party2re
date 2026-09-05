package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	apihttp "github.com/witchcraze/party2re/internal/api/http"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/database"
)

func TestTokenEndpoints(t *testing.T) {
	player := coreplayer.Player{ID: "player-1", Username: "alice"}

	now := time.Now().UTC().Truncate(time.Second)
	sampleToken := coreplayer.APIToken{
		ID:        "tok-1",
		PlayerID:  "player-1",
		TokenHash: "hash123",
		Name:      "test-cli",
		CreatedAt: now,
	}

	players := &stubPlayerService{
		authenticateFn: func(ctx context.Context, sessionID string) (coreplayer.Player, error) {
			if sessionID == "valid-session" || sessionID == "p2_sk_validtoken" {
				return player, nil
			}
			return coreplayer.Player{}, coreplayer.ErrInvalidSession
		},
		createAPITokenFn: func(ctx context.Context, playerID, name string, expiresAt *time.Time) (coreplayer.APIToken, string, error) {
			if name == "" {
				return coreplayer.APIToken{}, "", coreplayer.ErrInvalidAPITokenName
			}
			if expiresAt != nil && expiresAt.Before(now) {
				return coreplayer.APIToken{}, "", coreplayer.ErrInvalidAPITokenExpiration
			}
			tok := coreplayer.APIToken{
				ID:        "tok-1",
				PlayerID:  playerID,
				Name:      name,
				CreatedAt: now,
				ExpiresAt: expiresAt,
			}
			return tok, "p2_sk_abc123plaintext", nil
		},
		listAPITokensFn: func(ctx context.Context, playerID string) ([]coreplayer.APIToken, error) {
			if playerID == "player-1" {
				return []coreplayer.APIToken{sampleToken}, nil
			}
			return []coreplayer.APIToken{}, nil
		},
		revokeAPITokenFn: func(ctx context.Context, playerID, tokenID string) error {
			if tokenID == "tok-not-found" {
				return database.ErrAPITokenNotFound
			}
			if tokenID == "tok-other-player" {
				return database.ErrAPITokenForbidden
			}
			return nil
		},
	}

	h := newTestHandler(t, players, &stubCharacterService{}, &stubAdventureService{}, &stubShopService{})
	router := h.Router()

	t.Run("POST /player/tokens - success", func(t *testing.T) {
		body := `{"name":"test-cli"}`
		req := httptest.NewRequest(http.MethodPost, "/player/tokens", bytes.NewBufferString(body))
		req.Header.Set("Authorization", "Bearer valid-session")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201 Created, got %d: %s", rec.Code, rec.Body.String())
		}

		var resp apihttp.CreateAPITokenResponse
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp.ID != "tok-1" || resp.Name != "test-cli" {
			t.Errorf("unexpected token data: %+v", resp)
		}
		if resp.Token != "p2_sk_abc123plaintext" {
			t.Errorf("expected plaintext token 'p2_sk_abc123plaintext', got %q", resp.Token)
		}
	})

	t.Run("POST /player/tokens - invalid name returns 400", func(t *testing.T) {
		body := `{"name":""}`
		req := httptest.NewRequest(http.MethodPost, "/player/tokens", bytes.NewBufferString(body))
		req.Header.Set("Authorization", "Bearer valid-session")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /player/tokens - unauthorized returns 401", func(t *testing.T) {
		body := `{"name":"cli"}`
		req := httptest.NewRequest(http.MethodPost, "/player/tokens", bytes.NewBufferString(body))
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 Unauthorized, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("GET /player/tokens - success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/player/tokens", nil)
		req.Header.Set("Authorization", "Bearer valid-session")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}

		var resp apihttp.APITokensResponse
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if len(resp.Tokens) != 1 {
			t.Fatalf("expected 1 token, got %d", len(resp.Tokens))
		}
		if resp.Tokens[0].ID != "tok-1" || resp.Tokens[0].Name != "test-cli" {
			t.Errorf("unexpected token in list: %+v", resp.Tokens[0])
		}
		if bytes.Contains(rec.Body.Bytes(), []byte("hash123")) {
			t.Errorf("GET /player/tokens must never leak token hash")
		}
	})

	t.Run("GET /player/tokens - unauthorized returns 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/player/tokens", nil)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 Unauthorized, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("DELETE /player/tokens/{id} - success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/player/tokens/tok-1", nil)
		req.Header.Set("Authorization", "Bearer valid-session")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}

		var resp apihttp.RevokeAPITokenResponse
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if !resp.Revoked || resp.TokenID != "tok-1" {
			t.Errorf("unexpected revoke response: %+v", resp)
		}
	})

	t.Run("DELETE /player/tokens/{id} - not found returns 404", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/player/tokens/tok-not-found", nil)
		req.Header.Set("Authorization", "Bearer valid-session")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 Not Found, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("DELETE /player/tokens/{id} - forbidden returns 403", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/player/tokens/tok-other-player", nil)
		req.Header.Set("Authorization", "Bearer valid-session")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected 403 Forbidden, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("Dual authentication - authenticate with API token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/player/tokens", nil)
		req.Header.Set("Authorization", "Bearer p2_sk_validtoken")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK via API token auth, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}
