package http

import (
	"errors"
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
	if errors.Is(err, corecharacter.ErrNotFound) || errors.Is(err, shop.ErrUnownedItem) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if errors.Is(err, shop.ErrInsufficientFunds) ||
		errors.Is(err, shop.ErrDepotFull) ||
		errors.Is(err, shop.ErrItemUnavailable) {
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
			NPCMessage:     result.NPCMessage,
		})
	})
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
