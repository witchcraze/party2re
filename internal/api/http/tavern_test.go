package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	apihttp "github.com/witchcraze/party2re/internal/api/http"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/tavern"
)

type stubTavernService struct {
	getMenuFn         func() []tavern.MenuItem
	getStatusFn       func(ctx context.Context, characterID string) (tavern.TavernStatus, error)
	orderMealFn       func(ctx context.Context, characterID string, itemID string) (tavern.OrderResult, error)
	reserveDeliveryFn func(ctx context.Context, characterID string, itemID string) (tavern.DeliveryReservation, error)
	getDeliveryFn     func(ctx context.Context, characterID string) (tavern.DeliveryReservation, error)
	cancelDeliveryFn  func(ctx context.Context, characterID string) error
	claimDeliveryFn   func(ctx context.Context, characterID string) (tavern.OrderResult, error)
	talkFn            func(ctx context.Context, characterID string) (tavern.TalkResult, error)
	resetFullnessFn   func(ctx context.Context, characterID string) error
}

func (s *stubTavernService) GetMenu() []tavern.MenuItem {
	if s.getMenuFn != nil {
		return s.getMenuFn()
	}
	return []tavern.MenuItem{
		{
			ID:          "tavern_curry",
			Name:        "スパイス香る特製カレー",
			Category:    "Food",
			Price:       400,
			HPHeal:      250,
			MPHeal:      0,
			Tickets:     5,
			Description: "カレー",
		},
	}
}

func (s *stubTavernService) GetStatus(ctx context.Context, characterID string) (tavern.TavernStatus, error) {
	if s.getStatusFn != nil {
		return s.getStatusFn(ctx, characterID)
	}
	return tavern.TavernStatus{
		CharacterID:   characterID,
		CharacterName: "Hero",
		LocationName:  tavern.LocationName,
		NPCName:       tavern.NPCName,
		Gold:          1000,
		HP:            50,
		MaxHP:         100,
		MP:            20,
		MaxMP:         50,
		IsFull:        false,
	}, nil
}

func (s *stubTavernService) OrderMeal(ctx context.Context, characterID string, itemID string) (tavern.OrderResult, error) {
	if s.orderMealFn != nil {
		return s.orderMealFn(ctx, characterID, itemID)
	}
	return tavern.OrderResult{
		CharacterID:    characterID,
		Item:           tavern.MenuItem{ID: itemID, Name: "スパイス香る特製カレー", Price: 400},
		HPHealed:       50,
		MPHealed:       0,
		CurrentHP:      100,
		CurrentMP:      20,
		RemainingGold:  600,
		TicketsAwarded: 5,
		TotalTickets:   5,
		Message:        "美味しく召し上がれ！",
	}, nil
}

func (s *stubTavernService) ReserveDelivery(ctx context.Context, characterID string, itemID string) (tavern.DeliveryReservation, error) {
	if s.reserveDeliveryFn != nil {
		return s.reserveDeliveryFn(ctx, characterID, itemID)
	}
	return tavern.DeliveryReservation{
		CharacterID: characterID,
		ItemID:      itemID,
		ItemName:    "ふわとろオムライス",
		Price:       750,
		HPHeal:      500,
		MPHeal:      100,
		Tickets:     7,
		CreatedAt:   time.Now().UTC(),
	}, nil
}

func (s *stubTavernService) GetDelivery(ctx context.Context, characterID string) (tavern.DeliveryReservation, error) {
	if s.getDeliveryFn != nil {
		return s.getDeliveryFn(ctx, characterID)
	}
	return tavern.DeliveryReservation{
		CharacterID: characterID,
		ItemID:      "tavern_omelet_rice",
		ItemName:    "ふわとろオムライス",
		Price:       750,
		CreatedAt:   time.Now().UTC(),
	}, nil
}

func (s *stubTavernService) CancelDelivery(ctx context.Context, characterID string) error {
	if s.cancelDeliveryFn != nil {
		return s.cancelDeliveryFn(ctx, characterID)
	}
	return nil
}

func (s *stubTavernService) ClaimDelivery(ctx context.Context, characterID string) (tavern.OrderResult, error) {
	if s.claimDeliveryFn != nil {
		return s.claimDeliveryFn(ctx, characterID)
	}
	return tavern.OrderResult{
		CharacterID:   characterID,
		Item:          tavern.MenuItem{ID: "tavern_omelet_rice", Name: "ふわとろオムライス", Price: 750},
		HPHealed:      80,
		MPHealed:      30,
		CurrentHP:     100,
		CurrentMP:     50,
		RemainingGold: 250,
		Message:       "配達完了！",
	}, nil
}

