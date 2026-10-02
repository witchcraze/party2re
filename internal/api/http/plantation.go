package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/core/random"
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

type plantationStatusResponse struct {
	Plot        *plantation.Plot        `json:"plot,omitempty"`
	Status      plantation.PlotStatus   `json:"status"`
	Seeds       []plantation.Seed       `json:"seeds"`
	Fertilizers []plantation.Fertilizer `json:"fertilizers"`
	Dialogue    string                  `json:"dialogue"`
}

type plantationSowResponse struct {
	Plot    plantation.Plot `json:"plot"`
	Message string          `json:"message"`
}

type plantationFertilizeResponse struct {
	Plot    plantation.Plot `json:"plot"`
	Message string          `json:"message"`
}

type plantationHarvestResponse struct {
	Withered bool                      `json:"withered"`
	Yields   []plantation.HarvestYield `json:"yields,omitempty"`
	Message  string                    `json:"message"`
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

		dialogue := ""
		if len(plantation.DefaultLotusDialogues) > 0 {
			dialogue = plantation.DefaultLotusDialogues[random.IntN(len(plantation.DefaultLotusDialogues))]
		}

		writeJSON(w, http.StatusOK, plantationStatusResponse{
			Plot:        status.Plot,
			Status:      status.Status,
			Seeds:       status.Seeds,
			Fertilizers: status.Fertilizers,
			Dialogue:    dialogue,
		})
	})
}

func (h *Handler) handlePlantationSow(w http.ResponseWriter, r *http.Request) {
	if h.plantation == nil {
		writeError(w, http.StatusNotImplemented, errors.New("plantation service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedActionCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req plantationSowRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		res, err := h.plantation.Sow(r.Context(), char.ID, req.SeedID)
		if err != nil {
			h.writePlantationError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, plantationSowResponse{
			Plot:    res.Plot,
			Message: fmt.Sprintf("%sをまいたよ！", res.SeedName),
		})
	})
}

func (h *Handler) handlePlantationFertilize(w http.ResponseWriter, r *http.Request) {
	if h.plantation == nil {
		writeError(w, http.StatusNotImplemented, errors.New("plantation service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedActionCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req plantationFertilizeRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		res, err := h.plantation.Fertilize(r.Context(), char.ID, req.FertilizerID)
		if err != nil {
			h.writePlantationError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, plantationFertilizeResponse{
			Plot:    res.Plot,
			Message: fmt.Sprintf("%sをまくよ！", res.FertilizerName),
		})
	})
}

func (h *Handler) handlePlantationHarvest(w http.ResponseWriter, r *http.Request) {
	if h.plantation == nil {
		writeError(w, http.StatusNotImplemented, errors.New("plantation service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedActionCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		res, err := h.plantation.Harvest(r.Context(), char.ID)
		if err != nil {
			h.writePlantationError(w, err)
			return
		}

		var msg string
		if res.Withered {
			msg = fmt.Sprintf("%sは芽が出なかったよ…", res.SeedName)
		} else {
			var msgBuilder strings.Builder
			msgBuilder.WriteString("収穫したよ！\n")
			for _, y := range res.Yields {
				msgBuilder.WriteString(fmt.Sprintf("%sを%d個\n", y.ItemName, y.Quantity))
			}
			msgBuilder.WriteString("倉庫に送っておいたよ")
			msg = msgBuilder.String()
		}

		writeJSON(w, http.StatusOK, plantationHarvestResponse{
			Withered: res.Withered,
			Yields:   res.Yields,
			Message:  msg,
		})
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
