package rescue

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

type RescueRepository interface {
	Save(ctx context.Context, record RescueRecord) error
	FindRecentByCharacterID(ctx context.Context, characterID string, since time.Time) ([]RescueRecord, error)
	FindLatestByCharacterID(ctx context.Context, characterID string) (RescueRecord, error)
}

type CharacterRepository interface {
	FindByID(ctx context.Context, id string) (corecharacter.Character, error)
	Update(ctx context.Context, c corecharacter.Character) error
}

type ActionCleaner interface {
	ClearActiveActions(ctx context.Context, characterID string) error
}

type Service struct {
	rescues    RescueRepository
	characters CharacterRepository
	cleaner    ActionCleaner
}

func NewService(
	rescues RescueRepository,
	characters CharacterRepository,
	cleaner ActionCleaner,
) *Service {
	return &Service{
		rescues:    rescues,
		characters: characters,
		cleaner:    cleaner,
	}
}

// EmergencyRescue resets player character state when stuck or encountering errors, applying a sleep penalty cooldown.
func (s *Service) EmergencyRescue(ctx context.Context, characterID, reason string, now time.Time) (RescueRecord, error) {
	if strings.TrimSpace(characterID) == "" {
		return RescueRecord{}, ErrInvalidCharacterID
	}
	if strings.TrimSpace(reason) == "" {
		return RescueRecord{}, ErrInvalidReason
	}

	char, err := s.characters.FindByID(ctx, characterID)
	if err != nil {
		return RescueRecord{}, err
	}

	// Calculate penalty
	penalty := DefaultPenaltySeconds
	recentSince := now.Add(-24 * time.Hour)
	recent, err := s.rescues.FindRecentByCharacterID(ctx, characterID, recentSince)
	if err == nil && len(recent) > 0 {
		penalty *= 2
	}

	// Clear active scheduled actions or ongoing activities
	if s.cleaner != nil {
		_ = s.cleaner.ClearActiveActions(ctx, characterID)
	}

	recID, err := newRecordID()
	if err != nil {
		return RescueRecord{}, err
	}

	rec := RescueRecord{
		ID:             recID,
		CharacterID:    char.ID,
		Reason:         strings.TrimSpace(reason),
		PenaltySeconds: penalty,
		CreatedAt:      now,
	}

	if err := s.rescues.Save(ctx, rec); err != nil {
		return RescueRecord{}, err
	}

	return rec, nil
}

// IsUnderPenalty checks if the character is currently restricted by a rescue penalty cooldown.
func (s *Service) IsUnderPenalty(ctx context.Context, characterID string, now time.Time) (bool, time.Duration, error) {
	if strings.TrimSpace(characterID) == "" {
		return false, 0, ErrInvalidCharacterID
	}

	rec, err := s.rescues.FindLatestByCharacterID(ctx, characterID)
	if err != nil {
		if errors.Is(err, ErrNoRescueRecord) {
			return false, 0, nil
		}
		return false, 0, err
	}

	if rec.IsActive(now) {
		remaining := rec.ExpiresAt().Sub(now)
		return true, remaining, nil
	}

	return false, 0, nil
}

// CheckActionAllowed returns ErrCharacterUnderPenalty if the character is currently under rescue penalty.
func (s *Service) CheckActionAllowed(ctx context.Context, characterID string, now time.Time) error {
	underPenalty, _, err := s.IsUnderPenalty(ctx, characterID, now)
	if err != nil {
		return err
	}
	if underPenalty {
		return ErrCharacterUnderPenalty
	}
	return nil
}

func newRecordID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
