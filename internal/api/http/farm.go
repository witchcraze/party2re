package http

import (
	"context"
	"errors"
	"net/http"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/farm"
)

// FarmService defines the plantation operations exposed over HTTP.
type FarmService interface {
	GetPlots(ctx context.Context, characterID string) ([]farm.FarmPlot, error)
	PlantSeed(ctx context.Context, characterID string, plotIndex int, seedType farm.SeedType) (farm.FarmPlot, error)
	WaterPlot(ctx context.Context, characterID string, plotIndex int) (farm.FarmPlot, error)
	FertilizePlot(ctx context.Context, characterID string, plotIndex int) (farm.FarmPlot, error)
	HarvestPlot(ctx context.Context, characterID string, plotIndex int) (farm.HarvestResult, farm.FarmPlot, error)
	ClearPlot(ctx context.Context, characterID string, plotIndex int) (farm.FarmPlot, error)
}

// WithFarm configures the farm service for the Handler.
func WithFarm(f FarmService) Option {
	return func(h *Handler) {
		h.farm = f
	}
}

type getFarmResponse struct {
	Plots []farm.FarmPlot `json:"plots"`
}

type farmPlotRequest struct {
	PlotIndex int    `json:"plot_index"`
	SeedType  string `json:"seed_type,omitempty"`
}

type farmPlotResponse struct {
	Plot farm.FarmPlot `json:"plot"`
}

type farmHarvestResponse struct {
	Result farm.HarvestResult `json:"result"`
	Plot   farm.FarmPlot      `json:"plot"`
}

func (h *Handler) handleGetFarm(w http.ResponseWriter, r *http.Request) {
	if h.farm == nil {
		writeError(w, http.StatusNotImplemented, errors.New("farm service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		plots, err := h.farm.GetPlots(r.Context(), char.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, getFarmResponse{
			Plots: plots,
		})
	})
}

func (h *Handler) handleFarmPlant(w http.ResponseWriter, r *http.Request) {
	if h.farm == nil {
		writeError(w, http.StatusNotImplemented, errors.New("farm service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req farmPlotRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		plot, err := h.farm.PlantSeed(r.Context(), char.ID, req.PlotIndex, farm.SeedType(req.SeedType))
		if err != nil {
			if errors.Is(err, farm.ErrInvalidPlotIndex) || errors.Is(err, farm.ErrInvalidSeedType) {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			if errors.Is(err, farm.ErrPlotNotEmpty) || errors.Is(err, farm.ErrInsufficientSeed) {
				writeError(w, http.StatusUnprocessableEntity, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, farmPlotResponse{Plot: plot})
	})
}

func (h *Handler) handleFarmWater(w http.ResponseWriter, r *http.Request) {
	if h.farm == nil {
		writeError(w, http.StatusNotImplemented, errors.New("farm service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req farmPlotRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		plot, err := h.farm.WaterPlot(r.Context(), char.ID, req.PlotIndex)
		if err != nil {
			if errors.Is(err, farm.ErrInvalidPlotIndex) {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			if errors.Is(err, farm.ErrPlotNotGrowing) || errors.Is(err, farm.ErrPlotWithered) || errors.Is(err, farm.ErrAlreadyWatered) {
				writeError(w, http.StatusUnprocessableEntity, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, farmPlotResponse{Plot: plot})
	})
}

func (h *Handler) handleFarmFertilize(w http.ResponseWriter, r *http.Request) {
	if h.farm == nil {
		writeError(w, http.StatusNotImplemented, errors.New("farm service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req farmPlotRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		plot, err := h.farm.FertilizePlot(r.Context(), char.ID, req.PlotIndex)
		if err != nil {
			if errors.Is(err, farm.ErrInvalidPlotIndex) {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			if errors.Is(err, farm.ErrPlotNotGrowing) || errors.Is(err, farm.ErrPlotWithered) || errors.Is(err, farm.ErrAlreadyFertilized) || errors.Is(err, farm.ErrInsufficientFertilizer) {
				writeError(w, http.StatusUnprocessableEntity, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, farmPlotResponse{Plot: plot})
	})
}

func (h *Handler) handleFarmHarvest(w http.ResponseWriter, r *http.Request) {
	if h.farm == nil {
		writeError(w, http.StatusNotImplemented, errors.New("farm service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req farmPlotRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		res, plot, err := h.farm.HarvestPlot(r.Context(), char.ID, req.PlotIndex)
		if err != nil {
			if errors.Is(err, farm.ErrInvalidPlotIndex) {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			if errors.Is(err, farm.ErrPlotNotGrowing) || errors.Is(err, farm.ErrPlotNotMature) || errors.Is(err, farm.ErrPlotWithered) {
				writeError(w, http.StatusUnprocessableEntity, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, farmHarvestResponse{
			Result: res,
			Plot:   plot,
		})
	})
}

func (h *Handler) handleFarmClear(w http.ResponseWriter, r *http.Request) {
	if h.farm == nil {
		writeError(w, http.StatusNotImplemented, errors.New("farm service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req farmPlotRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		plot, err := h.farm.ClearPlot(r.Context(), char.ID, req.PlotIndex)
		if err != nil {
			if errors.Is(err, farm.ErrInvalidPlotIndex) {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, farmPlotResponse{Plot: plot})
	})
}
