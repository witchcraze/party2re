package http

import (
	"errors"
	"fmt"
	"net/http"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/shop"
)

type batchPurchaseItemJSON struct {
	ItemDefinitionID string `json:"item_definition_id"`
	Quantity         int    `json:"quantity"`
}

type batchPurchaseRequest struct {
	ShopType string                  `json:"shop_type"`
	Items    []batchPurchaseItemJSON `json:"items"`
}

type batchPurchaseResponse struct {
	CharacterID    string `json:"character_id"`
	TotalPrice     int    `json:"total_price"`
	PurchasedCount int    `json:"purchased_count"`
	NPCMessage     string `json:"npc_message"`
}

type npcDialogueResponse struct {
	Dialogue       string `json:"dialogue"`
	SecretShopHint string `json:"secret_shop_hint,omitempty"`
}

type discoverSecretResponse struct {
	Unlocked bool   `json:"unlocked"`
	Message  string `json:"message"`
}

func mapShopHTTPError(w http.ResponseWriter, err error) {
	if errors.Is(err, corecharacter.ErrNotFound) ||
		errors.Is(err, shop.ErrUnownedItem) ||
		errors.Is(err, shop.ErrRecipeNotFound) ||
		errors.Is(err, shop.ErrItemNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if errors.Is(err, shop.ErrInsufficientFunds) ||
		errors.Is(err, shop.ErrDepotFull) ||
		errors.Is(err, shop.ErrItemUnavailable) ||
		errors.Is(err, shop.ErrMaterialsNotFound) {
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}
	if errors.Is(err, shop.ErrInvalidQuantity) ||
		errors.Is(err, shop.ErrInvalidShopType) ||
		errors.Is(err, shop.ErrPriceOverflow) ||
		errors.Is(err, shop.ErrEmptyPurchaseList) {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeError(w, http.StatusInternalServerError, err)
}

func (h *Handler) handleGetShopCatalog(w http.ResponseWriter, r *http.Request) {
	charID := r.PathValue("id")
	shopTypeStr := r.PathValue("type")
	shopType := shop.ShopType(shopTypeStr)

	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		catalog, err := h.shops.GetCatalog(r.Context(), shopType, char.ID)
		if err != nil {
			mapShopHTTPError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, catalog)
	})
}

func (h *Handler) handleShopBatchPurchase(w http.ResponseWriter, r *http.Request) {
	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req batchPurchaseRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		shopType := shop.ShopType(req.ShopType)
		var items []shop.BatchPurchaseItemRequest
		for _, it := range req.Items {
			items = append(items, shop.BatchPurchaseItemRequest{
				ItemDefinitionID: it.ItemDefinitionID,
				Quantity:         it.Quantity,
			})
		}

		result, err := h.shops.BatchPurchase(r.Context(), char.ID, shopType, items)
		if err != nil {
			mapShopHTTPError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, batchPurchaseResponse{
			CharacterID:    result.Character.ID,
			TotalPrice:     result.TotalPrice,
			PurchasedCount: len(result.Purchased),
			NPCMessage:     batchPurchaseNPCMessage(shopType, char.Name),
		})
	})
}

func batchPurchaseNPCMessage(shopType shop.ShopType, characterName string) string {
	switch shopType {
	case shop.ShopTypeWeapon:
		return "まいど！預かり所に送っておいたぜ！"
	case shop.ShopTypeArmor:
		return "お買い上げありがとうッス！預かり所に送っておいたッス！"
	case shop.ShopTypeItem:
		return fmt.Sprintf("%sニャンの預かり所の方に投げましたニャ！", characterName)
	default:
		return "Transferred to depot."
	}
}

func (h *Handler) handleShopInspectNPC(w http.ResponseWriter, r *http.Request) {
	charID := r.PathValue("id")
	shopTypeStr := r.PathValue("type")
	shopType := shop.ShopType(shopTypeStr)

	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		res, err := h.shops.InspectNPC(r.Context(), shopType, char.ID)
		if err != nil {
			mapShopHTTPError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, npcDialogueResponse{
			Dialogue:       res.Dialogue,
			SecretShopHint: res.SecretShopHint,
		})
	})
}

func (h *Handler) handleShopTalkNPC(w http.ResponseWriter, r *http.Request) {
	charID := r.PathValue("id")
	shopTypeStr := r.PathValue("type")
	shopType := shop.ShopType(shopTypeStr)

	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		msg, err := h.shops.TalkNPC(r.Context(), shopType)
		if err != nil {
			mapShopHTTPError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, npcDialogueResponse{
			Dialogue: msg,
		})
	})
}

