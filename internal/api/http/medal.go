package http

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/medal"
)

type MedalService interface {
	GetRewards() []medal.Reward
	Claim(ctx context.Context, characterID string, cost int) (corecharacter.Character, coreinventory.Inventory, error)
}

func (h *Handler) GetMedalRewards(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, errors.New("Method not allowed"))
		return
	}
	if h.medals == nil {
		writeError(w, http.StatusNotImplemented, errors.New("Medal service not available"))
		return
	}

	rewards := h.medals.GetRewards()
	writeJSON(w, http.StatusOK, rewards)
}

func (h *Handler) ClaimMedalReward(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, errors.New("Method not allowed"))
		return
	}
	if h.medals == nil {
		writeError(w, http.StatusNotImplemented, errors.New("Medal service not available"))
		return
	}

	// For simplicity, we get character ID from headers or context, but let's assume it's passed somehow.
	// Normally we would parse it from path or session.
	characterID := r.Header.Get("X-Character-ID")
	if characterID == "" {
		writeError(w, http.StatusBadRequest, errors.New("Missing character ID"))
		return
	}

	costStr := r.URL.Query().Get("cost")
	cost, err := strconv.Atoi(costStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, errors.New("Invalid cost"))
		return
	}

	char, inv, err := h.medals.Claim(r.Context(), characterID, cost)
	if errors.Is(err, medal.ErrInsufficientMedals) {
		writeError(w, http.StatusBadRequest, err)
		return
	} else if errors.Is(err, medal.ErrRewardNotFound) {
		writeError(w, http.StatusBadRequest, err)
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"character": char,
		"inventory": inv,
	})
}
