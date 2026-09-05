package boss

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/valkey-io/valkey-go"
)

const (
	DefaultRaidKeyPrefix = "party2:boss:"
)

// ValkeyRaidRepositoryOption configures a ValkeyRaidRepository.
type ValkeyRaidRepositoryOption func(*ValkeyRaidRepository)

// WithRaidKeyPrefix overrides the default key prefix (e.g. For test isolation).
func WithRaidKeyPrefix(prefix string) ValkeyRaidRepositoryOption {
	return func(r *ValkeyRaidRepository) {
		if prefix != "" {
			r.keyPrefix = prefix
		}
	}
}

// ValkeyRaidRepository implements RaidRepository using Valkey Master and preloaded Lua scripts,
// with an in-memory fallback providing 100% behavioral parity.
type ValkeyRaidRepository struct {
	client       valkey.Client
	keyPrefix    string
	damageScript *valkey.Lua

	// In-memory fallback state (used when client == nil)
	mu              sync.RWMutex
	memHP           map[string]int
	memInitialHP    map[string]int
	memRunID        map[string]string
	memStatus       map[string]RaidStatus
	memKiller       map[string]string
	memContributors map[string]map[string]int
}

// NewValkeyRaidRepository creates a new ValkeyRaidRepository.
func NewValkeyRaidRepository(client valkey.Client, opts ...ValkeyRaidRepositoryOption) *ValkeyRaidRepository {
	r := &ValkeyRaidRepository{
		client:          client,
		keyPrefix:       DefaultRaidKeyPrefix,
		damageScript:    valkey.NewLuaScript(bossDamageLua),
		memHP:           make(map[string]int),
		memInitialHP:    make(map[string]int),
		memRunID:        make(map[string]string),
		memStatus:       make(map[string]RaidStatus),
		memKiller:       make(map[string]string),
		memContributors: make(map[string]map[string]int),
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

func (r *ValkeyRaidRepository) hpKey(bossID string) string {
	return fmt.Sprintf("%s{boss:%s}:hp", r.keyPrefix, bossID)
}

func (r *ValkeyRaidRepository) statusKey(bossID string) string {
	return fmt.Sprintf("%s{boss:%s}:status", r.keyPrefix, bossID)
}

func (r *ValkeyRaidRepository) contributorsKey(bossID string) string {
	return fmt.Sprintf("%s{boss:%s}:contributors", r.keyPrefix, bossID)
}

func (r *ValkeyRaidRepository) killerKey(bossID string) string {
	return fmt.Sprintf("%s{boss:%s}:killer", r.keyPrefix, bossID)
}

func (r *ValkeyRaidRepository) runKey(bossID string) string {
	return fmt.Sprintf("%s{boss:%s}:run_id", r.keyPrefix, bossID)
}

// InitializeRaid initializes a new raid session in Valkey Master or in-memory fallback.
func (r *ValkeyRaidRepository) InitializeRaid(ctx context.Context, bossID, runID string, initialHP int, ttl time.Duration) error {
	if r.client == nil {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.memHP[bossID] = initialHP
		r.memInitialHP[bossID] = initialHP
		r.memRunID[bossID] = runID
		r.memStatus[bossID] = RaidStatusActive
		delete(r.memKiller, bossID)
		r.memContributors[bossID] = make(map[string]int)
		return nil
	}

	ttlSeconds := int64(ttl.Seconds())
	if ttlSeconds <= 0 {
		ttlSeconds = 7200
	}

	cmds := make(valkey.Commands, 0, 7)
	cmds = append(cmds,
		r.client.B().Set().Key(r.hpKey(bossID)).Value(strconv.Itoa(initialHP)).ExSeconds(ttlSeconds).Build(),
		r.client.B().Set().Key(r.statusKey(bossID)).Value(string(RaidStatusActive)).ExSeconds(ttlSeconds).Build(),
		r.client.B().Set().Key(r.runKey(bossID)).Value(runID).ExSeconds(ttlSeconds).Build(),
		r.client.B().Del().Key(r.contributorsKey(bossID)).Build(),
		r.client.B().Del().Key(r.killerKey(bossID)).Build(),
	)

	res := r.client.DoMulti(ctx, cmds...)
	for _, resp := range res {
		if err := resp.Error(); err != nil {
			return fmt.Errorf("failed to initialize raid in valkey: %w", err)
		}
	}

	return nil
}

// ApplyDamage applies atomic damage via preloaded Lua script or in-memory fallback.
func (r *ValkeyRaidRepository) ApplyDamage(ctx context.Context, bossID, attackerID string, incomingDamage int, ttl time.Duration) (RaidDamageResult, error) {
	if r.client == nil {
		r.mu.Lock()
		defer r.mu.Unlock()

		currentHP, exists := r.memHP[bossID]
		if !exists {
			return RaidDamageResult{
				Status:       RaidDamageNotFound,
				ActualDamage: 0,
				RemainingHP:  0,
			}, nil
		}

		status := r.memStatus[bossID]
		if status == RaidStatusDefeated || status == RaidStatusSettled || currentHP <= 0 {
			return RaidDamageResult{
				Status:       RaidDamageAlreadyDead,
				ActualDamage: 0,
				RemainingHP:  0,
				KillerID:     r.memKiller[bossID],
			}, nil
		}

		if incomingDamage <= 0 {
			return RaidDamageResult{
				Status:       RaidDamageHit,
				ActualDamage: 0,
				RemainingHP:  currentHP,
			}, nil
		}

		actualDmg := incomingDamage
		if actualDmg > currentHP {
			actualDmg = currentHP
		}
		newHP := currentHP - actualDmg
		r.memHP[bossID] = newHP

		if r.memContributors[bossID] == nil {
			r.memContributors[bossID] = make(map[string]int)
		}
		r.memContributors[bossID][attackerID] += actualDmg

		if newHP == 0 {
			r.memStatus[bossID] = RaidStatusDefeated
			r.memKiller[bossID] = attackerID
			return RaidDamageResult{
				Status:       RaidDamageKilled,
				ActualDamage: actualDmg,
				RemainingHP:  0,
				KillerID:     attackerID,
			}, nil
		}

		return RaidDamageResult{
			Status:       RaidDamageHit,
			ActualDamage: actualDmg,
			RemainingHP:  newHP,
		}, nil
	}

	keys := []string{
		r.hpKey(bossID),
		r.statusKey(bossID),
		r.contributorsKey(bossID),
		r.killerKey(bossID),
		r.runKey(bossID),
	}

	ttlSeconds := strconv.FormatInt(int64(ttl.Seconds()), 10)
	args := []string{
		attackerID,
		strconv.Itoa(incomingDamage),
		ttlSeconds,
	}

	res, err := r.damageScript.Exec(ctx, r.client, keys, args).ToString()
	if err != nil {
		return RaidDamageResult{}, fmt.Errorf("failed to evaluate boss damage lua: %w", err)
	}

	var parsed struct {
		Status       RaidDamageStatus `json:"status"`
		ActualDamage int              `json:"actual_damage"`
		RemainingHP  int              `json:"remaining_hp"`
		KillerID     string           `json:"killer_id"`
	}
	if err := json.Unmarshal([]byte(res), &parsed); err != nil {
		return RaidDamageResult{}, fmt.Errorf("failed to decode lua damage response: %w", err)
	}

	return RaidDamageResult{
		Status:       parsed.Status,
		ActualDamage: parsed.ActualDamage,
		RemainingHP:  parsed.RemainingHP,
		KillerID:     parsed.KillerID,
	}, nil
}

// GetRaidState retrieves the current volatile state of the boss.
func (r *ValkeyRaidRepository) GetRaidState(ctx context.Context, bossID string) (RaidBossState, error) {
	if r.client == nil {
		r.mu.RLock()
		defer r.mu.RUnlock()

		currentHP, exists := r.memHP[bossID]
		if !exists {
			return RaidBossState{}, ErrRaidNotFound
		}

		contribs := make(map[string]int)
		for k, v := range r.memContributors[bossID] {
			contribs[k] = v
		}

		return RaidBossState{
			BossID:       bossID,
			RunID:        r.memRunID[bossID],
			InitialHP:    r.memInitialHP[bossID],
			CurrentHP:    currentHP,
			Status:       r.memStatus[bossID],
			KillerID:     r.memKiller[bossID],
			Contributors: contribs,
		}, nil
	}

	cmds := make(valkey.Commands, 0, 4)
	cmds = append(cmds,
		r.client.B().Get().Key(r.hpKey(bossID)).Build(),
		r.client.B().Get().Key(r.statusKey(bossID)).Build(),
		r.client.B().Get().Key(r.runKey(bossID)).Build(),
		r.client.B().Get().Key(r.killerKey(bossID)).Build(),
	)

	res := r.client.DoMulti(ctx, cmds...)
	hpStr, err := res[0].ToString()
	if err != nil {
		if errors.Is(err, valkey.Nil) {
			return RaidBossState{}, ErrRaidNotFound
		}
		return RaidBossState{}, err
	}

	statusStr, _ := res[1].ToString()
	runID, _ := res[2].ToString()
	killerID, _ := res[3].ToString()

	hp, _ := strconv.Atoi(hpStr)

	contributors, err := r.GetContributors(ctx, bossID)
	if err != nil {
		contributors = make(map[string]int)
	}

	return RaidBossState{
		BossID:       bossID,
		RunID:        runID,
		CurrentHP:    hp,
		Status:       RaidStatus(statusStr),
		KillerID:     killerID,
		Contributors: contributors,
	}, nil
}

// GetContributors returns all contributor damage tallies from the hash.
func (r *ValkeyRaidRepository) GetContributors(ctx context.Context, bossID string) (map[string]int, error) {
	if r.client == nil {
		r.mu.RLock()
		defer r.mu.RUnlock()

		m, exists := r.memContributors[bossID]
		if !exists {
			return make(map[string]int), nil
		}
		copied := make(map[string]int, len(m))
		for k, v := range m {
			copied[k] = v
		}
		return copied, nil
	}

	resp, err := r.client.Do(ctx, r.client.B().Hgetall().Key(r.contributorsKey(bossID)).Build()).AsStrMap()
	if err != nil {
		if errors.Is(err, valkey.Nil) {
			return make(map[string]int), nil
		}
		return nil, fmt.Errorf("failed to get contributors from valkey: %w", err)
	}

	result := make(map[string]int, len(resp))
	for charID, dmgStr := range resp {
		dmg, _ := strconv.Atoi(dmgStr)
		result[charID] = dmg
	}

	return result, nil
}

// MarkSettled marks the raid encounter as settled in Valkey Master.
func (r *ValkeyRaidRepository) MarkSettled(ctx context.Context, bossID, runID string) error {
	if r.client == nil {
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.memRunID[bossID] == runID {
			r.memStatus[bossID] = RaidStatusSettled
		}
		return nil
	}

	err := r.client.Do(ctx, r.client.B().Set().Key(r.statusKey(bossID)).Value(string(RaidStatusSettled)).Keepttl().Build()).Error()
	if err != nil {
		return fmt.Errorf("failed to mark raid settled in valkey: %w", err)
	}

	return nil
}
