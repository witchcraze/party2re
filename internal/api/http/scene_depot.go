package http

import (
	"context"

	"github.com/witchcraze/party2re/internal/playercontext"
)

type DepotSceneItem struct {
	ID               string `json:"id"`
	DefinitionID     string `json:"definition_id"`
	Quantity         int    `json:"quantity"`
	EnhancementLevel int    `json:"enhancement_level"`
}

type DepotSceneData struct {
	Parent      string           `json:"parent"`
	CharacterID string           `json:"character_id"`
	Capacity    int              `json:"capacity"`
	ExDepot     int              `json:"ex_depot"`
	ItemCount   int              `json:"item_count"`
	Items       []DepotSceneItem `json:"items"`
	Page        ScenePage        `json:"page"`
}

func (h *Handler) depotSceneData(ctx context.Context, r playercontext.Result) (any, error) {
	if h.depot == nil {
		return nil, errSceneNotConfigured
	}
	dep, err := h.depot.GetDepot(ctx, r.Snapshot.Character.ID)
	if err != nil {
		return nil, err
	}
	n := r.Navigation.Selection
	limit := n.Limit
	if limit == 0 {
		limit = 20
	}
	start := min(n.Offset, len(dep.Items))
	end := min(start+limit, len(dep.Items))
	items := make([]DepotSceneItem, 0, end-start)
	for _, it := range dep.Items[start:end] {
		items = append(items, DepotSceneItem{ID: it.ID, DefinitionID: it.DefinitionID, Quantity: it.Quantity, EnhancementLevel: it.EnhancementLevel})
	}
	page := ScenePage{Mode: "offset", Offset: n.Offset, Limit: limit}
	if end < len(dep.Items) {
		page.Next = &playercontext.PageParams{Destination: n.Destination, Offset: end, Limit: limit}
	}
	return DepotSceneData{Parent: "town", CharacterID: dep.CharacterID, Capacity: dep.Capacity, ExDepot: dep.ExDepot,
		ItemCount: len(dep.Items), Items: items, Page: page}, nil
}
