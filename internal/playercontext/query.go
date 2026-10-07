package playercontext

import (
	"context"
	"fmt"
	"time"

	"github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/core/timer"
)

// CharacterReader provides the persisted character state used by the query.
type CharacterReader interface {
	FindByID(ctx context.Context, id string) (character.Character, error)
}

// ActionReader provides unfinished work without depending on queue implementation.
type ActionReader interface {
	FindPendingByActorID(ctx context.Context, actorID string) ([]scheduling.ScheduledAction, error)
}

// TimerReader observes sleep duration and pending wake recovery without writes.
type TimerReader interface {
	GetRemainingLock(ctx context.Context, category, targetID string) (time.Duration, error)
	IsLocked(ctx context.Context, category, targetID string) (bool, error)
}

// Result shares the already-read state with presentation callers alongside ActionIDs.
type Result struct {
	Snapshot         Snapshot
	AvailableActions []string
	Navigation       *NavigationObservation
}

// Service builds uncached player observations using existing read contracts.
type Service struct {
	characters CharacterReader
	actions    ActionReader
	timers     TimerReader
	navigation NavigationRepository
	scenes     map[string]SceneDefinition
}

// NewService requires character, scheduled-action and sleep-timer readers.
func NewService(characters CharacterReader, actions ActionReader, timers TimerReader, options ...func(*Service)) *Service {
	s := &Service{characters: characters, actions: actions, timers: timers}
	for _, option := range options {
		option(s)
	}
	return s
}

// Query verifies the owned viewer before any feature reads. Entry evaluation
// remains separate from HTTP scene composition and command connection support.
// Reads are not a cross-store transaction; execution must revalidate.
func (s *Service) Query(ctx context.Context, charID, ownerID string) (Result, error) {
	if ownerID == "" {
		return Result{}, ErrNavigationForbidden
	}
	return s.query(ctx, charID, ownerID)
}

func (s *Service) query(ctx context.Context, charID, ownerID string) (Result, error) {
	char, err := s.characters.FindByID(ctx, charID)
	if err != nil {
		return Result{}, fmt.Errorf("load context character: %w", err)
	}
	if ownerID != "" && (char.PlayerID != ownerID || char.ID != charID) {
		return Result{}, ErrNavigationForbidden
	}
	actions, err := s.actions.FindPendingByActorID(ctx, charID)
	if err != nil {
		return Result{}, fmt.Errorf("load context scheduled actions: %w", err)
	}
	remaining, err := s.timers.GetRemainingLock(ctx, timer.CategorySleep, charID)
	if err != nil {
		return Result{}, fmt.Errorf("load context sleep timer: %w", err)
	}
	asleep, err := s.timers.IsLocked(ctx, timer.CategoryAsleep, charID)
	if err != nil {
		return Result{}, fmt.Errorf("load context wake recovery: %w", err)
	}
	snapshot := Snapshot{
		Character:      char,
		OngoingActions: actions,
		SleepRemaining: remaining,
		Sleeping:       remaining > 0 || asleep,
		CanWake:        remaining <= 0 && asleep,
		LocationID:     LocationTown,
	}
	navigation, err := s.observeNavigation(ctx, charID)
	if err != nil {
		return Result{}, err
	}
	return Result{Snapshot: snapshot, AvailableActions: Evaluate(snapshot), Navigation: navigation}, nil
}
