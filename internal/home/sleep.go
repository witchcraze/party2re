package home

import (
	"context"
	"errors"
	"fmt"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/economy"
	"github.com/witchcraze/party2re/internal/id"
)

var (
	ErrAlreadySleeping = errors.New("character is already sleeping")
	ErrStillSleeping   = errors.New("character is still sleeping")
	ErrNotSleeping     = errors.New("character is not sleeping")
	ErrWakeInProgress  = errors.New("wake is already in progress")
)

const (
	DefaultBaseSleepDuration = 60 * time.Second

	// sleepRollbackTimeout bounds best-effort SQL/Valkey cleanup when Sleep fails
	// even if the caller's request context was canceled or timed out.
	sleepRollbackTimeout = 5 * time.Second
)

// SleepResult represents the outcome of entering sleep.
type SleepResult struct {
	Sleeping         bool   `json:"sleeping"`
	DurationSeconds  int    `json:"duration_seconds"`
	RemainingSeconds int    `json:"remaining_seconds"`
	HomeCharacterID  string `json:"home_character_id"`
	Message          string `json:"message"`
}

// SleepStatus represents current sleeping state.
type SleepStatus struct {
	Sleeping         bool   `json:"sleeping"`
	RemainingSeconds int    `json:"remaining_seconds"`
	CanWake          bool   `json:"can_wake"`
	Message          string `json:"message"`
}

// CalculateSleepDuration calculates sleep duration scaled by concurrent online player count.
// Parity with legacy Party2 (home.cgi: &neru):
// - count >= 30: 3x base
// - count >= 20: 2x base
// - count < 20:  1x base
func CalculateSleepDuration(base time.Duration, onlineCount int) time.Duration {
	if base <= 0 {
		base = DefaultBaseSleepDuration
	}
	if onlineCount >= 30 {
		return base * 3
	}
	if onlineCount >= 20 {
		return base * 2
	}
	return base
}

// Sleep puts the character to sleep in their own or a visited player's house.
func (s *Service) Sleep(ctx context.Context, characterID, targetHomeID string) (SleepResult, error) {
	if s.timer == nil {
		return SleepResult{}, errors.New("timer service not configured")
	}

	char, err := s.charReader.FindByID(ctx, characterID)
	if err != nil {
		return SleepResult{}, err
	}

	if targetHomeID == "" {
		targetHomeID = characterID
	} else if targetHomeID != characterID {
		// Only verify the target character exists; a private home is always available
		// regardless of whether the host has an active town lease.
		// Legacy parity: lib/home.cgi:1-7 checks user directory existence (created at registration),
		// not a paid town lease. lib/system.cgi:418-435 checks for the private home.cgi file,
		// which is created when the character is registered.
		if _, err := s.charReader.FindByID(ctx, targetHomeID); err != nil {
			return SleepResult{}, ErrCharacterNotFound
		}
	}

	isSleeping, err := s.timer.IsLocked(ctx, timer.CategorySleep, characterID)
	if err != nil {
		return SleepResult{}, err
	}
	if isSleeping {
		return SleepResult{}, ErrAlreadySleeping
	}

	sleepClaimToken := id.New()
	claimed, err := s.timer.TryLockWithToken(ctx, timer.CategoryWaking, characterID, sleepClaimToken, s.claimTTL())
	if err != nil {
		return SleepResult{}, err
	}
	if !claimed {
		return SleepResult{}, ErrWakeInProgress
	}
	defer s.releaseWakingClaim(ctx, characterID, sleepClaimToken)

	sleepCtx, sleepCancel := context.WithCancel(ctx)
	defer sleepCancel()

	keeper := s.newWakeClaimKeeper(characterID, sleepClaimToken, sleepCancel)
	defer keeper.stop()

	if isSleeping, err := s.timer.IsLocked(ctx, timer.CategorySleep, characterID); err != nil {
		return SleepResult{}, err
	} else if isSleeping {
		return SleepResult{}, ErrAlreadySleeping
	}
	if asleep, err := s.timer.IsLocked(ctx, timer.CategoryAsleep, characterID); err != nil {
		return SleepResult{}, err
	} else if asleep {
		return SleepResult{}, ErrAlreadySleeping
	}

	onlineCount := 0
	if s.onlineCounter != nil {
		if count, err := s.onlineCounter.GetOnlineCount(ctx); err == nil {
			onlineCount = count
		}
	}
	duration := CalculateSleepDuration(s.baseSleepDuration, onlineCount)

	if err := keeper.checkOwnership(ctx); err != nil {
		return SleepResult{}, err
	}

	claimCtx := timer.WithClaim(newWakeClaimContext(sleepCtx, keeper), timer.CategoryWaking, sleepClaimToken)

	hasTempJob, err := s.markPendingWake(ctx, characterID, char, keeper)
	if err != nil {
		return SleepResult{}, s.normalizeSleepClaimError(ctx, keeper, err)
	}

	if err := keeper.checkOwnership(ctx); err != nil {
		return SleepResult{}, s.rollbackSleepStart(ctx, characterID, sleepClaimToken, keeper, err, false, false)
	}
	if err := s.timer.SetLock(claimCtx, timer.CategorySleep, characterID, duration); err != nil {
		err = s.normalizeSleepClaimError(ctx, keeper, err)
		return SleepResult{}, s.rollbackSleepStart(ctx, characterID, sleepClaimToken, keeper, err, false, false)
	}
	if err := keeper.checkOwnership(ctx); err != nil {
		return SleepResult{}, s.rollbackSleepStart(ctx, characterID, sleepClaimToken, keeper, err, true, false)
	}
	// Ephemeral pending wake flag valid up to 365 days (retained for backward-compatibility with ephemeral query observers)
	if err := s.timer.SetLock(claimCtx, timer.CategoryAsleep, characterID, 365*24*time.Hour); err != nil {
		err = s.normalizeSleepClaimError(ctx, keeper, err)
		return SleepResult{}, s.rollbackSleepStart(ctx, characterID, sleepClaimToken, keeper, err, true, false)
	}
	if err := keeper.checkOwnership(ctx); err != nil {
		return SleepResult{}, s.rollbackSleepStart(ctx, characterID, sleepClaimToken, keeper, err, true, true)
	}

	if hasTempJob {
		if err := s.revertSleepJobMemory(ctx, characterID, char, keeper); err != nil {
			err = s.normalizeSleepClaimError(ctx, keeper, err)
			return SleepResult{}, s.rollbackSleepStart(ctx, characterID, sleepClaimToken, keeper, err, true, true)
		}
		if err := keeper.checkOwnership(ctx); err != nil {
			return SleepResult{}, s.rollbackSleepStart(ctx, characterID, sleepClaimToken, keeper, err, true, true)
		}
	}

	return SleepResult{
		Sleeping:         true,
		DurationSeconds:  int(duration.Seconds()),
		RemainingSeconds: int(duration.Seconds()),
		HomeCharacterID:  targetHomeID,
		Message:          fmt.Sprintf("%sはベッドにもぐりこんだ！", char.Name),
	}, nil
}

