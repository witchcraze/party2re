package timer

import (
	"context"
	_ "embed"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/valkey-io/valkey-go"
)

//go:embed lua/renew_lock.lua
var renewLockLua string

//go:embed lua/release_lock.lua
var releaseLockLua string

//go:embed lua/release_with_claim.lua
var releaseWithClaimLua string

var (
	renewLockScript        = valkey.NewLuaScript(renewLockLua)
	releaseLockScript      = valkey.NewLuaScript(releaseLockLua)
	releaseWithClaimScript = valkey.NewLuaScript(releaseWithClaimLua)
)

const (
	PrefixTimer = "party2:timer:"
	PrefixDaily = "party2:daily:"

	CategorySleep  = "sleep"
	CategoryAsleep = "asleep"
	// CategoryWaking is a short-lived claim held while one Wake runs recovery.
	CategoryWaking      = "waking"
	CategoryHouse       = "house"
	CategoryStore       = "store"
	CategoryDungeonOnce = "dungeon_once"
)

var (
	ErrInvalidCategory = errors.New("invalid timer category")
	ErrInvalidAction   = errors.New("invalid daily action")
	ErrEmptyID         = errors.New("target id cannot be empty")
	ErrEmptyToken      = errors.New("lock token cannot be empty")
)

// JST is Japan Standard Time (UTC+9), used for canonical daily resets in Party2.
var JST = time.FixedZone("JST", 9*60*60)

// NextMidnightJST calculates the next 00:00:00 JST after the provided time.
func NextMidnightJST(now time.Time) time.Time {
	inJST := now.In(JST)
	nextDay := inJST.AddDate(0, 0, 1)
	return time.Date(nextDay.Year(), nextDay.Month(), nextDay.Day(), 0, 0, 0, 0, JST)
}

// LockService defines ephemeral timer lock operations.
type LockService interface {
	SetLock(ctx context.Context, category, targetID string, duration time.Duration) error
	TryLock(ctx context.Context, category, targetID string, duration time.Duration) (bool, error)
	TryLockWithToken(ctx context.Context, category, targetID, token string, duration time.Duration) (bool, error)
	RenewLock(ctx context.Context, category, targetID, token string, duration time.Duration) (bool, error)
	IsLocked(ctx context.Context, category, targetID string) (bool, error)
	GetRemainingLock(ctx context.Context, category, targetID string) (time.Duration, error)
	ReleaseLock(ctx context.Context, category, targetID string) error
	ReleaseLockWithToken(ctx context.Context, category, targetID, token string) (bool, error)
	ReleaseLockWithClaim(ctx context.Context, category, targetID, claimCategory, claimToken string) (bool, error)
	VerifyLockToken(ctx context.Context, category, targetID, token string) (bool, error)
}

// DailyQuotaService defines daily quota tracking and reset operations.
type DailyQuotaService interface {
	ConsumeDailyQuota(ctx context.Context, action, targetID string, now time.Time) (bool, error)
	HasUsedDailyQuota(ctx context.Context, action, targetID string) (bool, error)
	ResetDailyQuota(ctx context.Context, action, targetID string) error
	ResetDailyQuotaWithClaim(ctx context.Context, action, targetID, claimCategory, claimToken string) (bool, error)
	ResetDungeonOnceWithClaim(ctx context.Context, targetID, claimCategory, claimToken string) (bool, error)
}

// Service defines ephemeral timer locks and daily quota operations.
type Service interface {
	LockService
	DailyQuotaService
}

type memTimerEntry struct {
	expiresAt time.Time
	value     string
}

type service struct {
	client valkey.Client
	memMu  sync.RWMutex
	memory map[string]memTimerEntry
}

// NewService creates a new timer.Service backed by Valkey if client is provided,
// or a thread-safe in-memory store for offline/testing use if client is nil.
func NewService(client valkey.Client) Service {
	return &service{
		client: client,
		memory: make(map[string]memTimerEntry),
	}
}

