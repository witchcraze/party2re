package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/monster"
)

type MonsterService interface {
	GetSummary(ctx context.Context, characterID, locationFilter string) (monster.MonsterBoxSummary, error)
	TameMonster(ctx context.Context, characterID, monsterID, customName string) (monster.MonsterInstance, error)
	BringToHome(ctx context.Context, characterID, instanceID string) (monster.MonsterInstance, error)
	DepositToBox(ctx context.Context, characterID, instanceID string) (monster.MonsterInstance, error)
	Rename(ctx context.Context, characterID, instanceID, newName string) (monster.MonsterInstance, error)
	SendMonster(ctx context.Context, senderCharID, recipientCharID, instanceID string) error
	ReleaseMonster(ctx context.Context, characterID, instanceID string) error
	GetDialogue() monster.Dialogue
}

// WithMonster configures the MonsterService for the Handler.
func WithMonster(m MonsterService) Option {
	return func(h *Handler) {
		h.monster = m
	}
}

type tameMonsterRequest struct {
	MonsterID  string `json:"monster_id"`
	CustomName string `json:"custom_name"`
}

type renameMonsterRequest struct {
	CustomName string `json:"custom_name"`
}

type sendMonsterRequest struct {
	RecipientCharacterID string `json:"recipient_character_id"`
}

func (h *Handler) handleGetMonsterDialogue(w http.ResponseWriter, r *http.Request) {
	if h.monster == nil {
		writeError(w, http.StatusNotImplemented, errors.New("monster service not configured"))
		return
	}
	writeJSON(w, http.StatusOK, h.monster.GetDialogue())
}

