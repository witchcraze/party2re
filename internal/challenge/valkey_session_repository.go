package challenge

import (
	"context"
	_ "embed"
	"fmt"
	"strconv"
	"strings"
	"time"

	valkeygo "github.com/valkey-io/valkey-go"
)

const (
	DefaultSessionTTL = 2 * time.Hour
)

//go:embed lua/challenge_round.lua
var challengeRoundLua string

type valkeyConfig struct {
	ttl time.Duration
}

// ValkeyOption configures optional parameters on ValkeySessionRepository and MemorySessionRepository.
type ValkeyOption func(*valkeyConfig)

// WithSessionTTL overrides the default 2-hour sliding expiration.
func WithSessionTTL(ttl time.Duration) ValkeyOption {
	return func(c *valkeyConfig) {
		if ttl > 0 {
			c.ttl = ttl
		}
	}
}

// ValkeySessionRepository manages active challenge session buffers in Valkey Master
// using atomic Lua scripts and sliding TTL eviction (Candidate D).
type ValkeySessionRepository struct {
	client      valkeygo.Client
	roundScript *valkeygo.Lua
	ttl         time.Duration
	fallback    *MemorySessionRepository
}

// NewValkeySessionRepository creates a new Valkey-backed challenge session store.
// If client is nil, it falls back to a thread-safe in-memory store.
func NewValkeySessionRepository(client valkeygo.Client, opts ...ValkeyOption) (*ValkeySessionRepository, error) {
	cfg := &valkeyConfig{
		ttl: DefaultSessionTTL,
	}
	for _, opt := range opts {
		opt(cfg)
	}

	repo := &ValkeySessionRepository{
		client:      client,
		roundScript: valkeygo.NewLuaScript(challengeRoundLua),
		ttl:         cfg.ttl,
		fallback:    NewMemorySessionRepository(opts...),
	}

	return repo, nil
}

func (r *ValkeySessionRepository) sessionKey(charID string) string {
	return fmt.Sprintf("party2:challenge:{char:%s}:session", charID)
}

func (r *ValkeySessionRepository) rewardsKey(charID string) string {
	return fmt.Sprintf("party2:challenge:{char:%s}:rewards", charID)
}

func mapLuaError(err error) error {
	if err == nil {
		return nil
	}
	errStr := err.Error()
	if strings.Contains(errStr, "ERR_SESSION_NOT_FOUND") {
		return ErrSessionNotFound
	}
	if strings.Contains(errStr, "ERR_SESSION_NOT_ACTIVE") {
		return ErrSessionNotActive
	}
	if strings.Contains(errStr, "ERR_SESSION_ID_MISMATCH") {
		return ErrSessionIDMismatch
	}
	return err
}

func (r *ValkeySessionRepository) GetActiveSession(ctx context.Context, characterID string) (*ChallengeSession, error) {
	if r.client == nil {
		return r.fallback.GetActiveSession(ctx, characterID)
	}

	sKey := r.sessionKey(characterID)
	rKey := r.rewardsKey(characterID)

	cmds := r.client.DoMulti(ctx,
		r.client.B().Hgetall().Key(sKey).Build(),
		r.client.B().Hgetall().Key(rKey).Build(),
	)

	sMap, err := cmds[0].AsStrMap()
	if err != nil {
		if valkeygo.IsValkeyNil(err) {
			return nil, nil
		}
		return nil, err
	}
	if len(sMap) == 0 {
		return nil, nil
	}

	rMap, _ := cmds[1].AsStrMap()

	round, _ := strconv.Atoi(sMap["current_round"])
	hp, _ := strconv.Atoi(sMap["character_current_hp"])

	exp, _ := strconv.Atoi(rMap["exp"])
	gold, _ := strconv.Atoi(rMap["gold"])
	items, _ := DecodeJSON[[]string](rMap["items"])
	if items == nil {
		items = []string{}
	}

	createdAt, _ := time.Parse(time.RFC3339Nano, sMap["created_at"])
	updatedAt, _ := time.Parse(time.RFC3339Nano, sMap["updated_at"])

	session := &ChallengeSession{
		ID:                 sMap["session_id"],
		CharacterID:        sMap["character_id"],
		TierID:             sMap["tier_id"],
		CurrentRound:       round,
		CharacterCurrentHP: hp,
		AccumulatedExp:     exp,
		AccumulatedGold:    gold,
		AccumulatedItems:   items,
		Status:             SessionStatus(sMap["status"]),
		CreatedAt:          createdAt,
		UpdatedAt:          updatedAt,
	}

	return session, nil
}

