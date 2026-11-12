package playercontext

import (
	"context"
	"fmt"
	"time"

	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/id"
)

// DefaultActionWait is the legacy $act_time (tracked config.cgi).
const DefaultActionWait = 4 * time.Second

// ActionWaitWriter claims and releases the per-character wait atomically.
type ActionWaitWriter interface {
	TryLockWithToken(ctx context.Context, category, targetID, token string, duration time.Duration) (bool, error)
	ReleaseLockWithToken(ctx context.Context, category, targetID, token string) (bool, error)
}

// ActionWaitGate rejects every action while the legacy wait runs, except the
// recovery actions legacy serves outside party.cgi and plain speech, which is
// not a ＠ action (lib/system.cgi:637-645).
// It applies to continuations too: legacy rejects every registered action.
type ActionWaitGate struct{}

func (ActionWaitGate) Allows(action ActionDefinition, snapshot *Snapshot) bool {
	return snapshot.ActionWait <= 0 || action.WaitExempt == WaitRecovery || action.WaitExempt == WaitSpeech || action.WaitExempt == WaitReload
}

// WithActionWait enables the legacy action wait. A non-positive duration or a
// nil writer leaves it disabled.
func WithActionWait(writer ActionWaitWriter, duration time.Duration) func(*Service) {
	return func(s *Service) {
		if writer != nil && duration > 0 {
			s.waitWriter, s.waitDuration = writer, duration
		}
	}
}

// StartsActionWait reports whether a successful action starts the wait.
func StartsActionWait(action ActionDefinition) bool { return action.WaitExempt == "" }

// ActionWaitClaim is held by one in-flight command; Release undoes it when the
// command is rejected or its outcome is unknown.
type ActionWaitClaim struct {
	s             *Service
	charID, token string
}

// ClaimActionWait atomically starts the wait before a wait-starting command
// runs, so concurrent commands cannot all pass the read-only check. Like legacy
// `$m{wt} = $time + $act_time`, the wait is measured from the request start.
// ok is false while another wait is held; a disabled wait always succeeds.
func (s *Service) ClaimActionWait(ctx context.Context, charID string) (claim ActionWaitClaim, ok bool, err error) {
	if s.waitWriter == nil {
		return ActionWaitClaim{}, true, nil
	}
	token := id.New()
	ok, err = s.waitWriter.TryLockWithToken(ctx, timer.CategoryActionWait, charID, token, s.waitDuration)
	if err != nil {
		return ActionWaitClaim{}, false, fmt.Errorf("claim action wait: %w", err)
	}
	if !ok {
		return ActionWaitClaim{}, false, nil
	}
	return ActionWaitClaim{s: s, charID: charID, token: token}, true, nil
}

// Release removes the claim only if it is still this command's.
func (c ActionWaitClaim) Release(ctx context.Context) error {
	if c.s == nil {
		return nil
	}
	if _, err := c.s.waitWriter.ReleaseLockWithToken(ctx, timer.CategoryActionWait, c.charID, c.token); err != nil {
		return fmt.Errorf("release action wait: %w", err)
	}
	return nil
}
