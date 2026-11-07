package character

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// GetProfile retrieves a character's stats and public profile. It propagates
// repository storage and context cancellation errors; genuinely missing profiles
// remain supported by returning a default profile.
func (s *Service) GetProfile(ctx context.Context, characterID string) (ProfileView, error) {
	char, err := s.repository.FindByID(ctx, characterID)
	if err != nil {
		return ProfileView{}, err
	}

	profile := Profile{
		CharacterID: characterID,
		BioData:     make(map[string]string),
		UpdatedAt:   time.Now().UTC(),
	}

	if s.profileRepo != nil {
		stored, err := s.profileRepo.GetProfile(ctx, characterID)
		if err != nil {
			if !errors.Is(err, ErrNotFound) {
				return ProfileView{}, err
			}
		} else if stored.CharacterID != "" {
			profile = stored
		}
	}

	return ProfileView{
		Character: char,
		Profile:   profile,
	}, nil
}

// UpdateProfile modifies a character's custom bio, comment, and avatar URL.
// It validates incoming fields, verifies the character exists and acquires a row
// lock within a transaction, preserving concurrent unrelated fields across partial updates.
// If reading the existing profile fails due to a repository error (other than absent profile),
// the error is returned immediately without persisting changes, preserving existing profile fields.
func (s *Service) UpdateProfile(ctx context.Context, characterID string, req UpdateProfileRequest) (Profile, error) {
	if req.Comment != nil {
		if err := ValidateComment(*req.Comment); err != nil {
			return Profile{}, err
		}
	}

	if req.AvatarURL != nil {
		if err := ValidateAvatarURL(*req.AvatarURL); err != nil {
			return Profile{}, err
		}
	}

	if req.AuraEffect != nil {
		if err := ValidateAuraEffect(*req.AuraEffect); err != nil {
			return Profile{}, err
		}
	}

	if req.BioData != nil {
		if err := ValidateBioData(req.BioData); err != nil {
			return Profile{}, err
		}
	}

	var result Profile
	err := s.runInTx(ctx, func(txCtx context.Context) error {
		if _, err := s.findForUpdate(txCtx, characterID); err != nil {
			return err
		}

		currentProfile := Profile{
			CharacterID: characterID,
			BioData:     make(map[string]string),
		}

		if s.profileRepo != nil {
			stored, err := s.profileRepo.GetProfile(txCtx, characterID)
			if err != nil {
				if !errors.Is(err, ErrNotFound) {
					return err
				}
			} else if stored.CharacterID != "" {
				currentProfile = stored
			}
		}

		if currentProfile.BioData == nil {
			currentProfile.BioData = make(map[string]string)
		}

		if req.Comment != nil {
			currentProfile.Comment = strings.TrimSpace(*req.Comment)
		}

		if req.AvatarURL != nil {
			currentProfile.AvatarURL = *req.AvatarURL
		}

		if req.AuraEffect != nil {
			currentProfile.AuraEffect = *req.AuraEffect
		}

		if req.BioData != nil {
			currentProfile.BioData = req.BioData
		}

		currentProfile.UpdatedAt = time.Now().UTC()

		if s.profileRepo != nil {
			if err := s.profileRepo.SaveProfile(txCtx, currentProfile); err != nil {
				return err
			}
		}

		result = currentProfile
		return nil
	})
	if err != nil {
		return Profile{}, err
	}

	return result, nil
}

// SetPersonality sets the profile's personality field (legacy profile key
// "character") while preserving other fields, creating the profile when absent,
// and announces it publicly in the same transaction.
// Transaction: RunInTx (joins an ambient transaction)
// Lock Order: characters(2) -> character_profiles
func (s *Service) SetPersonality(ctx context.Context, characterID, personality string) error {
	return s.runInTx(ctx, func(ctx context.Context) error {
		char, err := s.findForUpdate(ctx, characterID)
		if err != nil {
			return err
		}
		if s.profileRepo == nil {
			return errors.New("profile repository is not configured")
		}
		profile, err := s.profileRepo.GetProfile(ctx, characterID)
		if errors.Is(err, ErrNotFound) || (err == nil && profile.CharacterID == "") {
			profile, err = Profile{CharacterID: characterID}, nil
		}
		if err != nil {
			return err
		}
		if profile.BioData == nil {
			profile.BioData = make(map[string]string)
		}
		profile.BioData[PersonalityBioKey] = personality
		profile.UpdatedAt = time.Now().UTC()
		if err := s.profileRepo.SaveProfile(ctx, profile); err != nil {
			return err
		}
		if s.news == nil {
			return nil
		}
		msg := fmt.Sprintf("%sが%sになりました！", char.Name, personality)
		return s.news.PublishNews(ctx, "character", msg, msg, char.Name, time.Now().UTC())
	})
}
