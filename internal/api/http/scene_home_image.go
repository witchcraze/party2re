package http

import (
	"context"
	"errors"
	"net/http"

	"github.com/witchcraze/party2re/internal/character"
	"github.com/witchcraze/party2re/internal/playercontext"
)

type HomeImageService interface {
	GetImage(ctx context.Context, characterID string) (character.ImageView, error)
	UpdateImage(ctx context.Context, characterID string, aura int, avatarID string) (character.Profile, error)
}

// ＠いめーじ is registered only in one's own home after the image unlock
// (home.cgi:87,104-106); the scene's CanEnter applies that unlock.
var homeImageFacility = facility{title: "イメージ", commands: []string{"home_image_update"}, read: (*Handler).homeImageSceneData}

type HomeImageEffect struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// homeImageEffects are the selectable effects in the legacy form's order
// (custom_image.cgi:104, names from _data.cgi @effects).
var homeImageEffects = []HomeImageEffect{
	{0, "なし"}, {1, "炎"}, {4, "青い炎"}, {5, "黒い炎"}, {2, "翼1"}, {3, "翼2"}, {6, "翼3"}, {7, "あやしいオブジェ1"}, {8, "あやしいオブジェ2"},
}

type HomeImageSceneData struct {
	Parent string `json:"parent"`
	character.ImageView
	Effects []HomeImageEffect `json:"effects"`
}

func WithHomeImage(images HomeImageService) Option {
	return func(h *Handler) {
		h.homeImages = images
		if images == nil {
			withActionCommand[homeImageUpdateRequest]("home_image_update", nil, nil)(h)
			return
		}
		withActionCommand("home_image_update", func(ctx context.Context, actor string, p homeImageUpdateRequest) (any, error) {
			return images.UpdateImage(ctx, actor, p.AuraEffect, p.AvatarID)
		}, homeImageRejection)(h)
	}
}

// homeImageSceneData always observes the actor, as home_image_update does.
func (h *Handler) homeImageSceneData(ctx context.Context, r playercontext.Result) (any, error) {
	if h.homeImages == nil {
		return nil, errSceneNotConfigured
	}
	view, err := h.homeImages.GetImage(ctx, r.Snapshot.Character.ID)
	if err != nil {
		return nil, err
	}
	return HomeImageSceneData{Parent: "home", ImageView: view, Effects: homeImageEffects}, nil
}

type homeImageUpdateRequest struct {
	AuraEffect int    `json:"aura_effect"`
	AvatarID   string `json:"avatar_id"`
}

func homeImageRejection(err error) (int, ErrorDetail) {
	switch {
	case errors.Is(err, character.ErrInvalidAuraEffect):
		return http.StatusBadRequest, ErrorDetail{Code: "IMAGE_EFFECT_INVALID", Message: "Choose an effect from 0 to 8."}
	case errors.Is(err, character.ErrInvalidAvatarURL):
		return http.StatusBadRequest, ErrorDetail{Code: "IMAGE_AVATAR_INVALID", Message: "The avatar is an imgur image ID of 5 to 10 letters or digits."}
	case errors.Is(err, character.ErrImageLocked), errors.Is(err, character.ErrAvatarLocked):
		return http.StatusForbidden, ErrorDetail{Code: "IMAGE_LOCKED", Message: "This image setting is not unlocked yet."}
	default:
		return 0, ErrorDetail{}
	}
}
