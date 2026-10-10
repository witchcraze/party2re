package http

import (
	"context"
	"errors"
	"net/http"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/depot"
)

// WithDepot configures the Depot service and its Gateway expansion command.
func WithDepot(service DepotService) Option {
	return func(h *Handler) {
		h.depot = service
		if service == nil {
			withActionCommand[struct{}]("depot_expand", nil, nil)(h)
			return
		}
		withActionCommand("depot_expand", func(ctx context.Context, actorID string, _ struct{}) (any, error) {
			value, err := service.Expand(ctx, actorID)
			if err != nil {
				return nil, err
			}
			return toDepotResponse(value), nil
		}, depotExpansionRejection)(h)
	}
}

func depotExpansionRejection(err error) (int, ErrorDetail) {
	switch {
	case errors.Is(err, depot.ErrInsufficientFunds):
		return http.StatusBadRequest, ErrorDetail{Code: "DEPOT_INSUFFICIENT_FUNDS", Message: "Insufficient wallet funds."}
	case errors.Is(err, depot.ErrDepotMaxExpanded):
		return http.StatusBadRequest, ErrorDetail{Code: "DEPOT_MAX_EXPANDED", Message: "Depot is already at maximum expansion."}
	case errors.Is(err, depot.ErrInvalidCharacterID):
		return http.StatusBadRequest, ErrorDetail{Code: "DEPOT_INVALID_CHARACTER_ID", Message: "Invalid depot character ID."}
	case errors.Is(err, corecharacter.ErrNotFound):
		return http.StatusNotFound, ErrorDetail{Code: "CHARACTER_NOT_FOUND", Message: "Character not found."}
	default:
		return 0, ErrorDetail{}
	}
}
