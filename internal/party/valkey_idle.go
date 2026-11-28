package party

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/valkey-io/valkey-go"
)

func (r *ValkeyRepository) DueLobbies(ctx context.Context, now time.Time, limit int) ([]string, error) {
	before := "(" + strconv.FormatInt(now.UnixMilli(), 10)
	cmd := r.client.B().Zrangebyscore().Key(r.lobbiesIndexKey).Min("-inf").Max(before).Limit(0, int64(limit)).Build()
	return r.client.Do(ctx, cmd).AsStrSlice()
}

func (r *ValkeyRepository) BeginLobbyTimeout(ctx context.Context, id string, now time.Time) ([]Member, time.Time, error) {
	data, err := r.mutateLobby(ctx, "timeout", id, strconv.FormatInt(now.Add(lobbyIdleSleep).UnixMilli(), 10), now).AsBytes()
	if valkey.IsValkeyNil(err) {
		return nil, time.Time{}, nil
	}
	if err != nil {
		return nil, time.Time{}, mapLuaError(err)
	}
	var state storedLobbyState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, time.Time{}, err
	}
	members := make([]Member, 0, len(state.Members))
	for _, member := range state.Members {
		members = append(members, member.Member)
	}
	return members, time.UnixMilli(state.SleepUntil).UTC(), nil
}

func (r *ValkeyRepository) RetryLobbyTimeout(ctx context.Context, id string, now time.Time) error {
	// XX GT moves a failed candidate behind older work without touching a
	// deadline renewed by a command or recreating a removed index entry.
	cmd := r.client.B().Zadd().Key(r.lobbiesIndexKey).Xx().Gt().ScoreMember().ScoreMember(float64(now.UnixMilli()), id).Build()
	return r.client.Do(ctx, cmd).Error()
}
