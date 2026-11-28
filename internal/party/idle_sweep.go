package party

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/runhost"
)

const (
	lobbyIdleTimeout   = 30 * time.Minute
	lobbyIdleSleep     = time.Hour
	lobbyIdleBatchSize = 50
)

var errIdleSweepDone = errors.New("lobby sweep completed using Valkey")

type SleepTimer interface {
	ExtendLockUntil(ctx context.Context, category, actor string, until time.Time) error
}

type LobbyTimeoutRepository interface {
	DueLobbies(ctx context.Context, now time.Time, limit int) ([]string, error)
	BeginLobbyTimeout(ctx context.Context, id string, now time.Time) ([]Member, time.Time, error)
	RetryLobbyTimeout(ctx context.Context, id string, now time.Time) error
}

func WithIdleSweep(store LobbyTimeoutRepository, sleep SleepTimer) Option {
	return func(s *Service) {
		s.idleStore = store
		s.sleep = sleep
	}
}

// SweepIdle handles waiting lobbies on the existing worker tick, including
// interrupted sleep publication. Quest list reads leave timeouts to the worker.
func (s *Service) SweepIdle(ctx context.Context) error {
	ids, err := s.idleStore.DueLobbies(ctx, s.now(), lobbyIdleBatchSize)
	if err != nil {
		return err
	}
	var errs []error
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return errors.Join(append(errs, err)...)
		}
		err := s.sweepLobby(ctx, id)
		if err == nil {
			continue
		}
		errs = append(errs, s.idleStore.RetryLobbyTimeout(ctx, id, s.now()))
		if !errors.Is(err, ErrPartyNotRecruiting) && !errors.Is(err, runhost.ErrBusy) {
			errs = append(errs, fmt.Errorf("party %s: %w", id, err))
		}
	}
	return errors.Join(errs...)
}

func (s *Service) sweepLobby(ctx context.Context, id string) error {
	return s.repo.WithPartyAdventureLock(ctx, id, func(ctx context.Context) error {
		// Prune missing entries without opening a SQL handoff transaction.
		_, err := s.repo.GetParty(ctx, id)
		if errors.Is(err, ErrNotFound) {
			_, _, err := s.idleStore.BeginLobbyTimeout(ctx, id, s.now())
			return err
		}
		if err != nil {
			return err
		}

		err = s.withWaitingLobby(ctx, id, func(ctx context.Context) error {
			members, until, err := s.idleStore.BeginLobbyTimeout(ctx, id, s.now())
			if err != nil {
				return err
			}
			if until.IsZero() {
				return errIdleSweepDone
			}
			// Waiting rosters contain only characters. Reuse the same absolute
			// deadline after a partial write to preserve any longer sleep.
			for _, member := range members {
				if err := s.sleep.ExtendLockUntil(ctx, timer.CategorySleep, member.CharacterID, until); err != nil {
					return err
				}
			}
			if err := s.repo.DeleteParty(ctx, id); err != nil {
				return err
			}
			// The SQL handoff lock may provision an empty coordination row.
			// Roll that back because the timeout only persists to Valkey.
			return errIdleSweepDone
		})
		if errors.Is(err, errIdleSweepDone) {
			return nil
		}
		return err
	})
}
