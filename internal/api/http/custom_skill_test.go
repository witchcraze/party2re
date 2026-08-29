package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	apihttp "github.com/witchcraze/party2re/internal/api/http"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/custom_skill"
)

type stubCustomSkillService struct {
	getLoadoutFn         func(ctx context.Context, characterID string) (*custom_skill.CharacterSkillLoadout, error)
	getAvailableSkillsFn func(ctx context.Context, characterID string) ([]custom_skill.SkillEntry, error)
	equipSkillFn         func(ctx context.Context, characterID string, slotIndex int, skillID string, priority int) (*custom_skill.CharacterSkillLoadout, error)
	unequipSlotFn        func(ctx context.Context, characterID string, slotIndex int) (*custom_skill.CharacterSkillLoadout, error)
	listCatalogFn        func() []custom_skill.SkillEntry
}

func (s *stubCustomSkillService) GetLoadout(ctx context.Context, characterID string) (*custom_skill.CharacterSkillLoadout, error) {
	if s.getLoadoutFn != nil {
		return s.getLoadoutFn(ctx, characterID)
	}
	return &custom_skill.CharacterSkillLoadout{CharacterID: characterID}, nil
}

func (s *stubCustomSkillService) GetAvailableSkills(ctx context.Context, characterID string) ([]custom_skill.SkillEntry, error) {
	if s.getAvailableSkillsFn != nil {
		return s.getAvailableSkillsFn(ctx, characterID)
	}
	return nil, nil
}

func (s *stubCustomSkillService) EquipSkill(ctx context.Context, characterID string, slotIndex int, skillID string, priority int) (*custom_skill.CharacterSkillLoadout, error) {
	if s.equipSkillFn != nil {
		return s.equipSkillFn(ctx, characterID, slotIndex, skillID, priority)
	}
	return &custom_skill.CharacterSkillLoadout{CharacterID: characterID}, nil
}

func (s *stubCustomSkillService) UnequipSlot(ctx context.Context, characterID string, slotIndex int) (*custom_skill.CharacterSkillLoadout, error) {
	if s.unequipSlotFn != nil {
		return s.unequipSlotFn(ctx, characterID, slotIndex)
	}
	return &custom_skill.CharacterSkillLoadout{CharacterID: characterID}, nil
}

func (s *stubCustomSkillService) ListCatalog() []custom_skill.SkillEntry {
	if s.listCatalogFn != nil {
		return s.listCatalogFn()
	}
	return nil
}

func TestCustomSkillEndpoints(t *testing.T) {
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
	csService := &stubCustomSkillService{
		getLoadoutFn: func(_ context.Context, characterID string) (*custom_skill.CharacterSkillLoadout, error) {
			return &custom_skill.CharacterSkillLoadout{
				CharacterID: characterID,
				Slots: []custom_skill.EquippedSkillSlot{
					{SlotIndex: 1, SkillID: "fireball", Priority: 5},
				},
			}, nil
		},
		getAvailableSkillsFn: func(_ context.Context, characterID string) ([]custom_skill.SkillEntry, error) {
			return []custom_skill.SkillEntry{
				{ID: "fireball", Name: "Fireball"},
			}, nil
		},
		equipSkillFn: func(_ context.Context, characterID string, slotIndex int, skillID string, priority int) (*custom_skill.CharacterSkillLoadout, error) {
			return &custom_skill.CharacterSkillLoadout{
				CharacterID: characterID,
				Slots: []custom_skill.EquippedSkillSlot{
					{SlotIndex: slotIndex, SkillID: skillID, Priority: priority},
				},
			}, nil
		},
		unequipSlotFn: func(_ context.Context, characterID string, slotIndex int) (*custom_skill.CharacterSkillLoadout, error) {
			return &custom_skill.CharacterSkillLoadout{
				CharacterID: characterID,
				Slots:       []custom_skill.EquippedSkillSlot{},
			}, nil
		},
	}

	h := newTestHandler(
		t,
		pService,
		cService,
		&stubAdventureService{},
		&stubShopService{},
		apihttp.WithCustomSkill(csService),
	)
	router := h.Router()

	t.Run("GET /characters/{id}/custom-skills - success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/characters/c1/custom-skills", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /characters/{id}/custom-skills - equip success", func(t *testing.T) {
		req := jsonRequest(t, http.MethodPost, "/characters/c1/custom-skills", `{"slot_index":1,"skill_id":"fireball","priority":5}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
	})

	t.Run("DELETE /characters/{id}/custom-skills/{slot} - unequip success", func(t *testing.T) {
		req := jsonRequest(t, http.MethodDelete, "/characters/c1/custom-skills/1", "")
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
	})
}
