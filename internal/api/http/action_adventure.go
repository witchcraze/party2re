package http

import (
	"errors"
	"net/http"

	"github.com/witchcraze/party2re/internal/adventure"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

func adventureActionRejection(err error) (int, ErrorDetail) {
	switch {
	case errors.Is(err, adventure.ErrInvalidQuestName):
		return http.StatusBadRequest, ErrorDetail{Code: "ADVENTURE_INVALID_NAME", Message: "Quest name must be 1 to 50 characters without whitespace or prohibited characters."}
	case errors.Is(err, adventure.ErrStageNotFound):
		return http.StatusUnprocessableEntity, ErrorDetail{Code: "ADVENTURE_STAGE_NOT_FOUND", Message: "Adventure stage not found."}
	case errors.Is(err, adventure.ErrStageNotAvailable):
		return http.StatusUnprocessableEntity, ErrorDetail{Code: "ADVENTURE_STAGE_NOT_AVAILABLE", Message: "Adventure stage is not currently available."}
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
