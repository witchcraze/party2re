package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/auction"
	"github.com/witchcraze/party2re/internal/character"
	"github.com/witchcraze/party2re/internal/customskill"
	"github.com/witchcraze/party2re/internal/guild"
)

type inspectSources struct {
	guild    guild.Guild
	guildErr error
	skill    *customskill.CustomSkill
	skillErr error
	state    character.PresenceState
	stateErr error
}

func (s *inspectSources) Inspect(_ context.Context, _, targetID, _ string) (auction.InspectResult, error) {
	return auction.InspectResult{CharacterID: targetID, Name: "相手"}, nil
}
func (s *inspectSources) GetByCharacter(context.Context, string) (guild.Guild, guild.Member, error) {
	return s.guild, guild.Member{}, s.guildErr
}
func (s *inspectSources) GetCustomSkill(context.Context, string) (*customskill.CustomSkill, error) {
	return s.skill, s.skillErr
}
func (s *inspectSources) State(context.Context, string) (character.PresenceState, error) {
	return s.state, s.stateErr
}

func TestPlayerInspectAdapter(t *testing.T) {
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	adapter := func(s *inspectSources) playerInspectAdapter {
		return playerInspectAdapter{status: s, guild: s, skills: s, presence: s, now: func() time.Time { return now }}
	}

	s := &inspectSources{guild: guild.Guild{ID: "g1", Name: "ギルド"}, skill: &customskill.CustomSkill{Gems: [3]string{"gem-1", "", "gem-3"}},
		state: character.PresenceState{LoginMessage: "やあ", SampledAt: now.Add(-time.Minute)}}
	got, err := adapter(s).InspectPlayer(context.Background(), "hero", "peer", "")
	if err != nil || got.CharacterID != "peer" || got.GuildName != "ギルド" || len(got.Gems) != 2 || got.Gems[1] != "gem-3" ||
		got.LoginMessage != "やあ" || !got.Online || got.LastUpdatedAt == nil {
		t.Fatalf("%+v %v", got, err)
	}

	// No guild, skill or sampled save are empty facts, not failures.
	s = &inspectSources{guildErr: guild.ErrCharacterNotInGuild, state: character.PresenceState{SampledAt: now.Add(-time.Hour)}}
	got, err = adapter(s).InspectPlayer(context.Background(), "hero", "peer", "")
	if err != nil || got.GuildID != "" || got.Gems == nil || len(got.Gems) != 0 || got.Online {
		t.Fatalf("%+v %v", got, err)
	}

	storage := errors.New("storage down")
	for name, s := range map[string]*inspectSources{
		"guild": {guildErr: storage}, "skill": {guildErr: guild.ErrCharacterNotInGuild, skillErr: storage},
		"presence": {guildErr: guild.ErrCharacterNotInGuild, stateErr: storage},
	} {
		if _, err := adapter(s).InspectPlayer(context.Background(), "hero", "peer", ""); !errors.Is(err, storage) {
			t.Fatalf("%s: %v", name, err)
		}
	}
}
