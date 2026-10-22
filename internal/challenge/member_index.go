package challenge

import (
	"context"
	_ "embed"
	"fmt"
	valkey "github.com/valkey-io/valkey-go"
)

// References have an owner cleanup lifecycle and never expire before a renewed run.
// shortcut: expired runs can leave one inert reference per member; add owner cleanup if storage use warrants it.
func (r *ValkeySessionRepository) memberKey(id string) string {
	return fmt.Sprintf("party2:challenge:{char:%s}:member", id)
}

func (r *ValkeySessionRepository) getMemberRun(ctx context.Context, actor string) (*ChallengeSession, error) {
	value, err := r.client.Do(ctx, r.client.B().Get().Key(r.memberKey(actor)).Build()).ToString()
	if valkey.IsValkeyNil(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	ids := decodeMemberReference(value)
	if len(ids) == 0 {
		run, err := r.ReadRun(ctx, value)
		if err != nil || run == nil {
			return run, err
		}
		if !run.ContainsCharacter(actor) {
			return nil, ErrForbidden
		}
		return run, nil
	}
	if len(ids) != 2 || ids[0] == actor {
		return nil, fmt.Errorf("invalid challenge member reference")
	}
	run, err := r.getActiveSession(ctx, ids[0], false)
	if err != nil {
		return nil, err
	}
	if run == nil || run.ID != ids[1] || !run.ContainsCharacter(actor) {
		return nil, nil
	}
	return run, nil
}

func (r *ValkeySessionRepository) saveMemberIndex(ctx context.Context, run ChallengeSession) error {
	for _, m := range run.Members {
		if m.CharacterID == run.CharacterID {
			continue
		}
		value := EncodeJSON([]string{run.CharacterID, run.ID})
		if err := r.client.Do(ctx, r.client.B().Set().Key(r.memberKey(m.CharacterID)).Value(value).Build()).Error(); err != nil {
			return err
		}
	}
	return nil
}
func decodeMemberReference(value string) []string {
	ids, _ := DecodeJSON[[]string](value)
	return ids
}

//go:embed lua/release_member.lua
var releaseMemberLua string
var releaseMemberScript = valkey.NewLuaScript(releaseMemberLua)

func (r *ValkeySessionRepository) deleteMemberIndex(ctx context.Context, run ChallengeSession) error {
	for _, m := range run.Members {
		if m.CharacterID == run.CharacterID {
			continue
		}
		if err := releaseMemberScript.Exec(ctx, r.client, []string{r.memberKey(m.CharacterID)}, []string{EncodeJSON([]string{run.CharacterID, run.ID})}).Error(); err != nil {
			return err
		}
	}
	return nil
}
