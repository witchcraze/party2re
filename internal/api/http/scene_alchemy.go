package http

import (
	"context"

	"github.com/witchcraze/party2re/internal/alchemy"
	"github.com/witchcraze/party2re/internal/playercontext"
)

// AlchemySceneData replaces the retired status GET; compendium entries are paged.
type AlchemySceneData struct {
	Parent     string             `json:"parent"`
	Status     alchemy.Synthesis  `json:"status"`
	Compendium alchemy.Compendium `json:"compendium"`
	Page       ScenePage          `json:"page"`
}

func (h *Handler) alchemySceneData(ctx context.Context, r playercontext.Result) (any, error) {
	if h.alchemy == nil {
		return nil, errSceneNotConfigured
	}
	id := r.Snapshot.Character.ID
	status, err := h.alchemy.GetStatus(ctx, id)
	if err != nil {
		return nil, err
	}
	compendium, err := h.alchemy.GetCompendium(ctx, id)
	if err != nil {
		return nil, err
	}
	n := r.Navigation.Selection
	limit := n.Limit
	if limit == 0 {
		limit = 20
	}
	start := min(n.Offset, len(compendium.Entries))
	end := min(start+limit, len(compendium.Entries))
	page := ScenePage{Mode: "offset", Offset: n.Offset, Limit: limit}
	if end < len(compendium.Entries) {
		page.Next = &playercontext.PageParams{Destination: n.Destination, Offset: end, Limit: limit}
	}
	compendium.Entries = append([]alchemy.CompendiumEntry{}, compendium.Entries[start:end]...)
	return AlchemySceneData{Parent: "town", Status: status, Compendium: compendium, Page: page}, nil
}
