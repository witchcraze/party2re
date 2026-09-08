package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	apihttp "github.com/witchcraze/party2re/internal/api/http"
	"github.com/witchcraze/party2re/internal/chapel"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
)

type stubChapelService struct {
	getBlessingFn    func(ctx context.Context, characterID string) (chapel.CharacterBlessing, error)
	getStatusFn      func(ctx context.Context, characterID, characterName string) (chapel.ChapelStatus, error)
	selectBlessingFn func(ctx context.Context, characterID string, blessing chapel.BlessingType) (chapel.CharacterBlessing, error)
	clearBlessingFn  func(ctx context.Context, characterID string) error
}

func (s *stubChapelService) GetBlessing(ctx context.Context, characterID string) (chapel.CharacterBlessing, error) {
	if s.getBlessingFn != nil {
		return s.getBlessingFn(ctx, characterID)
	}
	return chapel.CharacterBlessing{CharacterID: characterID, ActiveBlessing: chapel.BlessingNone}, nil
}

func (s *stubChapelService) GetStatus(ctx context.Context, characterID, characterName string) (chapel.ChapelStatus, error) {
	if s.getStatusFn != nil {
		return s.getStatusFn(ctx, characterID, characterName)
	}
	return chapel.ChapelStatus{
		LocationName:       chapel.LocationName,
		NPCName:            chapel.NPCName,
		BackgroundImage:    chapel.BackgroundImage,
		AvailableBlessings: chapel.AvailableBlessings,
	}, nil
}

func (s *stubChapelService) SelectBlessing(ctx context.Context, characterID string, blessing chapel.BlessingType) (chapel.CharacterBlessing, error) {
	if s.selectBlessingFn != nil {
		return s.selectBlessingFn(ctx, characterID, blessing)
	}
	return chapel.CharacterBlessing{CharacterID: characterID, ActiveBlessing: blessing}, nil
}

func (s *stubChapelService) ClearBlessing(ctx context.Context, characterID string) error {
	if s.clearBlessingFn != nil {
		return s.clearBlessingFn(ctx, characterID)
	}
	return nil
}

func TestChapelEndpoints(t *testing.T) {
	player := coreplayer.Player{ID: "p1", Username: "hero"}
	char := corecharacter.Character{ID: "c1", PlayerID: "p1", Name: "Hero", Money: 1000}

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
	chService := &stubChapelService{
		getBlessingFn: func(_ context.Context, characterID string) (chapel.CharacterBlessing, error) {
			return chapel.CharacterBlessing{
				CharacterID:    characterID,
				ActiveBlessing: chapel.BlessingGold,
				PrayedAt:       time.Now(),
			}, nil
		},
	}

	h := newTestHandler(
		t,
		pService,
		cService,
		&stubAdventureService{},
		&stubShopService{},
		apihttp.WithChapel(chService),
	)
	router := h.Router()

	t.Run("GET /characters/{id}/chapel - success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/characters/c1/chapel", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /characters/{id}/chapel/pray - success MONSTER", func(t *testing.T) {
		req := jsonRequest(t, http.MethodPost, "/characters/c1/chapel/pray", `{"blessing":"MONSTER"}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /characters/{id}/chapel/pray - conflict when already prayed", func(t *testing.T) {
		chService.selectBlessingFn = func(_ context.Context, _ string, _ chapel.BlessingType) (chapel.CharacterBlessing, error) {
			return chapel.CharacterBlessing{}, chapel.ErrAlreadyPrayed
		}
		defer func() { chService.selectBlessingFn = nil }()

		req := jsonRequest(t, http.MethodPost, "/characters/c1/chapel/pray", `{"blessing":"EXP"}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusConflict {
			t.Fatalf("expected 409 Conflict, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /characters/{id}/chapel/pray - bad request for invalid blessing", func(t *testing.T) {
		req := jsonRequest(t, http.MethodPost, "/characters/c1/chapel/pray", `{"blessing":"UNKNOWN"}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /characters/{id}/chapel/donate - route eliminated (404/405)", func(t *testing.T) {
		req := jsonRequest(t, http.MethodPost, "/characters/c1/chapel/donate", `{"amount":500}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 404 or 405 for eliminated donate endpoint, got %d", rec.Code)
		}
	})
}
