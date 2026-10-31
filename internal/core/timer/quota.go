package timer

import (
	"context"
	"strings"
	"time"

	"github.com/valkey-io/valkey-go"
)

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
	if exists && now.Before(exp.expiresAt) {
		return false, nil
	}
	s.memory[k] = memTimerEntry{expiresAt: nextMidnight, value: "1"}
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
	if !exists || time.Now().After(exp.expiresAt) {
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

func (s *service) ResetDailyQuotaWithClaim(ctx context.Context, action, targetID, claimCategory, claimToken string) (bool, error) {
	claimToken = strings.TrimSpace(claimToken)
	if claimToken == "" {
		return false, ErrEmptyToken
	}
	claimK, err := timerKey(claimCategory, targetID)
	if err != nil {
		return false, err
	}
	quotaK, err := dailyKey(action, targetID)
	if err != nil {
		return false, err
	}

	if s.client != nil {
		res := releaseWithClaimScript.Exec(ctx, s.client, []string{claimK, quotaK}, []string{claimToken})
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
	if !exists || (!exp.expiresAt.IsZero() && time.Now().After(exp.expiresAt)) || exp.value != claimToken {
		return false, nil
	}
	delete(s.memory, quotaK)
	return true, nil
}

func (s *service) ResetDungeonOnceWithClaim(ctx context.Context, targetID, claimCategory, claimToken string) (bool, error) {
	claimToken = strings.TrimSpace(claimToken)
	if claimToken == "" {
		return false, ErrEmptyToken
	}
	claimK, err := timerKey(claimCategory, targetID)
	if err != nil {
		return false, err
	}
	lockK, err := timerKey(CategoryDungeonOnce, targetID)
	if err != nil {
		return false, err
	}
	quotaK, err := dailyKey("dungeon_once", targetID)
	if err != nil {
		return false, err
	}

	if s.client != nil {
		res := releaseWithClaimScript.Exec(ctx, s.client, []string{claimK, lockK, quotaK}, []string{claimToken})
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
	if !exists || (!exp.expiresAt.IsZero() && time.Now().After(exp.expiresAt)) || exp.value != claimToken {
		return false, nil
	}
	delete(s.memory, lockK)
	delete(s.memory, quotaK)
	return true, nil
}
