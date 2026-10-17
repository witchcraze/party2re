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

func (r *ValkeySessionRepository) leaderKey(charID string) string {
	return fmt.Sprintf("party2:challenge:{char:%s}:leader", charID)
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

func (r *ValkeySessionRepository) getActiveSessionDirect(ctx context.Context, characterID string) (*ChallengeSession, error) {
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

	rMap, err := cmds[1].AsStrMap()
	if err != nil {
		return nil, err
	}

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
	members, _ := DecodeJSON[[]ChallengeMember](sMap["members"])

	session := &ChallengeSession{
		ID:                 sMap["session_id"],
		CharacterID:        sMap["character_id"],
		PartyID:            sMap["party_id"],
		PartyName:          sMap["party_name"],
		PartyColor:         sMap["party_color"],
		Members:            members,
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

func (r *ValkeySessionRepository) GetActiveSession(ctx context.Context, characterID string) (*ChallengeSession, error) {
	if r.client == nil {
		return r.fallback.GetActiveSession(ctx, characterID)
	}

	session, err := r.getActiveSessionDirect(ctx, characterID)
	if err != nil {
		return nil, err
	}
	if session != nil {
		return session, nil
	}

	// Check if characterID is a non-leader member mapped to a leader
	leaderID, err := r.client.Do(ctx, r.client.B().Get().Key(r.leaderKey(characterID)).Build()).ToString()
	if err != nil {
		if valkeygo.IsValkeyNil(err) {
			return nil, nil
		}
		return nil, err
	}
	if leaderID == "" {
		return nil, nil
	}

	leaderSession, err := r.getActiveSessionDirect(ctx, leaderID)
	if err != nil {
		return nil, err
	}
	if leaderSession == nil {
		_ = r.client.Do(ctx, r.client.B().Del().Key(r.leaderKey(characterID)).Build())
		return nil, nil
	}

	// Privacy check: verify characterID is an authorized member of leaderSession
	found := false
	for _, m := range leaderSession.Members {
		if m.CharacterID == characterID {
			found = true
			break
		}
	}
	if !found {
		_ = r.client.Do(ctx, r.client.B().Del().Key(r.leaderKey(characterID)).Build())
		return nil, nil
	}

	return leaderSession, nil
}

func (r *ValkeySessionRepository) SaveActiveSession(ctx context.Context, session ChallengeSession) error {
	if r.client == nil {
		return r.fallback.SaveActiveSession(ctx, session)
	}

	sKey := r.sessionKey(session.CharacterID)
	rKey := r.rewardsKey(session.CharacterID)

	itemsJSON := EncodeJSON(session.AccumulatedItems)
	membersJSON := EncodeJSON(session.Members)
	ttlSec := int64(r.ttl.Seconds())

	cmds := []valkeygo.Completed{
		r.client.B().Hset().Key(sKey).FieldValue().
			FieldValue("session_id", session.ID).
			FieldValue("character_id", session.CharacterID).
			FieldValue("party_id", session.PartyID).
			FieldValue("party_name", session.PartyName).
			FieldValue("party_color", session.PartyColor).
			FieldValue("members", membersJSON).
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
	}

	for _, mem := range session.Members {
		if mem.CharacterID != session.CharacterID {
			cmds = append(cmds,
				r.client.B().Set().Key(r.leaderKey(mem.CharacterID)).Value(session.CharacterID).Ex(r.ttl).Build(),
			)
		}
	}

	res := r.client.DoMulti(ctx, cmds...)
	for _, cmd := range res {
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

	leaderID := characterID
	sess, _ := r.getActiveSessionDirect(ctx, characterID)
	if sess == nil {
		lid, err := r.client.Do(ctx, r.client.B().Get().Key(r.leaderKey(characterID)).Build()).ToString()
		if err == nil && lid != "" {
			leaderID = lid
			sess, _ = r.getActiveSessionDirect(ctx, leaderID)
		}
	}

	sKey := r.sessionKey(leaderID)
	rKey := r.rewardsKey(leaderID)

	delCmds := []valkeygo.Completed{
		r.client.B().Del().Key(sKey).Build(),
		r.client.B().Del().Key(rKey).Build(),
	}
	if sess != nil {
		for _, m := range sess.Members {
			delCmds = append(delCmds, r.client.B().Del().Key(r.leaderKey(m.CharacterID)).Build())
		}
	} else {
		delCmds = append(delCmds, r.client.B().Del().Key(r.leaderKey(characterID)).Build())
	}

	_ = r.client.DoMulti(ctx, delCmds...)
	return nil
}

func (r *ValkeySessionRepository) AdvanceRound(ctx context.Context, characterID string, params AdvanceRoundParams) (AdvanceRoundOutcome, error) {
	if r.client == nil {
		return r.fallback.AdvanceRound(ctx, characterID, params)
	}

	targetID := characterID
	if lid, err := r.client.Do(ctx, r.client.B().Get().Key(r.leaderKey(characterID)).Build()).ToString(); err == nil && lid != "" {
		targetID = lid
	}

	sKey := r.sessionKey(targetID)
	rKey := r.rewardsKey(targetID)

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

	partyID := ""
	partyName := ""
	partyColor := ""
	var members []ChallengeMember
	if len(vals) >= 12 {
		partyID = vals[8]
		partyName = vals[9]
		partyColor = vals[10]
		if vals[11] != "" {
			members, _ = DecodeJSON[[]ChallengeMember](vals[11])
		}
	}

	session := ChallengeSession{
		ID:                 params.ExpectedSessionID,
		CharacterID:        targetID,
		PartyID:            partyID,
		PartyName:          partyName,
		PartyColor:         partyColor,
		Members:            members,
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

	if len(members) > 0 {
		var expireCmds []valkeygo.Completed
		for _, m := range members {
			if m.CharacterID != targetID {
				expireCmds = append(expireCmds, r.client.B().Expire().Key(r.leaderKey(m.CharacterID)).Seconds(int64(r.ttl.Seconds())).Build())
			}
		}
		if len(expireCmds) > 0 {
			//lint:ignore error-swallow best-effort sliding TTL refresh on member indices
			_ = r.client.DoMulti(ctx, expireCmds...)
		}
	}

	return AdvanceRoundOutcome{
		Session: session,
		Status:  StatusActive,
	}, nil
}
