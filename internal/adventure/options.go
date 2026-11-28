package adventure

import (
	"context"
	"time"
)

type Clock interface {
	Now() time.Time
}

type RealClock struct{}

func (RealClock) Now() time.Time { return time.Now().UTC() }

// BlessingProvider queries active chapel blessings for a character.
type BlessingProvider interface {
	GetActiveBlessing(ctx context.Context, characterID string) (string, error)
}

// TimerService manages daily dungeon locks.
type TimerService interface {
	IsLocked(ctx context.Context, category, targetID string) (bool, error)
	SetLock(ctx context.Context, category, targetID string, duration time.Duration) error
	TryLock(ctx context.Context, category, targetID string, duration time.Duration) (bool, error)
	TryLockWithToken(ctx context.Context, category, targetID, token string, duration time.Duration) (bool, error)
	ReleaseLockWithToken(ctx context.Context, category, targetID, token string) (bool, error)
}

// TransactionProvider executes a callback within an ambient or new database transaction.
type TransactionProvider interface {
	RunInTx(ctx context.Context, fn func(ctx context.Context) error) error
}
