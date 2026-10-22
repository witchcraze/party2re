package challenge

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	valkey "github.com/valkey-io/valkey-go"
)

//go:embed lua/publish_run.lua
var publishRunLua string
var publishRunScript = valkey.NewLuaScript(publishRunLua)

//go:embed lua/claim_member.lua
var claimMemberLua string
var claimMemberScript = valkey.NewLuaScript(claimMemberLua)

//go:embed lua/freeze_legacy.lua
var freezeLegacyLua string
var freezeLegacyScript = valkey.NewLuaScript(freezeLegacyLua)

type runRewardLedger struct {
	Exp, Gold int
	Items     []string
	Awards    []TreasureAward
}

func runKeys(id string) (string, string) {
	return fmt.Sprintf("party2:challenge:{run:%s}:session", id), fmt.Sprintf("party2:challenge:{run:%s}:rewards", id)
}

func (r *ValkeySessionRepository) ReadRun(ctx context.Context, id string) (*ChallengeSession, error) {
	if r.client == nil {
		return r.fallback.ReadRun(ctx, id)
	}
	skey, rkey := runKeys(id)
	results := r.client.DoMulti(ctx, r.client.B().Hgetall().Key(skey).Build(), r.client.B().Hgetall().Key(rkey).Build())
	sm, err := results[0].AsStrMap()
	if err != nil {
		return nil, err
	}
	if len(sm) == 0 {
		return nil, nil
	}
	if sm["status"] != string(StatusActive) {
		return nil, nil
	}
	rm, err := results[1].AsStrMap()
	if err != nil {
		return nil, err
	}
	if sm["revision"] != rm["revision"] || rm["ledger"] == "" {
		return nil, ErrRecoveryRequired
	}
	var run ChallengeSession
	if err := json.Unmarshal([]byte(sm["publication"]), &run); err != nil {
		return nil, err
	}
	if run.ID != id || strconv.Itoa(run.Revision) != sm["revision"] {
		return nil, ErrRevisionConflict
	}
	if rm["ledger"] != EncodeJSON(runRewardLedger{run.AccumulatedExp, run.AccumulatedGold, run.AccumulatedItems, run.TreasureAwards}) {
		return nil, ErrRecoveryRequired
	}
	return &run, nil
}

func (r *ValkeySessionRepository) PublishRun(ctx context.Context, t Transition) error {
	if r.client == nil {
		return r.fallback.PublishRun(ctx, t)
	}
	if t.Publication == nil || t.Publication.ID != t.RunID || t.Publication.Revision != t.Revision {
		return ErrRevisionConflict
	}
	if t.LegacyOwner != "" {
		if err := freezeLegacyScript.Exec(ctx, r.client, []string{r.sessionKey(t.LegacyOwner)}, []string{t.RunID}).Error(); err != nil {
			return mapLuaError(err)
		}
	}
	skey, rkey := runKeys(t.RunID)
	data, err := json.Marshal(t.Publication)
	if err != nil {
		return err
	}
	ledger, err := json.Marshal(runRewardLedger{t.Publication.AccumulatedExp, t.Publication.AccumulatedGold, t.Publication.AccumulatedItems, t.Publication.TreasureAwards})
	if err != nil {
		return err
	}
	if err := publishRunScript.Exec(ctx, r.client, []string{skey, rkey}, []string{strconv.Itoa(t.Revision), string(data), string(t.Publication.Status), string(ledger), strconv.FormatInt(t.Deadline.UnixMilli(), 10)}).Error(); err != nil {
		if strings.Contains(err.Error(), "ERR_REVISION_CONFLICT") {
			return ErrRevisionConflict
		}
		return err
	}
	if t.Publication.Status == StatusActive {
		for _, member := range t.Publication.Members {
			legacy := ""
			if t.LegacyOwner != "" {
				legacy = EncodeJSON([]string{t.LegacyOwner, t.RunID})
			}
			if err := claimMemberScript.Exec(ctx, r.client, []string{r.memberKey(member.CharacterID)}, []string{t.RunID, legacy}).Error(); err != nil {
				return err
			}
		}
	}
	for _, actor := range t.RemovedIDs {
		if err := releaseMemberScript.Exec(ctx, r.client, []string{r.memberKey(actor)}, []string{t.RunID}).Error(); err != nil {
			return err
		}
	}
	if t.LegacyOwner != "" {
		if err := r.discardScript.Exec(ctx, r.client, []string{r.sessionKey(t.LegacyOwner), r.rewardsKey(t.LegacyOwner)}, []string{t.RunID}).Error(); err != nil {
			return err
		}
	}
	return nil
}

func (r *ValkeySessionRepository) ReadLegacyRun(ctx context.Context, owner string) (*ChallengeSession, time.Time, error) {
	if r.client == nil {
		return r.fallback.ReadLegacyRun(ctx, owner)
	}
	run, err := r.getActiveSession(ctx, owner, false)
	if err != nil {
		return nil, time.Time{}, err
	}
	if run == nil {
		return nil, time.Time{}, ErrRecoveryRequired
	}
	keys := []string{r.sessionKey(owner), r.rewardsKey(owner)}
	values := r.client.DoMulti(ctx, r.client.B().Hgetall().Key(keys[0]).Build(), r.client.B().Hgetall().Key(keys[1]).Build())
	sm, err := values[0].AsStrMap()
	if err != nil {
		return nil, time.Time{}, err
	}
	rm, err := values[1].AsStrMap()
	if err != nil {
		return nil, time.Time{}, err
	}
	for _, field := range []string{"exp", "gold"} {
		n, err := strconv.Atoi(rm[field])
		if err != nil || n < 0 {
			return nil, time.Time{}, ErrRecoveryRequired
		}
	}
	var items []string
	var members []ChallengeMember
	if err := json.Unmarshal([]byte(rm["items"]), &items); err != nil {
		return nil, time.Time{}, ErrRecoveryRequired
	}
	if err := json.Unmarshal([]byte(sm["members"]), &members); err != nil {
		return nil, time.Time{}, ErrRecoveryRequired
	}
	if run.ID == "" || run.CharacterID != owner || run.CurrentRound < 1 || run.CreatedAt.IsZero() || run.UpdatedAt.IsZero() {
		return nil, time.Time{}, ErrRecoveryRequired
	}
	results := r.client.DoMulti(ctx, r.client.B().Pexpiretime().Key(r.sessionKey(owner)).Build(), r.client.B().Pexpiretime().Key(r.rewardsKey(owner)).Build())
	a, err := results[0].AsInt64()
	if err != nil {
		return nil, time.Time{}, err
	}
	b, err := results[1].AsInt64()
	if err != nil {
		return nil, time.Time{}, err
	}
	if a <= 0 || b <= 0 {
		return nil, time.Time{}, ErrRecoveryRequired
	}
	return run, time.UnixMilli(min(a, b)).UTC(), nil
}

func (r *ValkeySessionRepository) SessionTTL() time.Duration { return r.ttl }
