package home

import (
	"context"
	"errors"
	"fmt"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/economy"
)

var (
	ErrAlreadySleeping = errors.New("character is already sleeping")
	ErrStillSleeping   = errors.New("character is still sleeping")
	ErrNotSleeping     = errors.New("character is not sleeping")
	ErrWakeInProgress  = errors.New("wake is already in progress")
)

const (
	DefaultBaseSleepDuration = 60 * time.Second

	// wakeClaimTTL bounds the per-character Wake claim if the process dies mid-Wake.
	wakeClaimTTL = 30 * time.Second
)

// CharacterUpdater abstracts updating full character state.
type CharacterUpdater interface {
	FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error)
	Update(ctx context.Context, char corecharacter.Character) error
}

// TimerService manages sleep timers and daily quotas.
type TimerService interface {
	SetLock(ctx context.Context, category, targetID string, duration time.Duration) error
	TryLock(ctx context.Context, category, targetID string, duration time.Duration) (bool, error)
	IsLocked(ctx context.Context, category, targetID string) (bool, error)
	GetRemainingLock(ctx context.Context, category, targetID string) (time.Duration, error)
	ReleaseLock(ctx context.Context, category, targetID string) error
	ResetDailyQuota(ctx context.Context, action, targetID string) error
}

// FullnessResetter resets character eating/fullness state upon rest.
type FullnessResetter interface {
	ResetFullness(ctx context.Context, characterID string) error
}

// BlessingCleaner clears chapel daily prayers and blessings upon rest.
type BlessingCleaner interface {
	ClearBlessing(ctx context.Context, characterID string) error
}

// AlchemyCompleter completes ongoing alchemy synthesis upon rest.
type AlchemyCompleter interface {
	CompleteOngoingSynthesis(ctx context.Context, characterID string) error
}

// CostumeResetter resets rented costume state upon rest.
type CostumeResetter interface {
	ResetCostume(ctx context.Context, characterID string) error
}

// JobStateRestorer restores character_jobs.current_job_id upon rest when reverting JobMemory.
type JobStateRestorer interface {
	RestoreActiveJob(ctx context.Context, characterID, jobID string) error
}

// OnlineCounter counts currently logged in players for sleep duration scaling.
type OnlineCounter interface {
	GetOnlineCount(ctx context.Context) (int, error)
}

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

