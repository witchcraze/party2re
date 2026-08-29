package http

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/witchcraze/party2re/internal/boss"
	"github.com/witchcraze/party2re/internal/challenge"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/dungeon"
	"github.com/witchcraze/party2re/internal/pvp"
)

// ChallengeService defines endurance challenge operations exposed over HTTP.
type ChallengeService interface {
	ListTiers() []challenge.ChallengeTier
	GetTier(tierID string) (*challenge.ChallengeTier, error)
	StartSession(ctx context.Context, characterID string, tierID string) (*challenge.ChallengeSession, error)
	AdvanceRound(ctx context.Context, sessionID string) (*challenge.RoundResult, *challenge.ChallengeSession, error)
	RetireSession(ctx context.Context, sessionID string) (*challenge.ChallengeSession, error)
	GetCharacterRecords(ctx context.Context, characterID string) ([]challenge.CharacterChallengeRecord, error)
}

// BossService defines boss encounters operations exposed over HTTP.
type BossService interface {
	ListBosses(ctx context.Context, characterID string) ([]boss.BossEncounterStatus, error)
	ChallengeBoss(ctx context.Context, characterID, bossID string) (boss.ChallengeResult, error)
	GetCharacterRecord(ctx context.Context, characterID string) (boss.CharacterBossRecord, error)
}

// DungeonService defines dungeon explorations operations exposed over HTTP.
type DungeonService interface {
	ListDungeons(ctx context.Context, characterID string) ([]dungeon.DungeonOverview, error)
	StartExpedition(ctx context.Context, characterID string, dungeonID string) (*dungeon.ActiveExpedition, error)
	Move(ctx context.Context, characterID string, dir dungeon.Direction) (*dungeon.ExpeditionStepResult, error)
	Escape(ctx context.Context, characterID string) (*dungeon.ExpeditionStepResult, error)
	GetActiveExpedition(ctx context.Context, characterID string) (*dungeon.ActiveExpedition, error)
}

// PvPService defines PvP arena operations exposed over HTTP.
type PvPService interface {
	GetRating(ctx context.Context, characterID string) (pvp.ArenaRating, error)
	FindOpponents(ctx context.Context, characterID string, limit int) ([]pvp.OpponentCandidate, error)
	Challenge(ctx context.Context, attackerID, defenderID string) (pvp.ChallengeResult, error)
}

// WithChallenge configures the challenge service for the Handler.
func WithChallenge(c ChallengeService) Option {
	return func(h *Handler) {
		h.challenges = c
	}
}

// WithBoss configures the boss service for the Handler.
func WithBoss(b BossService) Option {
	return func(h *Handler) {
		h.bosses = b
	}
}

// WithDungeon configures the dungeon service for the Handler.
func WithDungeon(d DungeonService) Option {
	return func(h *Handler) {
		h.dungeons = d
	}
}

// WithPvP configures the pvp arena service for the Handler.
func WithPvP(p PvPService) Option {
	return func(h *Handler) {
		h.pvp = p
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
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, _ corecharacter.Character) {
		var req advanceChallengeRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		if req.SessionID == "" {
			writeError(w, http.StatusBadRequest, errors.New("session_id is required"))
			return
		}

		roundRes, session, err := h.challenges.AdvanceRound(r.Context(), req.SessionID)
		if err != nil {
			if errors.Is(err, challenge.ErrSessionNotFound) {
				writeError(w, http.StatusNotFound, err)
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
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, _ corecharacter.Character) {
		var req retireChallengeRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		if req.SessionID == "" {
			writeError(w, http.StatusBadRequest, errors.New("session_id is required"))
			return
		}

		session, err := h.challenges.RetireSession(r.Context(), req.SessionID)
		if err != nil {
			if errors.Is(err, challenge.ErrSessionNotFound) {
				writeError(w, http.StatusNotFound, err)
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
// Boss Handlers
// -------------------------------------------------------------------

type challengeBossRequest struct {
	BossID string `json:"boss_id"`
}

type bossListResponse struct {
	Bosses []boss.BossEncounterStatus `json:"bosses"`
}

type bossChallengeResponse struct {
	Result boss.ChallengeResult `json:"result"`
}

func (h *Handler) handleListBosses(w http.ResponseWriter, r *http.Request) {
	if h.bosses == nil {
		writeError(w, http.StatusNotImplemented, errors.New("boss service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		statuses, err := h.bosses.ListBosses(r.Context(), char.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, bossListResponse{
			Bosses: statuses,
		})
	})
}

func (h *Handler) handleChallengeBoss(w http.ResponseWriter, r *http.Request) {
	if h.bosses == nil {
		writeError(w, http.StatusNotImplemented, errors.New("boss service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req challengeBossRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		if req.BossID == "" {
			writeError(w, http.StatusBadRequest, errors.New("boss_id is required"))
			return
		}

		res, err := h.bosses.ChallengeBoss(r.Context(), char.ID, req.BossID)
		if err != nil {
			if errors.Is(err, boss.ErrBossNotFound) || errors.Is(err, boss.ErrInvalidBossID) || errors.Is(err, boss.ErrCharacterNotFound) {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			if errors.Is(err, boss.ErrDailyAttemptsExhausted) || errors.Is(err, boss.ErrPrerequisiteNotMet) || errors.Is(err, boss.ErrLevelRequirementNotMet) {
				writeError(w, http.StatusUnprocessableEntity, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, bossChallengeResponse{
			Result: res,
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
	Result *dungeon.ExpeditionStepResult `json:"result"`
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

// -------------------------------------------------------------------
// PvP Arena Handlers
// -------------------------------------------------------------------

type pvpRatingResponse struct {
	Rating pvp.ArenaRating `json:"rating"`
}

type pvpOpponentsResponse struct {
	Opponents []pvp.OpponentCandidate `json:"opponents"`
}

type pvpFightRequest struct {
	DefenderID string `json:"defender_id"`
}

type pvpFightResponse struct {
	Result pvp.ChallengeResult `json:"result"`
}

func (h *Handler) handleGetPvPRating(w http.ResponseWriter, r *http.Request) {
	if h.pvp == nil {
		writeError(w, http.StatusNotImplemented, errors.New("pvp service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		rating, err := h.pvp.GetRating(r.Context(), char.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, pvpRatingResponse{
			Rating: rating,
		})
	})
}

func (h *Handler) handleFindPvPOpponents(w http.ResponseWriter, r *http.Request) {
	if h.pvp == nil {
		writeError(w, http.StatusNotImplemented, errors.New("pvp service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		limit := 5
		if l := r.URL.Query().Get("limit"); l != "" {
			if val, err := strconv.Atoi(l); err == nil && val > 0 {
				limit = val
			}
		}

		opponents, err := h.pvp.FindOpponents(r.Context(), char.ID, limit)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, pvpOpponentsResponse{
			Opponents: opponents,
		})
	})
}

func (h *Handler) handlePvPFight(w http.ResponseWriter, r *http.Request) {
	if h.pvp == nil {
		writeError(w, http.StatusNotImplemented, errors.New("pvp service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req pvpFightRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		if req.DefenderID == "" {
			writeError(w, http.StatusBadRequest, errors.New("defender_id is required"))
			return
		}

		res, err := h.pvp.Challenge(r.Context(), char.ID, req.DefenderID)
		if err != nil {
			if errors.Is(err, pvp.ErrCannotChallengeSelf) || errors.Is(err, pvp.ErrInvalidCharacterID) {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			if errors.Is(err, pvp.ErrCharacterDefeated) {
				writeError(w, http.StatusUnprocessableEntity, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, pvpFightResponse{
			Result: res,
		})
	})
}