func (r *ValkeySessionRepository) SaveActiveSession(ctx context.Context, session ChallengeSession) error {
	if r.client == nil {
		return r.fallback.SaveActiveSession(ctx, session)
	}

	sKey := r.sessionKey(session.CharacterID)
	rKey := r.rewardsKey(session.CharacterID)

	itemsJSON := EncodeJSON(session.AccumulatedItems)
	ttlSec := int64(r.ttl.Seconds())

	cmds := r.client.DoMulti(ctx,
		r.client.B().Hset().Key(sKey).FieldValue().
			FieldValue("session_id", session.ID).
			FieldValue("character_id", session.CharacterID).
			FieldValue("tier_id", session.TierID).
			FieldValue("current_round", strconv.Itoa(session.CurrentRound)).
			FieldValue("character_current_hp", strconv.Itoa(session.CharacterCurrentHP)).
			FieldValue("status", string(session.Status)).
			FieldValue("created_at", session.CreatedAt.Format(time.RFC3339Nano)).
			FieldValue("updated_at", session.UpdatedAt.Format(time.RFC3339Nano)).
			Build(),
		r.client.B().Hset().Key(rKey).FieldValue().
			FieldValue("exp", strconv.Itoa(session.AccumulatedExp)).
			FieldValue("gold", strconv.Itoa(session.AccumulatedGold)).
			FieldValue("items", itemsJSON).
			Build(),
		r.client.B().Expire().Key(sKey).Seconds(ttlSec).Build(),
		r.client.B().Expire().Key(rKey).Seconds(ttlSec).Build(),
	)

	for _, cmd := range cmds {
		if err := cmd.Error(); err != nil {
			return err
		}
	}

	return nil
}

func (r *ValkeySessionRepository) DeleteActiveSession(ctx context.Context, characterID string) error {
	if r.client == nil {
		return r.fallback.DeleteActiveSession(ctx, characterID)
	}

	sKey := r.sessionKey(characterID)
	rKey := r.rewardsKey(characterID)

	_ = r.client.DoMulti(ctx,
		r.client.B().Del().Key(sKey).Build(),
		r.client.B().Del().Key(rKey).Build(),
	)

	return nil
}

func (r *ValkeySessionRepository) AdvanceRound(ctx context.Context, characterID string, params AdvanceRoundParams) (AdvanceRoundOutcome, error) {
	if r.client == nil {
		return r.fallback.AdvanceRound(ctx, characterID, params)
	}

	sKey := r.sessionKey(characterID)
	rKey := r.rewardsKey(characterID)

	res := r.roundScript.Exec(ctx, r.client, []string{sKey, rKey}, []string{
		params.ExpectedSessionID,
		strconv.Itoa(params.SurvivingHP),
		strconv.Itoa(params.ExpDelta),
		strconv.Itoa(params.GoldDelta),
		params.RewardItemID,
		params.Now.Format(time.RFC3339Nano),
		strconv.FormatInt(int64(r.ttl.Seconds()), 10),
	})

	if err := res.Error(); err != nil {
		return AdvanceRoundOutcome{}, mapLuaError(err)
	}

	vals, err := res.AsStrSlice()
	if err != nil || len(vals) < 8 {
		return AdvanceRoundOutcome{}, fmt.Errorf("unexpected lua advance round response format: %v", err)
	}

	newRound, _ := strconv.Atoi(vals[0])
	survivingHP, _ := strconv.Atoi(vals[1])
	totExp, _ := strconv.Atoi(vals[2])
	totGold, _ := strconv.Atoi(vals[3])
	items, _ := DecodeJSON[[]string](vals[4])
	if items == nil {
		items = []string{}
	}
	updatedAt, _ := time.Parse(time.RFC3339Nano, vals[5])
	tierID := vals[6]
	createdAt, _ := time.Parse(time.RFC3339Nano, vals[7])

	session := ChallengeSession{
		ID:                 params.ExpectedSessionID,
		CharacterID:        characterID,
		TierID:             tierID,
		CurrentRound:       newRound,
		CharacterCurrentHP: survivingHP,
		AccumulatedExp:     totExp,
		AccumulatedGold:    totGold,
		AccumulatedItems:   items,
		Status:             StatusActive,
		CreatedAt:          createdAt,
		UpdatedAt:          updatedAt,
	}

	return AdvanceRoundOutcome{
		Session: session,
		Status:  StatusActive,
	}, nil
}