// WakeResult represents the outcome of waking up from sleep.
type WakeResult struct {
	Success   bool                    `json:"success"`
	Message   string                  `json:"message"`
	Character corecharacter.Character `json:"character"`
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

	onlineCount := 0
	if s.onlineCounter != nil {
		if count, err := s.onlineCounter.GetOnlineCount(ctx); err == nil {
			onlineCount = count
		}
	}

	duration := CalculateSleepDuration(s.baseSleepDuration, onlineCount)

	if err := s.timer.SetLock(ctx, timer.CategorySleep, characterID, duration); err != nil {
		return SleepResult{}, err
	}
	// Ephemeral pending wake flag valid up to 365 days (retained for backward-compatibility with ephemeral query observers)
	if err := s.timer.SetLock(ctx, timer.CategoryAsleep, characterID, 365*24*time.Hour); err != nil {
		_ = s.timer.ReleaseLock(ctx, timer.CategorySleep, characterID)
		return SleepResult{}, err
	}

	// Persist durable pending_wake obligation in SQL, and revert temporary job memory if active
	if s.runner != nil {
		req := economy.TransactionRequest{CharacterID: characterID}
		_, err := s.runner.ExecuteTransaction(ctx, req, func(tc *economy.TxContext) error {
			tc.Character.PendingWake = true
			if tc.Character.JobMemory != nil {
				memoryJobID := tc.Character.JobMemory.JobID
				tc.Character.RevertJobMemory()
				if s.jobRestorer != nil {
					if err := s.jobRestorer.RestoreActiveJob(tc.Context, characterID, memoryJobID); err != nil {
						return err
					}
				}
			}
			return nil
		})
		if err != nil {
			_ = s.timer.ReleaseLock(ctx, timer.CategorySleep, characterID)
			_ = s.timer.ReleaseLock(ctx, timer.CategoryAsleep, characterID)
			return SleepResult{}, err
		}
	} else if s.charUpdater != nil {
		c, err := s.charUpdater.FindByIDForUpdate(ctx, characterID)
		if err != nil {
			_ = s.timer.ReleaseLock(ctx, timer.CategorySleep, characterID)
			_ = s.timer.ReleaseLock(ctx, timer.CategoryAsleep, characterID)
			return SleepResult{}, err
		}
		c.PendingWake = true
		if c.JobMemory != nil {
			memoryJobID := c.JobMemory.JobID
			c.RevertJobMemory()
			if s.jobRestorer != nil {
				if err := s.jobRestorer.RestoreActiveJob(ctx, characterID, memoryJobID); err != nil {
					_ = s.timer.ReleaseLock(ctx, timer.CategorySleep, characterID)
					_ = s.timer.ReleaseLock(ctx, timer.CategoryAsleep, characterID)
					return SleepResult{}, err
				}
			}
		}
		if err := s.charUpdater.Update(ctx, c); err != nil {
			_ = s.timer.ReleaseLock(ctx, timer.CategorySleep, characterID)
			_ = s.timer.ReleaseLock(ctx, timer.CategoryAsleep, characterID)
			return SleepResult{}, err
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

// Wake completes the sleeping cycle, restoring full HP/MP/tired, resetting fullness and chapel prayers.
func (s *Service) Wake(ctx context.Context, characterID string) (WakeResult, error) {
	if s.timer == nil {
		return WakeResult{}, errors.New("timer service not configured")
	}

	rem, err := s.timer.GetRemainingLock(ctx, timer.CategorySleep, characterID)
	if err != nil {
		return WakeResult{}, err
	}
	if rem > 0 {
		mins := int(rem.Minutes())
		secs := int(rem.Seconds()) % 60
		return WakeResult{}, fmt.Errorf("%w: お休み中「Zzz...」 目覚めるまで %d分%02d秒", ErrStillSleeping, mins, secs)
	}

	char, err := s.charReader.FindByID(ctx, characterID)
	if err != nil {
		return WakeResult{}, err
	}

	isPendingWake := char.PendingWake
	if !isPendingWake && s.timer != nil {
		asleep, err := s.timer.IsLocked(ctx, timer.CategoryAsleep, characterID)
		if err != nil {
			return WakeResult{}, err
		}
		if asleep {
			isPendingWake = true
		}
	}
	if !isPendingWake {
		return WakeResult{
			Success:   true,
			Message:   fmt.Sprintf("%sはすでに目覚めています", char.Name),
			Character: char,
		}, nil
	}

	// Serialize recovery per character: a concurrent Wake must not repeat side effects
	// (e.g. the dungeon_once reset) after another Wake has completed and play resumed.
	claimed, err := s.timer.TryLock(ctx, timer.CategoryWaking, characterID, wakeClaimTTL)
	if err != nil {
		return WakeResult{}, err
	}
	if !claimed {
		return WakeResult{}, ErrWakeInProgress
	}
	defer func() {
		//lint:ignore error-swallow best-effort claim release; the claim also expires by TTL
		_ = s.timer.ReleaseLock(ctx, timer.CategoryWaking, characterID)
	}()

	// Re-verify after claim in case another Wake finished concurrently
	if s.charReader != nil {
		if c, err := s.charReader.FindByID(ctx, characterID); err == nil && !c.PendingWake {
			asleep, _ := s.timer.IsLocked(ctx, timer.CategoryAsleep, characterID)
			if !asleep {
				return WakeResult{
					Success:   true,
					Message:   fmt.Sprintf("%sはすでに目覚めています", c.Name),
					Character: c,
				}, nil
			}
		}
	}

	if s.runner != nil {
		// Commit vitality recovery before mandatory hooks and Valkey cleanup.
		// Mutate the runner's current locked state, preserving unrelated assets.
		res, err := s.runner.ExecuteTransaction(ctx, economy.TransactionRequest{CharacterID: characterID}, func(tc *economy.TxContext) error {
			tc.Character.RecoverVitality()
			tc.Character.ResetTired()
			return nil
		})
		if err != nil {
			return WakeResult{}, err
		}
		char = res.Character
	} else if s.charUpdater != nil {
		c, err := s.charUpdater.FindByIDForUpdate(ctx, characterID)
		if err != nil {
			return WakeResult{}, err
		}
		char = c
		char.RecoverVitality()
		char.ResetTired()
		if err := s.charUpdater.Update(ctx, char); err != nil {
			return WakeResult{}, err
		}
	} else {
		char.RecoverVitality()
		char.ResetTired()
	}

	if s.fullness != nil {
		if err := s.fullness.ResetFullness(ctx, characterID); err != nil {
			return WakeResult{}, err
		}
	}
	if s.chapel != nil {
		if err := s.chapel.ClearBlessing(ctx, characterID); err != nil {
			return WakeResult{}, err
		}
	}
	if s.alchemy != nil {
		if err := s.alchemy.CompleteOngoingSynthesis(ctx, characterID); err != nil {
			return WakeResult{}, err
		}
	}
	if s.costume != nil {
		if err := s.costume.ResetCostume(ctx, characterID); err != nil {
			return WakeResult{}, err
		}
	}

	if err := s.timer.ReleaseLock(ctx, timer.CategoryDungeonOnce, characterID); err != nil {
		return WakeResult{}, err
	}
	if err := s.timer.ResetDailyQuota(ctx, "dungeon_once", characterID); err != nil {
		return WakeResult{}, err
	}

	// Finalize durable pending_wake obligation in SQL after all hooks succeed
	if char.PendingWake {
		if s.runner != nil {
			res, err := s.runner.ExecuteTransaction(ctx, economy.TransactionRequest{CharacterID: characterID}, func(tc *economy.TxContext) error {
				tc.Character.PendingWake = false
				return nil
			})
			if err != nil {
				return WakeResult{}, err
			}
			char = res.Character
		} else if s.charUpdater != nil {
			c, err := s.charUpdater.FindByIDForUpdate(ctx, characterID)
			if err != nil {
				return WakeResult{}, err
			}
			c.PendingWake = false
			if err := s.charUpdater.Update(ctx, c); err != nil {
				return WakeResult{}, err
			}
			char = c
		} else {
			char.PendingWake = false
		}
	}

	if err := s.timer.ReleaseLock(ctx, timer.CategoryAsleep, characterID); err != nil {
		return WakeResult{}, err
	}

	return WakeResult{
		Success:   true,
		Message:   fmt.Sprintf("%sのHP・MP・疲労度が回復した！", char.Name),
		Character: char,
	}, nil
}

// SetFullnessResetter registers a cross-domain fullness reset hook.
func (s *Service) SetFullnessResetter(f FullnessResetter) {
	s.fullness = f
}

// SetBlessingCleaner registers a cross-domain chapel blessing cleaner hook.
func (s *Service) SetBlessingCleaner(b BlessingCleaner) {
	s.chapel = b
}

// SetAlchemyCompleter registers a cross-domain alchemy synthesis completer hook.
func (s *Service) SetAlchemyCompleter(a AlchemyCompleter) {
	s.alchemy = a
}

// SetCostumeResetter registers a cross-domain costume reset hook.
func (s *Service) SetCostumeResetter(c CostumeResetter) {
	s.costume = c
}

// SetJobStateRestorer registers a cross-domain job state restorer hook.
func (s *Service) SetJobStateRestorer(r JobStateRestorer) {
	s.jobRestorer = r
}
