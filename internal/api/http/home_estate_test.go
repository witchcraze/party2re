package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	apihttp "github.com/witchcraze/party2re/internal/api/http"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/home"
)

func TestHomeEstateEndpoints(t *testing.T) {
	player := coreplayer.Player{ID: "player-1", Username: "user1"}
	char := corecharacter.Character{ID: "char-1", PlayerID: "player-1", Name: "Hero"}

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
			return corecharacter.Character{}, corecharacter.ErrNotFound
		},
	}
	advs := &stubAdventureService{}
	shops := &stubShopService{}
	homeSvc := &mockHomeService{}

	handler, err := apihttp.NewHandler(
		players,
		chars,
		advs,
		shops,
		apihttp.WithHome(homeSvc),
	)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}
	router := handler.Router()

	t.Run("POST /towns/{town_id}/houses - success", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"character_id": "char-1",
			"house_style":  "001",
		})
		req := httptest.NewRequest(http.MethodPost, "/towns/town1/houses", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer valid-session")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201 Created, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("GET /towns/{town_id}/houses - success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/towns/town1/houses", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("GET /houses/check - success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/houses/check?target=Hero", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /characters/{id}/color - success", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"color": "#123456",
		})
		req := httptest.NewRequest(http.MethodPost, "/characters/char-1/color", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer valid-session")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("GET /characters/{id}/home/items - success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/characters/char-1/home/items", nil)
		req.Header.Set("Authorization", "Bearer valid-session")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /characters/{id}/home/items/use - success", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"instance_id": "inst-1",
			"source":      "inventory",
		})
		req := httptest.NewRequest(http.MethodPost, "/characters/char-1/home/items/use", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer valid-session")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /characters/{id}/home/items/use - cannot use here returns 400 Bad Request", func(t *testing.T) {
		homeSvc.useHomeItemFn = func(ctx context.Context, characterID, instanceID, source string) (*home.UseHomeItemResult, error) {
			return nil, home.ErrCannotUseHere
		}
		defer func() { homeSvc.useHomeItemFn = nil }()

		body, _ := json.Marshal(map[string]string{
			"instance_id": "inst-combat-item",
			"source":      "inventory",
		})
		req := httptest.NewRequest(http.MethodPost, "/characters/char-1/home/items/use", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer valid-session")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /characters/{id}/home/items/use - item not found returns 404 Not Found", func(t *testing.T) {
		homeSvc.useHomeItemFn = func(ctx context.Context, characterID, instanceID, source string) (*home.UseHomeItemResult, error) {
			return nil, home.ErrItemNotFound
		}
		defer func() { homeSvc.useHomeItemFn = nil }()

		body, _ := json.Marshal(map[string]string{
			"instance_id": "inst-nonexistent",
			"source":      "inventory",
		})
		req := httptest.NewRequest(http.MethodPost, "/characters/char-1/home/items/use", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer valid-session")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 Not Found, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}
