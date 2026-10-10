package http

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/witchcraze/party2re/internal/playercontext"
	"github.com/witchcraze/party2re/internal/secretshop"
)

type SecretShopPurchaseParams struct {
	ItemID string `json:"item_id"`
}

type SecretShopProduct struct {
	secretshop.Item
	SelectParams   playercontext.Subject    `json:"select_params"`
	PurchaseParams SecretShopPurchaseParams `json:"purchase_params"`
}

type SecretShopSceneInfo struct {
	Parent     string             `json:"parent"`
	Title      string             `json:"title"`
	NPCName    string             `json:"npc_name"`
	IsEligible bool               `json:"is_eligible"`
	Quantity   ShopQuantityBounds `json:"quantity"`
}

type SecretShopCatalogSceneData struct {
	SecretShopSceneInfo
	Items []SecretShopProduct `json:"items"`
	Page  ScenePage           `json:"page"`
}

type SecretShopProductSceneData struct {
	SecretShopSceneInfo
	Product SecretShopProduct `json:"product"`
}

func (h *Handler) secretShopSceneData(ctx context.Context, r playercontext.Result) (any, error) {
	if h.secretshop == nil {
		return nil, errSceneNotConfigured
	}
	n := r.Navigation.Selection
	status, err := h.secretshop.GetShopStatus(ctx, r.Snapshot.Character.ID)
	if errors.Is(err, secretshop.ErrAccessDenied) {
		return nil, playercontext.ErrSelectionNotFound
	}
	if err != nil {
		return nil, err
	}
	info := SecretShopSceneInfo{Parent: "town", Title: status.LocationName, NPCName: status.NPCName, IsEligible: status.IsEligible,
		Quantity: ShopQuantityBounds{Minimum: 1, Maximum: secretshop.MaxPurchaseQuantity}}
	products := make([]SecretShopProduct, 0, len(status.Items))
	for _, p := range status.Items {
		product := SecretShopProduct{Item: p, SelectParams: playercontext.Subject{Kind: "item", ID: p.ID}, PurchaseParams: SecretShopPurchaseParams{ItemID: p.ID}}
		if n.Subject != (playercontext.Subject{}) && p.ID == n.Subject.ID {
			return SecretShopProductSceneData{SecretShopSceneInfo: info, Product: product}, nil
		}
		products = append(products, product)
	}
	if n.Subject != (playercontext.Subject{}) {
		return nil, playercontext.ErrSelectionNotFound
	}
	slices.SortFunc(products, func(a, b SecretShopProduct) int { return strings.Compare(a.ID, b.ID) })
	limit := n.Limit
	if limit == 0 {
		limit = 20
	}
	start := min(n.Offset, len(products))
	end := min(start+limit, len(products))
	page := ScenePage{Mode: "offset", Offset: n.Offset, Limit: limit}
	if end < len(products) {
		page.Next = &playercontext.PageParams{Destination: n.Destination, Offset: end, Limit: limit}
	}
	return SecretShopCatalogSceneData{SecretShopSceneInfo: info, Items: append([]SecretShopProduct{}, products[start:end]...), Page: page}, nil
}
