package http

import (
	"context"
	"fmt"

	"github.com/witchcraze/party2re/internal/secretshop"
)

// SecretShopService defines the secret shop operations behind its scene and purchase command.
type SecretShopService interface {
	GetShopStatus(ctx context.Context, characterID string) (*secretshop.ShopStatus, error)
	PurchaseItem(ctx context.Context, characterID string, itemID string, quantity int) (*secretshop.PurchaseResult, error)
}

type secretShopPurchaseRequest struct {
	ItemID   string `json:"item_id"`
	Quantity int    `json:"quantity"`
}

type secretShopPurchaseResponse struct {
	*secretshop.PurchaseResult
	NPCMessage string `json:"npc_message"`
}

func toSecretShopPurchaseResponse(result *secretshop.PurchaseResult, charName string) secretShopPurchaseResponse {
	message := fmt.Sprintf("%sメェ〜。持ってけメェ〜", result.Item.Name)
	if result.TransferredToDepot {
		message = fmt.Sprintf("%sは%sメェ〜の預かり所の方に投げましたメェ〜", result.Item.Name, charName)
	}
	return secretShopPurchaseResponse{PurchaseResult: result, NPCMessage: message}
}
