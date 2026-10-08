package http

import (
	"context"
	"slices"
	"strings"

	"github.com/witchcraze/party2re/internal/playercontext"
	"github.com/witchcraze/party2re/internal/shop"
)

type ShopQuantityBounds struct {
	Minimum int `json:"minimum"`
	Maximum int `json:"maximum"`
}

type ShopPurchaseParams struct {
	ItemDefinitionID string `json:"item_definition_id"`
}

type ShopProduct struct {
	shop.CatalogItem
	SelectParams   playercontext.Subject `json:"select_params"`
	PurchaseParams ShopPurchaseParams    `json:"purchase_params"`
}

type ShopNPCObservation struct {
	shop.NPCInspectResult
	Dialogue string `json:"dialogue"`
}

type ShopSceneInfo struct {
	Parent   string             `json:"parent"`
	Title    string             `json:"title"`
	ShopType shop.ShopType      `json:"shop_type"`
	NPC      ShopNPCObservation `json:"npc"`
	Quantity ShopQuantityBounds `json:"quantity"`
}

type ShopCatalogSceneData struct {
	ShopSceneInfo
	Items []ShopProduct `json:"items"`
	Page  ScenePage     `json:"page"`
}

type ShopProductSceneData struct {
	ShopSceneInfo
	Product ShopProduct `json:"product"`
}

func (h *Handler) shopSceneData(ctx context.Context, r playercontext.Result) (any, error) {
	if h.shops == nil {
		return nil, errSceneNotConfigured
	}
	n := r.Navigation.Selection
	kind := shop.ShopType(strings.TrimPrefix(n.Destination, "shop_"))
	catalog, err := h.shops.GetCatalog(ctx, kind, r.Snapshot.Character.ID)
	if err != nil {
		return nil, err
	}
	inspect, err := h.shops.InspectNPC(ctx, kind, r.Snapshot.Character.ID)
	if err != nil {
		return nil, err
	}
	info := ShopSceneInfo{Parent: "town", Title: catalog.Title, ShopType: catalog.ShopType,
		NPC:      ShopNPCObservation{NPCInspectResult: inspect, Dialogue: shopInspectDialogue(inspect.ShopType)},
		Quantity: ShopQuantityBounds{Minimum: 1, Maximum: shop.MaxTransactionQuantity}}
	products := make([]ShopProduct, 0, len(catalog.Items))
	for _, p := range catalog.Items {
		product := ShopProduct{CatalogItem: p, SelectParams: playercontext.Subject{Kind: "item", ID: p.ID}, PurchaseParams: ShopPurchaseParams{ItemDefinitionID: p.ID}}
		if n.Subject != (playercontext.Subject{}) && p.ID == n.Subject.ID {
			return ShopProductSceneData{ShopSceneInfo: info, Product: product}, nil
		}
		products = append(products, product)
	}
	if n.Subject != (playercontext.Subject{}) {
		// A catalog may change between the selector read and this projection.
		return nil, playercontext.ErrSelectionNotFound
	}
	slices.SortFunc(products, func(a, b ShopProduct) int { return strings.Compare(a.ID, b.ID) })
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
	return ShopCatalogSceneData{ShopSceneInfo: info, Items: append([]ShopProduct{}, products[start:end]...), Page: page}, nil
}
