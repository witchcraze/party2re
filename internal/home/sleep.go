package home

import (
	"context"
	"errors"
	"fmt"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/timer"
)

var (
	ErrAlreadySleeping = errors.New("character is already sleeping")
	ErrStillSleeping   = errors.New("character is still sleeping")
	ErrNotSleeping     = errors.New("character is not sleeping")
)

const (
	DefaultBaseSleepDuration = 60 * time.Second
)

// CharacterUpdater abstracts updating full character state.
type CharacterUpdater interface {
	FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error)
	Update(ctx context.Context, char corecharacter.Character) error
}

// TimerService manages sleep timers and daily quotas.
type TimerService interface {
	SetLock(ctx context.Context, category, targetID string, duration time.Duration) error
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
		if _, err := s.charReader.FindByID(ctx, targetHomeID); err != nil {
			return SleepResult{}, ErrCharacterNotFound
		}
		targetHome, err := s.repo.GetHome(ctx, targetHomeID)
		if err != nil {
			return SleepResult{}, err
		}
		now := s.nowFunc()
		if now.IsZero() {
			now = time.Now()
		}
		if !targetHome.IsActive(now.UTC()) {
			return SleepResult{}, ErrHouseNotFound
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
	// Ephemeral pending wake flag valid up to 24 hours
	_ = s.timer.SetLock(ctx, timer.CategoryAsleep, characterID, 24*time.Hour)

	// Legacy parity: Revert temporary job memory if active upon going to sleep
	if char.JobMemory != nil && s.charUpdater != nil {
		char.RevertJobMemory()
		_ = s.charUpdater.Update(ctx, char)
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

	asleep, _ := s.timer.IsLocked(ctx, timer.CategoryAsleep, characterID)
	if asleep {
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

	asleep, _ := s.timer.IsLocked(ctx, timer.CategoryAsleep, characterID)
	if !asleep {
		char, err := s.charReader.FindByID(ctx, characterID)
		if err != nil {
			return WakeResult{}, err
		}
		return WakeResult{
			Success:   true,
			Message:   fmt.Sprintf("%sはすでに目覚めています", char.Name),
			Character: char,
		}, nil
	}

	var char corecharacter.Character
	if s.charUpdater != nil {
		c, err := s.charUpdater.FindByIDForUpdate(ctx, characterID)
		if err != nil {
			return WakeResult{}, err
		}
		char = c
	} else {
		c, err := s.charReader.FindByID(ctx, characterID)
		if err != nil {
			return WakeResult{}, err
		}
		char = c
	}

	char.Stats.HP = char.Stats.MaxHP
	char.Stats.MP = char.Stats.MaxMP
	char.ResetTired()
	char.RevertJobMemory()

	if s.charUpdater != nil {
		if err := s.charUpdater.Update(ctx, char); err != nil {
			return WakeResult{}, err
		}
	}

	if s.fullness != nil {
		_ = s.fullness.ResetFullness(ctx, characterID)
	}
	if s.chapel != nil {
		_ = s.chapel.ClearBlessing(ctx, characterID)
	}

	_ = s.timer.ReleaseLock(ctx, timer.CategoryAsleep, characterID)

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