func (h *Handler) handleShopDiscoverSecret(w http.ResponseWriter, r *http.Request) {
	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		unlocked, msg, err := h.shops.DiscoverSecretShop(r.Context(), char.ID)
		if err != nil {
			mapShopHTTPError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, discoverSecretResponse{
			Unlocked: unlocked,
			Message:  msg,
		})
	})
}

type accessoryBuyRequest struct {
	ItemDefinitionID string `json:"item_definition_id"`
	Quantity         int    `json:"quantity"`
}

type accessorySellRequest struct {
	ItemInstanceID string `json:"item_instance_id"`
	Quantity       int    `json:"quantity"`
}

type accessorySynthesizeRequest struct {
	RecipeTarget string `json:"recipe_target"`
	RecipeID     string `json:"recipe_id,omitempty"`
	Target       string `json:"target,omitempty"`
}

func (r accessorySynthesizeRequest) resolvedTarget() string {
	if r.RecipeTarget != "" {
		return r.RecipeTarget
	}
	if r.RecipeID != "" {
		return r.RecipeID
	}
	return r.Target
}

func (h *Handler) handleAccessoryBuy(w http.ResponseWriter, r *http.Request) {
	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req accessoryBuyRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		result, err := h.shops.PurchaseInShop(r.Context(), char.ID, shop.ShopTypeAccessory, req.ItemDefinitionID, req.Quantity)
		if err != nil {
			mapShopHTTPError(w, err)
			return
		}
		itemName := req.ItemDefinitionID
		if cat, err := h.shops.GetCatalog(r.Context(), shop.ShopTypeAccessory, char.ID); err == nil {
			for _, it := range cat.Items {
				if it.ID == req.ItemDefinitionID {
					itemName = it.Name
					break
				}
			}
		}
		var npcMsg string
		if result.TransferredToDepot {
			npcMsg = fmt.Sprintf("%sは%sの預かり所の方に投げたわ", itemName, char.Name)
		} else {
			npcMsg = fmt.Sprintf("%sね。はい", itemName)
		}
		writeSuccess(w, http.StatusOK, purchaseResponse{
			CharacterID:        result.Character.ID,
			ItemDefinitionID:   result.ItemInstance.DefinitionID,
			Quantity:           result.ItemInstance.Quantity,
			TotalCost:          result.TotalPrice,
			TransferredToDepot: result.TransferredToDepot,
			NPCMessage:         npcMsg,
		}, npcMsg)
	})
}

func (h *Handler) handleAccessorySell(w http.ResponseWriter, r *http.Request) {
	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req accessorySellRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		result, err := h.shops.Sell(r.Context(), char.ID, req.ItemInstanceID, req.Quantity)
		if err != nil {
			mapShopHTTPError(w, err)
			return
		}
		payoutMsg := fmt.Sprintf("%d Gで買い取ったわ", result.TotalPayout)
		writeSuccess(w, http.StatusOK, saleResponse{
			CharacterID: result.Character.ID,
			InstanceID:  result.SoldInstance.ID,
			Quantity:    result.SoldInstance.Quantity,
			TotalPayout: result.TotalPayout,
		}, payoutMsg)
	})
}

func (h *Handler) handleAccessorySynthesize(w http.ResponseWriter, r *http.Request) {
	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req accessorySynthesizeRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		target := req.resolvedTarget()
		result, err := h.shops.Synthesize(r.Context(), char.ID, target)
		if err != nil {
			mapShopHTTPError(w, err)
			return
		}
		var npcMsg string
		if result.Success {
			if result.UsedHiyaku {
				npcMsg = fmt.Sprintf("合成の秘薬を使って、%s が完成したわ。預り所にあるはずよ", result.Recipe.ProductName)
			} else {
				npcMsg = fmt.Sprintf("%s が完成したわ。預り所にあるはずよ", result.Recipe.ProductName)
			}
		} else {
			npcMsg = "あー…。失敗しちゃったみたいね。ドンマイ！"
		}
		type accessorySynthesizeResponse struct {
			shop.SynthesisResult
			NPCMessage string `json:"npc_message"`
		}
		writeSuccess(w, http.StatusOK, accessorySynthesizeResponse{
			SynthesisResult: result,
			NPCMessage:      npcMsg,
		}, npcMsg)
	})
}

func (h *Handler) handleAccessoryRecipes(w http.ResponseWriter, r *http.Request) {
	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		recipes := shop.AllSynthesisRecipes()
		writeSuccess(w, http.StatusOK, recipes, "")
	})
}
