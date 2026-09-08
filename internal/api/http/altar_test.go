package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/altar"
	apihttp "github.com/witchcraze/party2re/internal/api/http"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
)

type stubAltarService struct {
	getStatusFn func(ctx context.Context, characterID string) (altar.AltarStatus, error)
	prayFn      func(ctx context.Context, characterID string) (altar.PrayResult, error)
	wishFn      func(ctx context.Context, characterID string, itemID string) (altar.WishResult, error)
	offerOrbFn  func(ctx context.Context, characterID string, orb rune) (altar.OfferResult, error)
}

func (s *stubAltarService) GetStatus(ctx context.Context, characterID string) (altar.AltarStatus, error) {
	if s.getStatusFn != nil {
		return s.getStatusFn(ctx, characterID)
	}
	return altar.AltarStatus{CharacterID: characterID}, nil
}

func (s *stubAltarService) Pray(ctx context.Context, characterID string) (altar.PrayResult, error) {
	if s.prayFn != nil {
		return s.prayFn(ctx, characterID)
	}
	return altar.PrayResult{CharacterID: characterID, RamiaAwakened: true}, nil
}

func (s *stubAltarService) Wish(ctx context.Context, characterID string, itemID string) (altar.WishResult, error) {
	if s.wishFn != nil {
		return s.wishFn(ctx, characterID, itemID)
	}
	return altar.WishResult{CharacterID: characterID, ItemID: itemID, DeliveredTo: "inventory"}, nil
}

func (s *stubAltarService) OfferOrb(ctx context.Context, characterID string, orb rune) (altar.OfferResult, error) {
	if s.offerOrbFn != nil {
		return s.offerOrbFn(ctx, characterID, orb)
	}
	return altar.OfferResult{CharacterID: characterID, Orb: orb}, nil
}

func TestAltarEndpoints(t *testing.T) {
	player := coreplayer.Player{ID: "p1", Username: "hero"}
	char := corecharacter.Character{ID: "c1", PlayerID: "p1", Name: "Hero"}

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
	aService := &stubAltarService{
		getStatusFn: func(_ context.Context, characterID string) (altar.AltarStatus, error) {
			return altar.AltarStatus{
				CharacterID:  characterID,
				Orbs:         "srbgyp",
				OrbCount:     6,
				HasAllOrbs:   true,
				MikoDialogue: altar.MsgMikoReadyToPray,
			}, nil
		},
		prayFn: func(_ context.Context, characterID string) (altar.PrayResult, error) {
			return altar.PrayResult{
				CharacterID:   characterID,
				Message:       altar.MsgRamiaAwakening,
				RamiaAwakened: true,
				ExpiresAt:     time.Now().Add(30 * time.Minute),
			}, nil
		},
		wishFn: func(_ context.Context, characterID string, itemID string) (altar.WishResult, error) {
			if itemID == "invalid" {
				return altar.WishResult{}, altar.ErrInvalidWishItem
			}
			return altar.WishResult{
				CharacterID: characterID,
				ItemID:      itemID,
				ItemName:    "真実の鏡",
				DeliveredTo: "inventory",
				Message:     "真実の鏡 ですね。",
			}, nil
		},
		offerOrbFn: func(_ context.Context, characterID string, orb rune) (altar.OfferResult, error) {
			if orb == 's' {
				return altar.OfferResult{CharacterID: characterID, Orb: orb, Message: "シルバーオーブを復活の祭壇にささげた！"}, nil
			}
			if orb == 'y' {
				return altar.OfferResult{}, altar.ErrOrbItemNotFound
			}
			return altar.OfferResult{CharacterID: characterID, Orb: orb, Message: altar.MsgAlreadyOffered}, altar.ErrOrbAlreadyOffered
		},
	}

	handler := newTestHandler(
		t,
		pService,
		cService,
		&stubAdventureService{},
		&stubShopService{},
		apihttp.WithAltar(aService),
	)
	router := handler.Router()

	// 1. GET /characters/c1/altar
	t.Run("GET Altar Status", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/characters/c1/altar", nil)
		req.Header.Set("Authorization", "Bearer valid_token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rec.Code)
		}
	})

	// 2. POST /characters/c1/altar/offer (success)
	t.Run("POST Altar Offer success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/characters/c1/altar/offer", strings.NewReader(`{"orb":"silver"}`))
		req.Header.Set("Authorization", "Bearer valid_token")
		req.Header.Set("Authorization", "Bearer test-session")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	// 3. POST /characters/c1/altar/offer (conflict / already offered)
	t.Run("POST Altar Offer already offered", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/characters/c1/altar/offer", strings.NewReader(`{"orb":"red"}`))
		req.Header.Set("Authorization", "Bearer valid_token")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusConflict {
			t.Fatalf("expected status 409, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	// 4. POST /characters/c1/altar/offer (missing inventory item)
	t.Run("POST Altar Offer missing item", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/characters/c1/altar/offer", strings.NewReader(`{"orb":"yellow"}`))
		req.Header.Set("Authorization", "Bearer test-session")
		req.Header.Set("Authorization", "Bearer test-session")
		req.Header.Set("Content-Type", "application/json")
		/*
				req.Header.Set("Authorization", "Bearer test-session")
			req := httptest.NewRequest(http.MethodPost, "/characters/c1/altar/offer", strings.NewReader(`{"orb":"yellow"}`))
			req.Header.Set("Authorization", "Bearer ******")
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

		*/
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	// 5. POST /characters/c1/altar/pray (success)
	t.Run("POST Altar Pray success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/characters/c1/altar/pray", nil)
		req.Header.Set("Authorization", "Bearer valid_token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	// 6. POST /characters/c1/altar/wish (success)
	t.Run("POST Altar Wish success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/characters/c1/altar/wish", strings.NewReader(`{"item_id":"item-066"}`))
		req.Header.Set("Authorization", "Bearer valid_token")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	// 7. POST /characters/c1/altar/wish (invalid item)
	t.Run("POST Altar Wish invalid", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/characters/c1/altar/wish", strings.NewReader(`{"item_id":"invalid"}`))
		req.Header.Set("Authorization", "Bearer valid_token")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}
