package party

import (
	"context"
	"time"

	"github.com/witchcraze/party2re/internal/core/battle"
)

// Option configures optional parameters on Service.
type Option func(*Service)

// ActionWaitWriter extends a kicked member's action wait until an absolute deadline.
type ActionWaitWriter interface {
	ImposeActionWaitUntil(ctx context.Context, characterID string, until time.Time) error
}

// WithActionWait configures the kick wait writer.
func WithActionWait(writer ActionWaitWriter) Option {
	return func(s *Service) {
		s.actionWait = writer
	}
}

// SetActionWait configures the kick wait writer after service construction.
func (s *Service) SetActionWait(writer ActionWaitWriter) {
	s.actionWait = writer
}

// WithClock overrides the clock used for lobby timeout and action wait deadlines.
func WithClock(clock func() time.Time) Option {
	return func(s *Service) {
		s.clock = clock
	}
}

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

// LifetimeActorBuilder constructs a battle LifetimeActor snapshot from a character ID.
type LifetimeActorBuilder interface {
	BuildLifetimeActor(ctx context.Context, characterID, icon string) (battle.LifetimeActor, error)
}

// WithLifetimeActorBuilder configures the member battle-row snapshot builder.
func WithLifetimeActorBuilder(builder LifetimeActorBuilder) Option {
	return func(s *Service) {
		s.lifetimeActorBuilder = builder
	}
}

// WithRNG sets a custom random number generator function.
func WithRNG(rng func(n int) int) Option {
	return func(s *Service) {
		s.rng = rng
	}
}

// TavernExitSettler resets Tavern fullness and settles any standing delivery on lobby exit.
type TavernExitSettler interface {
	ResetFullness(ctx context.Context, characterID string) error
	SettleDelivery(ctx context.Context, characterID string) (*DeliveryOutcome, error)
}

// WithTavernExitSettler configures the TavernExitSettler for waiting-lobby exits.
func WithTavernExitSettler(settler TavernExitSettler) Option {
	return func(s *Service) {
		s.tavernExit = settler
	}
}

// SetTavernExitSettler configures the TavernExitSettler on an existing service.
func (s *Service) SetTavernExitSettler(settler TavernExitSettler) {
	s.tavernExit = settler
}
