package http

import (
	"context"
	"errors"
	"net/http"

	"github.com/witchcraze/party2re/internal/boss"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
)

// BossService defines boss encounters operations exposed over HTTP.
type BossService interface {
	ListBosses(ctx context.Context, characterID string) ([]boss.BossEncounterStatus, error)
	ChallengeBoss(ctx context.Context, characterID, bossID string) (boss.ChallengeResult, error)
	StartSealingBattle(ctx context.Context, partyID, leaderCharID string) (boss.SealingBattleResult, error)
	GetCharacterRecord(ctx context.Context, characterID string) (boss.CharacterBossRecord, error)
}

// WithBoss configures the boss service for the Handler.
func WithBoss(b BossService) Option {
	return func(h *Handler) {
		h.bosses = b
	}
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
			if errors.Is(err, boss.ErrNeedJoinNotMet) || errors.Is(err, boss.ErrCharacterExhausted) || errors.Is(err, boss.ErrCharacterUnconscious) {
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
// Sealing Battle Handler (vs_king.cgi)
// -------------------------------------------------------------------

type startSealingBattleRequest struct {
	CharacterID string `json:"character_id"`
}

type sealingBattleResponse struct {
	Result boss.SealingBattleResult `json:"result"`
}

// handleStartSealingBattle starts an authentic Party Sealing Battle (封印戦).
// Only the party leader may trigger this; all members must be ready.
// POST /parties/{id}/sealing-battle
func (h *Handler) handleStartSealingBattle(w http.ResponseWriter, r *http.Request) {
	if h.bosses == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "boss service not configured"})
		return
	}

	partyID := r.PathValue("id")
	if partyID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "party id is required"})
		return
	}

	withAuthenticatedCharacterAndJSON(h, w, r, func(req *startSealingBattleRequest) string {
		return req.CharacterID
	}, func(player coreplayer.Player, char corecharacter.Character, req startSealingBattleRequest) {
		res, err := h.bosses.StartSealingBattle(r.Context(), partyID, char.ID)
		if err != nil {
			switch {
			case errors.Is(err, boss.ErrPartyNotFound):
				writeError(w, http.StatusNotFound, err)
			case errors.Is(err, boss.ErrNotPartyLeader), errors.Is(err, boss.ErrPartyNotReady):
				writeError(w, http.StatusForbidden, err)
			case errors.Is(err, boss.ErrNeedJoinNotMet), errors.Is(err, boss.ErrCharacterExhausted), errors.Is(err, boss.ErrCharacterUnconscious):
				writeError(w, http.StatusUnprocessableEntity, err)
			default:
				writeError(w, http.StatusInternalServerError, err)
			}
			return
		}
		writeJSON(w, http.StatusOK, sealingBattleResponse{Result: res})
	})
}
