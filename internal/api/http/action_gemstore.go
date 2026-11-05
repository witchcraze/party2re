package http

import (
	"context"
	"errors"
	"net/http"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/gemstore"
)

type gemStoreSellRequest struct {
	ItemID string `json:"item_id"`
}

// WithGemStore configures retained reads/operations and gem-box commands.
func WithGemStore(service GemStoreService) Option {
	return func(h *Handler) {
		h.gemstore = service
		if service == nil {
			withActionCommand[gemStoreSellRequest]("gemstore_sell", nil, nil)(h)
			withActionCommand[struct{}]("gemstore_sort", nil, nil)(h)
			return
		}
		withActionCommand("gemstore_sell", func(ctx context.Context, actor string, p gemStoreSellRequest) (any, error) {
			return service.SellGem(ctx, actor, p.ItemID)
		}, gemStoreActionRejection)(h)
		withActionCommand("gemstore_sort", func(ctx context.Context, actor string, _ struct{}) (any, error) {
			box, err := service.SortGemBox(ctx, actor)
			if err != nil {
				return nil, err
			}
			return map[string]any{"gem_box": box, "message": "お預かりしている宝石をせいとんしました"}, nil
		}, gemStoreActionRejection)(h)
	}
}

func gemStoreActionRejection(err error) (int, ErrorDetail) {
	switch {
	case errors.Is(err, corecharacter.ErrNotFound):
		return http.StatusNotFound, ErrorDetail{Code: "CHARACTER_NOT_FOUND", Message: "Character not found."}
	case errors.Is(err, gemstore.ErrGemNotFound):
		return http.StatusNotFound, ErrorDetail{Code: "GEMSTORE_GEM_NOT_FOUND", Message: "Gem not found."}
	case errors.Is(err, gemstore.ErrGemBoxNotFound):
		return http.StatusNotFound, ErrorDetail{Code: "GEMSTORE_BOX_NOT_FOUND", Message: "Gem box not found."}
	case errors.Is(err, gemstore.ErrInvalidCharacterID):
		return http.StatusBadRequest, ErrorDetail{Code: "GEMSTORE_INVALID_CHARACTER_ID", Message: "Invalid character ID."}
	case errors.Is(err, gemstore.ErrInvalidGemID):
		return http.StatusBadRequest, ErrorDetail{Code: "GEMSTORE_INVALID_GEM_ID", Message: "Invalid gem ID."}
	case errors.Is(err, gemstore.ErrItemNotOwned):
		return http.StatusBadRequest, ErrorDetail{Code: "GEMSTORE_ITEM_NOT_OWNED", Message: "Item not found in your gem box."}
	default:
		return 0, ErrorDetail{}
	}
}
