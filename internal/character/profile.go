package character

import (
	"context"
	"errors"
	"fmt"
	"maps"
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

// UpdateProfile replaces the character's legacy profile fields, as the legacy
// form submits every field (lib/profile.cgi:112-136). The row lock serializes it
// with other profile writers, which keep their own columns.
func (s *Service) UpdateProfile(ctx context.Context, characterID string, bio map[string]string) (Profile, error) {
	if err := ValidateBioData(bio); err != nil {
		return Profile{}, err
	}

	var result Profile
	err := s.runInTx(ctx, func(txCtx context.Context) error {
		if _, err := s.findForUpdate(txCtx, characterID); err != nil {
			return err
		}

		currentProfile, err := s.storedProfile(txCtx, characterID)
		if err != nil {
			return err
		}
		currentProfile.BioData = maps.Clone(bio)
		if currentProfile.BioData == nil {
			currentProfile.BioData = make(map[string]string)
		}
		currentProfile.UpdatedAt = time.Now().UTC()
		if err := s.profileRepo.SaveProfile(txCtx, currentProfile); err != nil {
			return err
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
		profile, err := s.storedProfile(ctx, characterID)
		if err != nil {
			return err
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

// storedProfile is the stored profile, or an empty one with a non-nil BioData.
func (s *Service) storedProfile(ctx context.Context, characterID string) (Profile, error) {
	if s.profileRepo == nil {
		return Profile{}, errors.New("profile repository is not configured")
	}
	profile, err := s.profileRepo.GetProfile(ctx, characterID)
	if errors.Is(err, ErrNotFound) || (err == nil && profile.CharacterID == "") {
		profile, err = Profile{CharacterID: characterID}, nil
	}
	if err != nil {
		return Profile{}, err
	}
	if profile.BioData == nil {
		profile.BioData = make(map[string]string)
	}
	return profile, nil
}
