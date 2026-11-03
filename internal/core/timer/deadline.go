package timer

import (
	"context"
	"time"
)

type DeadlineLockService interface {
	SetLockUntil(context.Context, string, string, time.Time) error
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
