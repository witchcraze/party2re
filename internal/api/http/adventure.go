package http

import (
	"errors"
	"net/http"
	"strconv"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
)

func (h *Handler) handleListCharacterAdventures(w http.ResponseWriter, r *http.Request) {
	if h.adventures == nil {
		writeError(w, http.StatusNotImplemented, errors.New("adventure service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		limit := 20
		offset := 0
		if l := r.URL.Query().Get("limit"); l != "" {
			if val, err := strconv.Atoi(l); err == nil {
				limit = val
			}
		}
		if o := r.URL.Query().Get("offset"); o != "" {
			if val, err := strconv.Atoi(o); err == nil {
				offset = val
			}
		}

		res, err := h.adventures.ListHistory(r.Context(), char.ID, limit, offset)
		if err != nil {
			if errors.Is(err, corecharacter.ErrNotFound) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
}

func (h *Handler) handleGetAdventureChronicle(w http.ResponseWriter, r *http.Request) {
	if h.adventures == nil {
		writeError(w, http.StatusNotImplemented, errors.New("adventure service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		chronicle, err := h.adventures.GetChronicle(r.Context(), char.ID)
		if err != nil {
			if errors.Is(err, corecharacter.ErrNotFound) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, chronicle)
	})
}
