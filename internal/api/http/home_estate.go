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
	withAuthenticatedCharacterAndJSON(h, w, r, func(req *buildHouseRequest) string {
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

type setCharacterColorRequest struct {
	Color string `json:"color"`
}

func (h *Handler) handleSetCharacterColor(w http.ResponseWriter, r *http.Request) {
	if h.homes == nil {
		writeError(w, http.StatusNotImplemented, errors.New("home service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req setCharacterColorRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		if err := h.homes.SetCharacterColor(r.Context(), char.ID, req.Color); err != nil {
			if errors.Is(err, corecharacter.ErrInvalidColor) {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{"color": req.Color})
	})
}

func (h *Handler) handleListHomeItems(w http.ResponseWriter, r *http.Request) {
	if h.homes == nil {
		writeError(w, http.StatusNotImplemented, errors.New("home service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		items, err := h.homes.ListHomeItems(r.Context(), char.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, items)
	})
}

type useHomeItemRequest struct {
	InstanceID string `json:"instance_id"`
	Source     string `json:"source"`
}

func (h *Handler) handleUseHomeItem(w http.ResponseWriter, r *http.Request) {
	if h.homes == nil {
		writeError(w, http.StatusNotImplemented, errors.New("home service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req useHomeItemRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		result, err := h.homes.UseHomeItem(r.Context(), char.ID, req.InstanceID, req.Source)
		if err != nil {
			if errors.Is(err, home.ErrItemNotFound) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			if errors.Is(err, home.ErrCannotUseHere) {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, result)
	})
}
