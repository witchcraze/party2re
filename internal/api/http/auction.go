package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

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

// WithAuction configures the auction service for the Handler.
func WithAuction(a AuctionService) Option {
	return func(h *Handler) {
		h.auctions = a
	}
}

type sendAuctionRequest struct {
	TargetCharacterID   string `json:"target_character_id,omitempty"`
	TargetCharacterName string `json:"target_character_name,omitempty"`
	Gold                int    `json:"gold,omitempty"`
	Slot                string `json:"slot,omitempty"`
	InstanceID          string `json:"instance_id,omitempty"`
}

type sendAuctionLegacyRequest struct {
	SenderCharacterID   string `json:"sender_character_id"`
	TargetCharacterID   string `json:"target_character_id,omitempty"`
	TargetCharacterName string `json:"target_character_name,omitempty"`
	Gold                int    `json:"gold,omitempty"`
	Slot                string `json:"slot,omitempty"`
	InstanceID          string `json:"instance_id,omitempty"`
}

func (h *Handler) handleAuctionSend(w http.ResponseWriter, r *http.Request) {
	if h.auctions == nil {
		writeError(w, http.StatusNotImplemented, errors.New("auction service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req sendAuctionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		res, err := h.auctions.Send(r.Context(), auction.SendRequest{
			SenderCharacterID:   char.ID,
			TargetCharacterID:   req.TargetCharacterID,
			TargetCharacterName: req.TargetCharacterName,
			Gold:                req.Gold,
			Slot:                req.Slot,
			InstanceID:          req.InstanceID,
		})
		if err != nil {
			h.writeAuctionError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, res)
	})
}

func (h *Handler) handleAuctionSendLegacy(w http.ResponseWriter, r *http.Request) {
	if h.auctions == nil {
		writeError(w, http.StatusNotImplemented, errors.New("auction service not configured"))
		return
	}

	p, ok := h.authenticatePlayer(w, r)
	if !ok {
		return
	}

	var req sendAuctionLegacyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	senderID := strings.TrimSpace(req.SenderCharacterID)
	if senderID == "" {
		writeError(w, http.StatusBadRequest, errors.New("sender_character_id is required"))
		return
	}

	senderChar, err := h.characters.Get(r.Context(), senderID)
	if err != nil {
		writeError(w, http.StatusNotFound, errors.New("sender character not found"))
		return
	}
	if senderChar.PlayerID != p.ID {
		writeError(w, http.StatusForbidden, errors.New("you do not own this character"))
		return
	}

	res, err := h.auctions.Send(r.Context(), auction.SendRequest{
		SenderCharacterID:   senderID,
		TargetCharacterID:   req.TargetCharacterID,
		TargetCharacterName: req.TargetCharacterName,
		Gold:                req.Gold,
		Slot:                req.Slot,
		InstanceID:          req.InstanceID,
	})
	if err != nil {
		h.writeAuctionError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, res)
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

func (h *Handler) handleAuctionVenueInfo(w http.ResponseWriter, r *http.Request) {
	if h.auctions == nil {
		writeError(w, http.StatusNotImplemented, errors.New("auction service not configured"))
		return
	}

	info := h.auctions.GetVenueInfo()
	writeJSON(w, http.StatusOK, info)
}

func (h *Handler) writeAuctionError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auction.ErrCannotSendToSelf),
		errors.Is(err, auction.ErrInvalidSendTarget),
		errors.Is(err, auction.ErrNothingToSend),
		errors.Is(err, auction.ErrInvalidSendAmount),
		errors.Is(err, auction.ErrInsufficientMoney),
		errors.Is(err, auction.ErrDepotFull),
		errors.Is(err, auction.ErrTabooItem),
		errors.Is(err, auction.ErrItemNotEquipped),
		errors.Is(err, auction.ErrInvalidSlot):
		writeError(w, http.StatusBadRequest, err)
	case errors.Is(err, auction.ErrTargetNotFound),
		errors.Is(err, auction.ErrItemNotFound):
		writeError(w, http.StatusNotFound, err)
	default:
		writeError(w, http.StatusInternalServerError, err)
	}
}
