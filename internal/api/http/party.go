package http

import (
	"context"
	"errors"
	"net/http"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/pagination"
	"github.com/witchcraze/party2re/internal/party"
)

// PartyService defines party and multiplayer co-op operations exposed over HTTP.
type PartyService interface {
	CreateParty(ctx context.Context, leaderCharID string, req party.CreatePartyRequest) (party.PartyDetail, error)
	GetParty(ctx context.Context, partyID string) (party.PartyDetail, error)
	ListParties(ctx context.Context, status string, limit, offset int) (pagination.Page[party.PartySummary], error)
	JoinParty(ctx context.Context, partyID, characterID, password string) (party.PartyDetail, error)
	LeaveParty(ctx context.Context, partyID, characterID string) error
	KickMember(ctx context.Context, partyID, leaderCharID, targetCharID string) error
	DisbandParty(ctx context.Context, partyID, leaderCharID string) error
	SetReady(ctx context.Context, partyID, characterID string, ready bool) (party.PartyDetail, error)
	StartPartyAdventure(ctx context.Context, partyID, leaderCharID string) (party.PartyAdventureResult, error)
}

// WithParty configures the PartyService for the Handler.
func WithParty(p PartyService) Option {
	return func(h *Handler) {
		h.parties = p
	}
}

type createPartyRequest struct {
	CharacterID string `json:"character_id"`
	party.CreatePartyRequest
}

type joinPartyRequest struct {
	CharacterID string `json:"character_id"`
	Password    string `json:"password,omitempty"`
}

type leavePartyRequest struct {
	CharacterID string `json:"character_id"`
}

type kickPartyMemberRequest struct {
	CharacterID       string `json:"character_id"`
	TargetCharacterID string `json:"target_character_id"`
}

type disbandPartyRequest struct {
	CharacterID string `json:"character_id"`
}

type setPartyReadyRequest struct {
	CharacterID string `json:"character_id"`
	Ready       bool   `json:"ready"`
}

type startPartyAdventureRequest struct {
	CharacterID string `json:"character_id"`
}

// handleListParties returns a list of recruiting parties.
func (h *Handler) handleListParties(w http.ResponseWriter, r *http.Request) {
	if h.parties == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "party service not configured"})
		return
	}

	status := r.URL.Query().Get("status")
	params := pagination.ParseRequestWithDefaults(r, 50, 100)

	list, err := h.parties.ListParties(r.Context(), status, params.Limit, params.Offset)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list parties"})
		return
	}

	writeJSON(w, http.StatusOK, list)
}

// handleCreateParty creates a new party.
func (h *Handler) handleCreateParty(w http.ResponseWriter, r *http.Request) {
	if h.parties == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "party service not configured"})
		return
	}

	withAuthenticatedCharacterAndJSON(h, w, r, func(req *createPartyRequest) string {
		return req.CharacterID
	}, func(player coreplayer.Player, char corecharacter.Character, req createPartyRequest) {
		detail, err := h.parties.CreateParty(r.Context(), char.ID, req.CreatePartyRequest)
		if err != nil {
			switch {
			case errors.Is(err, party.ErrInvalidPartyName),
				errors.Is(err, party.ErrInvalidMaxMembers),
				errors.Is(err, party.ErrAlreadyInParty),
				errors.Is(err, party.ErrCharacterUnconscious):
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			case errors.Is(err, party.ErrStageNotFound):
				writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			default:
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create party"})
			}
			return
		}

		writeJSON(w, http.StatusCreated, detail)
	})
}

// handleGetParty returns details of a single party.
func (h *Handler) handleGetParty(w http.ResponseWriter, r *http.Request) {
	if h.parties == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "party service not configured"})
		return
	}

	partyID := r.PathValue("id")
	if partyID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "party id is required"})
		return
	}

	detail, err := h.parties.GetParty(r.Context(), partyID)
	if err != nil {
		if errors.Is(err, party.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "party not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get party"})
		return
	}

	writeJSON(w, http.StatusOK, detail)
}

// handleJoinParty adds a character to an existing party.
func (h *Handler) handleJoinParty(w http.ResponseWriter, r *http.Request) {
	if h.parties == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "party service not configured"})
		return
	}

	partyID := r.PathValue("id")
	if partyID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "party id is required"})
		return
	}

	withAuthenticatedCharacterAndJSON(h, w, r, func(req *joinPartyRequest) string {
		return req.CharacterID
	}, func(player coreplayer.Player, char corecharacter.Character, req joinPartyRequest) {
		detail, err := h.parties.JoinParty(r.Context(), partyID, char.ID, req.Password)
		if err != nil {
			switch {
			case errors.Is(err, party.ErrPartyNotRecruiting),
				errors.Is(err, party.ErrPartyFull),
				errors.Is(err, party.ErrInvalidPassword),
				errors.Is(err, party.ErrAlreadyInParty),
				errors.Is(err, party.ErrLevelRequirementNotMet),
				errors.Is(err, party.ErrHPRequirementNotMet),
				errors.Is(err, party.ErrCharacterUnconscious):
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			case errors.Is(err, party.ErrNotFound):
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "party not found"})
			default:
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to join party"})
			}
			return
		}

		writeJSON(w, http.StatusOK, detail)
	})
}

