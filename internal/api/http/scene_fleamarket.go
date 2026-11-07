package http

import (
	"context"

	"github.com/witchcraze/party2re/internal/fleamarket"
	"github.com/witchcraze/party2re/internal/playercontext"
)

// FleaMarketSceneData replaces the retired listing GETs. Listing candidates come
// from the Depot scene, whose instances the legacy listing prompt also shows.
type FleaMarketSceneData struct {
	Parent        string               `json:"parent"`
	Listings      []fleamarket.Listing `json:"listings"`
	ListingCount  int                  `json:"listing_count"`
	OwnedListings []fleamarket.Listing `json:"owned_listings"`
	Page          ScenePage            `json:"page"`
}

func (h *Handler) fleaMarketSceneData(ctx context.Context, r playercontext.Result) (any, error) {
	if h.fleamarket == nil {
		return nil, errSceneNotConfigured
	}
	n := r.Navigation.Selection
	limit := n.Limit
	if limit == 0 {
		limit = 20
	}
	listings, total, err := h.fleamarket.ListActiveListings(ctx, limit, n.Offset)
	if err != nil {
		return nil, err
	}
	owned, err := h.fleamarket.GetActiveCharacterListings(ctx, r.Snapshot.Character.ID)
	if err != nil {
		return nil, err
	}
	page := ScenePage{Mode: "offset", Offset: n.Offset, Limit: limit}
	if end := n.Offset + len(listings); end < total {
		page.Next = &playercontext.PageParams{Destination: n.Destination, Offset: end, Limit: limit}
	}
	return FleaMarketSceneData{Parent: "town", Listings: append([]fleamarket.Listing{}, listings...), ListingCount: total,
		OwnedListings: append([]fleamarket.Listing{}, owned...), Page: page}, nil
}
