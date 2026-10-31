package timer

import (
	"context"
	_ "embed"
	"strings"
	"time"

	"github.com/valkey-io/valkey-go"
)

//go:embed lua/release_with_claim.lua
var releaseWithClaimLua string

//go:embed lua/set_with_claim.lua
var setWithClaimLua string

var (
	releaseWithClaimScript = valkey.NewLuaScript(releaseWithClaimLua)
	setWithClaimScript     = valkey.NewLuaScript(setWithClaimLua)
)

type claimContextKey struct{}

type claimSpec struct {
	category string
	token    string
}

// WithClaim binds a required claim category and token to ctx so SetLock
// atomically verifies claim ownership before writing the target lock.
func WithClaim(ctx context.Context, claimCategory, claimToken string) context.Context {
	return context.WithValue(ctx, claimContextKey{}, claimSpec{
		category: strings.TrimSpace(claimCategory),
		token:    strings.TrimSpace(claimToken),
	})
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
	if !exists || (!exp.expiresAt.IsZero() && time.Now().After(exp.expiresAt)) || exp.value != claimToken {
		return false, nil
	}
	delete(s.memory, targetK)
	return true, nil
}

func (s *service) ExecuteWithClaim(ctx context.Context, category, targetID, token string, fn func() error) (bool, error) {
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
		if val != token {
			return false, nil
		}
		if fn != nil {
			if err := fn(); err != nil {
				return false, err
			}
		}
		return true, nil
	}

	s.memMu.Lock()
	defer s.memMu.Unlock()
	exp, exists := s.memory[k]
	if !exists || (!exp.expiresAt.IsZero() && time.Now().After(exp.expiresAt)) || exp.value != token {
		return false, nil
	}
	if fn != nil {
		if err := fn(); err != nil {
			return false, err
		}
	}
	return true, nil
}
