package home

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/id"
)

const defaultWakeClaimTTL = 30 * time.Second

// WakeResult represents the outcome of waking up from sleep.
type WakeResult struct {
	Success   bool                    `json:"success"`
	Message   string                  `json:"message"`
	Character corecharacter.Character `json:"character"`
}

func (s *Service) claimTTL() time.Duration {
	if s.wakeClaimTTL > 0 {
		return s.wakeClaimTTL
	}
	return defaultWakeClaimTTL
}

func (s *Service) claimRenewInterval() time.Duration {
	if s.wakeClaimRenewInterval > 0 {
		return s.wakeClaimRenewInterval
	}
	ttl := s.claimTTL()
	interval := ttl / 3
	if interval < 5*time.Millisecond {
		interval = 5 * time.Millisecond
	}
	return interval
}

func (s *Service) releaseWakingClaim(ctx context.Context, characterID, token string) {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), sleepRollbackTimeout)
	defer cancel()
	//lint:ignore error-swallow best-effort claim release; the claim also expires by TTL
	_, _ = s.timer.ReleaseLockWithToken(cleanupCtx, timer.CategoryWaking, characterID, token)
}

type wakeClaimKeeper struct {
	characterID string
	token       string
	timer       TimerService
	ttl         time.Duration
	interval    time.Duration
	stopCh      chan struct{}
	doneCh      chan struct{}
	lost        atomic.Bool
	cancel      context.CancelFunc
	stopOnce    sync.Once
}

func (s *Service) newWakeClaimKeeper(characterID, token string, cancel context.CancelFunc) *wakeClaimKeeper {
	k := &wakeClaimKeeper{
		characterID: characterID,
		token:       token,
		timer:       s.timer,
		ttl:         s.claimTTL(),
		interval:    s.claimRenewInterval(),
		stopCh:      make(chan struct{}),
		doneCh:      make(chan struct{}),
		cancel:      cancel,
	}
	go k.run()
	return k
}

func (k *wakeClaimKeeper) run() {
	defer close(k.doneCh)
	ticker := time.NewTicker(k.interval)
	defer ticker.Stop()

	for {
		select {
		case <-k.stopCh:
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), sleepRollbackTimeout)
			renewed, err := k.timer.RenewLock(ctx, timer.CategoryWaking, k.characterID, k.token, k.ttl)
			cancel()
			if err != nil || !renewed {
				k.lost.Store(true)
				if k.cancel != nil {
					k.cancel()
				}
				return
			}
		}
	}
}

func (k *wakeClaimKeeper) stop() {
	k.stopOnce.Do(func() {
		close(k.stopCh)
		<-k.doneCh
		if k.cancel != nil {
			k.cancel()
		}
	})
}

