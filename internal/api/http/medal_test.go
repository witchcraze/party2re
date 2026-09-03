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
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/medal"
)

type mockMedalService struct {
	getRewardsFn         func() []medal.Reward
	claimFn              func(ctx context.Context, charID string, itemID string) (corecharacter.Character, coreinventory.Inventory, error)
	getAchievementsFn    func(ctx context.Context, charID string) ([]medal.AchievementProgress, error)
	claimAchievementFn   func(ctx context.Context, charID string, achievementID string) (medal.ClaimResult, error)
	getCharacterMedalsFn func(ctx context.Context, charID string) ([]medal.CharacterMedal, error)
}

func (m *mockMedalService) GetRewards() []medal.Reward {
	if m.getRewardsFn != nil {
		return m.getRewardsFn()
	}
	return []medal.Reward{
		{Cost: 3, ItemID: "armor-32"},
		{Cost: 10, ItemID: "weapon-32"},
	}
}

func (m *mockMedalService) Claim(ctx context.Context, charID string, itemID string) (corecharacter.Character, coreinventory.Inventory, error) {
	if m.claimFn != nil {
		return m.claimFn(ctx, charID, itemID)
	}
	inv, _ := coreinventory.New(charID)
	return corecharacter.Character{ID: charID, SmallMedals: 2}, inv, nil
}

func (m *mockMedalService) GetAchievements(ctx context.Context, charID string) ([]medal.AchievementProgress, error) {
	if m.getAchievementsFn != nil {
		return m.getAchievementsFn(ctx, charID)
	}
	return []medal.AchievementProgress{
		{
			ID:                   "adv_novice",
			Name:                 "冒険の第一歩",
			CurrentProgress:      1,
			Threshold:            1,
			CompletionPercentage: 100,
			IsCompleted:          true,
			MedalID:              "medal_adv_bronze",
			MedalName:            "青銅の冒険勲章",
			SmallMedalsReward:    1,
		},
	}, nil
}

func (m *mockMedalService) ClaimAchievement(ctx context.Context, charID string, achievementID string) (medal.ClaimResult, error) {
	if m.claimAchievementFn != nil {
		return m.claimAchievementFn(ctx, charID, achievementID)
	}
	return medal.ClaimResult{
		AchievementID:   achievementID,
		AchievementName: "冒険の第一歩",
		Medal: medal.CharacterMedal{
			CharacterID: charID,
			MedalID:     "medal_adv_bronze",
			MedalName:   "青銅の冒険勲章",
		},
		SmallMedalsAwarded: 1,
		Character:          corecharacter.Character{ID: charID, SmallMedals: 6},
	}, nil
}

func (m *mockMedalService) GetCharacterMedals(ctx context.Context, charID string) ([]medal.CharacterMedal, error) {
	if m.getCharacterMedalsFn != nil {
		return m.getCharacterMedalsFn(ctx, charID)
	}
	return []medal.CharacterMedal{
		{
			CharacterID: charID,
			MedalID:     "medal_adv_bronze",
			MedalName:   "青銅の冒険勲章",
			Category:    "adventure",
		},
	}, nil
}

