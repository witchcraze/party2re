package party

import (
	"context"
	_ "embed"

	"github.com/valkey-io/valkey-go"
)

//go:embed lua/cleanup_character.lua
var cleanupCharacterLua string

var cleanupCharacterScript = valkey.NewLuaScript(cleanupCharacterLua)

// CleanupParty retries committed room deletion using a durable roster, even after the lobby is gone.
// Reverse mappings are removed only when they still belong to this room.
func (r *ValkeyRepository) CleanupParty(ctx context.Context, state LobbyState) error {
	id := state.Party.ID
	for _, charID := range lobbyCharacterIDs(state) {
		if err := cleanupCharacterScript.Exec(ctx, r.client, []string{r.characterKey(charID)}, []string{id}).Error(); err != nil {
			return err
		}
		if err := r.client.Do(ctx, r.client.B().Del().Key(r.readyKey(id, charID)).Build()).Error(); err != nil {
			return err
		}
	}
	if err := r.client.Do(ctx, r.client.B().Del().Key(r.lobbyKey(id)).Build()).Error(); err != nil {
		return err
	}
	return r.client.Do(ctx, r.client.B().Zrem().Key(r.lobbiesIndexKey).Member(id).Build()).Error()
}

func lobbyCharacterIDs(state LobbyState) []string {
	ids := []string{state.Party.LeaderCharacterID}
	for _, m := range state.Members {
		ids = append(ids, m.CharacterID)
	}
	return ids
}