func timerKey(category, targetID string) (string, error) {
	category = strings.TrimSpace(category)
	targetID = strings.TrimSpace(targetID)
	if category == "" {
		return "", ErrInvalidCategory
	}
	if targetID == "" {
		return "", ErrEmptyID
	}
	return PrefixTimer + category + ":" + targetID, nil
}

func (s *service) SetLock(ctx context.Context, category, targetID string, duration time.Duration) error {
	k, err := timerKey(category, targetID)
	if err != nil {
		return err
	}
	if s.client != nil {
		secs := int64(duration.Seconds())
		if secs < 1 {
			secs = 1
		}
		cmd := s.client.B().Set().Key(k).Value("1").ExSeconds(secs).Build()
		return s.client.Do(ctx, cmd).Error()
	}

	s.memMu.Lock()
	s.memory[k] = memTimerEntry{expiresAt: time.Now().Add(duration), value: "1"}
	s.memMu.Unlock()
	return nil
}

func (s *service) TryLock(ctx context.Context, category, targetID string, duration time.Duration) (bool, error) {
	return s.TryLockWithToken(ctx, category, targetID, "1", duration)
}

func (s *service) TryLockWithToken(ctx context.Context, category, targetID, token string, duration time.Duration) (bool, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return false, ErrEmptyToken
	}
	k, err := timerKey(category, targetID)
	if err != nil {
		return false, err
	}
	if s.client != nil {
		secs := int64(duration.Seconds())
		if secs < 1 {
			secs = 1
		}
		cmd := s.client.B().Set().Key(k).Value(token).Nx().ExSeconds(secs).Build()
		err := s.client.Do(ctx, cmd).Error()
		if valkey.IsValkeyNil(err) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		return true, nil
	}

	s.memMu.Lock()
	defer s.memMu.Unlock()
	if exp, exists := s.memory[k]; exists && time.Now().Before(exp.expiresAt) {
		return false, nil
	}
	s.memory[k] = memTimerEntry{expiresAt: time.Now().Add(duration), value: token}
	return true, nil
}

func (s *service) RenewLock(ctx context.Context, category, targetID, token string, duration time.Duration) (bool, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return false, ErrEmptyToken
	}
	k, err := timerKey(category, targetID)
	if err != nil {
		return false, err
	}
	if s.client != nil {
		secs := int64(duration.Seconds())
		if secs < 1 {
			secs = 1
		}
		res := renewLockScript.Exec(ctx, s.client, []string{k}, []string{token, strconv.FormatInt(secs, 10)})
		if res.Error() != nil {
			if valkey.IsValkeyNil(res.Error()) {
				return false, nil
			}
			return false, res.Error()
		}
		n, err := res.AsInt64()
		if err != nil {
			return false, err
		}
		return n == 1, nil
	}

	s.memMu.Lock()
	defer s.memMu.Unlock()
	exp, exists := s.memory[k]
	if !exists || time.Now().After(exp.expiresAt) || exp.value != token {
		return false, nil
	}
	s.memory[k] = memTimerEntry{expiresAt: time.Now().Add(duration), value: token}
	return true, nil
}

func (s *service) IsLocked(ctx context.Context, category, targetID string) (bool, error) {
	k, err := timerKey(category, targetID)
	if err != nil {
		return false, err
	}
	if s.client != nil {
		cmd := s.client.B().Exists().Key(k).Build()
		count, err := s.client.Do(ctx, cmd).AsInt64()
		if err != nil {
			return false, err
		}
		return count > 0, nil
	}

	s.memMu.RLock()
	exp, exists := s.memory[k]
	s.memMu.RUnlock()
	if !exists || time.Now().After(exp.expiresAt) {
		return false, nil
	}
	return true, nil
}

