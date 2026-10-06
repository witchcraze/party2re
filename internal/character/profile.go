package character

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
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
			currentProfile.AvatarURL = strings.TrimSpace(*req.AvatarURL)
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

// UploadAvatar validates an image and sets the character's avatar URL to a safe data URI.
func (s *Service) UploadAvatar(ctx context.Context, characterID string, filename string, contentType string, data []byte) (string, error) {
	if len(data) == 0 {
		return "", ErrInvalidImageFormat
	}
	if len(data) > MaxAvatarSizeBytes {
		return "", ErrImageTooLarge
	}

	// Validate content type & image header
	normType := strings.ToLower(strings.TrimSpace(contentType))
	var mimeType string
	switch {
	case strings.Contains(normType, "png") || bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		mimeType = "image/png"
	case strings.Contains(normType, "jpeg") || strings.Contains(normType, "jpg") || (len(data) > 3 && data[0] == 0xFF && data[1] == 0xD8):
		mimeType = "image/jpeg"
	case strings.Contains(normType, "gif") || bytes.HasPrefix(data, []byte("GIF87a")) || bytes.HasPrefix(data, []byte("GIF89a")):
		mimeType = "image/gif"
	case strings.Contains(normType, "webp") || (len(data) > 12 && string(data[0:4]) == "RIFF" && string(data[8:12]) == "WEBP"):
		mimeType = "image/webp"
	case strings.Contains(normType, "svg") || bytes.Contains(data, []byte("<svg")):
		mimeType = "image/svg+xml"
	default:
		// Attempt standard decode
		_, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			return "", ErrInvalidImageFormat
		}
		mimeType = "image/png"
	}

	dataURI := fmt.Sprintf("data:%s;base64,%s", mimeType, base64.StdEncoding.EncodeToString(data))

	// Update avatar URL in profile
	_, err := s.UpdateProfile(ctx, characterID, UpdateProfileRequest{
		AvatarURL: &dataURI,
	})
	if err != nil {
		return "", err
	}

	return dataURI, nil
}
