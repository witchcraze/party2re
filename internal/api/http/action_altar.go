package http

import (
	"context"
	"errors"
	"net/http"

	"github.com/witchcraze/party2re/internal/altar"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/playercontext"
	"github.com/witchcraze/party2re/internal/wishingwell"
)

// AltarService is the Altar of Rebirth surface used by its scene and commands.
type AltarService interface {
	GetStatus(ctx context.Context, characterID string) (altar.AltarStatus, error)
	Pray(ctx context.Context, characterID string) (altar.PrayResult, error)
	Wish(ctx context.Context, characterID string, itemID string) (altar.WishResult, error)
}

// WishingWellService is the Wishing Well command surface.
type WishingWellService interface {
	Exchange(ctx context.Context, req wishingwell.ExchangeRequest) (wishingwell.ExchangeResult, error)
}

type altarWishRequest struct {
	ItemID string `json:"item_id"`
}

type wishingWellExchangeRequest struct {
	Stat string `json:"stat"`
	SP   int    `json:"sp"`
}

// AltarSceneData is the actor's orb progress and the shared Ramia presence.
type AltarSceneData struct {
	Parent string `json:"parent"`
	altar.AltarStatus
}

// WishingWellSceneData is the actor's exchange eligibility; SP and stats are
// already in the character context.
type WishingWellSceneData struct {
	Parent string `json:"parent"`
	wishingwell.WishingWellStatus
}

// WithAltar configures the altar scene observation and its commands.
func WithAltar(service AltarService) Option {
	return func(h *Handler) {
		h.altar = service
		if service == nil {
			withActionCommand[struct{}]("altar_pray", nil, nil)(h)
			withActionCommand[altarWishRequest]("altar_wish", nil, nil)(h)
			return
		}
		withActionCommand("altar_pray", func(ctx context.Context, actor string, _ struct{}) (any, error) {
			return service.Pray(ctx, actor)
		}, altarActionRejection)(h)
		withActionCommand("altar_wish", func(ctx context.Context, actor string, p altarWishRequest) (any, error) {
			return service.Wish(ctx, actor, p.ItemID)
		}, altarActionRejection)(h)
	}
}

// WithWishingWell configures the wishing well exchange command.
func WithWishingWell(service WishingWellService) Option {
	return func(h *Handler) {
		if service == nil {
			withActionCommand[wishingWellExchangeRequest]("wishingwell_exchange", nil, nil)(h)
			return
		}
		withActionCommand("wishingwell_exchange", func(ctx context.Context, actor string, p wishingWellExchangeRequest) (any, error) {
			return service.Exchange(ctx, wishingwell.ExchangeRequest{CharacterID: actor, Stat: p.Stat, SP: p.SP})
		}, wishingWellActionRejection)(h)
	}
}

func (h *Handler) altarSceneData(ctx context.Context, r playercontext.Result) (any, error) {
	if h.altar == nil {
		return nil, errSceneNotConfigured
	}
	status, err := h.altar.GetStatus(ctx, r.Snapshot.Character.ID)
	if err != nil {
		return nil, err
	}
	return AltarSceneData{Parent: "town", AltarStatus: status}, nil
}

func (h *Handler) wishingWellSceneData(_ context.Context, r playercontext.Result) (any, error) {
	return WishingWellSceneData{Parent: "town", WishingWellStatus: wishingwell.StatusOf(r.Snapshot.Character)}, nil
}

func altarActionRejection(err error) (int, ErrorDetail) {
	switch {
	case errors.Is(err, corecharacter.ErrNotFound):
		return http.StatusNotFound, ErrorDetail{Code: "CHARACTER_NOT_FOUND", Message: "Character not found."}
	case errors.Is(err, altar.ErrInsufficientOrbs):
		return http.StatusBadRequest, ErrorDetail{Code: "ALTAR_INSUFFICIENT_ORBS", Message: altar.MsgOrbsInsufficient}
	case errors.Is(err, altar.ErrRamiaAwakened):
		return http.StatusConflict, ErrorDetail{Code: "ALTAR_RAMIA_AWAKENED", Message: "Ramia is already awakened."}
	case errors.Is(err, altar.ErrRamiaNotAwakened):
		return http.StatusConflict, ErrorDetail{Code: "ALTAR_RAMIA_NOT_AWAKENED", Message: "Ramia is not awakened."}
	case errors.Is(err, altar.ErrInvalidWishItem):
		return http.StatusBadRequest, ErrorDetail{Code: "ALTAR_INVALID_WISH_ITEM", Message: "Choose one of the observed wish items."}
	default:
		return 0, ErrorDetail{}
	}
}

func wishingWellActionRejection(err error) (int, ErrorDetail) {
	switch {
	case errors.Is(err, corecharacter.ErrNotFound):
		return http.StatusNotFound, ErrorDetail{Code: "CHARACTER_NOT_FOUND", Message: "Character not found."}
	case errors.Is(err, wishingwell.ErrInvalidTargetStat):
		return http.StatusBadRequest, ErrorDetail{Code: "WISHINGWELL_INVALID_STAT", Message: "Choose mhp, mmp, at, df or ag."}
	case errors.Is(err, wishingwell.ErrInvalidSPAmount):
		return http.StatusBadRequest, ErrorDetail{Code: "WISHINGWELL_INVALID_SP", Message: "SP must be at least 1."}
	case errors.Is(err, wishingwell.ErrInsufficientSP):
		return http.StatusBadRequest, ErrorDetail{Code: "WISHINGWELL_INSUFFICIENT_SP", Message: "Not enough SP."}
	case errors.Is(err, wishingwell.ErrJobMemoryActive):
		return http.StatusConflict, ErrorDetail{Code: "WISHINGWELL_JOB_MEMORY", Message: "思いだした職業のSPは使えません"}
	case errors.Is(err, wishingwell.ErrOverLevelRestricted):
		return http.StatusConflict, ErrorDetail{Code: "WISHINGWELL_OVER_LEVEL", Message: "特別な強さを持った人はSPをささげてもステータスを上げられません"}
	default:
		return 0, ErrorDetail{}
	}
}
