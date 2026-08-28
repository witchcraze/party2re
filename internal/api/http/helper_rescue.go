package http

import (
	"errors"
	"net/http"
	"strings"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/helper"
)

type completeHelperQuestRequest struct {
	CharacterID string `json:"character_id"`
	QuestID     string `json:"quest_id"`
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
		quests = []helper.Quest{}
	}
	writeJSON(w, http.StatusOK, quests)
}

func (h *Handler) handleCompleteHelperQuest(w http.ResponseWriter, r *http.Request) {
	if h.helpers == nil {
		writeError(w, http.StatusNotImplemented, errors.New("helper service not available"))
		return
	}

	withAuthenticatedCharacterAndJSON(h, w, r, func(req *completeHelperQuestRequest) string {
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

type rescuePenaltyResponse struct {
	CharacterID      string `json:"character_id"`
	IsUnderPenalty   bool   `json:"is_under_penalty"`
	RemainingSeconds int    `json:"remaining_seconds"`
}

type requestRescueRequest struct {
	CharacterID string `json:"character_id"`
	Reason      string `json:"reason"`
}

func (h *Handler) handleGetRescuePenalty(w http.ResponseWriter, r *http.Request) {
	if h.rescues == nil {
		writeError(w, http.StatusNotImplemented, errors.New("rescue service not available"))
		return
	}

	charID := strings.TrimSpace(r.URL.Query().Get("character_id"))
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		isUnderPenalty, remaining, err := h.rescues.IsUnderPenalty(r.Context(), char.ID, time.Now().UTC())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, rescuePenaltyResponse{
			CharacterID:      char.ID,
			IsUnderPenalty:   isUnderPenalty,
			RemainingSeconds: int(remaining.Seconds()),
		})
	})
}

func (h *Handler) handleRequestRescue(w http.ResponseWriter, r *http.Request) {
	if h.rescues == nil {
		writeError(w, http.StatusNotImplemented, errors.New("rescue service not available"))
		return
	}

	withAuthenticatedCharacterAndJSON(h, w, r, func(req *requestRescueRequest) string {
		return req.CharacterID
	}, func(_ coreplayer.Player, char corecharacter.Character, req requestRescueRequest) {
		rec, err := h.rescues.EmergencyRescue(r.Context(), char.ID, req.Reason, time.Now().UTC())
		if err != nil {
			writeError(w, http.StatusUnprocessableEntity, err)
			return
		}

		writeJSON(w, http.StatusOK, rec)
	})
}
