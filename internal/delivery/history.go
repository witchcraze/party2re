package delivery

import (
	"context"
	"strings"
	"time"

	"github.com/witchcraze/party2re/internal/pagination"
)

// GetCharacterDeliveries returns all delivery quests accepted by the character.
func (s *Service) GetCharacterDeliveries(ctx context.Context, characterID string) ([]CharacterDelivery, error) {
	if strings.TrimSpace(characterID) == "" {
		return nil, ErrInvalidInput
	}
	return s.repo.GetCharacterDeliveries(ctx, characterID)
}

// GetIncomingParcels returns pending courier parcels for recipient.
func (s *Service) GetIncomingParcels(ctx context.Context, recipientID string) ([]Parcel, error) {
	if strings.TrimSpace(recipientID) == "" {
		return nil, ErrInvalidInput
	}
	return s.repo.GetIncomingParcels(ctx, recipientID)
}

// GetIncomingParcelsByCursor returns keyset / cursor-based paginated pending courier parcels for recipient.
func (s *Service) GetIncomingParcelsByCursor(ctx context.Context, recipientID string, limit int, cursor string) (pagination.CursorPage[Parcel], error) {
	if strings.TrimSpace(recipientID) == "" {
		return pagination.CursorPage[Parcel]{}, ErrInvalidInput
	}
	limit, _ = pagination.Normalize(limit, 0)
	beforeTime, beforeID := pagination.DecodeCursorParts(cursor)

	parcels, err := s.repo.GetIncomingParcelsByCursor(ctx, recipientID, limit+1, beforeTime, beforeID)
	if err != nil {
		return pagination.CursorPage[Parcel]{}, err
	}

	return pagination.BuildCursorPage(parcels, limit, cursor, func(p Parcel) (time.Time, string) {
		return p.CreatedAt, p.ID
	}), nil
}

// GetSentParcels returns parcels sent by character.
func (s *Service) GetSentParcels(ctx context.Context, senderID string) ([]Parcel, error) {
	if strings.TrimSpace(senderID) == "" {
		return nil, ErrInvalidInput
	}
	return s.repo.GetSentParcels(ctx, senderID)
}

// GetSentParcelsByCursor returns keyset / cursor-based paginated parcels sent by character.
func (s *Service) GetSentParcelsByCursor(ctx context.Context, senderID string, limit int, cursor string) (pagination.CursorPage[Parcel], error) {
	if strings.TrimSpace(senderID) == "" {
		return pagination.CursorPage[Parcel]{}, ErrInvalidInput
	}
	limit, _ = pagination.Normalize(limit, 0)
	beforeTime, beforeID := pagination.DecodeCursorParts(cursor)

	parcels, err := s.repo.GetSentParcelsByCursor(ctx, senderID, limit+1, beforeTime, beforeID)
	if err != nil {
		return pagination.CursorPage[Parcel]{}, err
	}

	return pagination.BuildCursorPage(parcels, limit, cursor, func(p Parcel) (time.Time, string) {
		return p.CreatedAt, p.ID
	}), nil
}
