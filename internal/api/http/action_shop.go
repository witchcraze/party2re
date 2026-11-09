package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/shop"
)

type shopPurchaseRequest struct {
	ShopType         shop.ShopType `json:"shop_type"`
	ItemDefinitionID string        `json:"item_definition_id"`
	Quantity         int           `json:"quantity"`
}

type shopSellRequest struct {
	ShopType       shop.ShopType `json:"shop_type"`
	ItemInstanceID string        `json:"item_instance_id"`
}

type shopBatchPurchaseRequest struct {
	ShopType shop.ShopType                   `json:"shop_type"`
	Items    []shop.BatchPurchaseItemRequest `json:"items"`
}

type shopSynthesizeRequest struct {
	RecipeTarget string `json:"recipe_target"`
}

type shopPurchaseResponse struct {
	ItemDefinitionID   string `json:"item_definition_id"`
	Quantity           int    `json:"quantity"`
	TotalCost          int    `json:"total_cost"`
	TransferredToDepot bool   `json:"transferred_to_depot"`
	NPCMessage         string `json:"npc_message"`
}

type shopSaleResponse struct {
	ItemInstanceID string `json:"item_instance_id"`
	Quantity       int    `json:"quantity"`
	TotalPayout    int    `json:"total_payout"`
	NPCMessage     string `json:"npc_message"`
}

type shopBatchPurchaseResponse struct {
	TotalPrice     int    `json:"total_price"`
	PurchasedCount int    `json:"purchased_count"`
	NPCMessage     string `json:"npc_message"`
}

type shopSynthesizeResponse struct {
	shop.SynthesisResult
	NPCMessage string `json:"npc_message"`
}

// shopLines are each shop keeper's purchase (hand/depot) and sale lines.
var shopLines = map[shop.ShopType]struct {
	hand, depot func(item, actor string) string
	sale        func(item string, payout int) string
}{
	shop.ShopTypeWeapon: {
		hand: func(i, _ string) string { return fmt.Sprintf("まいど！%sだ！受けとってくれ！", i) },
		depot: func(i, a string) string {
			return fmt.Sprintf("まいど！%sは%sの預かり所に送っておいたぜ！", i, a)
		},
		sale: func(i string, p int) string { return fmt.Sprintf("%s の買取代の %d Gだ！", i, p) },
	},
	shop.ShopTypeArmor: {
		hand: func(i, _ string) string {
			return fmt.Sprintf("お買い上げありがとうッス！%sどうぞ着てくださいッス", i)
		},
		depot: func(i, a string) string {
			return fmt.Sprintf("お買い上げありがとうッス！%sは%sさんの預かり所に送っておいたッス！", i, a)
		},
		sale: func(i string, p int) string { return fmt.Sprintf("%s の買取代の %d Gッス！", i, p) },
	},
	shop.ShopTypeItem: {
		hand: func(i, _ string) string { return fmt.Sprintf("%sですね。はい、どうぞ！", i) },
		depot: func(i, a string) string {
			return fmt.Sprintf("%sは%sニャンの預かり所の方に投げましたニャ！", i, a)
		},
		sale: func(i string, p int) string { return fmt.Sprintf("%d Gで %s を買い取りまちた", p, i) },
	},
	shop.ShopTypeAccessory: {
		hand:  func(i, _ string) string { return fmt.Sprintf("%sね。はい", i) },
		depot: func(i, a string) string { return fmt.Sprintf("%sは%sの預かり所の方に投げたわ", i, a) },
		sale:  func(i string, p int) string { return fmt.Sprintf("%d Gで %s を買い取ったわ", p, i) },
	},
}

