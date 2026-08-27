package http

import (
	"context"
	"errors"
	"net/http"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/medal"
)

type MedalService interface {
	GetRewards() []medal.Reward
	Claim(ctx context.Context, characterID string, itemID string) (corecharacter.Character, coreinventory.Inventory, error)
}

// WithMedal configures the MedalService for the handler.
func WithMedal(m MedalService) Option {
	return func(h *Handler) {
		h.medals = m
	}
}

type claimMedalRewardRequest struct {
	CharacterID string `json:"character_id"`
	ItemID      string `json:"item_id"`
}

type claimMedalRewardResponse struct {
	Character characterResponse       `json:"character"`
	Inventory coreinventory.Inventory `json:"inventory"`
}

func (h *Handler) handleGetMedalRewards(w http.ResponseWriter, r *http.Request) {
	if h.medals == nil {
		writeError(w, http.StatusNotImplemented, errors.New("medal service not available"))
		return
	}

	rewards := h.medals.GetRewards()
	writeJSON(w, http.StatusOK, rewards)
}

func (h *Handler) handleClaimMedalReward(w http.ResponseWriter, r *http.Request) {
	if h.medals == nil {
		writeError(w, http.StatusNotImplemented, errors.New("medal service not available"))
		return
	}

	sessionID := sessionIDFromRequest(r)
	if sessionID == "" {
		writeError(w, http.StatusUnauthorized, errors.New("missing session"))
		return
	}
	player, err := h.players.Authenticate(r.Context(), sessionID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, errors.New("invalid session"))
		return
	}

	var req claimMedalRewardRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	char, err := h.characters.Get(r.Context(), req.CharacterID)
	if err != nil {
		if errors.Is(err, corecharacter.ErrNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if char.PlayerID != player.ID {
		writeError(w, http.StatusForbidden, errors.New("forbidden: character belongs to another player"))
		return
	}

	updatedChar, updatedInv, err := h.medals.Claim(r.Context(), req.CharacterID, req.ItemID)
	if err != nil {
		if errors.Is(err, medal.ErrInsufficientMedals) {
			writeError(w, http.StatusUnprocessableEntity, err)
			return
		}
		if errors.Is(err, medal.ErrRewardNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, claimMedalRewardResponse{
		Character: toCharacterResponse(updatedChar),
		Inventory: updatedInv,
	})
}
