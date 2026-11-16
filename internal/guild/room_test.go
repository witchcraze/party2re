package guild_test

import (
	"context"
	"errors"
	"testing"

	"github.com/witchcraze/party2re/internal/guild"
)

func TestValidateMarkAcceptsLegacyMarksOnly(t *testing.T) {
	for _, ok := range []string{"0", "9", "65"} {
		if err := guild.ValidateMark(ok); err != nil {
			t.Fatalf("%q: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "66", "-1", "007", "a", "<b>", " 1"} {
		if err := guild.ValidateMark(bad); !errors.Is(err, guild.ErrInvalidMark) {
			t.Fatalf("%q accepted: %v", bad, err)
		}
	}
}

func TestUpdateNoticeRejectsLegacyMarkup(t *testing.T) {
	stored := ""
	repo := &mockGuildRepo{
		getGuildFn: func(context.Context, string) (guild.Guild, []guild.Member, error) {
			return guild.Guild{ID: "g1", LeaderCharacterID: "leader"}, []guild.Member{{CharacterID: "leader", Role: guild.RoleLeader}}, nil
		},
		updateNoticeFn: func(_ context.Context, _ string, notice string) error { stored = notice; return nil },
	}
	svc, _ := guild.NewService(repo)
	ctx := context.Background()
	for _, bad := range []string{"", "a<b", "x,y", `"q"`, "a&b"} {
		if err := svc.UpdateNotice(ctx, "g1", "leader", bad); !errors.Is(err, guild.ErrInvalidNotice) {
			t.Fatalf("%q: %v", bad, err)
		}
	}
	if err := svc.UpdateNotice(ctx, "g1", "leader", " 今夜集合！ "); err != nil || stored != "今夜集合！" {
		t.Fatalf("notice = %q, %v", stored, err)
	}
}

func TestRecordRoomPresenceTouchesGuild(t *testing.T) {
	var touched []string
	repo := &mockGuildRepo{touchActiveFn: func(_ context.Context, id string) error { touched = append(touched, id); return nil }}
	svc, _ := guild.NewService(repo)
	if err := svc.RecordRoomPresence(context.Background(), "g1"); err != nil || len(touched) != 1 || touched[0] != "g1" {
		t.Fatalf("touched %v, %v", touched, err)
	}
}
