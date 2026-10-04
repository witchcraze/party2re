package http

import (
	"context"
	"errors"
	"net/http"

	"github.com/witchcraze/party2re/internal/adventure"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

type adventureActionParams struct {
	StageID string `json:"stage_id"`
}

func adventureStartCommand(service AdventureService) Option {
	return withActionCommand("adventure_start", func(ctx context.Context, actorID string, params adventureActionParams) (any, error) {
		value, err := service.StartStage(ctx, actorID, params.StageID)
		if err != nil {
			return nil, err
		}
		return toAdventureResponse(value), nil
	}, adventureActionRejection)
}

func adventureActionRejection(err error) (int, ErrorDetail) {
	switch {
	case errors.Is(err, adventure.ErrStageNotFound):
		return http.StatusUnprocessableEntity, ErrorDetail{Code: "ADVENTURE_STAGE_NOT_FOUND", Message: "Adventure stage not found."}
	case errors.Is(err, adventure.ErrLevelRequirementNotMet):
		return http.StatusForbidden, ErrorDetail{Code: "ADVENTURE_LEVEL_REQUIRED", Message: "Adventure level requirement not met."}
	case errors.Is(err, adventure.ErrJobLevelRequirementNotMet):
		return http.StatusForbidden, ErrorDetail{Code: "ADVENTURE_JOB_LEVEL_REQUIRED", Message: "Adventure job level requirement not met."}
	case errors.Is(err, adventure.ErrCharacterUnconscious):
		return http.StatusUnprocessableEntity, ErrorDetail{Code: "ADVENTURE_UNCONSCIOUS", Message: "Character must recover HP before adventuring."}
	case errors.Is(err, adventure.ErrCharacterExhausted):
		return http.StatusUnprocessableEntity, ErrorDetail{Code: "ADVENTURE_EXHAUSTED", Message: "Character must rest before adventuring."}
	case errors.Is(err, adventure.ErrDailyOnceAlreadyUsed):
		return http.StatusUnprocessableEntity, ErrorDetail{Code: "ADVENTURE_DAILY_LIMIT", Message: "Daily adventure entry already used."}
	case errors.Is(err, corecharacter.ErrNotFound):
		return http.StatusNotFound, ErrorDetail{Code: "CHARACTER_NOT_FOUND", Message: "Character not found."}
	default:
		return 0, ErrorDetail{}
	}
}
