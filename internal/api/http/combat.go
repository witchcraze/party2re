package http

import (
	"context"
	"errors"
	"net/http"

	"github.com/witchcraze/party2re/internal/challenge"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
)

// ChallengeService defines endurance challenge operations exposed over HTTP.
type ChallengeService interface {
	ListTiers() []challenge.ChallengeTier
	GetTier(tierID string) (*challenge.ChallengeTier, error)
	StartSession(ctx context.Context, characterID string, tierID string) (*challenge.ChallengeSession, error)
	StartPartySession(ctx context.Context, leaderID string, memberIDs []string, tierID string, partyName string, partyColor string) (*challenge.ChallengeSession, error)
	AdvanceRound(ctx context.Context, characterID string, sessionID string) (*challenge.RoundResult, *challenge.ChallengeSession, error)
	RetireSession(ctx context.Context, characterID string, sessionID string) (*challenge.ChallengeSession, error)
	GetCharacterRecords(ctx context.Context, characterID string) ([]challenge.CharacterChallengeRecord, error)
	GetHallOfFame(ctx context.Context, tierID string) (*challenge.HallOfFameEntry, error)
	ListHallOfFame(ctx context.Context) ([]challenge.HallOfFameEntry, error)
}

// WithChallenge configures the challenge service for the Handler.
func WithChallenge(c ChallengeService) Option {
	return func(h *Handler) {
		h.challenges = c
	}
}

// -------------------------------------------------------------------
// Challenge Handlers & DTOs
// -------------------------------------------------------------------

type startChallengeRequest struct {
	TierID string `json:"tier_id"`
}

type startPartyChallengeRequest struct {
	TierID     string   `json:"tier_id"`
	MemberIDs  []string `json:"member_ids"`
	PartyName  string   `json:"party_name"`
	PartyColor string   `json:"party_color"`
}

type advanceChallengeRequest struct {
	SessionID string `json:"session_id"`
}

type retireChallengeRequest struct {
	SessionID string `json:"session_id"`
}

type challengeSessionResponse struct {
	Session *challenge.ChallengeSession `json:"session"`
}

type advanceChallengeResponse struct {
	RoundResult *challenge.RoundResult      `json:"round_result"`
	Session     *challenge.ChallengeSession `json:"session"`
}

type challengeRecordsResponse struct {
	Records []challenge.CharacterChallengeRecord `json:"records"`
}

func (h *Handler) handleListChallengeTiers(w http.ResponseWriter, r *http.Request) {
	if h.challenges == nil {
		writeError(w, http.StatusNotImplemented, errors.New("challenge service not configured"))
		return
	}
	tiers := h.challenges.ListTiers()
	writeJSON(w, http.StatusOK, tiers)
}

func (h *Handler) handleGetChallengeRecords(w http.ResponseWriter, r *http.Request) {
	if h.challenges == nil {
		writeError(w, http.StatusNotImplemented, errors.New("challenge service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		records, err := h.challenges.GetCharacterRecords(r.Context(), char.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, challengeRecordsResponse{
			Records: records,
		})
	})
}

func (h *Handler) handleStartChallenge(w http.ResponseWriter, r *http.Request) {
	if h.challenges == nil {
		writeError(w, http.StatusNotImplemented, errors.New("challenge service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req startChallengeRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		if req.TierID == "" {
			writeError(w, http.StatusBadRequest, errors.New("tier_id is required"))
			return
		}

		session, err := h.challenges.StartSession(r.Context(), char.ID, req.TierID)
		if err != nil {
			if errors.Is(err, challenge.ErrTierNotFound) || errors.Is(err, challenge.ErrCharacterNotFound) {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			if errors.Is(err, challenge.ErrLevelTooLow) || errors.Is(err, challenge.ErrActiveSessionExists) {
				writeError(w, http.StatusUnprocessableEntity, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, challengeSessionResponse{
			Session: session,
		})
	})
}

func (h *Handler) handleStartPartyChallenge(w http.ResponseWriter, r *http.Request) {
	if h.challenges == nil {
		writeError(w, http.StatusNotImplemented, errors.New("challenge service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req startPartyChallengeRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		if req.TierID == "" {
			writeError(w, http.StatusBadRequest, errors.New("tier_id is required"))
			return
		}

		memberIDs := req.MemberIDs
		if len(memberIDs) == 0 {
			memberIDs = []string{char.ID}
		}

		session, err := h.challenges.StartPartySession(r.Context(), char.ID, memberIDs, req.TierID, req.PartyName, req.PartyColor)
		if err != nil {
			if errors.Is(err, challenge.ErrTierNotFound) || errors.Is(err, challenge.ErrCharacterNotFound) {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			if errors.Is(err, challenge.ErrLevelTooLow) || errors.Is(err, challenge.ErrActiveSessionExists) || errors.Is(err, challenge.ErrTooManyPartyMembers) {
				writeError(w, http.StatusUnprocessableEntity, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, challengeSessionResponse{
			Session: session,
		})
	})
}

func (h *Handler) handleAdvanceChallenge(w http.ResponseWriter, r *http.Request) {
	if h.challenges == nil {
		writeError(w, http.StatusNotImplemented, errors.New("challenge service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req advanceChallengeRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		if req.SessionID == "" {
			writeError(w, http.StatusBadRequest, errors.New("session_id is required"))
			return
		}

		roundRes, session, err := h.challenges.AdvanceRound(r.Context(), char.ID, req.SessionID)
		if err != nil {
			if errors.Is(err, challenge.ErrSessionNotFound) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			if errors.Is(err, challenge.ErrForbidden) {
				writeError(w, http.StatusForbidden, err)
				return
			}
			if errors.Is(err, challenge.ErrSessionNotActive) {
				writeError(w, http.StatusUnprocessableEntity, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, advanceChallengeResponse{
			RoundResult: roundRes,
			Session:     session,
		})
	})
}

func (h *Handler) handleRetireChallenge(w http.ResponseWriter, r *http.Request) {
	if h.challenges == nil {
		writeError(w, http.StatusNotImplemented, errors.New("challenge service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req retireChallengeRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		if req.SessionID == "" {
			writeError(w, http.StatusBadRequest, errors.New("session_id is required"))
			return
		}

		session, err := h.challenges.RetireSession(r.Context(), char.ID, req.SessionID)
		if err != nil {
			if errors.Is(err, challenge.ErrSessionNotFound) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			if errors.Is(err, challenge.ErrForbidden) {
				writeError(w, http.StatusForbidden, err)
				return
			}
			if errors.Is(err, challenge.ErrSessionNotActive) {
				writeError(w, http.StatusUnprocessableEntity, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, challengeSessionResponse{
			Session: session,
		})
	})
}

func (h *Handler) handleGetChallengeHallOfFame(w http.ResponseWriter, r *http.Request) {
	if h.challenges == nil {
		writeError(w, http.StatusNotImplemented, errors.New("challenge service not configured"))
		return
	}

	tierID := r.PathValue("tier_id")
	if tierID == "" {
		writeError(w, http.StatusBadRequest, errors.New("tier_id is required"))
		return
	}

	hof, err := h.challenges.GetHallOfFame(r.Context(), tierID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if hof == nil {
		writeError(w, http.StatusNotFound, errors.New("hall of fame entry not found"))
		return
	}

	writeJSON(w, http.StatusOK, hof)
}

func (h *Handler) handleListChallengeHallOfFame(w http.ResponseWriter, r *http.Request) {
	if h.challenges == nil {
		writeError(w, http.StatusNotImplemented, errors.New("challenge service not configured"))
		return
	}

	list, err := h.challenges.ListHallOfFame(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, list)
}
