package http

import (
	"errors"
	"net/http"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/helperquest"
)

type completeHelperQuestRequest struct {
	CharacterID string `json:"character_id"`
	QuestID     string `json:"quest_id"` // Omitted or empty selects renewal without a turn-in.
}

func (h *Handler) handleListHelperQuests(w http.ResponseWriter, r *http.Request) {
	if h.helpers == nil {
		writeError(w, http.StatusNotImplemented, errors.New("helper service not available"))
		return
	}

	quests, err := h.helpers.ListQuests(r.Context(), time.Now().UTC())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if quests == nil {
		quests = []helperquest.Quest{}
	}
	writeJSON(w, http.StatusOK, quests)
}

func (h *Handler) handleCompleteHelperQuest(w http.ResponseWriter, r *http.Request) {
	if h.helpers == nil {
		writeError(w, http.StatusNotImplemented, errors.New("helper service not available"))
		return
	}

	withAuthenticatedActionCharacterAndJSON(h, w, r, func(req *completeHelperQuestRequest) string {
		return req.CharacterID
	}, func(_ coreplayer.Player, char corecharacter.Character, req completeHelperQuestRequest) {
		res, err := h.helpers.CompleteQuest(r.Context(), char.ID, req.QuestID, time.Now().UTC())
		if err != nil {
			writeError(w, http.StatusUnprocessableEntity, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
}
