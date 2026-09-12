package http

import (
	"context"
	"errors"
	"net/http"

	"github.com/witchcraze/party2re/internal/challenge"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/dungeon"
)

// ChallengeService defines endurance challenge operations exposed over HTTP.
type ChallengeService interface {
	ListTiers() []challenge.ChallengeTier
	GetTier(tierID string) (*challenge.ChallengeTier, error)
	StartSession(ctx context.Context, characterID string, tierID string) (*challenge.ChallengeSession, error)
	AdvanceRound(ctx context.Context, characterID string, sessionID string) (*challenge.RoundResult, *challenge.ChallengeSession, error)
	RetireSession(ctx context.Context, characterID string, sessionID string) (*challenge.ChallengeSession, error)
	GetCharacterRecords(ctx context.Context, characterID string) ([]challenge.CharacterChallengeRecord, error)
}

// DungeonService defines dungeon explorations operations exposed over HTTP.
type DungeonService interface {
	ListDungeons(ctx context.Context, characterID string) ([]dungeon.DungeonOverview, error)
	StartExpedition(ctx context.Context, characterID string, dungeonID string) (*dungeon.ActiveExpedition, error)
	Move(ctx context.Context, characterID string, dir dungeon.Direction) (dungeon.ExpeditionStepResult, error)
	Escape(ctx context.Context, characterID string) (dungeon.ExpeditionStepResult, error)
	GetActiveExpedition(ctx context.Context, characterID string) (*dungeon.ActiveExpedition, error)
}

// WithChallenge configures the challenge service for the Handler.
func WithChallenge(c ChallengeService) Option {
	return func(h *Handler) {
		h.challenges = c
	}
}

// WithDungeon configures the dungeon service for the Handler.
func WithDungeon(d DungeonService) Option {
	return func(h *Handler) {
		h.dungeons = d
	}
}

// -------------------------------------------------------------------
// Challenge Handlers
// -------------------------------------------------------------------

type startChallengeRequest struct {
	TierID string `json:"tier_id"`
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

// -------------------------------------------------------------------
// Dungeon Handlers
// -------------------------------------------------------------------

type startDungeonRequest struct {
	DungeonID string `json:"dungeon_id"`
}

type moveDungeonRequest struct {
	Direction string `json:"direction"` // "north", "south", "east", "west"
}

type dungeonListResponse struct {
	Dungeons []dungeon.DungeonOverview `json:"dungeons"`
}

type startDungeonResponse struct {
	Expedition *dungeon.ActiveExpedition `json:"expedition"`
}

type dungeonStepResponse struct {
	Result dungeon.ExpeditionStepResult `json:"result"`
}

func (h *Handler) handleListDungeons(w http.ResponseWriter, r *http.Request) {
	if h.dungeons == nil {
		writeError(w, http.StatusNotImplemented, errors.New("dungeon service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		dungeonsList, err := h.dungeons.ListDungeons(r.Context(), char.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, dungeonListResponse{
			Dungeons: dungeonsList,
		})
	})
}

func (h *Handler) handleStartDungeon(w http.ResponseWriter, r *http.Request) {
	if h.dungeons == nil {
		writeError(w, http.StatusNotImplemented, errors.New("dungeon service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req startDungeonRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		if req.DungeonID == "" {
			writeError(w, http.StatusBadRequest, errors.New("dungeon_id is required"))
			return
		}

		exp, err := h.dungeons.StartExpedition(r.Context(), char.ID, req.DungeonID)
		if err != nil {
			if errors.Is(err, dungeon.ErrDungeonNotFound) || errors.Is(err, dungeon.ErrCharacterNotFound) {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			if errors.Is(err, dungeon.ErrLevelRequirementNotMet) || errors.Is(err, dungeon.ErrActiveExpeditionExists) {
				writeError(w, http.StatusUnprocessableEntity, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, startDungeonResponse{
			Expedition: exp,
		})
	})
}

func (h *Handler) handleMoveDungeon(w http.ResponseWriter, r *http.Request) {
	if h.dungeons == nil {
		writeError(w, http.StatusNotImplemented, errors.New("dungeon service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req moveDungeonRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		dir := dungeon.Direction(req.Direction)
		res, err := h.dungeons.Move(r.Context(), char.ID, dir)
		if err != nil {
			if errors.Is(err, dungeon.ErrNoActiveExpedition) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			if errors.Is(err, dungeon.ErrInvalidDirection) || errors.Is(err, dungeon.ErrImpassableWall) {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, dungeonStepResponse{
			Result: res,
		})
	})
}

func (h *Handler) handleEscapeDungeon(w http.ResponseWriter, r *http.Request) {
	if h.dungeons == nil {
		writeError(w, http.StatusNotImplemented, errors.New("dungeon service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		res, err := h.dungeons.Escape(r.Context(), char.ID)
		if err != nil {
			if errors.Is(err, dungeon.ErrNoActiveExpedition) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, dungeonStepResponse{
			Result: res,
		})
	})
}
