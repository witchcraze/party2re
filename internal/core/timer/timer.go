package timer

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/valkey-io/valkey-go"
)

const (
	PrefixTimer = "party2:timer:"
	PrefixDaily = "party2:daily:"

	CategorySleep  = "sleep"
	CategoryAsleep = "asleep"
	CategoryHouse  = "house"
)

var (
	ErrInvalidCategory = errors.New("invalid timer category")
	ErrInvalidAction   = errors.New("invalid daily action")
	ErrEmptyID         = errors.New("target id cannot be empty")
)

// JST is Japan Standard Time (UTC+9), used for canonical daily resets in Party2.
var JST = time.FixedZone("JST", 9*60*60)

// NextMidnightJST calculates the next 00:00:00 JST after the provided time.
func NextMidnightJST(now time.Time) time.Time {
	inJST := now.In(JST)
	nextDay := inJST.AddDate(0, 0, 1)
	return time.Date(nextDay.Year(), nextDay.Month(), nextDay.Day(), 0, 0, 0, 0, JST)
}

// Service defines ephemeral timer locks and daily quota operations.
type Service interface {
	// Tier 1 Ephemeral Timers
	SetLock(ctx context.Context, category, targetID string, duration time.Duration) error
	IsLocked(ctx context.Context, category, targetID string) (bool, error)
	GetRemainingLock(ctx context.Context, category, targetID string) (time.Duration, error)
	ReleaseLock(ctx context.Context, category, targetID string) error

	// Tier 2 Daily Quotas
	ConsumeDailyQuota(ctx context.Context, action, targetID string, now time.Time) (bool, error)
	HasUsedDailyQuota(ctx context.Context, action, targetID string) (bool, error)
	ResetDailyQuota(ctx context.Context, action, targetID string) error
}

type service struct {
	client valkey.Client
	memMu  sync.RWMutex
	memory map[string]time.Time
}

// NewService creates a new timer.Service backed by Valkey if client is provided,
// or a thread-safe in-memory store for offline/testing use if client is nil.
func NewService(client valkey.Client) Service {
	return &service{
		client: client,
		memory: make(map[string]time.Time),
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

func dailyKey(action, targetID string) (string, error) {
	action = strings.TrimSpace(action)
	targetID = strings.TrimSpace(targetID)
	if action == "" {
		return "", ErrInvalidAction
	}
	if targetID == "" {
		return "", ErrEmptyID
	}
	return PrefixDaily + action + ":" + targetID, nil
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
	s.memory[k] = time.Now().Add(duration)
	s.memMu.Unlock()
	return nil
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
	if !exists || time.Now().After(exp) {
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
	remaining := exp.Sub(time.Now())
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

func (s *service) ConsumeDailyQuota(ctx context.Context, action, targetID string, now time.Time) (bool, error) {
	k, err := dailyKey(action, targetID)
	if err != nil {
		return false, err
	}
	nextMidnight := NextMidnightJST(now)
	ttl := nextMidnight.Sub(now)
	secs := int64(ttl.Seconds())
	if secs < 1 {
		secs = 1
	}

	if s.client != nil {
		cmd := s.client.B().Set().Key(k).Value("1").Nx().ExSeconds(secs).Build()
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
	exp, exists := s.memory[k]
	if exists && now.Before(exp) {
		return false, nil
	}
	s.memory[k] = nextMidnight
	return true, nil
}

func (s *service) HasUsedDailyQuota(ctx context.Context, action, targetID string) (bool, error) {
	k, err := dailyKey(action, targetID)
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
	defer s.memMu.RUnlock()
	exp, exists := s.memory[k]
	if !exists || time.Now().After(exp) {
		return false, nil
	}
	return true, nil
}

func (s *service) ResetDailyQuota(ctx context.Context, action, targetID string) error {
	k, err := dailyKey(action, targetID)
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
