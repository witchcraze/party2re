package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/witchcraze/party2re/internal/auction"
)

type auctionSendParams struct {
	TargetCharacterID   string `json:"target_character_id,omitempty"`
	TargetCharacterName string `json:"target_character_name,omitempty"`
	Gold                int    `json:"gold,omitempty"`
	Slot                string `json:"slot,omitempty"`
	InstanceID          string `json:"instance_id,omitempty"`
}

func (p *auctionSendParams) UnmarshalJSON(raw []byte) error {
	// Optional fields may be absent, but null must not become a zero value.
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return err
	}
	present := make([]string, 0, len(fields))
	for name := range fields {
		present = append(present, name)
	}
	type params auctionSendParams
	return decodeActionParams(raw, present, (*params)(p))
}

// WithAuction configures retained Auction reads and its Gateway send command.
func WithAuction(service AuctionService) Option {
	return func(h *Handler) {
		h.auctions = service
		if service == nil {
			withActionCommand[auctionSendParams]("auction_send", nil, nil)(h)
			return
		}
		withActionCommand("auction_send", func(ctx context.Context, actorID string, p auctionSendParams) (any, error) {
			return service.Send(ctx, auction.SendRequest{
				SenderCharacterID: actorID, TargetCharacterID: p.TargetCharacterID, TargetCharacterName: p.TargetCharacterName,
				Gold: p.Gold, Slot: p.Slot, InstanceID: p.InstanceID,
			})
		}, auctionActionRejection)(h)
	}
}

func auctionActionRejection(err error) (int, ErrorDetail) {
	switch {
	case errors.Is(err, auction.ErrCannotSendToSelf):
		return http.StatusBadRequest, ErrorDetail{Code: "AUCTION_SELF_TRANSFER", Message: "Cannot send to yourself."}
	case errors.Is(err, auction.ErrInvalidSendTarget):
		return http.StatusBadRequest, ErrorDetail{Code: "AUCTION_INVALID_TARGET", Message: "Recipient character ID or name is required."}
	case errors.Is(err, auction.ErrNothingToSend):
		return http.StatusBadRequest, ErrorDetail{Code: "AUCTION_NOTHING_TO_SEND", Message: "Specify gold or an item to send."}
	case errors.Is(err, auction.ErrInvalidSendAmount):
		return http.StatusBadRequest, ErrorDetail{Code: "AUCTION_INVALID_AMOUNT", Message: "Send amount must be at least 1 G."}
	case errors.Is(err, auction.ErrInsufficientMoney):
		return http.StatusBadRequest, ErrorDetail{Code: "AUCTION_INSUFFICIENT_FUNDS", Message: "Insufficient wallet funds."}
	case errors.Is(err, auction.ErrDepotFull):
		return http.StatusBadRequest, ErrorDetail{Code: "AUCTION_DEPOT_FULL", Message: "Recipient depot is full."}
	case errors.Is(err, auction.ErrTabooItem):
		return http.StatusBadRequest, ErrorDetail{Code: "AUCTION_TABOO_ITEM", Message: "This item cannot be sent."}
	case errors.Is(err, auction.ErrItemNotEquipped):
		return http.StatusBadRequest, ErrorDetail{Code: "AUCTION_ITEM_NOT_EQUIPPED", Message: "No item equipped in the specified slot."}
	case errors.Is(err, auction.ErrInvalidSlot):
		return http.StatusBadRequest, ErrorDetail{Code: "AUCTION_INVALID_SLOT", Message: "Invalid equipment slot."}
	case errors.Is(err, auction.ErrTargetNotFound):
		return http.StatusNotFound, ErrorDetail{Code: "AUCTION_TARGET_NOT_FOUND", Message: "Recipient character not found."}
	case errors.Is(err, auction.ErrItemNotFound):
		return http.StatusNotFound, ErrorDetail{Code: "AUCTION_ITEM_NOT_FOUND", Message: "Owned item not found."}
	default:
		return 0, ErrorDetail{}
	}
}
