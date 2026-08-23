package activity

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/progression"
)

const (
	TrainingType                       = "training"
	TrainingDuration                   = time.Hour
	TrainingReward                     = 10
	ActivityActionTypeTrainingComplete = "activity:training_complete"
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
	ClaimAndApply(ctx context.Context, id string, character corecharacter.Character) error
}

type CharacterRepository interface {
	FindByID(ctx context.Context, id string) (corecharacter.Character, error)
}

type Scheduler interface {
	Schedule(ctx context.Context, actionType, actorID string, params map[string]string, executeAt time.Time) (string, error)
}

type Logger interface {
	Warn(msg string, args ...any)
}

type nopLogger struct{}

func (nopLogger) Warn(msg string, args ...any) {}

type Service struct {
	activities ActivityRepository
	characters CharacterRepository
	scheduler  Scheduler
	logger     Logger
	clock      Clock
}

func NewService(activities ActivityRepository, characters CharacterRepository, scheduler Scheduler, logger Logger) (*Service, error) {
	return NewServiceWithClock(activities, characters, scheduler, logger, realClock{})
}

func NewServiceWithClock(activities ActivityRepository, characters CharacterRepository, scheduler Scheduler, logger Logger, clock Clock) (*Service, error) {
	if activities == nil || characters == nil {
		return nil, errors.New("activity dependencies are nil")
	}
	if clock == nil {
		return nil, errors.New("activity clock is nil")
	}
	if logger == nil {
		logger = nopLogger{}
	}
	return &Service{activities: activities, characters: characters, scheduler: scheduler, logger: logger, clock: clock}, nil
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

	if s.scheduler != nil {
		_, err = s.scheduler.Schedule(
			ctx,
			ActivityActionTypeTrainingComplete,
			characterID,
			map[string]string{"activity_id": value.ID},
			value.AvailableAt,
		)
		if err != nil {
			s.logger.Warn("failed to schedule training completion", "activity_id", value.ID, "error", err)
		}
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
	if _, err := progression.ApplyExperience(&character, value.ExperienceReward); err != nil {
		return Activity{}, fmt.Errorf("apply activity reward: %w", err)
	}
	if err := s.activities.ClaimAndApply(ctx, value.ID, character); err != nil {
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
