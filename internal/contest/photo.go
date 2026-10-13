package contest

import (
	"context"
	"errors"
	"strings"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/id"
)

// SavePhoto stores a new captured photo/screenshot for a character.
func (s *Service) SavePhoto(
	ctx context.Context,
	characterID string,
	title string,
	location string,
	imageURL string,
	caption string,
	metadata string,
) (Photo, error) {
	characterID = strings.TrimSpace(characterID)
	if characterID == "" {
		return Photo{}, ErrCharacterNotFound
	}
	if err := ValidateTitle(title); err != nil {
		return Photo{}, err
	}

	var saved Photo
	err := s.runInTx(ctx, func(txCtx context.Context) error {
		_, err := s.characters.FindByID(txCtx, characterID)
		if err != nil {
			if errors.Is(err, corecharacter.ErrNotFound) {
				return ErrCharacterNotFound
			}
			return err
		}

		count, err := s.contests.CountPhotosByCharacterID(txCtx, characterID)
		if err != nil {
			return err
		}
		if count >= MaxPhotosPerCharacter {
			return ErrMaxPhotosReached
		}

		now := s.nowFunc()
		saved = Photo{
			ID:          id.New(),
			CharacterID: characterID,
			Title:       strings.TrimSpace(title),
			Location:    strings.TrimSpace(location),
			ImageURL:    strings.TrimSpace(imageURL),
			Caption:     strings.TrimSpace(caption),
			Metadata:    metadata,
			CreatedAt:   now,
			UpdatedAt:   now,
		}

		return s.contests.SavePhoto(txCtx, saved)
	})
	if err != nil {
		return Photo{}, err
	}

	return saved, nil
}

// ListPhotos retrieves all photos owned by a character.
func (s *Service) ListPhotos(ctx context.Context, characterID string) ([]Photo, error) {
	characterID = strings.TrimSpace(characterID)
	if characterID == "" {
		return nil, ErrCharacterNotFound
	}
	_, err := s.characters.FindByID(ctx, characterID)
	if err != nil {
		if errors.Is(err, corecharacter.ErrNotFound) {
			return nil, ErrCharacterNotFound
		}
		return nil, err
	}
	return s.contests.ListPhotosByCharacterID(ctx, characterID)
}

// DeletePhoto removes a specific photo owned by a character.
func (s *Service) DeletePhoto(ctx context.Context, characterID, photoID string) error {
	characterID = strings.TrimSpace(characterID)
	photoID = strings.TrimSpace(photoID)
	if characterID == "" {
		return ErrCharacterNotFound
	}
	if photoID == "" {
		return ErrPhotoNotFound
	}

	return s.runInTx(ctx, func(txCtx context.Context) error {
		photo, err := s.contests.FindPhotoByID(txCtx, photoID)
		if err != nil {
			return err
		}
		if photo.CharacterID != characterID {
			return ErrForbidden
		}
		return s.contests.DeletePhoto(txCtx, photoID)
	})
}
