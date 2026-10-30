package party

import (
	"context"
	"time"

	"github.com/witchcraze/party2re/internal/core/battle"
)

// Option configures optional parameters on Service.
type Option func(*Service)

// TimerService manages daily dungeon once locks for party admission.
type TimerService interface {
	IsLocked(ctx context.Context, category, targetID string) (bool, error)
	TryLock(ctx context.Context, category, targetID string, duration time.Duration) (bool, error)
	TryLockWithToken(ctx context.Context, category, targetID, token string, duration time.Duration) (bool, error)
	ReleaseLockWithToken(ctx context.Context, category, targetID, token string) (bool, error)
	ReleaseLock(ctx context.Context, category, targetID string) error
}

// WithTimerService configures the TimerService.
func WithTimerService(timer TimerService) Option {
	return func(s *Service) {
		s.timer = timer
	}
}

// WithTransactionProvider configures the transaction boundary provider.
func WithTransactionProvider(txProvider TransactionProvider) Option {
	return func(s *Service) {
		s.txProvider = txProvider
	}
}

// WithNewsPublisher configures the news publisher.
func WithNewsPublisher(news NewsPublisher) Option {
	return func(s *Service) {
		s.news = news
	}
}

// WithVictoryHook configures the victory hook.
func WithVictoryHook(hook VictoryHook) Option {
	return func(s *Service) {
		s.victoryHook = hook
	}
}

// ParticipantBuilder constructs a Participant from a character ID.
type ParticipantBuilder interface {
	BuildParticipant(ctx context.Context, characterID string) (battle.Participant, error)
}

// WithParticipantBuilder configures the ParticipantBuilder.
func WithParticipantBuilder(builder ParticipantBuilder) Option {
	return func(s *Service) {
		s.participantBuilder = builder
	}
}

// PostAdventureHook is called after an adventure concludes and state is applied.
type PostAdventureHook func(ctx context.Context, characterID string) error

// WithPostAdventureHook configures the post-adventure hook.
func WithPostAdventureHook(hook PostAdventureHook) Option {
	return func(s *Service) {
		s.postAdventureHook = hook
	}
}

// BlessingProvider queries active chapel blessings for a character.
type BlessingProvider interface {
	GetActiveBlessing(ctx context.Context, characterID string) (string, error)
}

// WithBlessingProvider configures the blessing provider.
func WithBlessingProvider(provider BlessingProvider) Option {
	return func(s *Service) {
		s.blessingProvider = provider
	}
}

// WithRNG sets a custom random number generator function.
func WithRNG(rng func(n int) int) Option {
	return func(s *Service) {
		s.rng = rng
	}
}
