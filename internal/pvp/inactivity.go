package pvp

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/witchcraze/party2re/internal/core/timer"
)

// TimerService publishes monotonic sleep penalties without shortening longer sleep.
type TimerService interface {
	ExtendLockUntil(ctx context.Context, category, targetID string, until time.Time) error
	IsLocked(ctx context.Context, category, targetID string) (bool, error)
}

// WithTimerService configures the shared timer service for inactivity sleep penalties.
func WithTimerService(t TimerService) Option {
	return func(s *Service) { s.timers = t }
}

// WithClock overrides the service clock for deterministic boundary testing.
func WithClock(now func() time.Time) Option {
	return func(s *Service) { s.clock = now }
}

func (s *Service) now() time.Time {
	if s.clock != nil {
		return s.clock().UTC()
	}
	return time.Now().UTC()
}

func (s *Service) isInactive(lastActivity, now time.Time) bool {
	return !lastActivity.IsZero() && now.After(lastActivity.Add(InactivityTimeout))
}

func (s *Service) isCharacterSleeping(ctx context.Context, characterID string) (bool, error) {
	if s.timers == nil {
		return false, nil
	}
	return s.timers.IsLocked(ctx, timer.CategorySleep, characterID)
}

func (s *Service) saveLiveRoom(ctx context.Context, detail RoomDetail) error {
	ttl := DefaultLobbyTTL
	if !detail.Room.LastActivityAt.IsZero() {
		ttl = detail.Room.LastActivityAt.Add(DefaultLobbyTTL).Sub(s.now())
	}
	if ttl <= 0 {
		return ErrRoomInactive
	}
	ttlCtx := withRemainingTTL(ctx, ttl)
	if err := s.repo.SaveRoom(ttlCtx, detail.Room, detail.Members); err != nil {
		return err
	}
	for _, m := range detail.Members {
		if err := s.repo.SetCharacterRoom(ttlCtx, m.CharacterID, detail.Room.ID); err != nil {
			return err
		}
	}
	return nil
}

// cleanupInactiveRoom executes legacy auto_delete_quest cleanup (quest.cgi:86-91,172-184):
// closes remaining escrow without refund, marks pending_wake for remaining members,
// publishes the 3600s sleep penalty, and releases room/claim state.
func (s *Service) cleanupInactiveRoom(ctx context.Context, last Transition, now time.Time) (Transition, error) {
	if last.Version == 0 || last.Detail.Room.Status == StatusDisbanded {
		return Transition{}, ErrRoomNotFound
	}
	if s.timers == nil {
		return Transition{}, ErrInvalidDependencies
	}
	memberIDs := make([]string, 0, len(last.Detail.Members))
	for _, m := range last.Detail.Members {
		memberIDs = append(memberIDs, m.CharacterID)
	}
	memberIDs = sortCharacterIDs(memberIDs)

	beforePool := last.Detail.Room.PrizePool
	detail := last.Detail
	detail.Room.Status = StatusDisbanded
	detail.Room.PrizePool = 0
	detail.Room.UpdatedAt = now
	detail.Room.LastActivityAt = now

	penaltyUntil := now.Add(InactivitySleepDuration)
	actorID := last.ActorID
	if len(memberIDs) > 0 {
		actorID = memberIDs[0]
	}
	t := Transition{
		Kind:           "timeout",
		ActorID:        actorID,
		Detail:         detail,
		RemovedIDs:     memberIDs,
		ForfeitedPool:  beforePool,
		LastActivityAt: now,
		PenaltyUntil:   penaltyUntil,
	}
	if err := s.commitTransition(ctx, t, func(txCtx context.Context) error {
		for _, id := range memberIDs {
			c, err := s.characters.FindByIDForUpdate(txCtx, id)
			if err != nil {
				return err
			}
			c.PendingWake = true
			if err := s.characters.Update(txCtx, c); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return Transition{}, err
	}
	t.Version = last.Version + 1
	t.Pending = false
	return t, nil
}

// checkInactiveLocked verifies Valkey connectivity and cleans up the room if inactive.
func (s *Service) checkInactiveLocked(lockedCtx context.Context, roomID string) (RoomDetail, error) {
	detail, err := s.repo.GetRoom(lockedCtx, roomID)
	if err != nil && !errors.Is(err, ErrRoomNotFound) {
		return RoomDetail{}, err
	}
	last := lastTransitionFromContext(lockedCtx)
	if last.Version > 0 && last.Detail.Room.Status != StatusDisbanded && s.isInactive(last.LastActivityAt, s.now()) {
		if _, cleanErr := s.cleanupInactiveRoom(lockedCtx, last, s.now()); cleanErr != nil {
			return RoomDetail{}, cleanErr
		}
		return RoomDetail{}, ErrRoomNotFound
	}
	if err != nil {
		return RoomDetail{}, err
	}
	return detail, nil
}

// GetPendingSleepPenalty returns any unacknowledged PvP inactivity penalty deadline.
func (s *Service) GetPendingSleepPenalty(ctx context.Context, characterID string) (time.Time, bool, error) {
	roomID, err := s.transitions.FindCharacterClaim(ctx, characterID)
	if err != nil || roomID == "" {
		return time.Time{}, false, err
	}
	t, err := s.transitions.FindTransition(ctx, roomID)
	if err != nil {
		return time.Time{}, false, err
	}
	if t.Pending && t.Kind == "timeout" && slices.Contains(t.RemovedIDs, characterID) {
		return t.PenaltyUntil, true, nil
	}
	return time.Time{}, false, nil
}
