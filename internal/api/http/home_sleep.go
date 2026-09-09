package http

import (
	"encoding/json"
	"errors"
	"net/http"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/home"
)

type sleepRequest struct {
	TargetHomeID string `json:"target_home_id"`
}

type sleepResponse struct {
	Sleeping         bool   `json:"sleeping"`
	DurationSeconds  int    `json:"duration_seconds"`
	RemainingSeconds int    `json:"remaining_seconds"`
	HomeCharacterID  string `json:"home_character_id"`
	Message          string `json:"message"`
}

type sleepStatusResponse struct {
	Sleeping         bool   `json:"sleeping"`
	RemainingSeconds int    `json:"remaining_seconds"`
	CanWake          bool   `json:"can_wake"`
	Message          string `json:"message"`
}

type wakeResponse struct {
	Success   bool              `json:"success"`
	Message   string            `json:"message"`
	Character characterResponse `json:"character"`
}

func (h *Handler) handleHomeSleep(w http.ResponseWriter, r *http.Request) {
	if h.homes == nil {
		writeError(w, http.StatusNotImplemented, errors.New("home service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req sleepRequest
		if r.Body != nil && r.ContentLength > 0 {
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeError(w, http.StatusBadRequest, err)
				return
			}
		}

		res, err := h.homes.Sleep(r.Context(), char.ID, req.TargetHomeID)
		if err != nil {
			if errors.Is(err, home.ErrAlreadySleeping) {
				writeError(w, http.StatusConflict, err)
				return
			}
			if errors.Is(err, home.ErrCharacterNotFound) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, sleepResponse{
			Sleeping:         res.Sleeping,
			DurationSeconds:  res.DurationSeconds,
			RemainingSeconds: res.RemainingSeconds,
			HomeCharacterID:  res.HomeCharacterID,
			Message:          res.Message,
		})
	})
}

func (h *Handler) handleGetHomeSleep(w http.ResponseWriter, r *http.Request) {
	if h.homes == nil {
		writeError(w, http.StatusNotImplemented, errors.New("home service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		status, err := h.homes.GetSleepStatus(r.Context(), char.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, sleepStatusResponse{
			Sleeping:         status.Sleeping,
			RemainingSeconds: status.RemainingSeconds,
			CanWake:          status.CanWake,
			Message:          status.Message,
		})
	})
}

func (h *Handler) handleHomeWake(w http.ResponseWriter, r *http.Request) {
	if h.homes == nil {
		writeError(w, http.StatusNotImplemented, errors.New("home service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		res, err := h.homes.Wake(r.Context(), char.ID)
		if err != nil {
			if errors.Is(err, home.ErrStillSleeping) {
				writeError(w, http.StatusConflict, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, wakeResponse{
			Success:   res.Success,
			Message:   res.Message,
			Character: toCharacterResponse(res.Character),
		})
	})
}
