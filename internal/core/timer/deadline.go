package timer

import (
	"context"
	_ "embed"
	"strconv"
	"time"

	"github.com/valkey-io/valkey-go"
)

//go:embed lua/impose_lock_until.lua
var imposeLockUntilLua string

var imposeLockUntilScript = valkey.NewLuaScript(imposeLockUntilLua)

type DeadlineLockService interface {
	SetLockUntil(context.Context, string, string, time.Time) error
	ExtendLockUntil(context.Context, string, string, time.Time) error
	ImposeLockUntil(context.Context, string, string, time.Time) error
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

// ImposeLockUntil extends an ephemeral lock to at least until and takes
// non-claim ownership ("1") so an in-flight claim's Release cannot delete an
// imposed wait. Persistent locks, longer active waits, and expired deadlines
// are preserved without modification.
func (s *service) ImposeLockUntil(ctx context.Context, category, targetID string, until time.Time) error {
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
	now := time.Now()
	if s.client != nil {
		return imposeLockUntilScript.Exec(ctx, s.client, []string{k}, []string{
			strconv.FormatInt(until.UnixMilli(), 10),
			strconv.FormatInt(now.UnixMilli(), 10),
		}).Error()
	}
	if !until.After(now) {
		return nil
	}
	s.memMu.Lock()
	defer s.memMu.Unlock()
	current, exists := s.memory[k]
	if exists && current.expiresAt.IsZero() {
		return nil
	}
	if exists && current.expiresAt.After(now) && !current.expiresAt.Before(until) {
		if current.value != "1" {
			s.memory[k] = memTimerEntry{expiresAt: current.expiresAt, value: "1"}
		}
		return nil
	}
	s.memory[k] = memTimerEntry{expiresAt: until, value: "1"}
	return nil
}
