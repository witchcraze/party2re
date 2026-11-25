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

type bossListResponse struct {
	Bosses []boss.BossEncounterStatus `json:"bosses"`
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

// -------------------------------------------------------------------
// Sealing Battle Handler (vs_king.cgi)
// -------------------------------------------------------------------

type startSealingBattleRequest struct {
	CharacterID string `json:"character_id"`
}

type sealingBattleResponse struct {
	Result bossBattleResultResponse `json:"result"`
}

// handleStartSealingBattle starts a sealing room's battle (封印戦); any room member may trigger it.
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

	withAuthenticatedActionCharacterAndJSON(h, w, r, func(req *startSealingBattleRequest) string {
		return req.CharacterID
	}, func(player coreplayer.Player, char corecharacter.Character, req startSealingBattleRequest) {
		res, err := h.bosses.StartSealingBattle(r.Context(), partyID, char.ID)
		if err != nil {
			switch {
			case errors.Is(err, boss.ErrPartyNotFound), errors.Is(err, boss.ErrBossNotFound):
				writeError(w, http.StatusNotFound, err)
			case errors.Is(err, boss.ErrNotRoomMember), errors.Is(err, boss.ErrPartyNotReady):
				writeError(w, http.StatusForbidden, err)
			case errors.Is(err, boss.ErrNeedJoinNotMet), errors.Is(err, boss.ErrCharacterExhausted), errors.Is(err, boss.ErrCharacterUnconscious), errors.Is(err, boss.ErrPartyNotRecruiting):
				writeError(w, http.StatusUnprocessableEntity, err)
			default:
				writeError(w, http.StatusInternalServerError, err)
			}
			return
		}
		writeJSON(w, http.StatusOK, sealingBattleResponse{Result: toBossBattleResult(res)})
	})
}
