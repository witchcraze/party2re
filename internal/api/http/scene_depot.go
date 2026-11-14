package http

import (
	"context"
	"errors"

	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/playercontext"
)

type DepotSceneItem struct {
	ID               string `json:"id"`
	DefinitionID     string `json:"definition_id"`
	Quantity         int    `json:"quantity"`
	EnhancementLevel int    `json:"enhancement_level"`
}

type DepotSceneData struct {
	Parent            string           `json:"parent"`
	CharacterID       string           `json:"character_id"`
	Capacity          int              `json:"capacity"`
	ExDepot           int              `json:"ex_depot"`
	NextExpansionCost *int             `json:"next_expansion_cost"`
	ItemCount         int              `json:"item_count"`
	Items             []DepotSceneItem `json:"items"`
	Page              ScenePage        `json:"page"`
	// PendingRemittances is held gold that depot_withdraw collects before items.
	PendingRemittances []remittanceResponse `json:"pending_remittances"`
}

func (h *Handler) depotSceneData(ctx context.Context, r playercontext.Result) (any, error) {
	if h.depot == nil {
		return nil, errSceneNotConfigured
	}
	dep, err := h.depot.GetDepot(ctx, r.Snapshot.Character.ID)
	if err != nil {
		return nil, err
	}
	pending, err := h.depot.PendingRemittances(ctx, r.Snapshot.Character.ID)
	if err != nil {
		return nil, err
	}
	var nextCost *int
	if cost, err := depot.ExpansionCost(dep.ExDepot); err == nil {
		nextCost = &cost
	} else if !errors.Is(err, depot.ErrDepotMaxExpanded) {
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
		NextExpansionCost: nextCost, ItemCount: len(dep.Items), Items: items, Page: page, PendingRemittances: toRemittanceResponses(pending)}, nil
}

var depotFacility = facility{
	title: "預かり所", commands: []string{"depot_deposit", "depot_send_item", "depot_send_money", "depot_withdraw", "depot_expand", "depot_sell", "depot_sell_batch", "depot_sort"},
	read: (*Handler).depotSceneData, placeLog: true,
	present: func(_ *Handler, _ playercontext.Result, data any) scenePresentation {
		return scenePresentation{next: data.(DepotSceneData).Page.Next}
	},
	// Lines with capacities ($max_depot) are omitted until those facts are wired.
	npc: &npcProfile{name: "@ニキータ", words: []string{
		"ここは預かり所だけど、何か用かい？", "転職回数が増えるごとに預けられる個数も増えていくぜ",
		"＠おくる時は、送るアイテムと相手の名前を教えてくれな", "＠せいとんすると、武器、防具、道具の順に整頓できるぜ",
		"預かり所がまんぱんだと、相手からのアイテムが受け取れないぜ", "預かり所がまんぱんだと、クエストでの宝物を手に入れることができないぜ",
		"ここで売るのも専門店で売るのも売値は変わらないぜ",
	}},
}
