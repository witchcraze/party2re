package http

import (
	"errors"
	"net/http"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/dungeon"
)

func (h *Handler) handleFightDungeon(w http.ResponseWriter, r *http.Request) {
	if h.dungeons == nil {
		writeError(w, http.StatusNotImplemented, errors.New("dungeon service not configured"))
		return
	}
	h.withAuthenticatedActionCharacter(w, r, r.PathValue("id"), func(_ coreplayer.Player, char corecharacter.Character) {
		res, err := h.dungeons.Fight(r.Context(), char.ID)
		if err != nil {
			if writeCombatAdmissionError(w, err) {
				return
			}
			if errors.Is(err, dungeon.ErrNoActiveExpedition) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, dungeonStepResponse{Result: res})
	})
}
