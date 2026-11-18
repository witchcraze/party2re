package character

import (
	"context"
	"errors"
	"time"
)

var (
	ErrImageLocked  = errors.New("image settings are not unlocked")
	ErrAvatarLocked = errors.New("avatar setting is not unlocked")
)

// ImageUnlocks are the adventure record's durable いめーじ and avatar unlocks.
type ImageUnlocks struct {
	Image  bool
	Avatar bool
}

type ImageUnlockReader interface {
	ImageUnlocks(ctx context.Context, characterID string) (ImageUnlocks, error)
}

func WithImageUnlocks(reader ImageUnlockReader) Option {
	return func(s *Service) { s.imageUnlocks = reader }
}

type ImageView struct {
	AuraEffect     int    `json:"aura_effect"`
	AvatarURL      string `json:"avatar_url"`
	AvatarUnlocked bool   `json:"avatar_unlocked"`
}

// ImageUnlocks reads the unlocks that gate own-Home いめーじ (home.cgi:87,104-106).
func (s *Service) ImageUnlocks(ctx context.Context, characterID string) (ImageUnlocks, error) {
	if s.imageUnlocks == nil {
		return ImageUnlocks{}, errors.New("image unlock reader is not configured")
	}
	return s.imageUnlocks.ImageUnlocks(ctx, characterID)
}

func (s *Service) GetImage(ctx context.Context, characterID string) (ImageView, error) {
	unlocks, err := s.ImageUnlocks(ctx, characterID)
	if err != nil {
		return ImageView{}, err
	}
	profile, err := s.storedProfile(ctx, characterID)
	if err != nil {
		return ImageView{}, err
	}
	return ImageView{AuraEffect: profile.AuraEffect, AvatarURL: profile.AvatarURL, AvatarUnlocked: unlocks.Avatar}, nil
}

// UpdateImage sets the aura and, once unlocked, the imgur avatar ID ("" for no
// avatar), as custom_image.cgi write_exe. Before the avatar unlock legacy offers
// no avatar input, so avatarID must be empty and the stored avatar is kept.
// Lock order: characters(2) -> character_profiles.
func (s *Service) UpdateImage(ctx context.Context, characterID string, aura int, avatarID string) (Profile, error) {
	if err := ValidateAuraEffect(aura); err != nil {
		return Profile{}, err
	}
	avatarURL := ""
	if avatarID != "" {
		avatarURL = "https://i.imgur.com/" + avatarID + ".gif"
		if err := ValidateAvatarURL(avatarURL); err != nil {
			return Profile{}, err
		}
	}
	var result Profile
	err := s.runInTx(ctx, func(ctx context.Context) error {
		if _, err := s.findForUpdate(ctx, characterID); err != nil {
			return err
		}
		unlocks, err := s.ImageUnlocks(ctx, characterID)
		if err != nil {
			return err
		}
		if !unlocks.Image {
			return ErrImageLocked
		}
		if !unlocks.Avatar && avatarID != "" {
			return ErrAvatarLocked
		}
		profile, err := s.storedProfile(ctx, characterID)
		if err != nil {
			return err
		}
		profile.AuraEffect = aura
		if unlocks.Avatar {
			profile.AvatarURL = avatarURL
		}
		profile.UpdatedAt = time.Now().UTC()
		if err := s.profileRepo.SaveProfile(ctx, profile); err != nil {
			return err
		}
		result = profile
		return nil
	})
	return result, err
}
