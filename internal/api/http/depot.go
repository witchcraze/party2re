package http

import (
	"context"
	"errors"
	"net/http"
	"strings"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/depot"
)

// DepotService defines depot storage operations exposed over HTTP.
type DepotService interface {
	GetDepot(ctx context.Context, characterID string) (depot.Depot, error)
	DepositItem(ctx context.Context, characterID string, itemInstanceID string) (depot.Depot, error)
	WithdrawItem(ctx context.Context, characterID string, itemInstanceID string) (depot.Depot, error)
	SellItem(ctx context.Context, characterID string, itemInstanceID string) (depot.Depot, int, error)
	SellItems(ctx context.Context, characterID string, itemInstanceIDs []string) (depot.Depot, int, error)
	SortItems(ctx context.Context, characterID string) (depot.Depot, error)
	Expand(ctx context.Context, characterID string) (depot.Depot, error)
	SendMoney(ctx context.Context, fromCharacterID, toCharacterID string, amount int) (depot.Depot, error)
	SendItem(ctx context.Context, fromCharacterID, toCharacterID, itemInstanceID string) (depot.Depot, error)
}

// WithDepot configures the depot service for the Handler.
func WithDepot(d DepotService) Option {
	return func(h *Handler) {
		h.depot = d
	}
}

type depositItemRequest struct {
	ItemID string `json:"item_id"`
}

type withdrawItemRequest struct {
	ItemID string `json:"item_id"`
}

type sellDepotItemRequest struct {
	ItemID string `json:"item_id"`
}

type sellDepotBatchRequest struct {
	ItemIDs []string `json:"item_ids"`
}

type sendMoneyRequest struct {
	RecipientCharacterID string `json:"recipient_character_id"`
	Amount               int    `json:"amount"`
}

type sendItemRequest struct {
	RecipientCharacterID string `json:"recipient_character_id"`
	ItemID               string `json:"item_id"`
}

type depotResponse struct {
	CharacterID string              `json:"character_id"`
	Capacity    int                 `json:"capacity"`
	ExDepot     int                 `json:"ex_depot"`
	ItemCount   int                 `json:"item_count"`
	Items       []depotItemResponse `json:"items"`
}

type depotItemResponse struct {
	ID           string `json:"id"`
	DefinitionID string `json:"definition_id"`
	Quantity     int    `json:"quantity"`
}

type sellDepotResponse struct {
	Depot      depotResponse `json:"depot"`
	GoldEarned int           `json:"gold_earned"`
}

type sendMoneyResponse struct {
	Depot                depotResponse `json:"depot"`
	RecipientCharacterID string        `json:"recipient_character_id"`
	Amount               int           `json:"amount"`
}

type sendItemResponse struct {
	Depot                depotResponse `json:"depot"`
	RecipientCharacterID string        `json:"recipient_character_id"`
	ItemID               string        `json:"item_id"`
}

func toDepotResponse(d depot.Depot) depotResponse {
	items := make([]depotItemResponse, 0, len(d.Items))
	for _, it := range d.Items {
		items = append(items, depotItemResponse{
			ID:           it.ID,
			DefinitionID: it.DefinitionID,
			Quantity:     it.Quantity,
		})
	}
	return depotResponse{
		CharacterID: d.CharacterID,
		Capacity:    d.Capacity,
		ExDepot:     d.ExDepot,
		ItemCount:   len(d.Items),
		Items:       items,
	}
}

