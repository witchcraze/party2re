package home

import (
	"context"
	"time"

	"github.com/witchcraze/party2re/internal/core/timer"
)

// SetSleepPenaltyReader adds an authoritative deadline independently of the
// optional timer projection. pending forbids Wake until mandatory work completes.
func (s *Service) SetSleepPenaltyReader(read func(context.Context, string) (time.Time, bool, error)) {
	s.sleepPenalty = read
}

func (s *Service) SetSleepAdmissionCheck(check func(context.Context, string) error) {
	s.sleepAdmission = check
}

// GetRemainingSleep observes timer and durable penalty without rounding deadlines.
func (s *Service) GetRemainingSleep(ctx context.Context, characterID string) (time.Duration, bool, error) {
	remaining, err := s.timer.GetRemainingLock(ctx, timer.CategorySleep, characterID)
	if err != nil {
		return 0, false, err
	}
	if s.sleepPenalty == nil {
		return remaining, false, nil
	}
	until, pending, err := s.sleepPenalty(ctx, characterID)
	if err != nil {
		return 0, false, err
	}
	return max(remaining, until.Sub(s.nowFunc())), pending, nil
}
