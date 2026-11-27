package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

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

// WithDepot configures the Depot Gateway commands.
func WithDepot(service DepotService) Option {
	return func(h *Handler) {
		h.depot = service
		if service == nil {
			withEchoCommand[depositItemRequest, depotEchoResponse]("depot_deposit", nil, nil)(h)
			withEchoCommand[withdrawItemRequest, withdrawResponse]("depot_withdraw", nil, nil)(h)
			withEchoCommand[sendItemRequest, sendItemResponse]("depot_send_item", nil, nil)(h)
			withEchoCommand[sendMoneyRequest, sendMoneyResponse]("depot_send_money", nil, nil)(h)
			withEchoCommand[struct{}, depotEchoResponse]("depot_sort", nil, nil)(h)
			withEchoCommand[struct{}, depotEchoResponse]("depot_expand", nil, nil)(h)
			withEchoCommand[sellDepotItemRequest, sellDepotResponse]("depot_sell", nil, nil)(h)
			withEchoCommand[depotSaleBatchRequest, sellDepotResponse]("depot_sell_batch", nil, nil)(h)
			return
		}
		withEchoCommand("depot_deposit", func(ctx context.Context, actorID string, params depositItemRequest) (depotEchoResponse, error) {
			value, err := service.DepositItem(ctx, actorID, params.ItemID)
			if err != nil {
				return depotEchoResponse{}, err
			}
			return depotEchoResponse{toDepotResponse(value.Depot), commandEcho{value.ItemName, value.ItemName + "をお預かりしました"}}, nil
		}, depotActionRejection)(h)
		withEchoCommand("depot_withdraw", func(ctx context.Context, actorID string, params withdrawItemRequest) (withdrawResponse, error) {
			value, collected, err := service.WithdrawItem(ctx, actorID, params.ItemID)
			if err != nil {
				return withdrawResponse{}, err
			}
			res := withdrawResponse{depotResponse: toDepotResponse(value.Depot), CollectedRemittances: toRemittanceResponses(collected)}
			if len(collected) == 0 { // collecting held gold is a private $mes (depot.cgi:211-215)
				reply := value.ItemName + "をお返しします"
				if value.ReturnedName != "" {
					reply = value.ReturnedName + "をお預かりしました。" + reply
				}
				res.commandEcho = commandEcho{value.ItemName, reply}
			}
			return res, nil
		}, depotActionRejection)(h)
		withEchoCommand("depot_send_item", func(ctx context.Context, actorID string, params sendItemRequest) (sendItemResponse, error) {
			recipientID, itemID := strings.TrimSpace(params.RecipientCharacterID), strings.TrimSpace(params.ItemID)
			value, err := service.SendItem(ctx, actorID, recipientID, itemID)
			if err != nil {
				return sendItemResponse{}, err
			}
			echo := commandEcho{value.ItemName + "＠あいて>" + value.RecipientName, value.ItemName + "を" + value.RecipientName + "に送りました"}
			return sendItemResponse{Depot: toDepotResponse(value.Depot), RecipientCharacterID: recipientID, ItemID: itemID, commandEcho: echo}, nil
		}, depotActionRejection)(h)
		withEchoCommand("depot_send_money", func(ctx context.Context, actorID string, params sendMoneyRequest) (sendMoneyResponse, error) {
			recipientID := strings.TrimSpace(params.RecipientCharacterID)
			value, err := service.SendMoney(ctx, actorID, recipientID, params.Amount)
			if err != nil {
				return sendMoneyResponse{}, err
			}
			echo := commandEcho{fmt.Sprintf("%dG＠あいて>%s", params.Amount, value.RecipientName), fmt.Sprintf("%d Gを %s に送りました", params.Amount, value.RecipientName)}
			return sendMoneyResponse{Depot: toDepotResponse(value.Depot), RecipientCharacterID: recipientID, Amount: params.Amount, commandEcho: echo}, nil
		}, depotActionRejection)(h)
		withEchoCommand("depot_sort", func(ctx context.Context, actorID string, _ struct{}) (depotEchoResponse, error) {
			value, err := service.SortItems(ctx, actorID)
			if err != nil {
				return depotEchoResponse{}, err
			}
			return depotEchoResponse{toDepotResponse(value), commandEcho{"", "預かっているアイテムをせいとんした"}}, nil
		}, depotActionRejection)(h)
		withEchoCommand("depot_expand", func(ctx context.Context, actorID string, _ struct{}) (depotEchoResponse, error) {
			value, err := service.Expand(ctx, actorID)
			if err != nil {
				return depotEchoResponse{}, err
			}
			// Gateway expansion is the confirmed ＠かくちょう>はい (depot.cgi:527-534).
			return depotEchoResponse{toDepotResponse(value), commandEcho{"はい", "{name}の倉庫を拡張しました"}}, nil
		}, depotActionRejection)(h)
		withEchoCommand("depot_sell", func(ctx context.Context, actorID string, params sellDepotItemRequest) (sellDepotResponse, error) {
			value, earned, err := service.SellItem(ctx, actorID, params.ItemID)
			if err != nil {
				return sellDepotResponse{}, err
			}
			name := ""
			if len(value.Sales) > 0 {
				name = value.Sales[0].Name
			}
			return sellDepotResponse{toDepotResponse(value.Depot), earned, commandEcho{name, fmt.Sprintf("%s の買取代の %d Gです！", name, earned)}}, nil
		}, depotActionRejection)(h)
		withEchoCommand("depot_sell_batch", func(ctx context.Context, actorID string, params depotSaleBatchRequest) (sellDepotResponse, error) {
			value, earned, err := service.SellItems(ctx, actorID, params.ItemIDs)
			if err != nil {
				return sellDepotResponse{}, err
			}
			return sellDepotResponse{toDepotResponse(value.Depot), earned, batchSaleEcho(value.Sales, earned)}, nil
		}, depotActionRejection)(h)
	}
}

