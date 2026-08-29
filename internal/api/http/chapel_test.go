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
	selectBlessingFn func(ctx context.Context, characterID string, blessing chapel.BlessingType) (chapel.CharacterBlessing, error)
	donateFn         func(ctx context.Context, characterID string, goldAmount int) (chapel.CharacterBlessing, error)
}

func (s *stubChapelService) GetBlessing(ctx context.Context, characterID string) (chapel.CharacterBlessing, error) {
	if s.getBlessingFn != nil {
		return s.getBlessingFn(ctx, characterID)
	}
	return chapel.CharacterBlessing{CharacterID: characterID}, nil
}

func (s *stubChapelService) SelectBlessing(ctx context.Context, characterID string, blessing chapel.BlessingType) (chapel.CharacterBlessing, error) {
	if s.selectBlessingFn != nil {
		return s.selectBlessingFn(ctx, characterID, blessing)
	}
	return chapel.CharacterBlessing{CharacterID: characterID, ActiveBlessing: blessing}, nil
}

func (s *stubChapelService) Donate(ctx context.Context, characterID string, goldAmount int) (chapel.CharacterBlessing, error) {
	if s.donateFn != nil {
		return s.donateFn(ctx, characterID, goldAmount)
	}
	return chapel.CharacterBlessing{CharacterID: characterID, DonationGoldTotal: int64(goldAmount)}, nil
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
				CharacterID:       characterID,
				ActiveBlessing:    chapel.BlessingGold,
				PrayedAt:          time.Now(),
				DonationGoldTotal: 500,
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

	t.Run("POST /characters/{id}/chapel/pray - success", func(t *testing.T) {
		req := jsonRequest(t, http.MethodPost, "/characters/c1/chapel/pray", `{"blessing":"GOLD"}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /characters/{id}/chapel/donate - success", func(t *testing.T) {
		req := jsonRequest(t, http.MethodPost, "/characters/c1/chapel/donate", `{"amount":500}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}
