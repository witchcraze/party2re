package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/depot"
)

type depotSaleBatchRequest struct {
	ItemIDs depotSaleItemIDs `json:"item_ids"`
}

// A null array element must not silently decode to an empty instance ID.
type depotSaleItemIDs []string

func (ids *depotSaleItemIDs) UnmarshalJSON(raw []byte) error {
	var values []*string
	if err := json.Unmarshal(raw, &values); err != nil {
		return err
	}
	parsed := make([]string, len(values))
	for i, value := range values {
		if value == nil {
			return errors.New("item_ids must contain strings")
		}
		parsed[i] = *value
	}
	*ids = parsed
	return nil
}

// WithDepot configures Depot REST operations and its Gateway commands.
func WithDepot(service DepotService) Option {
	return func(h *Handler) {
		h.depot = service
		if service == nil {
			withActionCommand[struct{}]("depot_sort", nil, nil)(h)
			withActionCommand[struct{}]("depot_expand", nil, nil)(h)
			withActionCommand[sellDepotItemRequest]("depot_sell", nil, nil)(h)
			withActionCommand[depotSaleBatchRequest]("depot_sell_batch", nil, nil)(h)
			return
		}
		withActionCommand("depot_sort", func(ctx context.Context, actorID string, _ struct{}) (any, error) {
			value, err := service.SortItems(ctx, actorID)
			if err != nil {
				return nil, err
			}
			return toDepotResponse(value), nil
		}, depotActionRejection)(h)
		withActionCommand("depot_expand", func(ctx context.Context, actorID string, _ struct{}) (any, error) {
			value, err := service.Expand(ctx, actorID)
			if err != nil {
				return nil, err
			}
			return toDepotResponse(value), nil
		}, depotActionRejection)(h)
		withActionCommand("depot_sell", func(ctx context.Context, actorID string, params sellDepotItemRequest) (any, error) {
			value, earned, err := service.SellItem(ctx, actorID, params.ItemID)
			if err != nil {
				return nil, err
			}
			return sellDepotResponse{Depot: toDepotResponse(value), GoldEarned: earned}, nil
		}, depotActionRejection)(h)
		withActionCommand("depot_sell_batch", func(ctx context.Context, actorID string, params depotSaleBatchRequest) (any, error) {
			value, earned, err := service.SellItems(ctx, actorID, params.ItemIDs)
			if err != nil {
				return nil, err
			}
			return sellDepotResponse{Depot: toDepotResponse(value), GoldEarned: earned}, nil
		}, depotActionRejection)(h)
	}
}

func depotActionRejection(err error) (int, ErrorDetail) {
	switch {
	case errors.Is(err, depot.ErrInsufficientFunds):
		return http.StatusBadRequest, ErrorDetail{Code: "DEPOT_INSUFFICIENT_FUNDS", Message: "Insufficient wallet funds."}
	case errors.Is(err, depot.ErrDepotMaxExpanded):
		return http.StatusBadRequest, ErrorDetail{Code: "DEPOT_MAX_EXPANDED", Message: "Depot is already at maximum expansion."}
	case errors.Is(err, depot.ErrInvalidCharacterID):
		return http.StatusBadRequest, ErrorDetail{Code: "DEPOT_INVALID_CHARACTER_ID", Message: "Invalid depot character ID."}
	case errors.Is(err, depot.ErrInvalidItemInstanceID):
		return http.StatusBadRequest, ErrorDetail{Code: "DEPOT_INVALID_ITEM_INSTANCE_ID", Message: "Invalid depot item instance ID."}
	case errors.Is(err, depot.ErrEmptyItemList):
		return http.StatusBadRequest, ErrorDetail{Code: "DEPOT_EMPTY_ITEM_LIST", Message: "Select depot items to sell."}
	case errors.Is(err, depot.ErrInvalidAmount):
		return http.StatusBadRequest, ErrorDetail{Code: "DEPOT_INVALID_AMOUNT", Message: "Invalid depot sale amount."}
	case errors.Is(err, depot.ErrInvalidQuantity):
		return http.StatusBadRequest, ErrorDetail{Code: "DEPOT_INVALID_QUANTITY", Message: "Invalid depot item quantity."}
	case errors.Is(err, depot.ErrItemNotFound):
		return http.StatusNotFound, ErrorDetail{Code: "DEPOT_ITEM_NOT_FOUND", Message: "Depot item not found."}
	case errors.Is(err, depot.ErrNotFound):
		return http.StatusNotFound, ErrorDetail{Code: "DEPOT_NOT_FOUND", Message: "Depot not found."}
	case errors.Is(err, corecharacter.ErrNotFound):
		return http.StatusNotFound, ErrorDetail{Code: "CHARACTER_NOT_FOUND", Message: "Character not found."}
	default:
		return 0, ErrorDetail{}
	}
}