func (h *Handler) handleGetCharacterMonsters(w http.ResponseWriter, r *http.Request) {
	if h.monster == nil {
		writeError(w, http.StatusNotImplemented, errors.New("monster service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		location := r.URL.Query().Get("location")
		summary, err := h.monster.GetSummary(r.Context(), char.ID, location)
		if err != nil {
			switch {
			case errors.Is(err, monster.ErrInvalidLocation):
				writeError(w, http.StatusBadRequest, err)
			case errors.Is(err, monster.ErrCharacterNotFound):
				writeError(w, http.StatusNotFound, err)
			default:
				writeError(w, http.StatusInternalServerError, err)
			}
			return
		}

		writeJSON(w, http.StatusOK, summary)
	})
}

func (h *Handler) handleTameMonster(w http.ResponseWriter, r *http.Request) {
	if h.monster == nil {
		writeError(w, http.StatusNotImplemented, errors.New("monster service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req tameMonsterRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, errors.New("invalid request body"))
			return
		}

		inst, err := h.monster.TameMonster(r.Context(), char.ID, req.MonsterID, req.CustomName)
		if err != nil {
			switch {
			case errors.Is(err, monster.ErrBoxFull),
				errors.Is(err, monster.ErrInvalidName),
				errors.Is(err, monster.ErrNameTooLong):
				writeError(w, http.StatusBadRequest, err)
			case errors.Is(err, monster.ErrCharacterNotFound):
				writeError(w, http.StatusNotFound, err)
			default:
				writeError(w, http.StatusInternalServerError, err)
			}
			return
		}

		writeJSON(w, http.StatusOK, inst)
	})
}

func (h *Handler) handleBringMonsterToHome(w http.ResponseWriter, r *http.Request) {
	if h.monster == nil {
		writeError(w, http.StatusNotImplemented, errors.New("monster service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		instanceID := r.PathValue("instance_id")
		if instanceID == "" {
			writeError(w, http.StatusBadRequest, errors.New("instance_id is required"))
			return
		}

		inst, err := h.monster.BringToHome(r.Context(), char.ID, instanceID)
		if err != nil {
			switch {
			case errors.Is(err, monster.ErrHomeFull),
				errors.Is(err, monster.ErrDuplicatePetNameAtHome):
				writeError(w, http.StatusBadRequest, err)
			case errors.Is(err, monster.ErrForbidden):
				writeError(w, http.StatusForbidden, err)
			case errors.Is(err, monster.ErrMonsterNotFound),
				errors.Is(err, monster.ErrCharacterNotFound):
				writeError(w, http.StatusNotFound, err)
			default:
				writeError(w, http.StatusInternalServerError, err)
			}
			return
		}

		writeJSON(w, http.StatusOK, inst)
	})
}

func (h *Handler) handleDepositMonsterToBox(w http.ResponseWriter, r *http.Request) {
	if h.monster == nil {
		writeError(w, http.StatusNotImplemented, errors.New("monster service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		instanceID := r.PathValue("instance_id")
		if instanceID == "" {
			writeError(w, http.StatusBadRequest, errors.New("instance_id is required"))
			return
		}

		inst, err := h.monster.DepositToBox(r.Context(), char.ID, instanceID)
		if err != nil {
			switch {
			case errors.Is(err, monster.ErrBoxFull):
				writeError(w, http.StatusBadRequest, err)
			case errors.Is(err, monster.ErrForbidden):
				writeError(w, http.StatusForbidden, err)
			case errors.Is(err, monster.ErrMonsterNotFound),
				errors.Is(err, monster.ErrCharacterNotFound):
				writeError(w, http.StatusNotFound, err)
			default:
				writeError(w, http.StatusInternalServerError, err)
			}
			return
		}

		writeJSON(w, http.StatusOK, inst)
	})
}

func (h *Handler) handleRenameMonster(w http.ResponseWriter, r *http.Request) {
	if h.monster == nil {
		writeError(w, http.StatusNotImplemented, errors.New("monster service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		instanceID := r.PathValue("instance_id")
		if instanceID == "" {
			writeError(w, http.StatusBadRequest, errors.New("instance_id is required"))
			return
		}

		var req renameMonsterRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, errors.New("invalid request body"))
			return
		}

		inst, err := h.monster.Rename(r.Context(), char.ID, instanceID, req.CustomName)
		if err != nil {
			switch {
			case errors.Is(err, monster.ErrInvalidName),
				errors.Is(err, monster.ErrNameTooLong),
				errors.Is(err, monster.ErrDuplicatePetNameAtHome):
				writeError(w, http.StatusBadRequest, err)
			case errors.Is(err, monster.ErrForbidden):
				writeError(w, http.StatusForbidden, err)
			case errors.Is(err, monster.ErrMonsterNotFound),
				errors.Is(err, monster.ErrCharacterNotFound):
				writeError(w, http.StatusNotFound, err)
			default:
				writeError(w, http.StatusInternalServerError, err)
			}
			return
		}

		writeJSON(w, http.StatusOK, inst)
	})
}

func (h *Handler) handleSendMonster(w http.ResponseWriter, r *http.Request) {
	if h.monster == nil {
		writeError(w, http.StatusNotImplemented, errors.New("monster service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		instanceID := r.PathValue("instance_id")
		if instanceID == "" {
			writeError(w, http.StatusBadRequest, errors.New("instance_id is required"))
			return
		}

		var req sendMonsterRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, errors.New("invalid request body"))
			return
		}

		err := h.monster.SendMonster(r.Context(), char.ID, req.RecipientCharacterID, instanceID)
		if err != nil {
			switch {
			case errors.Is(err, monster.ErrCannotSendToSelf),
				errors.Is(err, monster.ErrRecipientBoxFull):
				writeError(w, http.StatusBadRequest, err)
			case errors.Is(err, monster.ErrForbidden):
				writeError(w, http.StatusForbidden, err)
			case errors.Is(err, monster.ErrRecipientNotFound),
				errors.Is(err, monster.ErrMonsterNotFound),
				errors.Is(err, monster.ErrCharacterNotFound):
				writeError(w, http.StatusNotFound, err)
			default:
				writeError(w, http.StatusInternalServerError, err)
			}
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"message": "Monster sent successfully",
		})
	})
}

func (h *Handler) handleReleaseMonster(w http.ResponseWriter, r *http.Request) {
	if h.monster == nil {
		writeError(w, http.StatusNotImplemented, errors.New("monster service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		instanceID := r.PathValue("instance_id")
		if instanceID == "" {
			writeError(w, http.StatusBadRequest, errors.New("instance_id is required"))
			return
		}

		err := h.monster.ReleaseMonster(r.Context(), char.ID, instanceID)
		if err != nil {
			switch {
			case errors.Is(err, monster.ErrForbidden):
				writeError(w, http.StatusForbidden, err)
			case errors.Is(err, monster.ErrMonsterNotFound),
				errors.Is(err, monster.ErrCharacterNotFound):
				writeError(w, http.StatusNotFound, err)
			default:
				writeError(w, http.StatusInternalServerError, err)
			}
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"message": "Monster released successfully",
		})
	})
}