func mapDepotHTTPError(w http.ResponseWriter, err error) {
	if errors.Is(err, depot.ErrNotFound) ||
		errors.Is(err, depot.ErrItemNotFound) ||
		errors.Is(err, depot.ErrRecipientNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if errors.Is(err, depot.ErrDepotFull) ||
		errors.Is(err, depot.ErrInventoryFull) ||
		errors.Is(err, depot.ErrRecipientDepotFull) ||
		errors.Is(err, depot.ErrInsufficientFunds) ||
		errors.Is(err, depot.ErrInvalidAmount) ||
		errors.Is(err, depot.ErrInvalidCharacterID) ||
		errors.Is(err, depot.ErrInvalidItemInstanceID) ||
		errors.Is(err, depot.ErrDepotMaxExpanded) ||
		errors.Is(err, depot.ErrSelfTransferNotAllowed) ||
		errors.Is(err, depot.ErrEmptyItemList) {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeError(w, http.StatusInternalServerError, err)
}

func (h *Handler) handleGetDepot(w http.ResponseWriter, r *http.Request) {
	if h.depot == nil {
		writeError(w, http.StatusNotImplemented, errors.New("depot service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		dep, err := h.depot.GetDepot(r.Context(), char.ID)
		if err != nil {
			mapDepotHTTPError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, toDepotResponse(dep))
	})
}

func (h *Handler) handleDepositDepotItem(w http.ResponseWriter, r *http.Request) {
	if h.depot == nil {
		writeError(w, http.StatusNotImplemented, errors.New("depot service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req depositItemRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		dep, err := h.depot.DepositItem(r.Context(), char.ID, req.ItemID)
		if err != nil {
			mapDepotHTTPError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, toDepotResponse(dep))
	})
}

func (h *Handler) handleWithdrawDepotItem(w http.ResponseWriter, r *http.Request) {
	if h.depot == nil {
		writeError(w, http.StatusNotImplemented, errors.New("depot service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req withdrawItemRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		dep, err := h.depot.WithdrawItem(r.Context(), char.ID, req.ItemID)
		if err != nil {
			mapDepotHTTPError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, toDepotResponse(dep))
	})
}

func (h *Handler) handleSellDepotItem(w http.ResponseWriter, r *http.Request) {
	if h.depot == nil {
		writeError(w, http.StatusNotImplemented, errors.New("depot service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req sellDepotItemRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		dep, goldEarned, err := h.depot.SellItem(r.Context(), char.ID, req.ItemID)
		if err != nil {
			mapDepotHTTPError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, sellDepotResponse{
			Depot:      toDepotResponse(dep),
			GoldEarned: goldEarned,
		})
	})
}

func (h *Handler) handleSellDepotBatch(w http.ResponseWriter, r *http.Request) {
	if h.depot == nil {
		writeError(w, http.StatusNotImplemented, errors.New("depot service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req sellDepotBatchRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		dep, goldEarned, err := h.depot.SellItems(r.Context(), char.ID, req.ItemIDs)
		if err != nil {
			mapDepotHTTPError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, sellDepotResponse{
			Depot:      toDepotResponse(dep),
			GoldEarned: goldEarned,
		})
	})
}

func (h *Handler) handleSortDepot(w http.ResponseWriter, r *http.Request) {
	if h.depot == nil {
		writeError(w, http.StatusNotImplemented, errors.New("depot service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		dep, err := h.depot.SortItems(r.Context(), char.ID)
		if err != nil {
			mapDepotHTTPError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, toDepotResponse(dep))
	})
}

func (h *Handler) handleExpandDepot(w http.ResponseWriter, r *http.Request) {
	if h.depot == nil {
		writeError(w, http.StatusNotImplemented, errors.New("depot service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		dep, err := h.depot.Expand(r.Context(), char.ID)
		if err != nil {
			mapDepotHTTPError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, toDepotResponse(dep))
	})
}

func (h *Handler) handleDepotSendMoney(w http.ResponseWriter, r *http.Request) {
	if h.depot == nil {
		writeError(w, http.StatusNotImplemented, errors.New("depot service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req sendMoneyRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		recipientID := strings.TrimSpace(req.RecipientCharacterID)
		dep, err := h.depot.SendMoney(r.Context(), char.ID, recipientID, req.Amount)
		if err != nil {
			mapDepotHTTPError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, sendMoneyResponse{
			Depot:                toDepotResponse(dep),
			RecipientCharacterID: recipientID,
			Amount:               req.Amount,
		})
	})
}

func (h *Handler) handleDepotSendItem(w http.ResponseWriter, r *http.Request) {
	if h.depot == nil {
		writeError(w, http.StatusNotImplemented, errors.New("depot service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req sendItemRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		recipientID := strings.TrimSpace(req.RecipientCharacterID)
		itemID := strings.TrimSpace(req.ItemID)
		dep, err := h.depot.SendItem(r.Context(), char.ID, recipientID, itemID)
		if err != nil {
			mapDepotHTTPError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, sendItemResponse{
			Depot:                toDepotResponse(dep),
			RecipientCharacterID: recipientID,
			ItemID:               itemID,
		})
	})
}
