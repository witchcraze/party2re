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
	"github.com/witchcraze/party2re/internal/delivery"
)

type stubDeliveryService struct {
	getAvailableQuestsFn           func(ctx context.Context, now time.Time) ([]delivery.Quest, error)
	getCharacterDeliveriesFn       func(ctx context.Context, characterID string) ([]delivery.CharacterDelivery, error)
	getActiveCharacterDeliveriesFn func(ctx context.Context, characterID string) ([]delivery.CharacterDelivery, error)
	acceptQuestFn                  func(ctx context.Context, characterID string, questID string, now time.Time) (*delivery.CharacterDelivery, error)
	completeDeliveryFn             func(ctx context.Context, characterID string, deliveryID string, now time.Time) (*delivery.DeliveryCompletionResult, error)
	cancelDeliveryFn               func(ctx context.Context, characterID string, deliveryID string) error
	sendParcelFn                   func(ctx context.Context, senderID string, req delivery.SendParcelRequest, now time.Time) (*delivery.Parcel, error)
	getIncomingParcelsFn           func(ctx context.Context, recipientID string) ([]delivery.Parcel, error)
	claimParcelFn                  func(ctx context.Context, recipientID string, parcelID string, now time.Time) (*delivery.ParcelClaimResult, error)
	cancelParcelFn                 func(ctx context.Context, senderID string, parcelID string) error
}

func (s *stubDeliveryService) GetAvailableQuests(ctx context.Context, now time.Time) ([]delivery.Quest, error) {
	if s.getAvailableQuestsFn != nil {
		return s.getAvailableQuestsFn(ctx, now)
	}
	return []delivery.Quest{
		{
			ID:               "quest-1",
			ClientName:       "薬草師のミレイユ",
			ClientMessage:    "薬草を届けてください",
			TargetItemID:     "item-001",
			TargetItemName:   "薬草",
			RequiredQuantity: 3,
			RecipientName:    "見習い調合師",
			Destination:      "薬草研究所",
			RewardGold:       180,
			RewardExp:        90,
		},
	}, nil
}

func (s *stubDeliveryService) GetCharacterDeliveries(ctx context.Context, characterID string) ([]delivery.CharacterDelivery, error) {
	if s.getCharacterDeliveriesFn != nil {
		return s.getCharacterDeliveriesFn(ctx, characterID)
	}
	return []delivery.CharacterDelivery{
		{
			ID:          "del-1",
			CharacterID: characterID,
			QuestID:     "quest-1",
			Status:      delivery.StatusInProgress,
		},
	}, nil
}

func (s *stubDeliveryService) GetActiveCharacterDeliveries(ctx context.Context, characterID string) ([]delivery.CharacterDelivery, error) {
	if s.getActiveCharacterDeliveriesFn != nil {
		return s.getActiveCharacterDeliveriesFn(ctx, characterID)
	}
	return []delivery.CharacterDelivery{
		{
			ID:          "del-1",
			CharacterID: characterID,
			QuestID:     "quest-1",
			Status:      delivery.StatusInProgress,
		},
	}, nil
}

func (s *stubDeliveryService) AcceptQuest(ctx context.Context, characterID string, questID string, now time.Time) (*delivery.CharacterDelivery, error) {
	if s.acceptQuestFn != nil {
		return s.acceptQuestFn(ctx, characterID, questID, now)
	}
	return &delivery.CharacterDelivery{
		ID:          "del-new",
		CharacterID: characterID,
		QuestID:     questID,
		Status:      delivery.StatusInProgress,
		AcceptedAt:  now,
	}, nil
}

func (s *stubDeliveryService) CompleteDelivery(ctx context.Context, characterID string, deliveryID string, now time.Time) (*delivery.DeliveryCompletionResult, error) {
	if s.completeDeliveryFn != nil {
		return s.completeDeliveryFn(ctx, characterID, deliveryID, now)
	}
	return &delivery.DeliveryCompletionResult{
		DeliveryID:   deliveryID,
		QuestID:      "quest-1",
		RewardedGold: 180,
		RewardedExp:  90,
		CurrentGold:  1180,
		CurrentExp:   590,
	}, nil
}

func (s *stubDeliveryService) CancelDelivery(ctx context.Context, characterID string, deliveryID string) error {
	if s.cancelDeliveryFn != nil {
		return s.cancelDeliveryFn(ctx, characterID, deliveryID)
	}
	return nil
}

func (s *stubDeliveryService) SendParcel(ctx context.Context, senderID string, req delivery.SendParcelRequest, now time.Time) (*delivery.Parcel, error) {
	if s.sendParcelFn != nil {
		return s.sendParcelFn(ctx, senderID, req, now)
	}
	return &delivery.Parcel{
		ID:                   "parcel-new",
		SenderCharacterID:    senderID,
		SenderCharacterName:  "Hero",
		RecipientCharacterID: req.RecipientCharacterID,
		GoldAmount:           req.GoldAmount,
		CourierFee:           50,
		Status:               delivery.ParcelStatusPending,
		CreatedAt:            now,
	}, nil
}

