package http

import (
	"context"

	"github.com/witchcraze/party2re/internal/depot"
)

// DepotService defines the depot operations behind the Gateway commands and scene.
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

type depositItemRequest struct {
	ItemID string `json:"item_id"`
}

type withdrawItemRequest struct {
	ItemID string `json:"item_id"`
}

type sellDepotItemRequest struct {
	ItemID string `json:"item_id"`
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
