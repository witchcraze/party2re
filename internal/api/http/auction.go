package http

import (
	"context"
	"errors"
	"net/http"

	"github.com/witchcraze/party2re/internal/auction"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
)

// AuctionService defines the live P2P auction operations exposed over HTTP.
type AuctionService interface {
	Send(ctx context.Context, req auction.SendRequest) (auction.SendResult, error)
	Inspect(ctx context.Context, inspectorCharacterID, targetCharacterID string) (auction.InspectResult, error)
	InspectByName(ctx context.Context, inspectorCharacterID, targetName string) (auction.InspectResult, error)
	GetVenueInfo() auction.VenueInfo
}

func (h *Handler) handleAuctionInspect(w http.ResponseWriter, r *http.Request) {
	if h.auctions == nil {
		writeError(w, http.StatusNotImplemented, errors.New("auction service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		targetID := r.URL.Query().Get("target_character_id")
		if targetID == "" {
			targetID = r.URL.Query().Get("target_id")
		}
		targetName := r.URL.Query().Get("target_name")

		if targetID == "" && targetName == "" {
			writeError(w, http.StatusBadRequest, errors.New("target_id or target_name is required"))
			return
		}

		var res auction.InspectResult
		var err error
		if targetID != "" {
			res, err = h.auctions.Inspect(r.Context(), char.ID, targetID)
		} else {
			res, err = h.auctions.InspectByName(r.Context(), char.ID, targetName)
		}

		if err != nil {
			if errors.Is(err, auction.ErrTargetNotFound) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, res)
	})
}

var auctionDialogue = []string{
	"ここはオークション会場です。他のプレイヤーとアイテム交換やアイテム売買をする場所です。",
	"入札や出品のようなシステムはないです。自由に競りをしてください。",
	"相手が実際にそのアイテムや落札金を持っているのか「＠しらべる」で見ることができます。",
}

type auctionVenueInfoResponse struct {
	Title    string   `json:"title"`
	NPCName  string   `json:"npc_name"`
	Dialogue []string `json:"dialogue"`
}

func (h *Handler) handleAuctionVenueInfo(w http.ResponseWriter, r *http.Request) {
	if h.auctions == nil {
		writeError(w, http.StatusNotImplemented, errors.New("auction service not configured"))
		return
	}

	info := h.auctions.GetVenueInfo()
	writeJSON(w, http.StatusOK, auctionVenueInfoResponse{
		Title:    info.Title,
		NPCName:  info.NPCName,
		Dialogue: auctionDialogue,
	})
}
