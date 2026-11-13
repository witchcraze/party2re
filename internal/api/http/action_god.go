package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/god"
	"github.com/witchcraze/party2re/internal/playercontext"
)

// GodService is the God surface used by the god/u_god scenes and god_wish.
type GodService interface {
	GetWishes(ctx context.Context, characterID string, realm god.Realm) ([]god.Wish, error)
	GrantWish(ctx context.Context, characterID, wishID string) (god.WishResult, error)
}

type godWishRequest struct {
	WishID string `json:"wish_id"`
}

// GodSceneData is the realm the actor's visit stands in and its wish list.
type GodSceneData struct {
	Parent string     `json:"parent"`
	Realm  god.Realm  `json:"realm"`
	Wishes []god.Wish `json:"wishes"`
}

// godWishResponse adds the god's HTTP-owned reply to the service result.
type godWishResponse struct {
	god.WishResult
	NPCSpeech string `json:"npc_speech"`
}

// WithGod configures the god scenes and the god_wish command.
func WithGod(service GodService) Option {
	return func(h *Handler) {
		h.god = service
		if service == nil {
			withActionCommand[godWishRequest]("god_wish", nil, nil)(h)
			return
		}
		withActionCommand("god_wish", func(ctx context.Context, actor string, p godWishRequest) (any, error) {
			res, err := service.GrantWish(ctx, actor, p.WishID)
			if err != nil {
				return nil, err
			}
			return godWishResponse{WishResult: res, NPCSpeech: godWishNPCSpeech(res.Wish, res.Character.Name)}, nil
		}, godActionRejection)(h)
	}
}

func (h *Handler) godSceneData(ctx context.Context, r playercontext.Result) (any, error) {
	if h.god == nil {
		return nil, errSceneNotConfigured
	}
	realm := god.Realm(r.Snapshot.Character.GodVisit)
	wishes, err := h.god.GetWishes(ctx, r.Snapshot.Character.ID, realm)
	if err != nil {
		return nil, err
	}
	return GodSceneData{Parent: "town", Realm: realm, Wishes: append([]god.Wish{}, wishes...)}, nil
}

// godWishNPCSpeech is ＠ねがう's reply; both realms share it (god.cgi:172–175, u_god.cgi:66–68).
func godWishNPCSpeech(wish god.Wish, characterName string) string {
	if wish.ID == god.WishLover {
		return "それは無理な願いだ…。アドバイスとしては積極的にアピールするのだ…"
	}
	return fmt.Sprintf("ふむ。%sの願いは「%s」だな。\n%sの願いを叶えたぞ…。機会があればまたあえるだろう…。さらばだ…", characterName, wish.Name, characterName)
}

func godActionRejection(err error) (int, ErrorDetail) {
	switch {
	case errors.Is(err, corecharacter.ErrNotFound), errors.Is(err, god.ErrCharacterNotFound):
		return http.StatusNotFound, ErrorDetail{Code: "CHARACTER_NOT_FOUND", Message: "Character not found."}
	case errors.Is(err, god.ErrNotAdmitted):
		return http.StatusConflict, ErrorDetail{Code: "GOD_NOT_ADMITTED", Message: "No god visit is recorded."}
	case errors.Is(err, god.ErrWishNotFound), errors.Is(err, god.ErrInvalidWishID):
		return http.StatusBadRequest, ErrorDetail{Code: "GOD_WISH_NOT_FOUND", Message: "Choose one of the observed wishes."}
	case errors.Is(err, god.ErrLimitBreakMaxed):
		return http.StatusConflict, ErrorDetail{Code: "GOD_LIMIT_BREAK_MAXED", Message: "This limit is already at its maximum tier."}
	case errors.Is(err, god.ErrWishRequirement):
		return http.StatusConflict, ErrorDetail{Code: "GOD_WISH_REQUIREMENT", Message: "The wish requirements are not met."}
	default:
		return 0, ErrorDetail{}
	}
}

// god.cgi/u_god.cgi @words and shiraberu_npc.
var (
	godWords    = []string{"{name}よ。よくぞここまできた。{name}の願いを一つだけ叶えてやろう"}
	godFacility = facility{title: "天界", commands: []string{"god_wish"}, read: (*Handler).godSceneData, placeLog: true,
		npc: &npcProfile{name: "@神", words: godWords, inspect: "@神「本当の願いは自分の力で叶えるのだ…」"}}
	underworldGodFacility = facility{title: "天界?", commands: []string{"god_wish"}, read: (*Handler).godSceneData, placeLog: true,
		npc: &npcProfile{name: "@神?", words: godWords, silent: true}}
)
