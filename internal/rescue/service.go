package rescue

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/id"
)

type RescueRepository interface {
	Save(ctx context.Context, record RescueRecord) error
	FindLatestByCharacterID(ctx context.Context, characterID string) (RescueRecord, error)
}

type CharacterRepository interface {
	FindByID(ctx context.Context, id string) (corecharacter.Character, error)
	Update(ctx context.Context, c corecharacter.Character) error
}

type ActionCleaner interface {
	ClearActiveActions(ctx context.Context, characterID string) (bool, error)
}

// ActivityReader lists feature activities still owned by the character.
type ActivityReader interface {
	ActivityKinds(ctx context.Context, ownerID, characterID string) ([]string, error)
}

type TimerService interface {
	SetLock(ctx context.Context, category, targetID string, duration time.Duration) error
	GetRemainingLock(ctx context.Context, category, targetID string) (time.Duration, error)
}

type Service struct {
	rescues    RescueRepository
	characters CharacterRepository
	cleaner    ActionCleaner
	timer      TimerService
	activities ActivityReader
}

// SetActivityReader wires the feature activity observation after composition.
func (s *Service) SetActivityReader(r ActivityReader) {
	s.activities = r
}

// ensureNoActivity rejects rescue while feature-owned state the cleaner cannot end remains.
func (s *Service) ensureNoActivity(ctx context.Context, char corecharacter.Character) error {
	if s.activities == nil {
		return nil
	}
	kinds, err := s.activities.ActivityKinds(ctx, char.PlayerID, char.ID)
	if err != nil {
		return err
	}
	if len(kinds) > 0 {
		return fmt.Errorf("%w: %s", ErrActivityRemaining, strings.Join(kinds, ","))
	}
	return nil
}

func NewService(
	rescues RescueRepository,
	characters CharacterRepository,
	cleaner ActionCleaner,
	timer TimerService,
) *Service {
	return &Service{
		rescues:    rescues,
		characters: characters,
		cleaner:    cleaner,
		timer:      timer,
	}
}

// Unstuck resets the character's active and pending scheduled actions without applying
// a rescue penalty cooldown (admin refresh per legacy party2/admin.cgi:121-131).
// It returns ErrActivityRemaining without changes while feature activity remains.
func (s *Service) Unstuck(ctx context.Context, characterID string) error {
	characterID = strings.TrimSpace(characterID)
	if characterID == "" {
		return ErrInvalidCharacterID
	}

	char, err := s.characters.FindByID(ctx, characterID)
	if err != nil {
		return err
	}
	if err := s.ensureNoActivity(ctx, char); err != nil {
		return err
	}

	if s.cleaner != nil {
		if _, err := s.cleaner.ClearActiveActions(ctx, characterID); err != nil {
			return err
		}
	}

	return nil
}

// EmergencyRescue resets player character state when stuck or encountering errors, applying a sleep penalty cooldown.
// If the character is safe in town with no active actions, returns early with 0 penalty cooldown per legacy rescue.cgi:56-65.
// Feature activity the scheduler cannot end returns ErrActivityRemaining without changes.
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
	if err := s.ensureNoActivity(ctx, char); err != nil {
		return RescueRecord{}, err
	}

	// Clear active scheduled actions or ongoing activities
	var cleared bool
	if s.cleaner != nil {
		cleared, err = s.cleaner.ClearActiveActions(ctx, characterID)
		if err != nil {
			return RescueRecord{}, err
		}
	}

	// Character is already safe in town (no ongoing or stuck activity to clear);
	// return safe status with 0 penalty cooldown without recording a new penalty.
	if !cleared {
		return RescueRecord{
			CharacterID:    char.ID,
			Reason:         strings.TrimSpace(reason),
			PenaltySeconds: 0,
			CreatedAt:      now,
		}, nil
	}

	recID := id.New()
	rec := RescueRecord{
		ID:             recID,
		CharacterID:    char.ID,
		Reason:         strings.TrimSpace(reason),
		PenaltySeconds: DefaultPenaltySeconds,
		CreatedAt:      now,
	}

	if err := s.rescues.Save(ctx, rec); err != nil {
		return RescueRecord{}, err
	}

	if s.timer != nil {
		duration := time.Duration(DefaultPenaltySeconds) * time.Second
		if rem, err := s.timer.GetRemainingLock(ctx, timer.CategorySleep, char.ID); err == nil && rem > 0 {
			duration += rem
		}
		if err := s.timer.SetLock(ctx, timer.CategorySleep, char.ID, duration); err != nil {
			return RescueRecord{}, err
		}
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