func (s *Service) markPendingWake(ctx context.Context, characterID string, initialChar corecharacter.Character, keeper *wakeClaimKeeper) (bool, error) {
	var hasTempJob bool
	_, err := s.mutateCharacter(ctx, characterID, initialChar, func(txCtx context.Context, c *corecharacter.Character) error {
		if err := keeper.checkOwnership(txCtx); err != nil {
			return err
		}
		if c.PendingWake {
			return ErrAlreadySleeping
		}
		c.PendingWake = true
		hasTempJob = c.JobMemory != nil && c.JobMemory.IsTemporary()
		return nil
	})
	return hasTempJob, err
}

func (s *Service) revertSleepJobMemory(ctx context.Context, characterID string, initialChar corecharacter.Character, keeper *wakeClaimKeeper) error {
	_, err := s.mutateCharacter(ctx, characterID, initialChar, func(txCtx context.Context, c *corecharacter.Character) error {
		if err := keeper.checkOwnership(txCtx); err != nil {
			return err
		}
		if c.JobMemory != nil && c.JobMemory.IsTemporary() {
			memoryJobID := c.JobMemory.JobID
			c.RevertJobMemory()
			if s.jobRestorer != nil {
				txClaimCtx := newWakeClaimContext(txCtx, keeper)
				if err := s.jobRestorer.RestoreActiveJob(txClaimCtx, characterID, memoryJobID); err != nil {
					return err
				}
			}
			return keeper.checkOwnership(txCtx)
		}
		return nil
	})
	return err
}

func (s *Service) normalizeSleepClaimError(callerCtx context.Context, keeper *wakeClaimKeeper, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, timer.ErrClaimLost) {
		return ErrWakeInProgress
	}
	if keeper.lost.Load() || (errors.Is(err, context.Canceled) && callerCtx.Err() == nil) {
		checkCtx, cancel := context.WithTimeout(context.Background(), sleepRollbackTimeout)
		defer cancel()
		if ownErr := keeper.checkOwnership(checkCtx); ownErr != nil {
			return ownErr
		}
	}
	return err
}

