package http

import (
	"context"
	"errors"
	"net/http"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/rescue"
)

type rescueActionParams struct {
	Reason string `json:"reason"`
}

// WithRescue configures the emergency rescue service and its Gateway command.
func WithRescue(rescues RescueService) Option {
	return func(h *Handler) {
		h.rescues = rescues
		rescueRequestCommand(rescues)(h)
	}
}

func rescueRequestCommand(service RescueService) Option {
	if service == nil {
		return withActionCommand[rescueActionParams]("rescue_request", nil, nil)
	}
	return withActionCommand("rescue_request", func(ctx context.Context, actorID string, params rescueActionParams) (any, error) {
		return service.EmergencyRescue(ctx, actorID, params.Reason, time.Now().UTC())
	}, rescueActionRejection)
}

func rescueActionRejection(err error) (int, ErrorDetail) {
	switch {
	case errors.Is(err, rescue.ErrInvalidCharacterID):
		return http.StatusBadRequest, ErrorDetail{Code: "RESCUE_INVALID_CHARACTER_ID", Message: "Invalid rescue character ID."}
	case errors.Is(err, rescue.ErrInvalidReason):
		return http.StatusUnprocessableEntity, ErrorDetail{Code: "RESCUE_INVALID_REASON", Message: "Rescue reason cannot be empty."}
	case errors.Is(err, corecharacter.ErrNotFound):
		return http.StatusNotFound, ErrorDetail{Code: "CHARACTER_NOT_FOUND", Message: "Character not found."}
	default:
		return 0, ErrorDetail{}
	}
}
