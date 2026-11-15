package timer

import (
	"context"
	"time"

	"github.com/valkey-io/valkey-go"
)

type DeadlineLockService interface {
	SetLockUntil(context.Context, string, string, time.Time) error
	ExtendLockUntil(context.Context, string, string, time.Time) error
}

// ExtendLockUntil publishes an absolute deadline without shortening a concurrent
// sleep, reviving an expired obligation, or replacing a persistent lock.
func (s *service) ExtendLockUntil(ctx context.Context, category, targetID string, until time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, ok := ctx.Value(claimContextKey{}).(claimSpec); ok {
		return ErrInvalidAction
	}
	k, err := timerKey(category, targetID)
	if err != nil {
		return err
	}
	if s.client != nil {
		err := s.client.Do(ctx, s.client.B().Set().Key(k).Value("1").Nx().Pxat(until).Build()).Error()
		if err != nil && !valkey.IsValkeyNil(err) {
			return err
		}
		return s.client.Do(ctx, s.client.B().Pexpireat().Key(k).MillisecondsTimestamp(until.UnixMilli()).Gt().Build()).Error()
	}
	s.memMu.Lock()
	defer s.memMu.Unlock()
	current, exists := s.memory[k]
	if until.After(time.Now()) && (!exists || (!current.expiresAt.IsZero() && current.expiresAt.Before(until))) {
		s.memory[k] = memTimerEntry{expiresAt: until, value: "1"}
	}
	return nil
}

// SetLockUntil projects a durable absolute deadline without adding transport or
// retry latency. Claim-bound callers must use the existing claim operations.
func (s *service) SetLockUntil(ctx context.Context, category, targetID string, until time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, ok := ctx.Value(claimContextKey{}).(claimSpec); ok {
		return ErrInvalidAction
	}
	k, err := timerKey(category, targetID)
	if err != nil {
		return err
	}
	if s.client != nil {
		return s.client.Do(ctx, s.client.B().Set().Key(k).Value("1").Pxat(until).Build()).Error()
	}
	s.memMu.Lock()
	defer s.memMu.Unlock()
	s.memory[k] = memTimerEntry{expiresAt: until, value: "1"}
	return nil
}
