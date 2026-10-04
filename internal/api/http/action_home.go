package http

import (
	"context"
	"errors"
	"net/http"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/home"
)

// WithHome configures the HomeService and its sleep/wake Gateway commands.
func WithHome(service HomeService) Option {
	return func(h *Handler) {
		h.homes = service
		homeSleepCommand(service)(h)
		homeWakeCommand(service)(h)
	}
}

func homeSleepCommand(service HomeService) Option {
	if service == nil {
		return withActionCommand[sleepRequest]("home_sleep", nil, nil)
	}
	return withActionCommand("home_sleep", func(ctx context.Context, actorID string, params sleepRequest) (any, error) {
		value, err := service.Sleep(ctx, actorID, params.TargetHomeID)
		if err != nil {
			return nil, err
		}
		return toSleepResponse(value), nil
	}, homeActionRejection)
}

func homeWakeCommand(service HomeService) Option {
	if service == nil {
		return withActionCommand[struct{}]("home_wake", nil, nil)
	}
	return withActionCommand("home_wake", func(ctx context.Context, actorID string, _ struct{}) (any, error) {
		value, err := service.Wake(ctx, actorID)
		if err != nil {
			return nil, err
		}
		return toWakeResponse(value), nil
	}, homeActionRejection)
}

func homeActionRejection(err error) (int, ErrorDetail) {
	switch {
	case errors.Is(err, home.ErrAlreadySleeping):
		return http.StatusConflict, ErrorDetail{Code: "HOME_ALREADY_SLEEPING", Message: "Character is already sleeping."}
	case errors.Is(err, home.ErrStillSleeping):
		return http.StatusConflict, ErrorDetail{Code: "HOME_STILL_SLEEPING", Message: "Sleep duration has not elapsed; recovery is not ready."}
	case errors.Is(err, home.ErrNotSleeping):
		return http.StatusConflict, ErrorDetail{Code: "HOME_NOT_SLEEPING", Message: "Character is not sleeping."}
	case errors.Is(err, home.ErrCharacterNotFound), errors.Is(err, corecharacter.ErrNotFound):
		return http.StatusNotFound, ErrorDetail{Code: "CHARACTER_NOT_FOUND", Message: "Character not found."}
	case errors.Is(err, home.ErrHouseNotFound):
		return http.StatusNotFound, ErrorDetail{Code: "HOME_HOUSE_NOT_FOUND", Message: "Target house not found."}
	case errors.Is(err, home.ErrHouseExpired):
		return http.StatusNotFound, ErrorDetail{Code: "HOME_HOUSE_EXPIRED", Message: "Target house has expired."}
	default:
		return 0, ErrorDetail{}
	}
}
