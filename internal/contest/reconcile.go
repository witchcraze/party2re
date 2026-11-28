package contest

import (
	"context"
	"errors"
	"fmt"
	"time"

	core_scheduling "github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/id"
)

// ReconcileSettlementSchedule ensures the settlement scheduled action exists for the current contest round.
// It inspects current SQL round state (active round, or preparing round if no active round exists),
// discovers active reservations via FindPendingByActorID, repairs missing queue membership for Pending
// reservations via RequeuePending without duplicating Pending or overwriting Processing actions, and
// registers the deterministic settlement reservation with a safe nonce suffix when none exists.
func (s *Service) ReconcileSettlementSchedule(ctx context.Context) error {
	return s.reconcileSettlementSchedule(ctx, false)
}

func (s *Service) reconcileSettlementSchedule(ctx context.Context, useCache bool) error {
	if s.scheduler == nil {
		return nil
	}

	s.reconcileMu.Lock()
	defer s.reconcileMu.Unlock()

	if useCache && s.verifiedRound > 0 {
		now := s.nowFunc().UTC()
		if now.Before(s.verifiedEndTime) {
			hasReservation, err := s.hasSettlementReservation(ctx, s.verifiedRound, s.verifiedEndTime)
			if err != nil {
				return err
			}
			if hasReservation {
				return nil
			}
		}
	}

	var targetRound int
	var targetEndTime time.Time

	activeRound, err := s.contests.GetActiveRound(ctx)
	if err == nil {
		targetRound = activeRound.Round
		targetEndTime = activeRound.EndTime
	} else if errors.Is(err, ErrContestNotFound) {
		prepRound, prepErr := s.EnsurePreparingRound(ctx)
		if prepErr != nil {
			return fmt.Errorf("ensure preparing contest round: %w", prepErr)
		}
		targetRound = prepRound.Round
		targetEndTime = prepRound.EndTime
	} else {
		return fmt.Errorf("read active contest round: %w", err)
	}

	if targetEndTime.IsZero() {
		return nil
	}

	hasReservation, err := s.hasSettlementReservation(ctx, targetRound, targetEndTime)
	if err != nil {
		return err
	}
	if hasReservation {
		s.verifiedRound = targetRound
		s.verifiedEndTime = targetEndTime
		return nil
	}

	actionID := fmt.Sprintf("%s:%s", SettlementActionID(targetRound, targetEndTime), id.New())
	if err := s.scheduler.ScheduleWithID(ctx, actionID, ActionTypeContestSettlement, s.SettlementActorID(), nil, targetEndTime); err != nil {
		return fmt.Errorf("%w: %v", ErrScheduleFailed, err)
	}

	s.verifiedRound = targetRound
	s.verifiedEndTime = targetEndTime
	return nil
}

func (s *Service) hasSettlementReservation(ctx context.Context, targetRound int, targetEndTime time.Time) (bool, error) {
	actorID := s.SettlementActorID()
	activeActions, err := s.scheduler.FindPendingByActorID(ctx, actorID)
	if err != nil {
		return false, fmt.Errorf("discover contest settlement reservation: %w", err)
	}
	if action, ok := findMatchingSettlementAction(activeActions, targetRound, targetEndTime); ok {
		return s.ensureQueuedReservation(ctx, action)
	}
	// Check legacy actor ID "system" for backward compatibility with pre-upgrade actions
	if actorID != "system" {
		legacyActions, err := s.scheduler.FindPendingByActorID(ctx, "system")
		if err != nil {
			return false, fmt.Errorf("discover legacy contest settlement reservation: %w", err)
		}
		if action, ok := findMatchingSettlementAction(legacyActions, targetRound, targetEndTime); ok {
			return s.ensureQueuedReservation(ctx, action)
		}
	}
	return false, nil
}

func (s *Service) ensureQueuedReservation(ctx context.Context, action core_scheduling.ScheduledAction) (bool, error) {
	if action.State == core_scheduling.StateProcessing {
		return true, nil
	}
	if err := s.scheduler.RequeuePending(ctx, action); err != nil {
		return false, fmt.Errorf("requeue contest settlement reservation: %w", err)
	}
	return true, nil
}

func findMatchingSettlementAction(actions []core_scheduling.ScheduledAction, targetRound int, targetEndTime time.Time) (core_scheduling.ScheduledAction, bool) {
	wantSec := targetEndTime.Truncate(time.Second)
	var pendingMatch core_scheduling.ScheduledAction
	var hasPending bool
	for _, action := range actions {
		if action.ActionType != ActionTypeContestSettlement {
			continue
		}
		round, targetTime, ok := ParseSettlementActionID(action.ID)
		if !ok {
			continue
		}
		if round == targetRound && action.ExecuteAt.Truncate(time.Second).Equal(wantSec) && targetTime.Equal(wantSec) {
			if action.State == core_scheduling.StateProcessing {
				return action, true
			}
			if action.State == core_scheduling.StatePending && !hasPending {
				pendingMatch = action
				hasPending = true
			}
		}
	}
	return pendingMatch, hasPending
}