// depotEchoResponse is a Depot state result with its place-log echo.
type depotEchoResponse struct {
	depotResponse
	commandEcho
}

// batchSaleEcho renders ＠まとめてうる's checked names and its sale list
// (depot.cgi:497-504); <br> becomes a line break and &nbsp; a no-break space.
func batchSaleEcho(sales []depot.Sale, earned int) commandEcho {
	var target, reply strings.Builder
	fmt.Fprintf(&reply, "買取代の %d Gです！\n-------買取リスト-------\n", earned)
	for _, sale := range sales {
		target.WriteString(sale.Name + "<>")
		fmt.Fprintf(&reply, "%s%s %d G\n", strings.Repeat("\u00a0", 12), sale.Name, sale.Price)
	}
	reply.WriteString("---------------------")
	return commandEcho{target.String(), reply.String()}
}

func depotActionRejection(err error) (int, ErrorDetail) {
	switch {
	case errors.Is(err, depot.ErrRecipientDepotFull):
		return http.StatusBadRequest, ErrorDetail{Code: "DEPOT_RECIPIENT_FULL", Message: "Recipient depot is full."}
	case errors.Is(err, depot.ErrSelfTransferNotAllowed):
		return http.StatusBadRequest, ErrorDetail{Code: "DEPOT_SELF_TRANSFER", Message: "Cannot send an item to yourself."}
	case errors.Is(err, depot.ErrRecipientNotFound):
		return http.StatusNotFound, ErrorDetail{Code: "DEPOT_RECIPIENT_NOT_FOUND", Message: "Recipient character not found."}
	case errors.Is(err, depot.ErrDepotFull):
		return http.StatusBadRequest, ErrorDetail{Code: "DEPOT_FULL", Message: "Depot is full."}
	case errors.Is(err, depot.ErrInventoryFull):
		return http.StatusBadRequest, ErrorDetail{Code: "DEPOT_HAND_CONFLICT", Message: "Held items cannot be swapped."}
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
		return http.StatusBadRequest, ErrorDetail{Code: "DEPOT_INVALID_AMOUNT", Message: "Invalid depot amount."}
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
