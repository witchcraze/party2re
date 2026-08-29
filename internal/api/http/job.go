package http

import (
	"context"
	"errors"
	"net/http"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	corejob "github.com/witchcraze/party2re/internal/core/job"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
)

// JobService defines the job operations exposed over HTTP.
type JobService interface {
	ListDefinitions() []corejob.Definition
	ChangeJob(ctx context.Context, characterID string, targetJobID string) (corecharacter.Character, corejob.CharacterJob, error)
}

// WithJob configures the job service for the Handler.
func WithJob(jobs JobService) Option {
	return func(h *Handler) {
		h.jobs = jobs
	}
}

type changeJobRequest struct {
	JobID string `json:"job_id"`
}

type changeJobResponse struct {
	Character characterResponse    `json:"character"`
	Job       corejob.CharacterJob `json:"job"`
}

type rebirthResponse struct {
	Character characterResponse `json:"character"`
}

func (h *Handler) handleListJobs(w http.ResponseWriter, r *http.Request) {
	if h.jobs == nil {
		writeError(w, http.StatusNotImplemented, errors.New("job service not configured"))
		return
	}
	jobs := h.jobs.ListDefinitions()
	writeJSON(w, http.StatusOK, jobs)
}

func (h *Handler) handleChangeJob(w http.ResponseWriter, r *http.Request) {
	if h.jobs == nil {
		writeError(w, http.StatusNotImplemented, errors.New("job service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req changeJobRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		if req.JobID == "" {
			writeError(w, http.StatusBadRequest, errors.New("job_id is required"))
			return
		}

		updatedChar, updatedJob, err := h.jobs.ChangeJob(r.Context(), char.ID, req.JobID)
		if err != nil {
			if errors.Is(err, corejob.ErrJobUnavailable) || errors.Is(err, corejob.ErrDefinitionNotFound) {
				writeError(w, http.StatusUnprocessableEntity, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, changeJobResponse{
			Character: toCharacterResponse(updatedChar),
			Job:       updatedJob,
		})
	})
}

func (h *Handler) handleRebirth(w http.ResponseWriter, r *http.Request) {
	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		updatedChar, err := h.characters.Rebirth(r.Context(), char.ID)
		if err != nil {
			writeError(w, http.StatusUnprocessableEntity, err)
			return
		}

		writeJSON(w, http.StatusOK, rebirthResponse{
			Character: toCharacterResponse(updatedChar),
		})
	})
}
