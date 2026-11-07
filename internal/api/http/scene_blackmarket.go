package http

import (
	"context"
	"errors"

	"github.com/witchcraze/party2re/internal/blackmarket"
	"github.com/witchcraze/party2re/internal/playercontext"
)

// BlackMarketSceneData replaces the retired status and points GETs.
type BlackMarketSceneData struct {
	Parent string `json:"parent"`
	blackmarket.Status
	CandidateCount int       `json:"candidate_count"`
	Page           ScenePage `json:"page"`
}

func (h *Handler) blackMarketSceneData(ctx context.Context, r playercontext.Result) (any, error) {
	if h.blackmarket == nil {
		return nil, errSceneNotConfigured
	}
	status, err := h.blackmarket.GetStatus(ctx, r.Snapshot.Character.ID)
	if errors.Is(err, blackmarket.ErrAccessDenied) {
		return nil, playercontext.ErrSelectionNotFound
	}
	if err != nil {
		return nil, err
	}
	n := r.Navigation.Selection
	limit := n.Limit
	if limit == 0 {
		limit = 20
	}
	candidates := status.SacrificeCandidates
	start := min(n.Offset, len(candidates))
	end := min(start+limit, len(candidates))
	page := ScenePage{Mode: "offset", Offset: n.Offset, Limit: limit}
	if end < len(candidates) {
		page.Next = &playercontext.PageParams{Destination: n.Destination, Offset: end, Limit: limit}
	}
	data := BlackMarketSceneData{Parent: "town", Status: *status, CandidateCount: len(candidates), Page: page}
	data.SacrificeCandidates = append([]blackmarket.SacrificeCandidate{}, candidates[start:end]...)
	return data, nil
}
