package guild_test

import (
	"context"
	"slices"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/guild"
)

// Each explicit guild milestone writes one trajectory entry. Leaving and
// disbanding record before the membership disappears, so the entry's snapshot
// still shows the guild.
func TestService_RecordsMemories(t *testing.T) {
	ctx := context.Background()
	var events []string
	repo := &mockGuildRepo{
		getGuildFn: func(_ context.Context, guildID string) (guild.Guild, []guild.Member, error) {
			return guild.Guild{ID: guildID, Name: "Knights", LeaderCharacterID: "leader"}, []guild.Member{
				{GuildID: guildID, CharacterID: "leader", Role: guild.RoleLeader},
				{GuildID: guildID, CharacterID: "member", Role: guild.RoleMember},
				{GuildID: guildID, CharacterID: "applicant", Role: guild.RoleMember, IsPending: true},
			}, nil
		},
		getGuildByCharFn: func(_ context.Context, characterID string) (guild.Guild, guild.Member, error) {
			if characterID != "member" {
				return guild.Guild{}, guild.Member{}, guild.ErrCharacterNotInGuild
			}
			return guild.Guild{ID: "g1", Name: "Knights"}, guild.Member{GuildID: "g1", CharacterID: characterID}, nil
		},
		removeMemberFn: func(_ context.Context, _, characterID string) error {
			events = append(events, "remove "+characterID)
			return nil
		},
		disbandGuildFn: func(context.Context, string) error {
			events = append(events, "disband")
			return nil
		},
	}
	svc, err := guild.NewService(repo, guild.WithMemoryRecorder(
		func(_ context.Context, id string, kind corecharacter.MemoryKind, content string) error {
			events = append(events, id+"|"+string(kind)+"|"+content)
			return nil
		}))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		run  func() error
		want []string
	}{
		{"found", func() error { _, _, _, err := svc.Create(ctx, "founder", "Knights"); return err },
			[]string{"founder|guild_found|新ギルドKnightsを設立！"}},
		{"approved applicant joins", func() error { return svc.ApproveApplication(ctx, "g1", "leader", "applicant", "新人") },
			[]string{"applicant|guild_join|Knightsギルドに加入"}},
		{"leave", func() error { return svc.Leave(ctx, "g1", "member") },
			[]string{"member|guild_leave|Knightsから脱退する", "remove member"}},
		{"pending applicant withdraws", func() error { return svc.Leave(ctx, "g1", "applicant") },
			[]string{"remove applicant"}},
		{"removal with a deleted character", func() error { return svc.RemoveFromGuilds(ctx, "member") },
			[]string{"remove member"}},
		{"disband", func() error { return svc.Disband(ctx, "g1", "leader") },
			[]string{"leader|guild_disband|Knightsを解散させる", "disband"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			events = nil
			if err := tc.run(); err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(events, tc.want) {
				t.Fatalf("events = %q, want %q", events, tc.want)
			}
		})
	}
}
