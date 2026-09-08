package http

import (
	"context"
	"errors"
	"net/http"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	corejob "github.com/witchcraze/party2re/internal/core/job"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	jobapp "github.com/witchcraze/party2re/internal/job"
)

// JobService defines the job operations exposed over HTTP.
type JobService interface {
	ListDefinitions() []corejob.Definition
	ChangeJob(ctx context.Context, characterID string, targetJobID string) (corecharacter.Character, corejob.CharacterJob, error)
	ExchangeJob(ctx context.Context, characterID string, targetJobID, targetOldJobID string) (corecharacter.Character, corejob.CharacterJob, error)
	SaveFutureMemory(ctx context.Context, characterID string) (corecharacter.FutureMemory, error)
	RecallFutureMemory(ctx context.Context, characterID, memoryID string) (corecharacter.Character, corejob.CharacterJob, error)
	ListFutureMemories(ctx context.Context, characterID string) ([]corecharacter.FutureMemory, error)
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

type exchangeJobRequest struct {
	JobID    string `json:"job_id"`
	OldJobID string `json:"old_job_id"`
}

type recallFutureMemoryRequest struct {
	MemoryID string `json:"memory_id"`
}

type changeJobResponse struct {
	Character characterResponse    `json:"character"`
	Job       corejob.CharacterJob `json:"job"`
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
			if errors.Is(err, corejob.ErrJobUnavailable) || errors.Is(err, corejob.ErrDefinitionNotFound) ||
				errors.Is(err, jobapp.ErrRequiredItem) {
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

func (h *Handler) handleExchangeJob(w http.ResponseWriter, r *http.Request) {
	if h.jobs == nil {
		writeError(w, http.StatusNotImplemented, errors.New("job service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req exchangeJobRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		updatedChar, updatedJob, err := h.jobs.ExchangeJob(r.Context(), char.ID, req.JobID, req.OldJobID)
		if err != nil {
			if errors.Is(err, corejob.ErrJobUnavailable) || errors.Is(err, corejob.ErrDefinitionNotFound) ||
				errors.Is(err, jobapp.ErrRequiredItem) {
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

func (h *Handler) handleListFutureMemories(w http.ResponseWriter, r *http.Request) {
	if h.jobs == nil {
		writeError(w, http.StatusNotImplemented, errors.New("job service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		memories, err := h.jobs.ListFutureMemories(r.Context(), char.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if memories == nil {
			memories = []corecharacter.FutureMemory{}
		}
		writeJSON(w, http.StatusOK, memories)
	})
}

func (h *Handler) handleSaveFutureMemory(w http.ResponseWriter, r *http.Request) {
	if h.jobs == nil {
		writeError(w, http.StatusNotImplemented, errors.New("job service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		snapshot, err := h.jobs.SaveFutureMemory(r.Context(), char.ID)
		if err != nil {
			if errors.Is(err, corejob.ErrJobUnavailable) || errors.Is(err, jobapp.ErrRequiredItem) ||
				err.Error() == "future memory slot limit reached" {
				writeError(w, http.StatusUnprocessableEntity, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusCreated, snapshot)
	})
}

func (h *Handler) handleRecallFutureMemory(w http.ResponseWriter, r *http.Request) {
	if h.jobs == nil {
		writeError(w, http.StatusNotImplemented, errors.New("job service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req recallFutureMemoryRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		if req.MemoryID == "" {
			writeError(w, http.StatusBadRequest, errors.New("memory_id is required"))
			return
		}

		updatedChar, updatedJob, err := h.jobs.RecallFutureMemory(r.Context(), char.ID, req.MemoryID)
		if err != nil {
			if errors.Is(err, corejob.ErrJobUnavailable) || err.Error() == "future memory not found" {
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
