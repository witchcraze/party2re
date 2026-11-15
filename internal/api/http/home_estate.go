package http

import (
	"errors"
	"net/http"
	"strings"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/home"
)

type buildHouseRequest struct {
	CharacterID string `json:"character_id"`
	HouseStyle  string `json:"house_style"`
}

func (h *Handler) handleBuildHouse(w http.ResponseWriter, r *http.Request) {
	if h.homes == nil {
		writeError(w, http.StatusNotImplemented, errors.New("home service not configured"))
		return
	}

	townID := r.PathValue("town_id")
	withAuthenticatedActionCharacterAndJSON(h, w, r, func(req *buildHouseRequest) string {
		return req.CharacterID
	}, func(_ coreplayer.Player, char corecharacter.Character, req buildHouseRequest) {
		result, err := h.homes.BuildHouse(r.Context(), char.ID, townID, req.HouseStyle)
		if err != nil {
			if errors.Is(err, home.ErrInvalidTownID) || errors.Is(err, home.ErrInvalidHouseStyle) {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			if errors.Is(err, home.ErrInsufficientFunds) {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			if errors.Is(err, home.ErrAlreadyOwnsHouse) || errors.Is(err, home.ErrTownMaxHousesReached) {
				writeError(w, http.StatusConflict, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusCreated, result)
	})
}

func (h *Handler) handleListTownHouses(w http.ResponseWriter, r *http.Request) {
	if h.homes == nil {
		writeError(w, http.StatusNotImplemented, errors.New("home service not configured"))
		return
	}

	townID := r.PathValue("town_id")
	houses, err := h.homes.ListTownHouses(r.Context(), townID)
	if err != nil {
		if errors.Is(err, home.ErrInvalidTownID) {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, houses)
}

func (h *Handler) handleCheckHouse(w http.ResponseWriter, r *http.Request) {
	if h.homes == nil {
		writeError(w, http.StatusNotImplemented, errors.New("home service not configured"))
		return
	}

	target := r.URL.Query().Get("target")
	if strings.TrimSpace(target) == "" {
		writeError(w, http.StatusBadRequest, errors.New("target is required"))
		return
	}

	result, err := h.homes.CheckHouse(r.Context(), target)
	if err != nil {
		if errors.Is(err, home.ErrHouseNotFound) || errors.Is(err, home.ErrCharacterNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, result)
}
