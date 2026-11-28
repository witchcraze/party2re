package http

import (
	"errors"
	"net/http"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/dungeon"
)

func (h *Handler) handleMineDungeon(w http.ResponseWriter, r *http.Request) {
	if h.dungeons == nil {
		writeError(w, http.StatusNotImplemented, errors.New("dungeon service not configured"))
		return
	}
	h.withAuthenticatedActionCharacter(w, r, r.PathValue("id"), func(_ coreplayer.Player, char corecharacter.Character) {
		res, err := h.dungeons.Mine(r.Context(), char.ID)
		if err != nil {
			if writeCombatAdmissionError(w, err) {
				return
			}
			switch {
			case errors.Is(err, dungeon.ErrNoActiveExpedition):
				writeError(w, http.StatusNotFound, err)
			case errors.Is(err, dungeon.ErrNoMiningSite):
				writeError(w, http.StatusConflict, err)
			default:
				writeError(w, http.StatusInternalServerError, err)
			}
			return
		}
		writeJSON(w, http.StatusOK, dungeonStepResponse{Result: toDungeonStepResult(res)})
	})
}