// handleLeaveParty removes the character from the party.
func (h *Handler) handleLeaveParty(w http.ResponseWriter, r *http.Request) {
	if h.parties == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "party service not configured"})
		return
	}

	partyID := r.PathValue("id")
	if partyID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "party id is required"})
		return
	}

	withAuthenticatedCharacterAndJSON(h, w, r, func(req *leavePartyRequest) string {
		return req.CharacterID
	}, func(player coreplayer.Player, char corecharacter.Character, req leavePartyRequest) {
		if err := h.parties.LeaveParty(r.Context(), partyID, char.ID); err != nil {
			switch {
			case errors.Is(err, party.ErrCharacterNotInParty):
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			case errors.Is(err, party.ErrNotFound):
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "party not found"})
			default:
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to leave party"})
			}
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{"status": "left"})
	})
}

// handleKickPartyMember removes a target member from the party (leader only).
func (h *Handler) handleKickPartyMember(w http.ResponseWriter, r *http.Request) {
	if h.parties == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "party service not configured"})
		return
	}

	partyID := r.PathValue("id")
	if partyID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "party id is required"})
		return
	}

	withAuthenticatedCharacterAndJSON(h, w, r, func(req *kickPartyMemberRequest) string {
		return req.CharacterID
	}, func(player coreplayer.Player, char corecharacter.Character, req kickPartyMemberRequest) {
		if err := h.parties.KickMember(r.Context(), partyID, char.ID, req.TargetCharacterID); err != nil {
			switch {
			case errors.Is(err, party.ErrNotPartyLeader),
				errors.Is(err, party.ErrCannotKickSelf),
				errors.Is(err, party.ErrCharacterNotInParty):
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			case errors.Is(err, party.ErrNotFound):
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "party not found"})
			default:
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to kick member"})
			}
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{"status": "kicked"})
	})
}

// handleDisbandParty disbands the entire party (leader only).
func (h *Handler) handleDisbandParty(w http.ResponseWriter, r *http.Request) {
	if h.parties == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "party service not configured"})
		return
	}

	partyID := r.PathValue("id")
	if partyID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "party id is required"})
		return
	}

	withAuthenticatedCharacterAndJSON(h, w, r, func(req *disbandPartyRequest) string {
		return req.CharacterID
	}, func(player coreplayer.Player, char corecharacter.Character, req disbandPartyRequest) {
		if err := h.parties.DisbandParty(r.Context(), partyID, char.ID); err != nil {
			switch {
			case errors.Is(err, party.ErrNotPartyLeader):
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			case errors.Is(err, party.ErrNotFound):
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "party not found"})
			default:
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to disband party"})
			}
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{"status": "disbanded"})
	})
}

// handleSetPartyReady toggles the member's ready state.
func (h *Handler) handleSetPartyReady(w http.ResponseWriter, r *http.Request) {
	if h.parties == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "party service not configured"})
		return
	}

	partyID := r.PathValue("id")
	if partyID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "party id is required"})
		return
	}

	withAuthenticatedCharacterAndJSON(h, w, r, func(req *setPartyReadyRequest) string {
		return req.CharacterID
	}, func(player coreplayer.Player, char corecharacter.Character, req setPartyReadyRequest) {
		detail, err := h.parties.SetReady(r.Context(), partyID, char.ID, req.Ready)
		if err != nil {
			switch {
			case errors.Is(err, party.ErrCharacterNotInParty):
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			case errors.Is(err, party.ErrNotFound):
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "party not found"})
			default:
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to set ready state"})
			}
			return
		}

		writeJSON(w, http.StatusOK, detail)
	})
}

// handleStartPartyAdventure starts the adventure (leader only).
func (h *Handler) handleStartPartyAdventure(w http.ResponseWriter, r *http.Request) {
	if h.parties == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "party service not configured"})
		return
	}

	partyID := r.PathValue("id")
	if partyID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "party id is required"})
		return
	}

	withAuthenticatedCharacterAndJSON(h, w, r, func(req *startPartyAdventureRequest) string {
		return req.CharacterID
	}, func(player coreplayer.Player, char corecharacter.Character, req startPartyAdventureRequest) {
		result, err := h.parties.StartPartyAdventure(r.Context(), partyID, char.ID)
		if err != nil {
			switch {
			case errors.Is(err, party.ErrNotPartyLeader),
				errors.Is(err, party.ErrPartyNotRecruiting),
				errors.Is(err, party.ErrPartyNotReady),
				errors.Is(err, party.ErrCharacterUnconscious):
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			case errors.Is(err, party.ErrNotFound),
				errors.Is(err, party.ErrStageNotFound):
				writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			default:
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to start party adventure"})
			}
			return
		}

		writeJSON(w, http.StatusOK, result)
	})
}
