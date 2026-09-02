package http

import (
	"errors"
	"net/http"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/pagination"
)

func (h *Handler) handleListCharacterAdventures(w http.ResponseWriter, r *http.Request) {
	if h.adventures == nil {
		writeError(w, http.StatusNotImplemented, errors.New("adventure service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		if r.URL != nil && r.URL.Query().Has("cursor") {
			cursorParams := pagination.ParseCursorRequest(r)
			res, err := h.adventures.ListHistoryByCursor(r.Context(), char.ID, cursorParams.Limit, cursorParams.Cursor)
			if err != nil {
				if errors.Is(err, corecharacter.ErrNotFound) {
					writeError(w, http.StatusNotFound, err)
					return
				}
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			writeJSON(w, http.StatusOK, res)
			return
		}

		params := pagination.ParseRequest(r)

		res, err := h.adventures.ListHistory(r.Context(), char.ID, params.Limit, params.Offset)
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