func TestMedalEndpoints(t *testing.T) {
	player := coreplayer.Player{ID: "player-1", Username: "user1"}
	char := corecharacter.Character{ID: "char-1", PlayerID: "player-1", Name: "MedalHero", SmallMedals: 5}
	otherChar := corecharacter.Character{ID: "char-2", PlayerID: "other-player", Name: "OtherHero", SmallMedals: 5}

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
	medalSvc := &mockMedalService{}

	handler, err := apihttp.NewHandler(
		players,
		chars,
		adv,
		shopSvc,
		apihttp.WithMedal(medalSvc),
	)
	if err != nil {
		t.Fatal(err)
	}

	router := handler.Router()

	t.Run("GET /medals/rewards", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/medals/rewards", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		var rewards []medal.Reward
		if err := json.Unmarshal(rec.Body.Bytes(), &rewards); err != nil {
			t.Fatalf("failed to unmarshal rewards: %v", err)
		}
		if len(rewards) != 2 {
			t.Errorf("expected 2 rewards, got %d", len(rewards))
		}
	})

	t.Run("POST /medals/claim - unauthorized", func(t *testing.T) {
		payload := []byte(`{"character_id":"char-1","item_id":"armor-32"}`)
		req := httptest.NewRequest(http.MethodPost, "/medals/claim", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rec.Code)
		}
	})

	t.Run("POST /medals/claim - forbidden character ownership", func(t *testing.T) {
		payload := []byte(`{"character_id":"char-2","item_id":"armor-32"}`)
		req := httptest.NewRequest(http.MethodPost, "/medals/claim", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer valid-session")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected 403, got %d", rec.Code)
		}
	})

	t.Run("POST /medals/claim - insufficient medals", func(t *testing.T) {
		mockSvc := &mockMedalService{
			claimFn: func(ctx context.Context, charID, itemID string) (corecharacter.Character, coreinventory.Inventory, error) {
				return corecharacter.Character{}, coreinventory.Inventory{}, medal.ErrInsufficientMedals
			},
		}
		h, _ := apihttp.NewHandler(players, chars, adv, shopSvc, apihttp.WithMedal(mockSvc))
		r := h.Router()

		payload := []byte(`{"character_id":"char-1","item_id":"weapon-32"}`)
		req := httptest.NewRequest(http.MethodPost, "/medals/claim", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer valid-session")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422, got %d", rec.Code)
		}
	})

	t.Run("POST /medals/claim - reward not found", func(t *testing.T) {
		mockSvc := &mockMedalService{
			claimFn: func(ctx context.Context, charID, itemID string) (corecharacter.Character, coreinventory.Inventory, error) {
				return corecharacter.Character{}, coreinventory.Inventory{}, medal.ErrRewardNotFound
			},
		}
		h, _ := apihttp.NewHandler(players, chars, adv, shopSvc, apihttp.WithMedal(mockSvc))
		r := h.Router()

		payload := []byte(`{"character_id":"char-1","item_id":"non-existent"}`)
		req := httptest.NewRequest(http.MethodPost, "/medals/claim", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer valid-session")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
	})

	t.Run("POST /medals/claim - success", func(t *testing.T) {
		payload := []byte(`{"character_id":"char-1","item_id":"armor-32"}`)
		req := httptest.NewRequest(http.MethodPost, "/medals/claim", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer valid-session")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
	})

	t.Run("GET /characters/{id}/achievements - unauthorized", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/characters/char-1/achievements", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rec.Code)
		}
	})

	t.Run("GET /characters/{id}/achievements - forbidden other character", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/characters/char-2/achievements", nil)
		req.Header.Set("Authorization", "Bearer valid-session")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected 403, got %d", rec.Code)
		}
	})

	t.Run("GET /characters/{id}/achievements - success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/characters/char-1/achievements", nil)
		req.Header.Set("Authorization", "Bearer valid-session")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		var resp struct {
			Achievements []medal.AchievementProgress `json:"achievements"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if len(resp.Achievements) != 1 || resp.Achievements[0].ID != "adv_novice" {
			t.Fatalf("unexpected achievements response: %+v", resp)
		}
	})

	t.Run("POST /characters/{id}/achievements/{achievement_id}/claim - success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/characters/char-1/achievements/adv_novice/claim", nil)
		req.Header.Set("Authorization", "Bearer valid-session")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		var resp struct {
			AchievementID      string `json:"achievement_id"`
			SmallMedalsAwarded int    `json:"small_medals_awarded"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp.AchievementID != "adv_novice" || resp.SmallMedalsAwarded != 1 {
			t.Fatalf("unexpected claim response: %+v", resp)
		}
	})

	t.Run("POST /characters/{id}/achievements/{achievement_id}/claim - not found", func(t *testing.T) {
		mockSvc := &mockMedalService{
			claimAchievementFn: func(ctx context.Context, charID string, achievementID string) (medal.ClaimResult, error) {
				return medal.ClaimResult{}, medal.ErrAchievementNotFound
			},
		}
		h, _ := apihttp.NewHandler(players, chars, adv, shopSvc, apihttp.WithMedal(mockSvc))
		r := h.Router()

		req := httptest.NewRequest(http.MethodPost, "/characters/char-1/achievements/invalid/claim", nil)
		req.Header.Set("Authorization", "Bearer valid-session")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
	})

	t.Run("POST /characters/{id}/achievements/{achievement_id}/claim - already claimed (409 Conflict)", func(t *testing.T) {
		mockSvc := &mockMedalService{
			claimAchievementFn: func(ctx context.Context, charID string, achievementID string) (medal.ClaimResult, error) {
				return medal.ClaimResult{}, medal.ErrAchievementAlreadyClaimed
			},
		}
		h, _ := apihttp.NewHandler(players, chars, adv, shopSvc, apihttp.WithMedal(mockSvc))
		r := h.Router()

		req := httptest.NewRequest(http.MethodPost, "/characters/char-1/achievements/adv_novice/claim", nil)
		req.Header.Set("Authorization", "Bearer valid-session")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d", rec.Code)
		}
	})

	t.Run("POST /characters/{id}/achievements/{achievement_id}/claim - not completed (422 Unprocessable)", func(t *testing.T) {
		mockSvc := &mockMedalService{
			claimAchievementFn: func(ctx context.Context, charID string, achievementID string) (medal.ClaimResult, error) {
				return medal.ClaimResult{}, medal.ErrAchievementNotCompleted
			},
		}
		h, _ := apihttp.NewHandler(players, chars, adv, shopSvc, apihttp.WithMedal(mockSvc))
		r := h.Router()

		req := httptest.NewRequest(http.MethodPost, "/characters/char-1/achievements/adv_novice/claim", nil)
		req.Header.Set("Authorization", "Bearer valid-session")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422, got %d", rec.Code)
		}
	})

	t.Run("GET /characters/{id}/medals - success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/characters/char-1/medals", nil)
		req.Header.Set("Authorization", "Bearer valid-session")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		var resp struct {
			Medals []medal.CharacterMedal `json:"medals"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if len(resp.Medals) != 1 || resp.Medals[0].MedalID != "medal_adv_bronze" {
			t.Fatalf("unexpected medals response: %+v", resp)
		}
	})
}
