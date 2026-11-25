package party

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/economy"
)

// DefaultSystemRoomKeyPrefix maps a stage to its system-owned room. A claim
// whose lobby is gone is stale and is replaced by the next opening.
const DefaultSystemRoomKeyPrefix = "party2:party:sealing:"

//go:embed lua/open_system_room.lua
var openSystemRoomLua string

var openSystemRoomScript = valkey.NewLuaScript(openSystemRoomLua)

var errSystemRoomContended = errors.New("system room claim kept changing")

// WithSystemRoomKeyPrefix overrides the stage-to-system-room key prefix.
func WithSystemRoomKeyPrefix(prefix string) ValkeyRepositoryOption {
	return func(r *repositoryConfig) {
		if prefix != "" {
			r.systemRoomPrefix = prefix
		}
	}
}

// OpenSystemRoom stores p unless its stage already has a live system room
// (make_vs_king skips an existing quest), reporting whether it opened p.
func (r *ValkeyRepository) OpenSystemRoom(ctx context.Context, p Party) (bool, error) {
	sp, err := newStoredParty(p)
	if err != nil {
		return false, err
	}
	data, err := json.Marshal(storedLobbyState{Party: sp, Members: []storedMember{}})
	if err != nil {
		return false, err
	}
	claim := r.systemRoomPrefix + p.StageID
	args := []string{"", p.ID, string(data), strconv.FormatInt(int64(r.lobbyTTL.Seconds()), 10), strconv.FormatInt(p.CreatedAt.Unix(), 10)}
	for range 3 {
		current, err := r.client.Do(ctx, r.client.B().Get().Key(claim).Build()).ToString()
		if err != nil && !valkey.IsValkeyNil(err) {
			return false, err
		}
		keys := []string{claim, r.lobbyKey(p.ID), r.lobbiesIndexKey}
		if current != "" {
			keys = append(keys, r.lobbyKey(current))
		}
		args[0] = current
		opened, err := openSystemRoomScript.Exec(ctx, r.client, keys, args).AsInt64()
		if err != nil {
			return false, err
		}
		if opened >= 0 {
			return opened == 1, nil
		}
	}
	return false, errSystemRoomContended
}

// OpenSystemRoom stores p unless its stage already has a live system room.
func (r *MemoryRepository) OpenSystemRoom(_ context.Context, p Party) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	for id, exp := range r.memLobbyExp {
		if other := r.memLobbies[id]; other.Kind != "" && other.StageID == p.StageID && exp.After(now) {
			return false, nil
		}
	}
	r.saveLocked(p)
	return true, nil
}

// joinSealingRoom admits a character to a sealing room (quest.cgi add_member,
// type 6), even mid-battle, and charges the join's +20 Tired in the same
// transaction. A failed join charges nothing.
func (s *Service) joinSealingRoom(ctx context.Context, p Party, characterID string) error {
	if p.Status != StatusRecruiting {
		return ErrPartyNotRecruiting
	}
	count, err := s.repo.CountMembers(ctx, p.ID)
	if err != nil {
		return err
	}
	if count >= p.MaxMembers {
		return ErrPartyFull
	}
	if _, _, err := s.repo.GetActivePartyByCharacter(ctx, characterID); err == nil {
		return ErrAlreadyInParty
	}
	char, err := s.charRepo.FindByIDForUpdate(ctx, characterID)
	if err != nil {
		return err
	}
	if s.activityCheck != nil {
		if err := s.activityCheck(ctx, characterID); err != nil {
			return err
		}
	}
	if char.Stats.HP <= 0 {
		return ErrCharacterUnconscious
	}
	if char.Tired >= 100 {
		return ErrCharacterExhausted
	}
	if err := ValidateNeedJoin(p.NeedJoin, char); err != nil {
		return err
	}
	battleActor, err := s.buildBattleActor(ctx, char.ID)
	if err != nil {
		return err
	}
	maxHP := char.Stats.MaxHP
	if battleActor != nil {
		maxHP = battleActor.MaxHP
	}
	char.AddTired(20)
	if err := s.charRepo.Update(ctx, char); err != nil {
		return err
	}
	if err := s.repo.AddMember(ctx, Member{
		PartyID: p.ID, CharacterID: char.ID, CharacterName: char.Name, JobID: char.JobID, Level: char.Level,
		HP: char.Stats.HP, MaxHP: maxHP, JoinedAt: time.Now().UTC(), BattleActor: battleActor,
	}); err != nil {
		return err
	}
	// The lobby is outside SQL: undo the membership if the charge rolls back.
	economy.OnRollback(ctx, func(ctx context.Context) error { return s.repo.RemoveMember(ctx, p.ID, char.ID) })
	return nil
}
