package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/witchcraze/party2re/internal/chapel"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

// ChapelService defines the chapel blessings operations exposed over HTTP.
type ChapelService interface {
	GetBlessing(ctx context.Context, characterID string) (chapel.CharacterBlessing, error)
	GetStatus(ctx context.Context, characterID, characterName string) (chapel.ChapelStatus, error)
	SelectBlessing(ctx context.Context, characterID string, blessing chapel.BlessingType) (chapel.CharacterBlessing, error)
	ClearBlessing(ctx context.Context, characterID string) error
}

// WithChapel configures the chapel service for the Handler.
func WithChapel(c ChapelService) Option {
	return func(h *Handler) {
		h.chapel = c
		if c == nil {
			withActionCommand[prayChapelRequest]("chapel_pray", nil, nil)(h)
			return
		}
		withActionCommand("chapel_pray", func(ctx context.Context, actorID string, params prayChapelRequest) (any, error) {
			kind, err := chapel.ParseBlessingType(params.Blessing)
			if err != nil || kind == chapel.BlessingNone {
				return nil, chapel.ErrInvalidBlessing
			}
			actor, err := h.characters.Get(ctx, actorID)
			if err != nil {
				return nil, err
			}
			blessing, err := c.SelectBlessing(ctx, actorID, kind)
			if err != nil {
				return nil, err
			}
			var blessingName string
			for _, info := range chapel.AvailableBlessings {
				if info.Type == blessing.ActiveBlessing {
					blessingName = info.Name
					break
				}
			}
			return chapelBlessingResponse{Blessing: blessing, Message: fmt.Sprintf("%sは「%s」と祈るのですね…", actor.Name, blessingName)}, nil
		}, chapelActionRejection)(h)
	}
}

func chapelActionRejection(err error) (int, ErrorDetail) {
	switch {
	case errors.Is(err, chapel.ErrInvalidBlessing):
		return http.StatusBadRequest, ErrorDetail{Code: "CHAPEL_INVALID_BLESSING", Message: "Select a valid prayer."}
	case errors.Is(err, chapel.ErrAlreadyPrayed):
		return http.StatusConflict, ErrorDetail{Code: "CHAPEL_ALREADY_PRAYED", Message: chapel.MsgAlreadyPrayed}
	case errors.Is(err, chapel.ErrInvalidCharacterID):
		return http.StatusBadRequest, ErrorDetail{Code: "CHAPEL_INVALID_CHARACTER_ID", Message: "Invalid chapel character ID."}
	case errors.Is(err, corecharacter.ErrNotFound):
		return http.StatusNotFound, ErrorDetail{Code: "CHARACTER_NOT_FOUND", Message: "Character not found."}
	default:
		return 0, ErrorDetail{}
	}
}

type prayChapelRequest struct {
	Blessing string `json:"blessing"`
}

type chapelBlessingResponse struct {
	Blessing chapel.CharacterBlessing `json:"blessing"`
	Message  string                   `json:"message"`
}
