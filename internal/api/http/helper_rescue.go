package http

import (
	"errors"
	"net/http"
	"strings"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
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

	sessionID := sessionIDFromRequest(r)
	if sessionID == "" {
		writeError(w, http.StatusUnauthorized, errors.New("missing session"))
		return
	}
	player, err := h.players.Authenticate(r.Context(), sessionID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, errors.New("invalid session"))
		return
	}

	var req completeHelperQuestRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	char, err := h.characters.Get(r.Context(), req.CharacterID)
	if err != nil {
		if errors.Is(err, corecharacter.ErrNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if char.PlayerID != player.ID {
		writeError(w, http.StatusForbidden, errors.New("forbidden: character belongs to another player"))
		return
	}

	res, err := h.helpers.CompleteQuest(r.Context(), req.CharacterID, req.QuestID, time.Now().UTC())
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}

	writeJSON(w, http.StatusOK, res)
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

	sessionID := sessionIDFromRequest(r)
	if sessionID == "" {
		writeError(w, http.StatusUnauthorized, errors.New("missing session"))
		return
	}
	player, err := h.players.Authenticate(r.Context(), sessionID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, errors.New("invalid session"))
		return
	}

	charID := strings.TrimSpace(r.URL.Query().Get("character_id"))
	if charID == "" {
		writeError(w, http.StatusBadRequest, errors.New("missing character_id parameter"))
		return
	}

	char, err := h.characters.Get(r.Context(), charID)
	if err != nil {
		if errors.Is(err, corecharacter.ErrNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if char.PlayerID != player.ID {
		writeError(w, http.StatusForbidden, errors.New("forbidden: character belongs to another player"))
		return
	}

	isUnderPenalty, remaining, err := h.rescues.IsUnderPenalty(r.Context(), charID, time.Now().UTC())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, rescuePenaltyResponse{
		CharacterID:      charID,
		IsUnderPenalty:   isUnderPenalty,
		RemainingSeconds: int(remaining.Seconds()),
	})
}

func (h *Handler) handleRequestRescue(w http.ResponseWriter, r *http.Request) {
	if h.rescues == nil {
		writeError(w, http.StatusNotImplemented, errors.New("rescue service not available"))
		return
	}

	sessionID := sessionIDFromRequest(r)
	if sessionID == "" {
		writeError(w, http.StatusUnauthorized, errors.New("missing session"))
		return
	}
	player, err := h.players.Authenticate(r.Context(), sessionID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, errors.New("invalid session"))
		return
	}

	var req requestRescueRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	char, err := h.characters.Get(r.Context(), req.CharacterID)
	if err != nil {
		if errors.Is(err, corecharacter.ErrNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if char.PlayerID != player.ID {
		writeError(w, http.StatusForbidden, errors.New("forbidden: character belongs to another player"))
		return
	}

	rec, err := h.rescues.EmergencyRescue(r.Context(), req.CharacterID, req.Reason, time.Now().UTC())
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}

	writeJSON(w, http.StatusOK, rec)
}
