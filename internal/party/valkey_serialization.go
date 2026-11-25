package party

import (
	"bytes"
	"encoding/json"

	"github.com/witchcraze/party2re/internal/core/battle"
)

// LobbyState represents the ephemeral composite state stored in Valkey Master for a party.
type LobbyState struct {
	Party   Party    `json:"party"`
	Members []Member `json:"members"`
}

type storedParty struct {
	Party
	PasswordHash string `json:"password_hash,omitempty"`
	// EnemiesJSON stays a string so Lua re-encoding cannot turn empty arrays into objects.
	EnemiesJSON string `json:"enemies_json,omitempty"`
}

func newStoredParty(p Party) (storedParty, error) {
	sp := storedParty{Party: p, PasswordHash: p.PasswordHash}
	if len(p.Enemies) > 0 {
		data, err := json.Marshal(p.Enemies)
		if err != nil {
			return storedParty{}, err
		}
		sp.EnemiesJSON = string(data)
	}
	return sp, nil
}

func marshalStoredParty(p Party) ([]byte, error) {
	sp, err := newStoredParty(p)
	if err != nil {
		return nil, err
	}
	return json.Marshal(sp)
}

func (sp storedParty) toParty() (Party, error) {
	p := sp.Party
	p.PasswordHash = sp.PasswordHash
	if sp.EnemiesJSON != "" {
		if err := json.Unmarshal([]byte(sp.EnemiesJSON), &p.Enemies); err != nil {
			return Party{}, err
		}
	}
	return p, nil
}

type storedMember struct {
	Member
	BattleActorJSON string `json:"battle_actor_json,omitempty"`
}

type storedLobbyState struct {
	Party   storedParty    `json:"party"`
	Members []storedMember `json:"members"`
}

func unmarshalJSONSlice[T any](raw json.RawMessage, dst *[]T) error {
	if len(raw) == 0 || string(raw) == "null" {
		*dst = []T{}
		return nil
	}
	trimmed := bytes.TrimSpace(raw)
	if bytes.Equal(trimmed, []byte("{}")) {
		*dst = []T{}
		return nil
	}
	return json.Unmarshal(raw, dst)
}

func cloneBattleActor(actor *battle.LifetimeActor) *battle.LifetimeActor {
	if actor == nil {
		return nil
	}
	cloned := actor.Clone()
	return &cloned
}

func marshalStoredMember(m Member) ([]byte, error) {
	sm := storedMember{Member: m}
	if m.BattleActor != nil {
		actorJSON, err := json.Marshal(m.BattleActor)
		if err != nil {
			return nil, err
		}
		sm.BattleActorJSON = string(actorJSON)
	}
	return json.Marshal(sm)
}

func decodeStoredMembers(stored []storedMember) ([]Member, error) {
	members := make([]Member, len(stored))
	for i, sm := range stored {
		m := sm.Member
		if sm.BattleActorJSON != "" {
			var actor battle.LifetimeActor
			if err := json.Unmarshal([]byte(sm.BattleActorJSON), &actor); err != nil {
				return nil, err
			}
			m.BattleActor = &actor
		}
		members[i] = m
	}
	return members, nil
}

// UnmarshalJSON implements json.Unmarshaler to handle cases where empty Lua cjson tables
// are serialized as "{}" instead of "[]" for Members.
func (s *LobbyState) UnmarshalJSON(data []byte) error {
	type Alias LobbyState
	aux := struct {
		Members json.RawMessage `json:"members"`
		*Alias
	}{
		Alias: (*Alias)(s),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	return unmarshalJSONSlice(aux.Members, &s.Members)
}

func (s *storedLobbyState) UnmarshalJSON(data []byte) error {
	type Alias storedLobbyState
	aux := struct {
		Members json.RawMessage `json:"members"`
		*Alias
	}{
		Alias: (*Alias)(s),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	return unmarshalJSONSlice(aux.Members, &s.Members)
}
