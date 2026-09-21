package http

import (
	"context"
	"errors"
	"net/http"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/store"
)

// OracleShopService defines Oracle Shop town facility operations exposed over HTTP.
type OracleShopService interface {
	GetOracleStatus(ctx context.Context, characterID string) (*store.OracleStatus, error)
	OracleTalk() string
	OracleInspect(jobLevel int) store.OracleInspectResult
	BuyCostumeItem(ctx context.Context, characterID string, itemNo int) (*store.CostumeBuyResult, error)
	BuyHomeWallpaper(ctx context.Context, characterID string, wallpaper string) (*store.HomeWallpaperResult, error)
	DiscoverBlackMarket(jobLevel int) error
}

// WithOracleShop configures the OracleShopService for the HTTP handler.
func WithOracleShop(oracle OracleShopService) Option {
	return func(h *Handler) {
		h.oracle = oracle
	}
}

type oracleBuyCostumeRequest struct {
	ItemNo int `json:"item_no"`
}

type oracleBuyWallpaperRequest struct {
	Wallpaper string `json:"wallpaper"`
}

type oracleDialogueResponse struct {
	CharacterID  string `json:"character_id"`
	LocationName string `json:"location_name"`
	NPCName      string `json:"npc_name"`
	Message      string `json:"message"`
}

type oracleInspectResponse struct {
	CharacterID  string `json:"character_id"`
	LocationName string `json:"location_name"`
	NPCName      string `json:"npc_name"`
	Message      string `json:"message"`
	Hint         string `json:"hint,omitempty"`
}

type oracleBlackMarketResponse struct {
	Unlocked bool   `json:"unlocked"`
	Message  string `json:"message"`
}

func (h *Handler) handleGetOracleStatus(w http.ResponseWriter, r *http.Request) {
	if h.oracle == nil {
		writeError(w, http.StatusNotImplemented, errors.New("oracle shop service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		status, err := h.oracle.GetOracleStatus(r.Context(), char.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, status)
	})
}

func (h *Handler) handleOracleTalk(w http.ResponseWriter, r *http.Request) {
	if h.oracle == nil {
		writeError(w, http.StatusNotImplemented, errors.New("oracle shop service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		msg := h.oracle.OracleTalk()
		writeJSON(w, http.StatusOK, oracleDialogueResponse{
			CharacterID:  char.ID,
			LocationName: store.OracleLocationName,
			NPCName:      store.OracleNPCName,
			Message:      msg,
		})
	})
}

func (h *Handler) handleOracleInspect(w http.ResponseWriter, r *http.Request) {
	if h.oracle == nil {
		writeError(w, http.StatusNotImplemented, errors.New("oracle shop service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		res := h.oracle.OracleInspect(char.JobLevel)
		writeJSON(w, http.StatusOK, oracleInspectResponse{
			CharacterID:  char.ID,
			LocationName: store.OracleLocationName,
			NPCName:      store.OracleNPCName,
			Message:      res.Message,
			Hint:         res.Hint,
		})
	})
}

func (h *Handler) handleOracleBuyCostume(w http.ResponseWriter, r *http.Request) {
	if h.oracle == nil {
		writeError(w, http.StatusNotImplemented, errors.New("oracle shop service not configured"))
		return
	}

	withAuthenticatedCharacterAndJSON(h, w, r, func(_ *oracleBuyCostumeRequest) string {
		return r.PathValue("id")
	}, func(_ coreplayer.Player, char corecharacter.Character, req oracleBuyCostumeRequest) {
		if req.ItemNo <= 0 {
			writeError(w, http.StatusBadRequest, errors.New("item_no must be positive"))
			return
		}

		res, err := h.oracle.BuyCostumeItem(r.Context(), char.ID, req.ItemNo)
		if err != nil {
			if errors.Is(err, store.ErrCostumeNotAvailable) ||
				errors.Is(err, store.ErrInsufficientFunds) ||
				errors.Is(err, store.ErrItemUnavailableInHelperQuest) ||
				errors.Is(err, depot.ErrDepotFull) {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
}

func (h *Handler) handleOracleBuyWallpaper(w http.ResponseWriter, r *http.Request) {
	if h.oracle == nil {
		writeError(w, http.StatusNotImplemented, errors.New("oracle shop service not configured"))
		return
	}

	withAuthenticatedCharacterAndJSON(h, w, r, func(_ *oracleBuyWallpaperRequest) string {
		return r.PathValue("id")
	}, func(_ coreplayer.Player, char corecharacter.Character, req oracleBuyWallpaperRequest) {
		if req.Wallpaper == "" {
			writeError(w, http.StatusBadRequest, errors.New("wallpaper cannot be empty"))
			return
		}

		res, err := h.oracle.BuyHomeWallpaper(r.Context(), char.ID, req.Wallpaper)
		if err != nil {
			if errors.Is(err, store.ErrInvalidWallpaper) || errors.Is(err, store.ErrInsufficientFunds) {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
}

func (h *Handler) handleOracleBlackMarket(w http.ResponseWriter, r *http.Request) {
	if h.oracle == nil {
		writeError(w, http.StatusNotImplemented, errors.New("oracle shop service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		if err := h.oracle.DiscoverBlackMarket(char.JobLevel); err != nil {
			if errors.Is(err, store.ErrBlackMarketNotDiscovered) {
				writeError(w, http.StatusForbidden, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, oracleBlackMarketResponse{
			Unlocked: true,
			Message:  "闇市場を見つけました！",
		})
	})
}
