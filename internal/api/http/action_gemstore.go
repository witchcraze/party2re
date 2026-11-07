package http

import (
	"context"
	"errors"
	"net/http"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/gemstore"
)

// GemStoreService is the Gem Store surface used by the scene and its commands.
type GemStoreService interface {
	BuyGem(ctx context.Context, characterID, gemID string) (gemstore.BuyResult, error)
	SellGem(ctx context.Context, characterID, itemInstanceOrDefID string) (gemstore.SellResult, error)
	SendGem(ctx context.Context, senderID, recipientID, itemInstanceOrDefID string) (gemstore.SendResult, error)
	SynthesizeGem(ctx context.Context, characterID, recipeID string) (gemstore.SynthesizeResult, error)
	AppraiseItem(ctx context.Context, characterID, itemInstanceOrDefID string) (gemstore.AppraiseResult, error)
	GetGemBox(ctx context.Context, characterID string) (gemstore.GemBox, error)
	SortGemBox(ctx context.Context, characterID string) (gemstore.GemBox, error)
	GetCatalog(jobLevel int) []gemstore.Gem
	GetRecipes() []gemstore.Recipe
	GetDialogue() []string
}

type gemStoreItemRequest struct {
	ItemID string `json:"item_id"`
}

type gemStoreBuyRequest struct {
	GemID string `json:"gem_id"`
}

type gemStoreSendRequest struct {
	RecipientCharacterID string `json:"recipient_character_id"`
	ItemID               string `json:"item_id"`
}

type gemStoreSynthesizeRequest struct {
	RecipeID string `json:"recipe_id"`
}

// WithGemStore configures the gemstore scene observation and its commands.
func WithGemStore(service GemStoreService) Option {
	return func(h *Handler) {
		h.gemstore = service
		if service == nil {
			withActionCommand[gemStoreBuyRequest]("gemstore_buy", nil, nil)(h)
			withActionCommand[gemStoreItemRequest]("gemstore_sell", nil, nil)(h)
			withActionCommand[gemStoreSendRequest]("gemstore_send", nil, nil)(h)
			withActionCommand[gemStoreSynthesizeRequest]("gemstore_synthesize", nil, nil)(h)
			withActionCommand[gemStoreItemRequest]("gemstore_appraise", nil, nil)(h)
			withActionCommand[struct{}]("gemstore_sort", nil, nil)(h)
			return
		}
		withActionCommand("gemstore_buy", func(ctx context.Context, actor string, p gemStoreBuyRequest) (any, error) {
			return service.BuyGem(ctx, actor, p.GemID)
		}, gemStoreActionRejection)(h)
		withActionCommand("gemstore_sell", func(ctx context.Context, actor string, p gemStoreItemRequest) (any, error) {
			return service.SellGem(ctx, actor, p.ItemID)
		}, gemStoreActionRejection)(h)
		withActionCommand("gemstore_send", func(ctx context.Context, actor string, p gemStoreSendRequest) (any, error) {
			return service.SendGem(ctx, actor, p.RecipientCharacterID, p.ItemID)
		}, gemStoreActionRejection)(h)
		withActionCommand("gemstore_synthesize", func(ctx context.Context, actor string, p gemStoreSynthesizeRequest) (any, error) {
			return service.SynthesizeGem(ctx, actor, p.RecipeID)
		}, gemStoreActionRejection)(h)
		withActionCommand("gemstore_appraise", func(ctx context.Context, actor string, p gemStoreItemRequest) (any, error) {
			return service.AppraiseItem(ctx, actor, p.ItemID)
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
	case errors.Is(err, gemstore.ErrRecipeNotFound):
		return http.StatusNotFound, ErrorDetail{Code: "GEMSTORE_RECIPE_NOT_FOUND", Message: "Recipe not found."}
	case errors.Is(err, gemstore.ErrGemBoxNotFound):
		return http.StatusNotFound, ErrorDetail{Code: "GEMSTORE_BOX_NOT_FOUND", Message: "Gem box not found."}
	case errors.Is(err, gemstore.ErrInvalidCharacterID):
		return http.StatusBadRequest, ErrorDetail{Code: "GEMSTORE_INVALID_CHARACTER_ID", Message: "Invalid character ID."}
	case errors.Is(err, gemstore.ErrInvalidGemID):
		return http.StatusBadRequest, ErrorDetail{Code: "GEMSTORE_INVALID_GEM_ID", Message: "Invalid gem ID."}
	case errors.Is(err, gemstore.ErrInvalidRecipeID):
		return http.StatusBadRequest, ErrorDetail{Code: "GEMSTORE_INVALID_RECIPE_ID", Message: "Invalid recipe ID."}
	case errors.Is(err, gemstore.ErrItemNotOwned):
		return http.StatusBadRequest, ErrorDetail{Code: "GEMSTORE_ITEM_NOT_OWNED", Message: "Item is not owned."}
	case errors.Is(err, gemstore.ErrLevelTooLow):
		return http.StatusBadRequest, ErrorDetail{Code: "GEMSTORE_JOB_LEVEL_TOO_LOW", Message: "Job level is too low for this gem."}
	case errors.Is(err, gemstore.ErrInsufficientFunds):
		return http.StatusBadRequest, ErrorDetail{Code: "GEMSTORE_INSUFFICIENT_FUNDS", Message: "Insufficient wallet funds."}
	case errors.Is(err, gemstore.ErrInsufficientMaterials):
		return http.StatusBadRequest, ErrorDetail{Code: "GEMSTORE_INSUFFICIENT_MATERIALS", Message: "Recipe materials are not owned."}
	case errors.Is(err, gemstore.ErrCannotSendToSelf):
		return http.StatusBadRequest, ErrorDetail{Code: "GEMSTORE_SELF_TRANSFER", Message: "Cannot send a gem to yourself."}
	case errors.Is(err, gemstore.ErrGemBoxFull):
		return http.StatusBadRequest, ErrorDetail{Code: "GEMSTORE_BOX_FULL", Message: "Gem box is full."}
	case errors.Is(err, gemstore.ErrRecipientGemBoxFull):
		return http.StatusBadRequest, ErrorDetail{Code: "GEMSTORE_RECIPIENT_BOX_FULL", Message: "Recipient gem box is full."}
	default:
		return 0, ErrorDetail{}
	}
}
