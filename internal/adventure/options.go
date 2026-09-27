package adventure

import (
	"context"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

type Clock interface {
	Now() time.Time
}

type RealClock struct{}

func (RealClock) Now() time.Time { return time.Now().UTC() }

type CharacterUpdater interface {
	Update(ctx context.Context, value corecharacter.Character) error
}

type Logger interface {
	Warn(msg string, args ...any)
}

type nopLogger struct{}

func (nopLogger) Warn(msg string, args ...any) {}

// VictoryHook is called when an adventure stage concludes with a player victory.
// It is invoked as a best-effort side-effect notification after adventure state and rewards are committed.
// Hook execution errors are logged via Logger without failing the completed adventure.
type VictoryHook func(ctx context.Context, characterID string, monstersDefeated int, goldEarned int) error

// PostAdventureHook is called after an adventure concludes and state is applied.
// It is invoked as a best-effort side-effect notification.
// Hook execution errors are logged via Logger without failing the completed adventure.
type PostAdventureHook func(ctx context.Context, characterID string) error

type partyBattleResolver interface {
	ResolvePartyBattle(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error)
}

// ParticipantBuilder constructs a Participant from a character ID.
type ParticipantBuilder interface {
	BuildParticipant(ctx context.Context, characterID string) (corebattle.Participant, error)
}

// Option configures optional parameters on Service.
type Option func(*Service)

// WithParticipantBuilder configures the ParticipantBuilder.
func WithParticipantBuilder(builder ParticipantBuilder) Option {
	return func(s *Service) {
		s.participantBuilder = builder
		if settler, ok := builder.(PostBattleSettler); ok && s.battleSettler == nil {
			s.battleSettler = settler
		}
	}
}

// WithPostBattleSettler configures the PostBattleSettler.
func WithPostBattleSettler(settler PostBattleSettler) Option {
	return func(s *Service) {
		s.battleSettler = settler
	}
}

// BlessingProvider queries active chapel blessings for a character.
type BlessingProvider interface {
	GetActiveBlessing(ctx context.Context, characterID string) (string, error)
}

// WithBlessingProvider configures the BlessingProvider.
func WithBlessingProvider(provider BlessingProvider) Option {
	return func(s *Service) {
		s.blessingProvider = provider
	}
}

// TimerService manages daily dungeon locks.
type TimerService interface {
	IsLocked(ctx context.Context, category, targetID string) (bool, error)
	SetLock(ctx context.Context, category, targetID string, duration time.Duration) error
}

// WithTimerService configures the TimerService.
func WithTimerService(timer TimerService) Option {
	return func(s *Service) {
		s.timer = timer
	}
}
