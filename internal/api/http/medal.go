package http

import (
	"context"
	"errors"
	"net/http"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
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

	withAuthenticatedCharacterAndJSON(h, w, r, func(req *claimMedalRewardRequest) string {
		return req.CharacterID
	}, func(_ coreplayer.Player, char corecharacter.Character, req claimMedalRewardRequest) {
		updatedChar, updatedInv, err := h.medals.Claim(r.Context(), char.ID, req.ItemID)
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
	})
}
