package http

import (
	"context"
	"errors"
	"net/http"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/medal"
)

type MedalService interface {
	GetRewards() []medal.Reward
	Claim(ctx context.Context, characterID string, itemID string) (corecharacter.Character, coreinventory.Inventory, error)
	GetAchievements(ctx context.Context, characterID string) ([]medal.AchievementProgress, error)
	ClaimAchievement(ctx context.Context, characterID string, achievementID string) (medal.ClaimResult, error)
	GetCharacterMedals(ctx context.Context, characterID string) ([]medal.CharacterMedal, error)
}

// WithMedal configures the MedalService for the handler.
func WithMedal(m MedalService) Option {
	return func(h *Handler) {
		h.medals = m
	}
}

type claimMedalRewardRequest struct {
	CharacterID string `json:"character_id"`
	ItemID      string `json:"item_id"`
}

type claimMedalRewardResponse struct {
	Character characterResponse       `json:"character"`
	Inventory coreinventory.Inventory `json:"inventory"`
}

func (h *Handler) handleGetMedalRewards(w http.ResponseWriter, r *http.Request) {
	if h.medals == nil {
		writeError(w, http.StatusNotImplemented, errors.New("medal service not available"))
		return
	}

	rewards := h.medals.GetRewards()
	writeJSON(w, http.StatusOK, rewards)
}

func (h *Handler) handleClaimMedalReward(w http.ResponseWriter, r *http.Request) {
	if h.medals == nil {
		writeError(w, http.StatusNotImplemented, errors.New("medal service not available"))
		return
	}

	withAuthenticatedCharacterAndJSON(h, w, r, func(req *claimMedalRewardRequest) string {
		return req.CharacterID
	}, func(_ coreplayer.Player, char corecharacter.Character, req claimMedalRewardRequest) {
		updatedChar, updatedInv, err := h.medals.Claim(r.Context(), char.ID, req.ItemID)
		if err != nil {
			if errors.Is(err, medal.ErrInsufficientMedals) {
				writeError(w, http.StatusUnprocessableEntity, err)
				return
			}
			if errors.Is(err, medal.ErrRewardNotFound) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, claimMedalRewardResponse{
			Character: toCharacterResponse(updatedChar),
			Inventory: updatedInv,
		})
	})
}

type characterAchievementsResponse struct {
	Achievements []medal.AchievementProgress `json:"achievements"`
}

type claimAchievementResponse struct {
	AchievementID      string               `json:"achievement_id"`
	AchievementName    string               `json:"achievement_name"`
	Medal              medal.CharacterMedal `json:"medal"`
	SmallMedalsAwarded int                  `json:"small_medals_awarded"`
	Character          characterResponse    `json:"character"`
}

type characterMedalsResponse struct {
	Medals []medal.CharacterMedal `json:"medals"`
}

func (h *Handler) handleGetCharacterAchievements(w http.ResponseWriter, r *http.Request) {
	if h.medals == nil {
		writeError(w, http.StatusNotImplemented, errors.New("medal service not available"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		achievements, err := h.medals.GetAchievements(r.Context(), char.ID)
		if err != nil {
			if errors.Is(err, corecharacter.ErrNotFound) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, characterAchievementsResponse{
			Achievements: achievements,
		})
	})
}

func (h *Handler) handleClaimAchievement(w http.ResponseWriter, r *http.Request) {
	if h.medals == nil {
		writeError(w, http.StatusNotImplemented, errors.New("medal service not available"))
		return
	}

	charID := r.PathValue("id")
	achievementID := r.PathValue("achievement_id")

	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		res, err := h.medals.ClaimAchievement(r.Context(), char.ID, achievementID)
		if err != nil {
			if errors.Is(err, medal.ErrAchievementNotFound) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			if errors.Is(err, medal.ErrAchievementAlreadyClaimed) {
				writeError(w, http.StatusConflict, err)
				return
			}
			if errors.Is(err, medal.ErrAchievementNotCompleted) {
				writeError(w, http.StatusUnprocessableEntity, err)
				return
			}
			if errors.Is(err, corecharacter.ErrNotFound) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, claimAchievementResponse{
			AchievementID:      res.AchievementID,
			AchievementName:    res.AchievementName,
			Medal:              res.Medal,
			SmallMedalsAwarded: res.SmallMedalsAwarded,
			Character:          toCharacterResponse(res.Character),
		})
	})
}

func (h *Handler) handleGetCharacterMedals(w http.ResponseWriter, r *http.Request) {
	if h.medals == nil {
		writeError(w, http.StatusNotImplemented, errors.New("medal service not available"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		medals, err := h.medals.GetCharacterMedals(r.Context(), char.ID)
		if err != nil {
			if errors.Is(err, corecharacter.ErrNotFound) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, characterMedalsResponse{
			Medals: medals,
		})
	})
}
