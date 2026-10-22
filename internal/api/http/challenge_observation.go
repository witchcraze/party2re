package http

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/witchcraze/party2re/internal/challenge"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
)

func writeChallengeRecoveryError(w http.ResponseWriter, err error) bool {
	if errors.Is(err, challenge.ErrRecoveryRequired) {
		writeError(w, http.StatusServiceUnavailable, challenge.ErrRecoveryRequired)
		return true
	}
	if errors.Is(err, challenge.ErrOutcomeUnknown) {
		writeError(w, http.StatusServiceUnavailable, challenge.ErrOutcomeUnknown)
		return true
	}
	return false
}

func (h *Handler) handleGetActiveChallenge(w http.ResponseWriter, r *http.Request) {
	if h.challenges == nil {
		writeError(w, http.StatusNotImplemented, errors.New("challenge service not configured"))
		return
	}
	h.withAuthenticatedCharacter(w, r, r.PathValue("id"), func(_ coreplayer.Player, char corecharacter.Character) {
		run, err := h.challenges.GetActiveSession(r.Context(), char.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, challengeSessionResponse{Session: run})
	})
}

func (h *Handler) handleGetChallengeCommand(w http.ResponseWriter, r *http.Request) {
	if h.challenges == nil {
		writeError(w, http.StatusNotImplemented, errors.New("challenge service not configured"))
		return
	}
	h.withAuthenticatedCharacter(w, r, r.PathValue("id"), func(_ coreplayer.Player, char corecharacter.Character) {
		revision, err := strconv.Atoi(r.PathValue("revision"))
		if err != nil || revision < 0 {
			writeError(w, http.StatusBadRequest, errors.New("invalid expected revision"))
			return
		}
		result, err := h.challenges.GetCommandResult(r.Context(), char.ID, r.PathValue("session_id"), revision)
		if errors.Is(err, challenge.ErrForbidden) {
			writeError(w, http.StatusNotFound, challenge.ErrForbidden)
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	})
}
