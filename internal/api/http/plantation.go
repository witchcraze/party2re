package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/plantation"
)

// PlantationService defines the plantation operations exposed over HTTP.
type PlantationService interface {
	GetStatus(ctx context.Context, characterID string) (plantation.StatusResponse, error)
	Sow(ctx context.Context, characterID string, seedID string) (plantation.SowResult, error)
	Fertilize(ctx context.Context, characterID string, fertilizerID string) (plantation.FertilizeResult, error)
	Harvest(ctx context.Context, characterID string) (plantation.HarvestResult, error)
}

// WithPlantation configures the PlantationService for the HTTP handler.
func WithPlantation(service PlantationService) Option {
	return func(h *Handler) {
		h.plantation = service
	}
}

type plantationSowRequest struct {
	SeedID string `json:"seed_id"`
}

type plantationFertilizeRequest struct {
	FertilizerID string `json:"fertilizer_id"`
}

func (h *Handler) handleGetCharacterPlantation(w http.ResponseWriter, r *http.Request) {
	if h.plantation == nil {
		writeError(w, http.StatusNotImplemented, errors.New("plantation service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		status, err := h.plantation.GetStatus(r.Context(), char.ID)
		if err != nil {
			h.writePlantationError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, status)
	})
}

func (h *Handler) handlePlantationSow(w http.ResponseWriter, r *http.Request) {
	if h.plantation == nil {
		writeError(w, http.StatusNotImplemented, errors.New("plantation service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req plantationSowRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, errors.New("invalid request payload"))
			return
		}

		res, err := h.plantation.Sow(r.Context(), char.ID, req.SeedID)
		if err != nil {
			h.writePlantationError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
}

func (h *Handler) handlePlantationFertilize(w http.ResponseWriter, r *http.Request) {
	if h.plantation == nil {
		writeError(w, http.StatusNotImplemented, errors.New("plantation service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req plantationFertilizeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, errors.New("invalid request payload"))
			return
		}

		res, err := h.plantation.Fertilize(r.Context(), char.ID, req.FertilizerID)
		if err != nil {
			h.writePlantationError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
}

func (h *Handler) handlePlantationHarvest(w http.ResponseWriter, r *http.Request) {
	if h.plantation == nil {
		writeError(w, http.StatusNotImplemented, errors.New("plantation service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		res, err := h.plantation.Harvest(r.Context(), char.ID)
		if err != nil {
			h.writePlantationError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
}

func (h *Handler) writePlantationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, plantation.ErrInvalidCharacterID),
		errors.Is(err, plantation.ErrInvalidSeedID),
		errors.Is(err, plantation.ErrInvalidFertilizerID):
		writeError(w, http.StatusBadRequest, err)
	case errors.Is(err, plantation.ErrNoActivePlot),
		errors.Is(err, plantation.ErrPlotNotFound):
		writeError(w, http.StatusNotFound, err)
	case errors.Is(err, plantation.ErrPlotAlreadySown),
		errors.Is(err, plantation.ErrFertilizerAlreadyApplied),
		errors.Is(err, plantation.ErrCropNotMatured),
		errors.Is(err, plantation.ErrInsufficientGold),
		errors.Is(err, plantation.ErrMissingFertilizerItem):
		writeError(w, http.StatusUnprocessableEntity, err)
	case errors.Is(err, depot.ErrDepotFull):
		writeError(w, http.StatusConflict, err)
	default:
		writeError(w, http.StatusInternalServerError, err)
	}
}
