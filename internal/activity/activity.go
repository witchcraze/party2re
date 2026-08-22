package activity

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

const (
	TrainingType     = "training"
	TrainingDuration = time.Hour
	TrainingReward   = 10
)

var (
	ErrInvalidActivity = errors.New("invalid activity")
	ErrNotFound        = errors.New("activity not found")
	ErrNotReady        = errors.New("activity is not ready")
	ErrAlreadyClaimed  = errors.New("activity result already claimed")
)

type Activity struct {
	ID               string
	CharacterID      string
	Type             string
	StartedAt        time.Time
	AvailableAt      time.Time
	ExperienceReward int
	Claimed          bool
}

type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time {
	return time.Now().UTC()
}

type ActivityRepository interface {
	Save(ctx context.Context, value Activity) error
	FindByID(ctx context.Context, id string) (Activity, error)
	Claim(ctx context.Context, id string) error
}

type CharacterRepository interface {
	FindByID(ctx context.Context, id string) (corecharacter.Character, error)
	Update(ctx context.Context, value corecharacter.Character) error
}

type Service struct {
	activities ActivityRepository
	characters CharacterRepository
	clock      Clock
}

func NewService(activities ActivityRepository, characters CharacterRepository) (*Service, error) {
	return NewServiceWithClock(activities, characters, realClock{})
}

func NewServiceWithClock(activities ActivityRepository, characters CharacterRepository, clock Clock) (*Service, error) {
	if activities == nil || characters == nil {
		return nil, errors.New("activity dependencies are nil")
	}
	if clock == nil {
		return nil, errors.New("activity clock is nil")
	}
	return &Service{activities: activities, characters: characters, clock: clock}, nil
}

func (s *Service) StartTraining(ctx context.Context, characterID string) (Activity, error) {
	if characterID == "" {
		return Activity{}, corecharacter.ErrNotFound
	}
	if _, err := s.characters.FindByID(ctx, characterID); err != nil {
		return Activity{}, err
	}

	now := s.clock.Now()
	id, err := newID()
	if err != nil {
		return Activity{}, err
	}
	value := Activity{
		ID:               id,
		CharacterID:      characterID,
		Type:             TrainingType,
		StartedAt:        now,
		AvailableAt:      now.Add(TrainingDuration),
		ExperienceReward: TrainingReward,
	}
	if err := s.activities.Save(ctx, value); err != nil {
		return Activity{}, err
	}
	return value, nil
}

func (s *Service) Claim(ctx context.Context, id string) (Activity, error) {
	value, err := s.activities.FindByID(ctx, id)
	if err != nil {
		return Activity{}, err
	}
	if value.Claimed {
		return Activity{}, ErrAlreadyClaimed
	}
	if s.clock.Now().Before(value.AvailableAt) {
		return Activity{}, ErrNotReady
	}

	character, err := s.characters.FindByID(ctx, value.CharacterID)
	if err != nil {
		return Activity{}, err
	}
	if err := character.AddExperience(value.ExperienceReward); err != nil {
		return Activity{}, fmt.Errorf("apply activity reward: %w", err)
	}
	if err := s.characters.Update(ctx, character); err != nil {
		return Activity{}, err
	}
	if err := s.activities.Claim(ctx, value.ID); err != nil {
		return Activity{}, err
	}
	value.Claimed = true
	return value, nil
}

func newID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate activity ID: %w", err)
	}
	return hex.EncodeToString(value), nil
}