func (s *service) GetRemainingLock(ctx context.Context, category, targetID string) (time.Duration, error) {
	k, err := timerKey(category, targetID)
	if err != nil {
		return 0, err
	}
	if s.client != nil {
		cmd := s.client.B().Ttl().Key(k).Build()
		ttlSec, err := s.client.Do(ctx, cmd).AsInt64()
		if err != nil {
			return 0, err
		}
		if ttlSec <= 0 {
			return 0, nil
		}
		return time.Duration(ttlSec) * time.Second, nil
	}

	s.memMu.RLock()
	exp, exists := s.memory[k]
	s.memMu.RUnlock()
	if !exists {
		return 0, nil
	}
	remaining := exp.expiresAt.Sub(time.Now())
	if remaining <= 0 {
		return 0, nil
	}
	return remaining, nil
}

func (s *service) ReleaseLock(ctx context.Context, category, targetID string) error {
	k, err := timerKey(category, targetID)
	if err != nil {
		return err
	}
	if s.client != nil {
		cmd := s.client.B().Del().Key(k).Build()
		return s.client.Do(ctx, cmd).Error()
	}

	s.memMu.Lock()
	delete(s.memory, k)
	s.memMu.Unlock()
	return nil
}

func (s *service) ReleaseLockWithToken(ctx context.Context, category, targetID, token string) (bool, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return false, ErrEmptyToken
	}
	k, err := timerKey(category, targetID)
	if err != nil {
		return false, err
	}
	if s.client != nil {
		res := releaseLockScript.Exec(ctx, s.client, []string{k}, []string{token})
		if res.Error() != nil {
			if valkey.IsValkeyNil(res.Error()) {
				return false, nil
			}
			return false, res.Error()
		}
		n, err := res.AsInt64()
		if err != nil {
			return false, err
		}
		return n == 1, nil
	}

	s.memMu.Lock()
	defer s.memMu.Unlock()
	exp, exists := s.memory[k]
	if !exists || time.Now().After(exp.expiresAt) || exp.value != token {
		return false, nil
	}
	delete(s.memory, k)
	return true, nil
}

func (s *service) ReleaseLockWithClaim(ctx context.Context, category, targetID, claimCategory, claimToken string) (bool, error) {
	claimToken = strings.TrimSpace(claimToken)
	if claimToken == "" {
		return false, ErrEmptyToken
	}
	claimK, err := timerKey(claimCategory, targetID)
	if err != nil {
		return false, err
	}
	targetK, err := timerKey(category, targetID)
	if err != nil {
		return false, err
	}

	if s.client != nil {
		res := releaseWithClaimScript.Exec(ctx, s.client, []string{claimK, targetK}, []string{claimToken})
		if res.Error() != nil {
			if valkey.IsValkeyNil(res.Error()) {
				return false, nil
			}
			return false, res.Error()
		}
		n, err := res.AsInt64()
		if err != nil {
			return false, err
		}
		return n == 1, nil
	}

	s.memMu.Lock()
	defer s.memMu.Unlock()
	exp, exists := s.memory[claimK]
	if !exists || time.Now().After(exp.expiresAt) || exp.value != claimToken {
		return false, nil
	}
	delete(s.memory, targetK)
	return true, nil
}

func (s *service) VerifyLockToken(ctx context.Context, category, targetID, token string) (bool, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return false, ErrEmptyToken
	}
	k, err := timerKey(category, targetID)
	if err != nil {
		return false, err
	}
	if s.client != nil {
		cmd := s.client.B().Get().Key(k).Build()
		resp := s.client.Do(ctx, cmd)
		if resp.Error() != nil {
			if valkey.IsValkeyNil(resp.Error()) {
				return false, nil
			}
			return false, resp.Error()
		}
		val, err := resp.ToString()
		if err != nil {
			return false, err
		}
		return val == token, nil
	}

	s.memMu.RLock()
	defer s.memMu.RUnlock()
	exp, exists := s.memory[k]
	if !exists || time.Now().After(exp.expiresAt) {
		return false, nil
	}
	return exp.value == token, nil
}