// shopCommands connects the four ordinary shops to the Action Gateway.
func shopCommands() Option {
	return func(h *Handler) {
		withActionCommand("shop_purchase", func(ctx context.Context, actor string, p shopPurchaseRequest) (any, error) {
			lines, ok := shopLines[p.ShopType]
			if !ok {
				return nil, shop.ErrInvalidShopType
			}
			// Read presentation inputs before mutation to preserve a known outcome.
			char, err := h.characters.Get(ctx, actor)
			if err != nil {
				return nil, err
			}
			catalog, err := h.shops.GetCatalog(ctx, p.ShopType, actor)
			if err != nil {
				return nil, err
			}
			name := p.ItemDefinitionID
			for _, it := range catalog.Items {
				if it.ID == p.ItemDefinitionID {
					name = it.Name
				}
			}
			res, err := h.shops.PurchaseInShop(ctx, actor, p.ShopType, p.ItemDefinitionID, p.Quantity)
			if err != nil {
				return nil, err
			}
			message := lines.hand(name, char.Name)
			if res.TransferredToDepot {
				message = lines.depot(name, char.Name)
			}
			return shopPurchaseResponse{ItemDefinitionID: res.ItemInstance.DefinitionID, Quantity: res.ItemInstance.Quantity,
				TotalCost: res.TotalPrice, TransferredToDepot: res.TransferredToDepot, NPCMessage: message}, nil
		}, shopActionRejection)(h)
		withActionCommand("shop_sell", func(ctx context.Context, actor string, p shopSellRequest) (any, error) {
			lines, ok := shopLines[p.ShopType]
			if !ok {
				return nil, shop.ErrInvalidShopType
			}
			offer, err := h.shops.GetSaleOffer(ctx, p.ShopType, actor)
			if err != nil {
				return nil, err
			}
			res, err := h.shops.Sell(ctx, actor, p.ItemInstanceID, 1)
			if err != nil {
				return nil, err
			}
			name := res.SoldInstance.DefinitionID
			if offer != nil && offer.ItemInstanceID == p.ItemInstanceID {
				name = offer.Name
			}
			return shopSaleResponse{ItemInstanceID: res.SoldInstance.ID, Quantity: res.SoldInstance.Quantity,
				TotalPayout: res.TotalPayout, NPCMessage: lines.sale(name, res.TotalPayout)}, nil
		}, shopActionRejection)(h)
		withActionCommand("shop_batch_purchase", func(ctx context.Context, actor string, p shopBatchPurchaseRequest) (any, error) {
			char, err := h.characters.Get(ctx, actor)
			if err != nil {
				return nil, err
			}
			res, err := h.shops.BatchPurchase(ctx, actor, p.ShopType, p.Items)
			if err != nil {
				return nil, err
			}
			return shopBatchPurchaseResponse{TotalPrice: res.TotalPrice, PurchasedCount: len(res.Purchased),
				NPCMessage: batchPurchaseNPCMessage(p.ShopType, char.Name)}, nil
		}, shopActionRejection)(h)
		withActionCommand("shop_synthesize", func(ctx context.Context, actor string, p shopSynthesizeRequest) (any, error) {
			res, err := h.shops.Synthesize(ctx, actor, p.RecipeTarget)
			if err != nil {
				return nil, err
			}
			message := "あー…。失敗しちゃったみたいね。ドンマイ！"
			if res.Success {
				message = fmt.Sprintf("%s が完成したわ。預り所にあるはずよ", res.Recipe.ProductName)
				if res.UsedHiyaku {
					message = "合成の秘薬を使って、" + message
				}
			}
			return shopSynthesizeResponse{SynthesisResult: res, NPCMessage: message}, nil
		}, shopActionRejection)(h)
	}
}

func batchPurchaseNPCMessage(shopType shop.ShopType, characterName string) string {
	switch shopType {
	case shop.ShopTypeWeapon:
		return fmt.Sprintf("まいど！ %sの預かり所に送っておいたぜ！", characterName)
	case shop.ShopTypeArmor:
		return fmt.Sprintf("お買い上げありがとうッス！%sさんの預かり所に送っておいたッス！", characterName)
	default:
		return fmt.Sprintf("%sニャンの預かり所の方に投げましたニャ！", characterName)
	}
}

func shopActionRejection(err error) (int, ErrorDetail) {
	switch {
	case errors.Is(err, corecharacter.ErrNotFound):
		return http.StatusNotFound, ErrorDetail{Code: "CHARACTER_NOT_FOUND", Message: "Character not found."}
	case errors.Is(err, shop.ErrInvalidShopType):
		return http.StatusBadRequest, ErrorDetail{Code: "SHOP_INVALID_TYPE", Message: "This shop does not offer that operation."}
	case errors.Is(err, shop.ErrItemNotFound):
		return http.StatusNotFound, ErrorDetail{Code: "SHOP_ITEM_NOT_FOUND", Message: "Item is not sold here."}
	case errors.Is(err, shop.ErrUnownedItem):
		return http.StatusNotFound, ErrorDetail{Code: "SHOP_ITEM_NOT_OWNED", Message: "Item is not owned."}
	case errors.Is(err, shop.ErrRecipeNotFound):
		return http.StatusNotFound, ErrorDetail{Code: "SHOP_RECIPE_NOT_FOUND", Message: "Recipe not found."}
	case errors.Is(err, shop.ErrItemUnavailable):
		return http.StatusConflict, ErrorDetail{Code: "SHOP_ITEM_UNAVAILABLE", Message: "Item unavailable during an active helper request."}
	case errors.Is(err, shop.ErrMaterialsNotFound):
		return http.StatusConflict, ErrorDetail{Code: "SHOP_MATERIALS_NOT_FOUND", Message: "Recipe materials are not in the depot."}
	case errors.Is(err, shop.ErrInsufficientFunds):
		return http.StatusBadRequest, ErrorDetail{Code: "SHOP_INSUFFICIENT_FUNDS", Message: "Insufficient wallet funds."}
	case errors.Is(err, shop.ErrInvalidQuantity):
		return http.StatusBadRequest, ErrorDetail{Code: "SHOP_INVALID_QUANTITY", Message: "Quantity is out of range."}
	case errors.Is(err, shop.ErrEmptyPurchaseList):
		return http.StatusBadRequest, ErrorDetail{Code: "SHOP_EMPTY_PURCHASE_LIST", Message: "Batch purchase needs at least one item."}
	case errors.Is(err, shop.ErrPriceOverflow):
		return http.StatusBadRequest, ErrorDetail{Code: "SHOP_PRICE_OVERFLOW", Message: "Total price exceeds the supported range."}
	default:
		return 0, ErrorDetail{}
	}
}
