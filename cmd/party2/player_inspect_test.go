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
	"github.com/witchcraze/party2re/internal/playercontext"
)

type inspectSources struct {
	guild         guild.Guild
	guildErr      error
	skill         *customskill.CustomSkill
	skillErr      error
	state         character.PresenceState
	stateErr      error
	location      playercontext.PublicLocation
	locationErr   error
	locationCalls int
}

func (s *inspectSources) InspectLocation(_ context.Context, id string) (playercontext.PublicLocation, error) {
	s.locationCalls++
	if id != "peer" {
		return playercontext.PublicLocation{}, errors.New("location read for wrong character")
	}
	return s.location, s.locationErr
}

func (s *inspectSources) FindGemByID(id string) (customskill.GemDefinition, bool) {
	return customskill.GemDefinition{ID: id, Name: "名前:" + id}, id != "missing"
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
		return playerInspectAdapter{status: s, guild: s, skills: s, presence: s, location: s, gems: s, now: func() time.Time { return now }}
	}

	s := &inspectSources{guild: guild.Guild{ID: "g1", Name: "ギルド"}, skill: &customskill.CustomSkill{Gems: [3]string{"gem-1", "", "gem-3"}},
		state: character.PresenceState{LoginMessage: "やあ", SampledAt: now.Add(-time.Minute)}, location: playercontext.PublicLocation{Kind: "quest"}}
	got, err := adapter(s).InspectPlayer(context.Background(), "hero", "peer", "")
	if err != nil || got.CharacterID != "peer" || got.GuildName != "ギルド" || len(got.Gems) != 2 || got.Gems[1] != "gem-3" ||
		got.LoginMessage != "やあ" || !got.Online || got.LastUpdatedAt == nil || got.Location == nil || got.Location.Kind != "quest" ||
		len(got.GemNames) != 2 || got.GemNames[1] != "名前:gem-3" {
		t.Fatalf("%+v %v", got, err)
	}

	// No guild, skill or sampled save are empty facts, not failures.
	s = &inspectSources{guildErr: guild.ErrCharacterNotInGuild, state: character.PresenceState{SampledAt: now.Add(-time.Hour)}}
	got, err = adapter(s).InspectPlayer(context.Background(), "hero", "peer", "")
	if err != nil || got.GuildID != "" || got.Gems == nil || len(got.Gems) != 0 || got.GemNames == nil || got.Online || got.Location != nil || s.locationCalls != 0 {
		t.Fatalf("%+v %v", got, err)
	}

	storage := errors.New("storage down")
	for name, s := range map[string]*inspectSources{
		"guild": {guildErr: storage}, "skill": {guildErr: guild.ErrCharacterNotInGuild, skillErr: storage},
		"presence": {guildErr: guild.ErrCharacterNotInGuild, stateErr: storage},
		"location": {state: character.PresenceState{SampledAt: now}, locationErr: storage},
	} {
		if _, err := adapter(s).InspectPlayer(context.Background(), "hero", "peer", ""); !errors.Is(err, storage) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	s = &inspectSources{skill: &customskill.CustomSkill{Gems: [3]string{"missing"}}}
	if _, err := adapter(s).InspectPlayer(context.Background(), "hero", "peer", ""); err == nil {
		t.Fatal("unresolved gem returned a partial inspection")
	}
}
