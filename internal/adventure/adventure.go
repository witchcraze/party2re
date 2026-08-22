package adventure

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/progression"
)

const (
	StarterAdventure  = "starter-adventure"
	AdventureDuration = time.Hour
	AdventureReward   = 20
	AdventureEnemyID  = "starter-opponent"
)

var (
	ErrNotFound       = errors.New("adventure not found")
	ErrNotReady       = errors.New("adventure is not ready")
	ErrAlreadyClaimed = errors.New("adventure result already claimed")
)

type Adventure struct {
	ID               string
	CharacterID      string
	Type             string
	StartedAt        time.Time
	AvailableAt      time.Time
	ExperienceReward int
	BattleResult     corebattle.Result
	Resolved         bool
	Claimed          bool
}

type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now().UTC() }

type Repository interface {
	Save(ctx context.Context, value Adventure) error
	FindByID(ctx context.Context, id string) (Adventure, error)
}

type CharacterRepository interface {
	FindByID(ctx context.Context, id string) (corecharacter.Character, error)
	Update(ctx context.Context, value corecharacter.Character) error
}

type Service struct {
	adventures Repository
	characters CharacterRepository
	battle     corebattle.Resolver
	clock      Clock
}

func NewService(adventures Repository, characters CharacterRepository, battle corebattle.Resolver) (*Service, error) {
	return NewServiceWithClock(adventures, characters, battle, realClock{})
}

func NewServiceWithClock(adventures Repository, characters CharacterRepository, battle corebattle.Resolver, clock Clock) (*Service, error) {
	if adventures == nil || characters == nil || battle == nil {
		return nil, errors.New("adventure dependencies are nil")
	}
	if clock == nil {
		return nil, errors.New("adventure clock is nil")
	}
	return &Service{adventures: adventures, characters: characters, battle: battle, clock: clock}, nil
}

func (s *Service) Start(ctx context.Context, characterID string) (Adventure, error) {
	if characterID == "" {
		return Adventure{}, corecharacter.ErrNotFound
	}
	if _, err := s.characters.FindByID(ctx, characterID); err != nil {
		return Adventure{}, err
	}
	now := s.clock.Now()
	id, err := newID()
	if err != nil {
		return Adventure{}, err
	}
	value := Adventure{
		ID:               id,
		CharacterID:      characterID,
		Type:             StarterAdventure,
		StartedAt:        now,
		AvailableAt:      now.Add(AdventureDuration),
		ExperienceReward: AdventureReward,
	}
	if err := s.adventures.Save(ctx, value); err != nil {
		return Adventure{}, err
	}
	return value, nil
}

func (s *Service) Claim(ctx context.Context, id string) (Adventure, error) {
	value, err := s.adventures.FindByID(ctx, id)
	if err != nil {
		return Adventure{}, err
	}
	if value.Claimed {
		return Adventure{}, ErrAlreadyClaimed
	}
	if s.clock.Now().Before(value.AvailableAt) {
		return Adventure{}, ErrNotReady
	}
	character, err := s.characters.FindByID(ctx, value.CharacterID)
	if err != nil {
		return Adventure{}, err
	}
	result, err := s.battle.Resolve(corebattle.Request{
		Participants: []corebattle.Participant{
			{ID: character.ID, HP: character.Stats.HP, Attack: character.Stats.Attack, Defense: character.Stats.Defense},
			{ID: AdventureEnemyID, HP: 8, Attack: 1, Defense: 0},
		},
	})
	if err != nil {
		return Adventure{}, fmt.Errorf("resolve adventure battle: %w", err)
	}
	value.BattleResult = result
	value.Resolved = true
	if result.Outcome == corebattle.OutcomeWin && result.WinnerID == character.ID {
		if _, err := progression.ApplyExperience(&character, value.ExperienceReward); err != nil {
			return Adventure{}, fmt.Errorf("apply adventure reward: %w", err)
		}
		if err := s.characters.Update(ctx, character); err != nil {
			return Adventure{}, err
		}
	}
	value.Claimed = true
	if err := s.adventures.Save(ctx, value); err != nil {
		return Adventure{}, err
	}
	return value, nil
}

func newID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate adventure ID: %w", err)
	}
	return hex.EncodeToString(value), nil
}