func (k *wakeClaimKeeper) checkOwnership(ctx context.Context) error {
	if k.lost.Load() {
		if k.cancel != nil {
			k.cancel()
		}
		return ErrWakeInProgress
	}
	renewCtx, cancel := context.WithTimeout(context.Background(), sleepRollbackTimeout)
	defer cancel()
	renewed, err := k.timer.RenewLock(renewCtx, timer.CategoryWaking, k.characterID, k.token, k.ttl)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}
	if !renewed {
		k.lost.Store(true)
		if k.cancel != nil {
			k.cancel()
		}
		return ErrWakeInProgress
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

// wakeClaimContext intercepts context checks to synchronously re-verify waking claim
// ownership against Valkey before downstream hooks issue storage mutations.
type wakeClaimContext struct {
	context.Context
	keeper *wakeClaimKeeper
}

func newWakeClaimContext(parent context.Context, keeper *wakeClaimKeeper) *wakeClaimContext {
	return &wakeClaimContext{
		Context: parent,
		keeper:  keeper,
	}
}

func (c *wakeClaimContext) Err() error {
	if err := c.Context.Err(); err != nil {
		return err
	}
	if c.keeper.lost.Load() {
		return context.Canceled
	}
	checkCtx, cancel := context.WithTimeout(context.Background(), sleepRollbackTimeout)
	defer cancel()
	if err := c.keeper.checkOwnership(checkCtx); err != nil {
		c.keeper.lost.Store(true)
		if c.keeper.cancel != nil {
			c.keeper.cancel()
		}
		return context.Canceled
	}
	return nil
}

func (c *wakeClaimContext) Done() <-chan struct{} {
	if c.keeper.lost.Load() {
		if c.keeper.cancel != nil {
			c.keeper.cancel()
		}
	}
	return c.Context.Done()
}

// Wake completes the sleeping cycle, restoring full HP/MP/tired, resetting fullness and chapel prayers.
func (s *Service) Wake(ctx context.Context, characterID string) (WakeResult, error) {
	if s.timer == nil {
		return WakeResult{}, errors.New("timer service not configured")
	}

	if err := s.checkNotSleeping(ctx, characterID); err != nil {
		return WakeResult{}, err
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
	claimToken := id.New()
	claimed, err := s.timer.TryLockWithToken(ctx, timer.CategoryWaking, characterID, claimToken, s.claimTTL())
	if err != nil {
		return WakeResult{}, err
	}
	if !claimed {
		return WakeResult{}, ErrWakeInProgress
	}
	defer s.releaseWakingClaim(ctx, characterID, claimToken)

	hookCtx, hookCancel := context.WithCancel(ctx)
	defer hookCancel()

	keeper := s.newWakeClaimKeeper(characterID, claimToken, hookCancel)
	defer keeper.stop()

	if err := s.checkNotSleeping(ctx, characterID); err != nil {
		return WakeResult{}, err
	}

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

	if err := keeper.checkOwnership(ctx); err != nil {
		return WakeResult{}, err
	}

	char, err = s.recoverVitality(ctx, characterID, char, keeper)
	if err != nil {
		return WakeResult{}, err
	}

	claimCtx := newWakeClaimContext(hookCtx, keeper)

	if s.fullness != nil {
		if err := s.executeRecoveryHook(ctx, claimCtx, keeper, func() error {
			return s.fullness.ResetFullness(claimCtx, characterID)
		}); err != nil {
			return WakeResult{}, err
		}
	}
	if s.chapel != nil {
		if err := s.executeRecoveryHook(ctx, claimCtx, keeper, func() error {
			return s.chapel.ClearBlessing(claimCtx, characterID)
		}); err != nil {
			return WakeResult{}, err
		}
	}
	if s.alchemy != nil {
		if err := s.executeRecoveryHook(ctx, claimCtx, keeper, func() error {
			return s.alchemy.CompleteOngoingSynthesis(claimCtx, characterID)
		}); err != nil {
			return WakeResult{}, err
		}
	}
	if s.costume != nil {
		if err := s.executeRecoveryHook(ctx, claimCtx, keeper, func() error {
			return s.costume.ResetCostume(claimCtx, characterID)
		}); err != nil {
			return WakeResult{}, err
		}
	}

	if err := keeper.checkOwnership(ctx); err != nil {
		return WakeResult{}, err
	}

	if err := s.checkNotSleeping(ctx, characterID); err != nil {
		return WakeResult{}, err
	}
	reset, err := s.timer.ResetDungeonOnceWithClaim(ctx, characterID, timer.CategoryWaking, claimToken)
	if err != nil {
		return WakeResult{}, err
	}
	if !reset {
		return WakeResult{}, ErrWakeInProgress
	}

	// Finalize durable pending_wake obligation in SQL after all hooks succeed
	if char.PendingWake {
		char, err = s.finalizePendingWake(ctx, characterID, keeper)
		if err != nil {
			return WakeResult{}, err
		}
	}

	released, err := s.timer.ReleaseLockWithClaim(ctx, timer.CategoryAsleep, characterID, timer.CategoryWaking, claimToken)
	if err != nil {
		return WakeResult{}, err
	}
	if !released {
		return WakeResult{}, ErrWakeInProgress
	}

	return WakeResult{
		Success:   true,
		Message:   fmt.Sprintf("%sのHP・MP・疲労度が回復した！", char.Name),
		Character: char,
	}, nil
}

func (s *Service) recoverVitality(ctx context.Context, characterID string, char corecharacter.Character, keeper *wakeClaimKeeper) (corecharacter.Character, error) {
	// Commit vitality recovery before mandatory hooks and Valkey cleanup.
	// Mutate the runner's current locked state, preserving unrelated assets.
	return s.mutateCharacter(ctx, characterID, char, func(txCtx context.Context, c *corecharacter.Character) error {
		if err := keeper.checkOwnership(txCtx); err != nil {
			return err
		}
		if err := s.checkNotSleeping(txCtx, characterID); err != nil {
			return err
		}
		c.RecoverVitality()
		c.ResetTired()
		return nil
	})
}

func (s *Service) finalizePendingWake(ctx context.Context, characterID string, keeper *wakeClaimKeeper) (corecharacter.Character, error) {
	return s.mutateCharacter(ctx, characterID, corecharacter.Character{}, func(txCtx context.Context, c *corecharacter.Character) error {
		if err := keeper.checkOwnership(txCtx); err != nil {
			return err
		}
		if err := s.checkNotSleeping(txCtx, characterID); err != nil {
			return err
		}
		if !c.PendingWake {
			return nil
		}
		c.PendingWake = false
		return nil
	})
}

func (s *Service) executeRecoveryHook(wakeCtx, hookCtx context.Context, keeper *wakeClaimKeeper, hookFn func() error) error {
	if err := keeper.checkOwnership(wakeCtx); err != nil {
		return err
	}
	if err := hookFn(); err != nil {
		if keeper.lost.Load() || errors.Is(err, context.Canceled) {
			checkCtx, cancel := context.WithTimeout(context.Background(), sleepRollbackTimeout)
			defer cancel()
			if ownErr := keeper.checkOwnership(checkCtx); ownErr != nil {
				return ownErr
			}
		}
		return err
	}
	return keeper.checkOwnership(wakeCtx)
}
