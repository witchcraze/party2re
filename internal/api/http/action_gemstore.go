package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/gemstore"
)

// GemStoreAppraiseResult adds the appraiser's reply.
type GemStoreAppraiseResult struct {
	gemstore.AppraiseResult
	Message string `json:"message"`
	commandEcho
}

// The other Gem Store results add the clerk's reply (gem_store.cgi:150,191,238,279,303).
type GemStoreBuyResult struct {
	gemstore.BuyResult
	Message string `json:"message"`
	commandEcho
}

type GemStoreSellResult struct {
	gemstore.SellResult
	Message string `json:"message"`
	commandEcho
}

type GemStoreSynthesizeResult struct {
	gemstore.SynthesizeResult
	Message string `json:"message"`
	commandEcho
}

type gemStoreSortResponse struct {
	GemBox  gemstore.GemBox `json:"gem_box"`
	Message string          `json:"message"`
	commandEcho
}

func gemStoreAppraiseReply(res gemstore.AppraiseResult) GemStoreAppraiseResult {
	msg := "これは… " + res.IdentifiedName + "ですね" // gem_store.cgi:350-378
	if res.IsGem {
		msg += "。" + res.Character.Name + "さんの宝石箱に入れておきました"
	}
	return GemStoreAppraiseResult{res, msg, commandEcho{target: res.ItemName, reply: msg}}
}

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

// gemStoreSendResponse keeps the recipient's private character and gem-box
// state out of the sender's result; only its public identity is returned.
type gemStoreSendResponse struct {
	GemBox                 gemstore.GemBox `json:"gem_box"`
	Gem                    gemstore.Gem    `json:"gem"`
	RecipientCharacterID   string          `json:"recipient_character_id"`
	RecipientCharacterName string          `json:"recipient_character_name"`
	Message                string          `json:"message"`
	commandEcho
}

// WithGemStore configures the gemstore scene observation and its commands.
func WithGemStore(service GemStoreService) Option {
	return func(h *Handler) {
		h.gemstore = service
		if service == nil {
			withEchoCommand[gemStoreBuyRequest, GemStoreBuyResult]("gemstore_buy", nil, nil)(h)
			withEchoCommand[gemStoreItemRequest, GemStoreSellResult]("gemstore_sell", nil, nil)(h)
			withEchoCommand[gemStoreSendRequest, gemStoreSendResponse]("gemstore_send", nil, nil)(h)
			withEchoCommand[gemStoreSynthesizeRequest, GemStoreSynthesizeResult]("gemstore_synthesize", nil, nil)(h)
			withEchoCommand[gemStoreItemRequest, GemStoreAppraiseResult]("gemstore_appraise", nil, nil)(h)
			withEchoCommand[struct{}, gemStoreSortResponse]("gemstore_sort", nil, nil)(h)
			return
		}
		withEchoCommand("gemstore_buy", func(ctx context.Context, actor string, p gemStoreBuyRequest) (GemStoreBuyResult, error) {
			res, err := service.BuyGem(ctx, actor, p.GemID)
			if err != nil {
				return GemStoreBuyResult{}, err
			}
			msg := res.Gem.Name + " を " + res.Character.Name + "さんの宝石箱に入れておきました"
			return GemStoreBuyResult{res, msg, commandEcho{target: res.Gem.Name, reply: msg}}, nil
		}, gemStoreActionRejection)(h)
		withEchoCommand("gemstore_sell", func(ctx context.Context, actor string, p gemStoreItemRequest) (GemStoreSellResult, error) {
			res, err := service.SellGem(ctx, actor, p.ItemID)
			if err != nil {
				return GemStoreSellResult{}, err
			}
			msg := fmt.Sprintf("%s の代金%d Gです", res.Gem.Name, res.Payout)
			return GemStoreSellResult{res, msg, commandEcho{target: res.Gem.Name, reply: msg}}, nil
		}, gemStoreActionRejection)(h)
		withEchoCommand("gemstore_send", func(ctx context.Context, actor string, p gemStoreSendRequest) (gemStoreSendResponse, error) {
			res, err := service.SendGem(ctx, actor, p.RecipientCharacterID, p.ItemID)
			if err != nil {
				return gemStoreSendResponse{}, err
			}
			name := res.RecipientCharacter.Name
			msg := res.Gem.Name + " を" + name + "さんに送りました"
			return gemStoreSendResponse{GemBox: res.SenderGemBox, Gem: res.Gem, RecipientCharacterID: res.RecipientCharacter.ID, RecipientCharacterName: name,
				Message: msg, commandEcho: commandEcho{target: res.Gem.Name + "＠あいて>" + name, reply: msg}}, nil
		}, gemStoreActionRejection)(h)
		withEchoCommand("gemstore_synthesize", func(ctx context.Context, actor string, p gemStoreSynthesizeRequest) (GemStoreSynthesizeResult, error) {
			res, err := service.SynthesizeGem(ctx, actor, p.RecipeID)
			var missing *gemstore.InsufficientMaterialsError
			if errors.As(err, &missing) {
				// Missing materials are a logged $npc_com that changes nothing (gem_store.cgi:312-315).
				r := missing.Recipe
				msg := fmt.Sprintf("%s と %s をお持ちでないようですね", r.Material1Name, r.Material2Name)
				return GemStoreSynthesizeResult{gemstore.SynthesizeResult{Recipe: r}, msg, commandEcho{target: r.Material1Name + "×" + r.Material2Name, reply: msg}}, nil
			}
			if err != nil {
				return GemStoreSynthesizeResult{}, err
			}
			r := res.Recipe
			msg := r.ResultName + " が完成しました。宝石箱に入れておきました"
			return GemStoreSynthesizeResult{res, msg, commandEcho{target: r.Material1Name + "×" + r.Material2Name, reply: msg}}, nil
		}, gemStoreActionRejection)(h)
		withEchoCommand("gemstore_appraise", func(ctx context.Context, actor string, p gemStoreItemRequest) (GemStoreAppraiseResult, error) {
			res, err := service.AppraiseItem(ctx, actor, p.ItemID)
			if err != nil {
				return GemStoreAppraiseResult{}, err
			}
			return gemStoreAppraiseReply(res), nil
		}, gemStoreActionRejection)(h)
		withEchoCommand("gemstore_sort", func(ctx context.Context, actor string, _ struct{}) (gemStoreSortResponse, error) {
			box, err := service.SortGemBox(ctx, actor)
			if err != nil {
				return gemStoreSortResponse{}, err
			}
			const msg = "お預かりしている宝石をせいとんしました"
			return gemStoreSortResponse{box, msg, commandEcho{reply: msg}}, nil
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
	case errors.Is(err, gemstore.ErrGemNotForSale):
		return http.StatusBadRequest, ErrorDetail{Code: "GEMSTORE_GEM_NOT_FOR_SALE", Message: "This gem is not on sale at your job level."}
	case errors.Is(err, gemstore.ErrInsufficientFunds):
		return http.StatusBadRequest, ErrorDetail{Code: "GEMSTORE_INSUFFICIENT_FUNDS", Message: "Insufficient wallet funds."}
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
