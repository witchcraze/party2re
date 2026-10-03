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
}

// Service builds uncached player observations using existing read contracts.
type Service struct {
	characters CharacterReader
	actions    ActionReader
	timers     TimerReader
}

// NewService requires character, scheduled-action and sleep-timer readers.
func NewService(characters CharacterReader, actions ActionReader, timers TimerReader) *Service {
	return &Service{characters: characters, actions: actions, timers: timers}
}

// Query reads each state source once and evaluates entry actions in the initial
// town scene. Reads are not a cross-store transaction; execution must revalidate.
func (s *Service) Query(ctx context.Context, charID string) (Result, error) {
	char, err := s.characters.FindByID(ctx, charID)
	if err != nil {
		return Result{}, fmt.Errorf("load context character: %w", err)
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
	return Result{Snapshot: snapshot, AvailableActions: Evaluate(snapshot)}, nil
}
