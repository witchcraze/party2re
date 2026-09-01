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
	"github.com/witchcraze/party2re/internal/god"
)

type stubGodService struct {
	getWishesFn   func(ctx context.Context, characterID string, realm god.Realm) ([]god.Wish, error)
	grantWishFn   func(ctx context.Context, characterID, wishID string, realm god.Realm) (god.WishResult, error)
	getDialogueFn func(realm god.Realm) []string
}

func (s *stubGodService) GetWishes(ctx context.Context, characterID string, realm god.Realm) ([]god.Wish, error) {
	if s.getWishesFn != nil {
		return s.getWishesFn(ctx, characterID, realm)
	}
	return nil, nil
}

func (s *stubGodService) GrantWish(ctx context.Context, characterID, wishID string, realm god.Realm) (god.WishResult, error) {
	if s.grantWishFn != nil {
		return s.grantWishFn(ctx, characterID, wishID, realm)
	}
	return god.WishResult{}, nil
}

func (s *stubGodService) GetDialogue(realm god.Realm) []string {
	if s.getDialogueFn != nil {
		return s.getDialogueFn(realm)
	}
	return []string{"hello"}
}

func TestGodEndpoints(t *testing.T) {
	player := coreplayer.Player{ID: "p1", Username: "hero"}
	char := corecharacter.Character{ID: "c1", PlayerID: "p1", Name: "Hero", Level: 99}

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
	gService := &stubGodService{
		getDialogueFn: func(realm god.Realm) []string {
			return []string{"Welcome to " + string(realm)}
		},
		getWishesFn: func(_ context.Context, characterID string, realm god.Realm) ([]god.Wish, error) {
			return []god.Wish{
				{
					ID:          "wish_stats",
					Name:        "強くなりたい",
					Realm:       realm,
					Description: "全ステータス 40 アップ",
					Available:   true,
				},
			}, nil
		},
		grantWishFn: func(_ context.Context, characterID, wishID string, realm god.Realm) (god.WishResult, error) {
			return god.WishResult{
				Character: char,
				Wish: god.Wish{
					ID:    wishID,
					Name:  "強くなりたい",
					Realm: realm,
				},
				Message:   "Wish granted",
				NPCSpeech: "Speech",
			}, nil
		},
	}

	h, err := apihttp.NewHandler(pService, cService, &stubAdventureService{}, &stubShopService{}, apihttp.WithGod(gService))
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	router := h.Router()

	// 1. GET /god/dialogue
	t.Run("GET /god/dialogue", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/god/dialogue?realm=heaven", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	// 2. GET /characters/{id}/god/wishes
	t.Run("GET /characters/{id}/god/wishes", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/characters/c1/god/wishes?realm=heaven", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}

		var res struct {
			Realm  string     `json:"realm"`
			Wishes []god.Wish `json:"wishes"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if len(res.Wishes) != 1 || res.Wishes[0].ID != "wish_stats" {
			t.Fatalf("unexpected wishes: %+v", res.Wishes)
		}
	})

	// 3. POST /characters/{id}/god/wish
	t.Run("POST /characters/{id}/god/wish", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"realm":   "heaven",
			"wish_id": "wish_stats",
		})
		req := httptest.NewRequest(http.MethodPost, "/characters/c1/god/wish", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer valid-token")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	// 4. IDOR check (different player)
	t.Run("IDOR forbidden for different player", func(t *testing.T) {
		otherPlayer := coreplayer.Player{ID: "p2", Username: "stranger"}
		pServiceOther := &stubPlayerService{
			authenticateFn: func(_ context.Context, _ string) (coreplayer.Player, error) {
				return otherPlayer, nil
			},
		}
		hOther, _ := apihttp.NewHandler(pServiceOther, cService, &stubAdventureService{}, &stubShopService{}, apihttp.WithGod(gService))
		routerOther := hOther.Router()

		req := httptest.NewRequest(http.MethodGet, "/characters/c1/god/wishes?realm=heaven", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		routerOther.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected 403 Forbidden for IDOR access, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}
