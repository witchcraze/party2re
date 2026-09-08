package http

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/custom_skill"
)

// CustomSkillService defines the custom skill loadout operations exposed over HTTP.
type CustomSkillService interface {
	GetLoadout(ctx context.Context, characterID string) (*custom_skill.CharacterSkillLoadout, error)
	GetAvailableSkills(ctx context.Context, characterID string) ([]custom_skill.SkillEntry, error)
	EquipSkill(ctx context.Context, characterID string, slotIndex int, skillID string, priority int) (*custom_skill.CharacterSkillLoadout, error)
	UnequipSlot(ctx context.Context, characterID string, slotIndex int) (*custom_skill.CharacterSkillLoadout, error)
	ListCatalog() []custom_skill.SkillEntry
}

type gemSynthesisService interface {
	GetCustomSkill(ctx context.Context, characterID string) (*custom_skill.CustomSkill, error)
	SetCustomSkill(ctx context.Context, characterID, name, comment string, gems [3]string) (*custom_skill.CustomSkill, error)
}

// WithCustomSkill configures the custom skill service for the Handler.
func WithCustomSkill(skills CustomSkillService) Option {
	return func(h *Handler) {
		h.customSkills = skills
	}
}

type getCustomSkillsResponse struct {
	Loadout         *custom_skill.CharacterSkillLoadout `json:"loadout"`
	AvailableSkills []custom_skill.SkillEntry           `json:"available_skills"`
	CustomSkill     *custom_skill.CustomSkill           `json:"custom_skill,omitempty"`
}

type equipCustomSkillRequest struct {
	SlotIndex int       `json:"slot_index"`
	SkillID   string    `json:"skill_id,omitempty"`
	Priority  int       `json:"priority,omitempty"`
	Action    string    `json:"action,omitempty"` // "equip" (default) or "unequip"
	Name      string    `json:"name,omitempty"`
	Comment   string    `json:"comment,omitempty"`
	Gems      [3]string `json:"gems,omitempty"`
	Gem1      string    `json:"gem1,omitempty"`
	Gem2      string    `json:"gem2,omitempty"`
	Gem3      string    `json:"gem3,omitempty"`
}

type equipCustomSkillResponse struct {
	Loadout *custom_skill.CharacterSkillLoadout `json:"loadout"`
}

func (h *Handler) handleGetCustomSkills(w http.ResponseWriter, r *http.Request) {
	if h.customSkills == nil {
		writeError(w, http.StatusNotImplemented, errors.New("custom skill service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		loadout, err := h.customSkills.GetLoadout(r.Context(), char.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		available, err := h.customSkills.GetAvailableSkills(r.Context(), char.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		response := getCustomSkillsResponse{
			Loadout:         loadout,
			AvailableSkills: available,
		}
		if synthesis, ok := h.customSkills.(gemSynthesisService); ok {
			response.CustomSkill, err = synthesis.GetCustomSkill(r.Context(), char.ID)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
		}
		writeJSON(w, http.StatusOK, response)
	})
}

func (h *Handler) handleEquipCustomSkill(w http.ResponseWriter, r *http.Request) {
	if h.customSkills == nil {
		writeError(w, http.StatusNotImplemented, errors.New("custom skill service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req equipCustomSkillRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		if req.Name != "" || req.Comment != "" || req.Gem1 != "" || req.Gem2 != "" || req.Gem3 != "" {
			synthesis, ok := h.customSkills.(gemSynthesisService)
			if !ok {
				writeError(w, http.StatusNotImplemented, custom_skill.ErrGemDependencies)
				return
			}
			gems := req.Gems
			if req.Gem1 != "" {
				gems[0] = req.Gem1
			}
			if req.Gem2 != "" {
				gems[1] = req.Gem2
			}
			if req.Gem3 != "" {
				gems[2] = req.Gem3
			}
			result, err := synthesis.SetCustomSkill(r.Context(), char.ID, req.Name, req.Comment, gems)
			if err != nil {
				switch {
				case errors.Is(err, custom_skill.ErrInvalidSkillName),
					errors.Is(err, custom_skill.ErrInvalidSkillComment),
					errors.Is(err, custom_skill.ErrTooManyGemSlots),
					errors.Is(err, custom_skill.ErrCMPTooHigh),
					errors.Is(err, custom_skill.ErrGemNotOwned),
					errors.Is(err, custom_skill.ErrGemNotFound):
					writeError(w, http.StatusBadRequest, err)
				default:
					writeError(w, http.StatusInternalServerError, err)
				}
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"custom_skill": result})
			return
		}

		if req.Action == "unequip" || req.SkillID == "" {
			loadout, err := h.customSkills.UnequipSlot(r.Context(), char.ID, req.SlotIndex)
			if err != nil {
				if errors.Is(err, custom_skill.ErrSlotOutOfBounds) {
					writeError(w, http.StatusBadRequest, err)
					return
				}
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			writeJSON(w, http.StatusOK, equipCustomSkillResponse{Loadout: loadout})
			return
		}

		if req.Priority <= 0 {
			req.Priority = 5
		}

		loadout, err := h.customSkills.EquipSkill(r.Context(), char.ID, req.SlotIndex, req.SkillID, req.Priority)
		if err != nil {
			if errors.Is(err, custom_skill.ErrSkillNotFound) ||
				errors.Is(err, custom_skill.ErrSlotOutOfBounds) ||
				errors.Is(err, custom_skill.ErrInvalidPriority) {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			if errors.Is(err, custom_skill.ErrSkillNotLearned) ||
				errors.Is(err, custom_skill.ErrLevelTooLow) ||
				errors.Is(err, custom_skill.ErrDuplicateSkillEquip) {
				writeError(w, http.StatusUnprocessableEntity, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, equipCustomSkillResponse{Loadout: loadout})
	})
}

func (h *Handler) handleUnequipCustomSkill(w http.ResponseWriter, r *http.Request) {
	if h.customSkills == nil {
		writeError(w, http.StatusNotImplemented, errors.New("custom skill service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		slotStr := r.PathValue("slot")
		slotIndex, err := strconv.Atoi(slotStr)
		if err != nil || slotIndex <= 0 {
			writeError(w, http.StatusBadRequest, errors.New("invalid slot index"))
			return
		}

		loadout, err := h.customSkills.UnequipSlot(r.Context(), char.ID, slotIndex)
		if err != nil {
			if errors.Is(err, custom_skill.ErrSlotOutOfBounds) {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, equipCustomSkillResponse{Loadout: loadout})
	})
}