func (s *stubTavernService) Talk(ctx context.Context, characterID string) (tavern.TalkResult, error) {
	if s.talkFn != nil {
		return s.talkFn(ctx, characterID)
	}
	return tavern.TalkResult{
		CharacterID:  characterID,
		LocationName: tavern.LocationName,
		NPCName:      tavern.NPCName,
		Message:      "いらっしゃい！冒険者の酒場へようこそ。",
	}, nil
}

func (s *stubTavernService) ResetFullness(ctx context.Context, characterID string) error {
	if s.resetFullnessFn != nil {
		return s.resetFullnessFn(ctx, characterID)
	}
	return nil
}

func setupTavernTestHandler(t *testing.T, tavernSvc *stubTavernService) (*apihttp.Handler, string) {
	player := coreplayer.Player{ID: "p1", Username: "hero"}
	char := corecharacter.Character{ID: "c1", PlayerID: "p1", Name: "Hero", Level: 10, Money: 1000}

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

	h := newTestHandler(
		t,
		pService,
		cService,
		&stubAdventureService{},
		&stubShopService{},
		apihttp.WithTavern(tavernSvc),
	)
	return h, "c1"
}

func TestTavern_GetMenuHTTP(t *testing.T) {
	h, _ := setupTavernTestHandler(t, &stubTavernService{})
	router := h.Router()

	req := httptest.NewRequest(http.MethodGet, "/tavern/menu", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "tavern_curry") {
		t.Errorf("expected menu item in response, got %s", rec.Body.String())
	}
}

func TestTavern_GetStatusHTTP(t *testing.T) {
	h, charID := setupTavernTestHandler(t, &stubTavernService{})
	router := h.Router()

	req := httptest.NewRequest(http.MethodGet, "/characters/"+charID+"/tavern", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "冒険者の酒場") {
		t.Errorf("expected tavern status in response, got %s", rec.Body.String())
	}
}

func TestTavern_OrderMealHTTP(t *testing.T) {
	h, charID := setupTavernTestHandler(t, &stubTavernService{})
	router := h.Router()

	body := `{"item_id":"tavern_curry"}`
	req := httptest.NewRequest(http.MethodPost, "/characters/"+charID+"/tavern/order", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer valid-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "美味しく召し上がれ") {
		t.Errorf("expected order result in response, got %s", rec.Body.String())
	}
}

func TestTavern_DeliveryHTTP(t *testing.T) {
	h, charID := setupTavernTestHandler(t, &stubTavernService{})
	router := h.Router()

	// 1. Reserve delivery
	body := `{"item_id":"tavern_omelet_rice"}`
	req := httptest.NewRequest(http.MethodPost, "/characters/"+charID+"/tavern/delivery", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer valid-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}

	// 2. Get delivery
	reqGet := httptest.NewRequest(http.MethodGet, "/characters/"+charID+"/tavern/delivery", nil)
	reqGet.Header.Set("Authorization", "Bearer valid-token")
	recGet := httptest.NewRecorder()
	router.ServeHTTP(recGet, reqGet)

	if recGet.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", recGet.Code, recGet.Body.String())
	}

	// 3. Claim delivery
	reqClaim := httptest.NewRequest(http.MethodPost, "/characters/"+charID+"/tavern/delivery/claim", nil)
	reqClaim.Header.Set("Authorization", "Bearer valid-token")
	recClaim := httptest.NewRecorder()
	router.ServeHTTP(recClaim, reqClaim)

	if recClaim.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", recClaim.Code, recClaim.Body.String())
	}

	// 4. Cancel delivery
	reqDel := httptest.NewRequest(http.MethodDelete, "/characters/"+charID+"/tavern/delivery", nil)
	reqDel.Header.Set("Authorization", "Bearer valid-token")
	recDel := httptest.NewRecorder()
	router.ServeHTTP(recDel, reqDel)

	if recDel.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", recDel.Code, recDel.Body.String())
	}
}

func TestTavern_TalkHTTP(t *testing.T) {
	h, charID := setupTavernTestHandler(t, &stubTavernService{})
	router := h.Router()

	req := httptest.NewRequest(http.MethodPost, "/characters/"+charID+"/tavern/talk", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "@エレナ") {
		t.Errorf("expected barkeep dialogue in response, got %s", rec.Body.String())
	}
}
