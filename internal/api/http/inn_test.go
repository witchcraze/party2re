package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	apihttp "github.com/witchcraze/party2re/internal/api/http"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/inn"
)

type stubInnService struct {
	restFn         func(ctx context.Context, characterID string) (corecharacter.Character, error)
	calculateFeeFn func(level int) int
}

func (s *stubInnService) Rest(ctx context.Context, characterID string) (corecharacter.Character, error) {
	if s.restFn != nil {
		return s.restFn(ctx, characterID)
	}
	return corecharacter.Character{}, nil
}

func (s *stubInnService) CalculateFee(level int) int {
	if s.calculateFeeFn != nil {
		return s.calculateFeeFn(level)
	}
	return 10
}

func TestInnEndpoints(t *testing.T) {
	player := coreplayer.Player{ID: "p1", Username: "hero"}
	char := corecharacter.Character{ID: "c1", PlayerID: "p1", Name: "Hero", Money: 100}

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
	iService := &stubInnService{
		restFn: func(_ context.Context, characterID string) (corecharacter.Character, error) {
			if characterID == "c1" {
				c := char
				c.Stats.HP = c.Stats.MaxHP
				c.Stats.MP = c.Stats.MaxMP
				return c, nil
			}
			return corecharacter.Character{}, inn.ErrInsufficientFunds
		},
	}

	h := newTestHandler(
		t,
		pService,
		cService,
		&stubAdventureService{},
		&stubShopService{},
		apihttp.WithInn(iService),
	)
	router := h.Router()

	t.Run("POST /characters/{id}/inn - unauthorized", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/characters/c1/inn", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 Unauthorized, got %d", rec.Code)
		}
	})

	t.Run("POST /characters/{id}/inn - success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/characters/c1/inn", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}