func (s *Service) rollbackSleepStart(ctx context.Context, characterID, claimToken string, keeper *wakeClaimKeeper, cause error, releaseSleep, releaseAsleep bool) error {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), sleepRollbackTimeout)
	defer cancel()

	errs := []error{cause}
	if releaseAsleep {
		if _, err := s.timer.ReleaseLockWithClaim(cleanupCtx, timer.CategoryAsleep, characterID, timer.CategoryWaking, claimToken); err != nil {
			errs = append(errs, fmt.Errorf("release asleep lock: %w", err))
		}
	}
	if releaseSleep {
		if _, err := s.timer.ReleaseLockWithClaim(cleanupCtx, timer.CategorySleep, characterID, timer.CategoryWaking, claimToken); err != nil {
			errs = append(errs, fmt.Errorf("release sleep lock: %w", err))
		}
	}
	if err := s.revertPendingWake(cleanupCtx, characterID, keeper); err != nil {
		errs = append(errs, fmt.Errorf("revert pending_wake: %w", err))
	}
	return errors.Join(errs...)
}

func (s *Service) revertPendingWake(ctx context.Context, characterID string, keeper *wakeClaimKeeper) error {
	if s.runner == nil && s.charUpdater == nil {
		return nil
	}
	if err := keeper.checkOwnership(ctx); err != nil {
		if errors.Is(err, ErrWakeInProgress) || keeper.lost.Load() {
			return nil
		}
		return err
	}
	_, err := s.mutateCharacter(ctx, characterID, corecharacter.Character{}, func(txCtx context.Context, c *corecharacter.Character) error {
		if err := keeper.checkOwnership(txCtx); err != nil {
			return err
		}
		c.PendingWake = false
		return nil
	})
	if errors.Is(err, ErrWakeInProgress) || keeper.lost.Load() {
		return nil
	}
	return err
}

// GetSleepStatus checks whether the character is currently sleeping or ready to wake.
func (s *Service) GetSleepStatus(ctx context.Context, characterID string) (SleepStatus, error) {
	if s.timer == nil {
		return SleepStatus{Sleeping: false, RemainingSeconds: 0, CanWake: false, Message: "起きています"}, nil
	}

	rem, err := s.timer.GetRemainingLock(ctx, timer.CategorySleep, characterID)
	if err != nil {
		return SleepStatus{}, err
	}

	if rem > 0 {
		mins := int(rem.Minutes())
		secs := int(rem.Seconds()) % 60
		return SleepStatus{
			Sleeping:         true,
			RemainingSeconds: int(rem.Seconds()),
			CanWake:          false,
			Message:          fmt.Sprintf("お休み中「Zzz...」 目覚めるまで %d分%02d秒", mins, secs),
		}, nil
	}

	var isPendingWake bool
	if s.charReader != nil {
		if c, err := s.charReader.FindByID(ctx, characterID); err == nil && c.PendingWake {
			isPendingWake = true
		}
	}
	if !isPendingWake {
		asleep, err := s.timer.IsLocked(ctx, timer.CategoryAsleep, characterID)
		if err != nil {
			return SleepStatus{}, err
		}
		if asleep {
			isPendingWake = true
		}
	}
	if isPendingWake {
		return SleepStatus{
			Sleeping:         false,
			RemainingSeconds: 0,
			CanWake:          true,
			Message:          "目を覚ます準備ができました",
		}, nil
	}

	return SleepStatus{
		Sleeping:         false,
		RemainingSeconds: 0,
		CanWake:          false,
		Message:          "起きています",
	}, nil
}

func (s *Service) checkNotSleeping(ctx context.Context, characterID string) error {
	rem, err := s.timer.GetRemainingLock(ctx, timer.CategorySleep, characterID)
	if err != nil {
		return err
	}
	if rem > 0 {
		mins := int(rem.Minutes())
		secs := int(rem.Seconds()) % 60
		return fmt.Errorf("%w: お休み中「Zzz...」 目覚めるまで %d分%02d秒", ErrStillSleeping, mins, secs)
	}
	return nil
}

func (s *Service) mutateCharacter(ctx context.Context, characterID string, fallback corecharacter.Character, fn func(context.Context, *corecharacter.Character) error) (corecharacter.Character, error) {
	if s.runner != nil {
		res, err := s.runner.ExecuteTransaction(ctx, economy.TransactionRequest{CharacterID: characterID}, func(tc *economy.TxContext) error {
			return fn(tc.Context, &tc.Character)
		})
		if err != nil {
			return corecharacter.Character{}, err
		}
		return res.Character, nil
	}
	if s.charUpdater != nil {
		c, err := s.charUpdater.FindByIDForUpdate(ctx, characterID)
		if err != nil {
			return corecharacter.Character{}, err
		}
		if err := fn(ctx, &c); err != nil {
			return corecharacter.Character{}, err
		}
		if err := s.charUpdater.Update(ctx, c); err != nil {
			return corecharacter.Character{}, err
		}
		return c, nil
	}
	if err := fn(ctx, &fallback); err != nil {
		return corecharacter.Character{}, err
	}
	return fallback, nil
}
