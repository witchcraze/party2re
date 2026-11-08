package playercontext

import (
	"context"
	"fmt"
	"time"

	"github.com/witchcraze/party2re/internal/core/timer"
)

// DefaultActionWait is the legacy $act_time (tracked config.cgi).
const DefaultActionWait = 4 * time.Second

// ActionWaitWriter starts the per-character wait timer.
type ActionWaitWriter interface {
	SetLock(ctx context.Context, category, targetID string, duration time.Duration) error
}

// ActionWaitGate rejects every action while the legacy wait runs, except the
// recovery actions legacy serves outside party.cgi (lib/system.cgi:637-645).
// It applies to continuations too: legacy rejects every registered action.
type ActionWaitGate struct{}

func (ActionWaitGate) Allows(action ActionDefinition, snapshot *Snapshot) bool {
	return snapshot.ActionWait <= 0 || action.WaitExempt == WaitRecovery
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

// StartActionWait begins the wait after a known successful command.
func (s *Service) StartActionWait(ctx context.Context, charID string) error {
	if s.waitWriter == nil {
		return nil
	}
	if err := s.waitWriter.SetLock(ctx, timer.CategoryActionWait, charID, s.waitDuration); err != nil {
		return fmt.Errorf("start action wait: %w", err)
	}
	return nil
}