func (s *stubDeliveryService) GetIncomingParcels(ctx context.Context, recipientID string) ([]delivery.Parcel, error) {
	if s.getIncomingParcelsFn != nil {
		return s.getIncomingParcelsFn(ctx, recipientID)
	}
	return []delivery.Parcel{
		{
			ID:                   "parcel-1",
			SenderCharacterID:    "c2",
			SenderCharacterName:  "SenderFriend",
			RecipientCharacterID: recipientID,
			GoldAmount:           500,
			Status:               delivery.ParcelStatusPending,
		},
	}, nil
}

func (s *stubDeliveryService) ClaimParcel(ctx context.Context, recipientID string, parcelID string, now time.Time) (*delivery.ParcelClaimResult, error) {
	if s.claimParcelFn != nil {
		return s.claimParcelFn(ctx, recipientID, parcelID, now)
	}
	return &delivery.ParcelClaimResult{
		ParcelID:    parcelID,
		SenderName:  "SenderFriend",
		GoldAmount:  500,
		CurrentGold: 1500,
	}, nil
}

func (s *stubDeliveryService) CancelParcel(ctx context.Context, senderID string, parcelID string) error {
	if s.cancelParcelFn != nil {
		return s.cancelParcelFn(ctx, senderID, parcelID)
	}
	return nil
}

func TestDeliveryEndpoints(t *testing.T) {
	player := coreplayer.Player{ID: "p1", Username: "hero"}
	char := corecharacter.Character{ID: "c1", PlayerID: "p1", Name: "Hero", Level: 10, Money: 5000}

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
	delService := &stubDeliveryService{}

	handler := newTestHandler(
		t,
		pService,
		cService,
		&stubAdventureService{},
		&stubShopService{},
		apihttp.WithDelivery(delService),
	)

	router := handler.Router()

	// 1. GET /characters/c1/delivery/quests
	req := httptest.NewRequest(http.MethodGet, "/characters/c1/delivery/quests", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for get delivery quests, got %d: %s", rr.Code, rr.Body.String())
	}

	// 2. GET /characters/c1/delivery/active
	req = httptest.NewRequest(http.MethodGet, "/characters/c1/delivery/active", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for get active deliveries, got %d: %s", rr.Code, rr.Body.String())
	}

	// 3. POST /characters/c1/delivery/accept
	body := `{"quest_id": "quest-1"}`
	req = httptest.NewRequest(http.MethodPost, "/characters/c1/delivery/accept", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer valid-token")
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for accept delivery quest, got %d: %s", rr.Code, rr.Body.String())
	}

	// 4. POST /characters/c1/delivery/complete
	body = `{"delivery_id": "del-1"}`
	req = httptest.NewRequest(http.MethodPost, "/characters/c1/delivery/complete", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer valid-token")
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for complete delivery, got %d: %s", rr.Code, rr.Body.String())
	}

	// 5. POST /characters/c1/delivery/cancel
	body = `{"delivery_id": "del-1"}`
	req = httptest.NewRequest(http.MethodPost, "/characters/c1/delivery/cancel", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer valid-token")
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for cancel delivery, got %d: %s", rr.Code, rr.Body.String())
	}

	// 6. POST /characters/c1/delivery/parcels/send
	body = `{"recipient_character_id": "c2", "gold_amount": 200, "message": "hello"}`
	req = httptest.NewRequest(http.MethodPost, "/characters/c1/delivery/parcels/send", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer valid-token")
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201 for send parcel, got %d: %s", rr.Code, rr.Body.String())
	}

	// 7. GET /characters/c1/delivery/parcels/incoming
	req = httptest.NewRequest(http.MethodGet, "/characters/c1/delivery/parcels/incoming", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for incoming parcels, got %d: %s", rr.Code, rr.Body.String())
	}

	// 8. POST /characters/c1/delivery/parcels/claim
	body = `{"parcel_id": "parcel-1"}`
	req = httptest.NewRequest(http.MethodPost, "/characters/c1/delivery/parcels/claim", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer valid-token")
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for claim parcel, got %d: %s", rr.Code, rr.Body.String())
	}

	// 9. POST /characters/c1/delivery/parcels/cancel
	body = `{"parcel_id": "parcel-1"}`
	req = httptest.NewRequest(http.MethodPost, "/characters/c1/delivery/parcels/cancel", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer valid-token")
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for cancel parcel, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestDeliveryErrorMappings(t *testing.T) {
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
	delService := &stubDeliveryService{
		acceptQuestFn: func(ctx context.Context, characterID string, questID string, now time.Time) (*delivery.CharacterDelivery, error) {
			return nil, delivery.ErrMaxActiveDeliveries
		},
	}

	handler := newTestHandler(
		t,
		pService,
		cService,
		&stubAdventureService{},
		&stubShopService{},
		apihttp.WithDelivery(delService),
	)
	router := handler.Router()

	body := `{"quest_id": "quest-1"}`
	req := httptest.NewRequest(http.MethodPost, "/characters/c1/delivery/accept", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer valid-token")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for ErrMaxActiveDeliveries, got %d", rr.Code)
	}
}
