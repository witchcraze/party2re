package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/delivery"
)

// DeliveryService defines the town delivery and parcel courier operations exposed over HTTP.
type DeliveryService interface {
	GetAvailableQuests(ctx context.Context, now time.Time) ([]delivery.Quest, error)
	GetCharacterDeliveries(ctx context.Context, characterID string) ([]delivery.CharacterDelivery, error)
	GetActiveCharacterDeliveries(ctx context.Context, characterID string) ([]delivery.CharacterDelivery, error)
	AcceptQuest(ctx context.Context, characterID string, questID string, now time.Time) (*delivery.CharacterDelivery, error)
	CompleteDelivery(ctx context.Context, characterID string, deliveryID string, now time.Time) (*delivery.DeliveryCompletionResult, error)
	CancelDelivery(ctx context.Context, characterID string, deliveryID string) error
	SendParcel(ctx context.Context, senderID string, req delivery.SendParcelRequest, now time.Time) (*delivery.Parcel, error)
	GetIncomingParcels(ctx context.Context, recipientID string) ([]delivery.Parcel, error)
	ClaimParcel(ctx context.Context, recipientID string, parcelID string, now time.Time) (*delivery.ParcelClaimResult, error)
	CancelParcel(ctx context.Context, senderID string, parcelID string) error
}

// WithDelivery configures the DeliveryService for the HTTP handler.
func WithDelivery(service DeliveryService) Option {
	return func(h *Handler) {
		h.delivery = service
	}
}

type acceptDeliveryQuestRequest struct {
	QuestID string `json:"quest_id"`
}

type completeDeliveryRequest struct {
	DeliveryID string `json:"delivery_id"`
}

type cancelDeliveryRequest struct {
	DeliveryID string `json:"delivery_id"`
}

type claimParcelRequest struct {
	ParcelID string `json:"parcel_id"`
}

type cancelParcelRequest struct {
	ParcelID string `json:"parcel_id"`
}

func (h *Handler) handleGetDeliveryQuests(w http.ResponseWriter, r *http.Request) {
	if h.delivery == nil {
		writeError(w, http.StatusNotImplemented, errors.New("delivery service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, _ corecharacter.Character) {
		quests, err := h.delivery.GetAvailableQuests(r.Context(), time.Now())
		if err != nil {
			h.writeDeliveryError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, quests)
	})
}

func (h *Handler) handleGetActiveDeliveries(w http.ResponseWriter, r *http.Request) {
	if h.delivery == nil {
		writeError(w, http.StatusNotImplemented, errors.New("delivery service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		deliveries, err := h.delivery.GetActiveCharacterDeliveries(r.Context(), char.ID)
		if err != nil {
			h.writeDeliveryError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, deliveries)
	})
}

func (h *Handler) handleAcceptDeliveryQuest(w http.ResponseWriter, r *http.Request) {
	if h.delivery == nil {
		writeError(w, http.StatusNotImplemented, errors.New("delivery service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req acceptDeliveryQuestRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, errors.New("invalid request body"))
			return
		}

		res, err := h.delivery.AcceptQuest(r.Context(), char.ID, req.QuestID, time.Now())
		if err != nil {
			h.writeDeliveryError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, res)
	})
}

func (h *Handler) handleCompleteDelivery(w http.ResponseWriter, r *http.Request) {
	if h.delivery == nil {
		writeError(w, http.StatusNotImplemented, errors.New("delivery service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req completeDeliveryRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, errors.New("invalid request body"))
			return
		}

		res, err := h.delivery.CompleteDelivery(r.Context(), char.ID, req.DeliveryID, time.Now())
		if err != nil {
			h.writeDeliveryError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, res)
	})
}

func (h *Handler) handleCancelDelivery(w http.ResponseWriter, r *http.Request) {
	if h.delivery == nil {
		writeError(w, http.StatusNotImplemented, errors.New("delivery service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req cancelDeliveryRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, errors.New("invalid request body"))
			return
		}

		if err := h.delivery.CancelDelivery(r.Context(), char.ID, req.DeliveryID); err != nil {
			h.writeDeliveryError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
	})
}

func (h *Handler) handleSendParcel(w http.ResponseWriter, r *http.Request) {
	if h.delivery == nil {
		writeError(w, http.StatusNotImplemented, errors.New("delivery service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req delivery.SendParcelRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, errors.New("invalid request body"))
			return
		}

		parcel, err := h.delivery.SendParcel(r.Context(), char.ID, req, time.Now())
		if err != nil {
			h.writeDeliveryError(w, err)
			return
		}

		writeJSON(w, http.StatusCreated, parcel)
	})
}

func (h *Handler) handleGetIncomingParcels(w http.ResponseWriter, r *http.Request) {
	if h.delivery == nil {
		writeError(w, http.StatusNotImplemented, errors.New("delivery service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		parcels, err := h.delivery.GetIncomingParcels(r.Context(), char.ID)
		if err != nil {
			h.writeDeliveryError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, parcels)
	})
}

func (h *Handler) handleClaimParcel(w http.ResponseWriter, r *http.Request) {
	if h.delivery == nil {
		writeError(w, http.StatusNotImplemented, errors.New("delivery service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req claimParcelRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, errors.New("invalid request body"))
			return
		}

		res, err := h.delivery.ClaimParcel(r.Context(), char.ID, req.ParcelID, time.Now())
		if err != nil {
			h.writeDeliveryError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, res)
	})
}

func (h *Handler) handleCancelParcel(w http.ResponseWriter, r *http.Request) {
	if h.delivery == nil {
		writeError(w, http.StatusNotImplemented, errors.New("delivery service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req cancelParcelRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, errors.New("invalid request body"))
			return
		}

		if err := h.delivery.CancelParcel(r.Context(), char.ID, req.ParcelID); err != nil {
			h.writeDeliveryError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
	})
}

func (h *Handler) writeDeliveryError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, delivery.ErrQuestNotFound),
		errors.Is(err, delivery.ErrDeliveryNotFound),
		errors.Is(err, delivery.ErrParcelNotFound),
		errors.Is(err, delivery.ErrRecipientNotFound):
		writeError(w, http.StatusNotFound, err)
	case errors.Is(err, delivery.ErrForbidden):
		writeError(w, http.StatusForbidden, err)
	case errors.Is(err, delivery.ErrQuestExpired),
		errors.Is(err, delivery.ErrMaxActiveDeliveries),
		errors.Is(err, delivery.ErrAlreadyAccepted),
		errors.Is(err, delivery.ErrDeliveryNotActive),
		errors.Is(err, delivery.ErrInsufficientItems),
		errors.Is(err, delivery.ErrInsufficientFunds),
		errors.Is(err, delivery.ErrParcelAlreadyClaimed),
		errors.Is(err, delivery.ErrSelfParcelNotAllowed),
		errors.Is(err, delivery.ErrInvalidParcelPayload),
		errors.Is(err, delivery.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, err)
	default:
		writeError(w, http.StatusInternalServerError, err)
	}
}
