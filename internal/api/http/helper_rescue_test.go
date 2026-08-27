package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	apihttp "github.com/witchcraze/party2re/internal/api/http"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/helper"
	"github.com/witchcraze/party2re/internal/rescue"
)

type mockHelperService struct {
	listQuestsFn    func(ctx context.Context, now time.Time) ([]helper.Quest, error)
	completeQuestFn func(ctx context.Context, characterID, questID string, now time.Time) (helper.CompletionResult, error)
}

func (m *mockHelperService) ListQuests(ctx context.Context, now time.Time) ([]helper.Quest, error) {
	if m.listQuestsFn != nil {
		return m.listQuestsFn(ctx, now)
	}
	return []helper.Quest{
		{
			ID:            "quest-1",
			Title:         "Test Quest",
			Kind:          helper.KindItem,
			TargetID:      "item-01",
			TargetName:    "Potion",
			RequiredCount: 1,
			RewardItemID:  "item-02",
			ExpiresAt:     now.Add(1 * time.Hour),
		},
	}, nil
}

func (m *mockHelperService) CompleteQuest(ctx context.Context, characterID, questID string, now time.Time) (helper.CompletionResult, error) {
	if m.completeQuestFn != nil {
		return m.completeQuestFn(ctx, characterID, questID, now)
	}
	inv, _ := coreinventory.New(characterID)
	return helper.CompletionResult{
		Character: corecharacter.Character{ID: characterID, Money: 100},
		Inventory: inv,
		CompletedQuest: helper.Quest{
			ID:           questID,
			Title:        "Completed Quest",
			Kind:         helper.KindItem,
			RewardItemID: "item-02",
		},
	}, nil
}

type mockRescueService struct {
	emergencyRescueFn func(ctx context.Context, characterID, reason string, now time.Time) (rescue.RescueRecord, error)
	isUnderPenaltyFn  func(ctx context.Context, characterID string, now time.Time) (bool, time.Duration, error)
}

func (m *mockRescueService) EmergencyRescue(ctx context.Context, characterID, reason string, now time.Time) (rescue.RescueRecord, error) {
	if m.emergencyRescueFn != nil {
		return m.emergencyRescueFn(ctx, characterID, reason, now)
	}
	return rescue.RescueRecord{
		ID:             "rec-1",
		CharacterID:    characterID,
		Reason:         reason,
		PenaltySeconds: 600,
		CreatedAt:      now,
	}, nil
}

func (m *mockRescueService) IsUnderPenalty(ctx context.Context, characterID string, now time.Time) (bool, time.Duration, error) {
	if m.isUnderPenaltyFn != nil {
		return m.isUnderPenaltyFn(ctx, characterID, now)
	}
	return false, 0, nil
}

func TestHelperAndRescueEndpoints(t *testing.T) {
	player := coreplayer.Player{ID: "player-1", Username: "user1"}
	char := corecharacter.Character{ID: "char-1", PlayerID: "player-1", Name: "HelperHero"}
	otherChar := corecharacter.Character{ID: "char-2", PlayerID: "other-player", Name: "OtherHero"}

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
			if id == "char-2" {
				return otherChar, nil
			}
			return corecharacter.Character{}, corecharacter.ErrNotFound
		},
	}
	adv := &stubAdventureService{}
	shopSvc := &stubShopService{}
	helperSvc := &mockHelperService{}
	rescueSvc := &mockRescueService{}

	handler, err := apihttp.NewHandler(
		players,
		chars,
		adv,
		shopSvc,
		apihttp.WithHelper(helperSvc),
		apihttp.WithRescue(rescueSvc),
	)
	if err != nil {
		t.Fatal(err)
	}

	router := handler.Router()

	t.Run("GET /helpers/quests - success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/helpers/quests", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		var quests []helper.Quest
		if err := json.Unmarshal(rec.Body.Bytes(), &quests); err != nil {
			t.Fatalf("failed to unmarshal quests: %v", err)
		}
		if len(quests) != 1 {
			t.Errorf("expected 1 quest, got %d", len(quests))
		}
	})

	t.Run("POST /helpers/complete - unauthorized", func(t *testing.T) {
		body := []byte(`{"character_id":"char-1","quest_id":"quest-1"}`)
		req := httptest.NewRequest(http.MethodPost, "/helpers/complete", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rec.Code)
		}
	})

	t.Run("POST /helpers/complete - forbidden for other player", func(t *testing.T) {
		body := []byte(`{"character_id":"char-2","quest_id":"quest-1"}`)
		req := httptest.NewRequest(http.MethodPost, "/helpers/complete", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer valid-session")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Errorf("expected 403, got %d", rec.Code)
		}
	})

	t.Run("POST /helpers/complete - success", func(t *testing.T) {
		body := []byte(`{"character_id":"char-1","quest_id":"quest-1"}`)
		req := httptest.NewRequest(http.MethodPost, "/helpers/complete", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer valid-session")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
		var result helper.CompletionResult
		if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
			t.Fatalf("failed to unmarshal completion result: %v", err)
		}
		if result.Character.Money != 100 {
			t.Errorf("expected 100 gold, got %d", result.Character.Money)
		}
	})

	t.Run("GET /rescues/penalty - unauthorized", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/rescues/penalty?character_id=char-1", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rec.Code)
		}
	})

	t.Run("GET /rescues/penalty - forbidden for other player", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/rescues/penalty?character_id=char-2", nil)
		req.Header.Set("Authorization", "Bearer valid-session")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Errorf("expected 403, got %d", rec.Code)
		}
	})

	t.Run("GET /rescues/penalty - success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/rescues/penalty?character_id=char-1", nil)
		req.Header.Set("Authorization", "Bearer valid-session")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		var res map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
			t.Fatalf("failed to unmarshal penalty response: %v", err)
		}
		if res["is_under_penalty"] != false {
			t.Errorf("expected is_under_penalty false, got %v", res["is_under_penalty"])
		}
	})

	t.Run("POST /rescues/request - unauthorized", func(t *testing.T) {
		body := []byte(`{"character_id":"char-1","reason":"stuck in loop"}`)
		req := httptest.NewRequest(http.MethodPost, "/rescues/request", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rec.Code)
		}
	})

	t.Run("POST /rescues/request - forbidden for other player", func(t *testing.T) {
		body := []byte(`{"character_id":"char-2","reason":"stuck in loop"}`)
		req := httptest.NewRequest(http.MethodPost, "/rescues/request", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer valid-session")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Errorf("expected 403, got %d", rec.Code)
		}
	})

	t.Run("POST /rescues/request - success", func(t *testing.T) {
		body := []byte(`{"character_id":"char-1","reason":"stuck in loop"}`)
		req := httptest.NewRequest(http.MethodPost, "/rescues/request", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer valid-session")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
		}
		var recRes rescue.RescueRecord
		if err := json.Unmarshal(rec.Body.Bytes(), &recRes); err != nil {
			t.Fatalf("failed to unmarshal rescue record: %v", err)
		}
		if recRes.PenaltySeconds != 600 {
			t.Errorf("expected 600 penalty seconds, got %d", recRes.PenaltySeconds)
		}
	})
}
