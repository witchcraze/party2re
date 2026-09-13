package guild_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/guild"
)

type sentLetterRecord struct {
	SenderID      string
	SenderName    string
	RecipientID   string
	RecipientName string
	Content       string
	Color         string
}

type mockLetterSender struct {
	letters []sentLetterRecord
}

func (m *mockLetterSender) SendLetter(ctx context.Context, senderID, senderName, recipientID, recipientName, content, color string) error {
	m.letters = append(m.letters, sentLetterRecord{
		SenderID:      senderID,
		SenderName:    senderName,
		RecipientID:   recipientID,
		RecipientName: recipientName,
		Content:       content,
		Color:         color,
	})
	return nil
}

type mockCharReader struct {
	chars map[string]corecharacter.Character
}

func (m *mockCharReader) FindByID(ctx context.Context, id string) (corecharacter.Character, error) {
	if c, ok := m.chars[id]; ok {
		return c, nil
	}
	return corecharacter.Character{ID: id, Name: id, Color: "#FFFFFF"}, nil
}

func TestLookupWallpaperPrice(t *testing.T) {
	tests := []struct {
		input     string
		wantFile  string
		wantPrice int
		wantErr   error
	}{
		{"farm", "farm.gif", 1000, nil},
		{"farm.gif", "farm.gif", 1000, nil},
		{"none", "none.gif", 0, nil},
		{"stage0", "stage0.gif", 6000, nil},
		{"stage20.gif", "stage20.gif", 50000, nil},
		{"", "", 0, guild.ErrInvalidWallpaper},
		{"unknown_stage", "", 0, guild.ErrInvalidWallpaper},
	}

	for _, tt := range tests {
		gotFile, gotPrice, err := guild.LookupWallpaperPrice(tt.input)
		if !errors.Is(err, tt.wantErr) {
			t.Errorf("LookupWallpaperPrice(%q) error = %v, want %v", tt.input, err, tt.wantErr)
		}
		if err == nil {
			if gotFile != tt.wantFile {
				t.Errorf("LookupWallpaperPrice(%q) file = %q, want %q", tt.input, gotFile, tt.wantFile)
			}
			if gotPrice != tt.wantPrice {
				t.Errorf("LookupWallpaperPrice(%q) price = %d, want %d", tt.input, gotPrice, tt.wantPrice)
			}
		}
	}
}

func TestService_ApplyToJoin(t *testing.T) {
	ctx := context.Background()
	guildID := "guild-1"
	leaderID := "leader-1"
	applicantID := "applicant-1"

	t.Run("successful application sends letter to leader", func(t *testing.T) {
		letterSender := &mockLetterSender{}
		charReader := &mockCharReader{
			chars: map[string]corecharacter.Character{
				leaderID:    {ID: leaderID, Name: "LeaderName", Color: "#FF0000"},
				applicantID: {ID: applicantID, Name: "ApplicantName", Color: "#00FF00"},
			},
		}

		var addedMember guild.Member
		touched := false
		repo := &mockGuildRepo{
			getGuildByCharFn: func(ctx context.Context, characterID string) (guild.Guild, guild.Member, error) {
				return guild.Guild{}, guild.Member{}, guild.ErrCharacterNotInGuild
			},
			getGuildFn: func(ctx context.Context, gID string) (guild.Guild, []guild.Member, error) {
				return guild.Guild{ID: guildID, Name: "TestGuild", LeaderCharacterID: leaderID}, nil, nil
			},
			addMemberFn: func(ctx context.Context, m guild.Member) (guild.Member, error) {
				addedMember = m
				return m, nil
			},
			touchActiveFn: func(ctx context.Context, gID string) error {
				touched = true
				return nil
			},
		}

		svc, err := guild.NewService(repo,
			guild.WithCharacterReader(charReader),
			guild.WithLetterSender(letterSender),
		)
		if err != nil {
			t.Fatalf("NewService error: %v", err)
		}

		err = svc.ApplyToJoin(ctx, guildID, applicantID)
		if err != nil {
			t.Fatalf("ApplyToJoin unexpected error: %v", err)
		}

		if !addedMember.IsPending {
			t.Errorf("expected member.IsPending to be true")
		}
		if addedMember.Title != guild.DefaultTitlePending {
			t.Errorf("expected member.Title = %q, got %q", guild.DefaultTitlePending, addedMember.Title)
		}
		if !touched {
			t.Errorf("expected guild to be touched active")
		}

		if len(letterSender.letters) != 1 {
			t.Fatalf("expected 1 letter sent, got %d", len(letterSender.letters))
		}
		letter := letterSender.letters[0]
		if letter.RecipientID != leaderID {
			t.Errorf("expected letter recipient %q, got %q", leaderID, letter.RecipientID)
		}
		if !strings.Contains(letter.Content, "【＋参加申請＋】") || !strings.Contains(letter.Content, "ApplicantName") {
			t.Errorf("unexpected letter content: %q", letter.Content)
		}
	})

	t.Run("fails if already active member of a guild", func(t *testing.T) {
		repo := &mockGuildRepo{
			getGuildByCharFn: func(ctx context.Context, characterID string) (guild.Guild, guild.Member, error) {
				return guild.Guild{ID: "other-guild"}, guild.Member{GuildID: "other-guild", IsPending: false}, nil
			},
		}
		svc, _ := guild.NewService(repo)
		err := svc.ApplyToJoin(ctx, guildID, applicantID)
		if !errors.Is(err, guild.ErrCharacterAlreadyInGuild) {
			t.Errorf("expected ErrCharacterAlreadyInGuild, got %v", err)
		}
	})

	t.Run("fails if application is already pending", func(t *testing.T) {
		repo := &mockGuildRepo{
			getGuildByCharFn: func(ctx context.Context, characterID string) (guild.Guild, guild.Member, error) {
				return guild.Guild{ID: guildID}, guild.Member{GuildID: guildID, IsPending: true}, nil
			},
		}
		svc, _ := guild.NewService(repo)
		err := svc.ApplyToJoin(ctx, guildID, applicantID)
		if !errors.Is(err, guild.ErrApplicationAlreadyPending) {
			t.Errorf("expected ErrApplicationAlreadyPending, got %v", err)
		}
	})
}

func TestService_ApproveApplication(t *testing.T) {
	ctx := context.Background()
	guildID := "guild-1"
	leaderID := "leader-1"
	applicantID := "applicant-1"

	t.Run("successful approval sets role title and sends acceptance letter", func(t *testing.T) {
		letterSender := &mockLetterSender{}
		charReader := &mockCharReader{
			chars: map[string]corecharacter.Character{
				leaderID:    {ID: leaderID, Name: "GuildMaster", Color: "#FF0000"},
				applicantID: {ID: applicantID, Name: "Recruit", Color: "#0000FF"},
			},
		}

		approved := false
		touched := false
		repo := &mockGuildRepo{
			getGuildFn: func(ctx context.Context, gID string) (guild.Guild, []guild.Member, error) {
				return guild.Guild{ID: guildID, Name: "Legendary", LeaderCharacterID: leaderID}, []guild.Member{
					{GuildID: guildID, CharacterID: leaderID, Role: guild.RoleLeader, IsPending: false},
					{GuildID: guildID, CharacterID: applicantID, Role: guild.RoleMember, IsPending: true, Title: guild.DefaultTitlePending},
				}, nil
			},
			approveMemberFn: func(ctx context.Context, gID string, charID string, title string) error {
				if gID == guildID && charID == applicantID && title == "突撃隊長" {
					approved = true
					return nil
				}
				return errors.New("unexpected params")
			},
			touchActiveFn: func(ctx context.Context, gID string) error {
				touched = true
				return nil
			},
		}

		svc, _ := guild.NewService(repo,
			guild.WithCharacterReader(charReader),
			guild.WithLetterSender(letterSender),
		)

		err := svc.ApproveApplication(ctx, guildID, leaderID, applicantID, "突撃隊長")
		if err != nil {
			t.Fatalf("ApproveApplication error: %v", err)
		}

		if !approved {
			t.Errorf("expected repository ApproveMember to be called")
		}
		if !touched {
			t.Errorf("expected guild to be touched active")
		}

		if len(letterSender.letters) != 1 {
			t.Fatalf("expected 1 letter sent, got %d", len(letterSender.letters))
		}
		letter := letterSender.letters[0]
		if letter.RecipientID != applicantID {
			t.Errorf("expected letter recipient %q, got %q", applicantID, letter.RecipientID)
		}
		if !strings.Contains(letter.Content, "【＋参加許可証＋】") || !strings.Contains(letter.Content, "GuildMaster") {
			t.Errorf("unexpected letter content: %q", letter.Content)
		}
	})

	t.Run("fails when non-leader tries to approve", func(t *testing.T) {
		repo := &mockGuildRepo{
			getGuildFn: func(ctx context.Context, gID string) (guild.Guild, []guild.Member, error) {
				return guild.Guild{ID: guildID, LeaderCharacterID: leaderID}, nil, nil
			},
		}
		svc, _ := guild.NewService(repo)
		err := svc.ApproveApplication(ctx, guildID, "stranger", applicantID, "隊長")
		if !errors.Is(err, guild.ErrUnauthorized) {
			t.Errorf("expected ErrUnauthorized, got %v", err)
		}
	})

	t.Run("fails when target is already an active member", func(t *testing.T) {
		repo := &mockGuildRepo{
			getGuildFn: func(ctx context.Context, gID string) (guild.Guild, []guild.Member, error) {
				return guild.Guild{ID: guildID, LeaderCharacterID: leaderID}, []guild.Member{
					{GuildID: guildID, CharacterID: applicantID, IsPending: false},
				}, nil
			},
		}
		svc, _ := guild.NewService(repo)
		err := svc.ApproveApplication(ctx, guildID, leaderID, applicantID, "隊長")
		if !errors.Is(err, guild.ErrMemberNotPending) {
			t.Errorf("expected ErrMemberNotPending, got %v", err)
		}
	})
}

func TestService_RejectApplication(t *testing.T) {
	ctx := context.Background()
	guildID := "guild-1"
	leaderID := "leader-1"
	applicantID := "applicant-1"

	t.Run("successful rejection removes member and sends rejection letter", func(t *testing.T) {
		letterSender := &mockLetterSender{}
		charReader := &mockCharReader{
			chars: map[string]corecharacter.Character{
				leaderID:    {ID: leaderID, Name: "Master", Color: "#FF0000"},
				applicantID: {ID: applicantID, Name: "Applicant", Color: "#0000FF"},
			},
		}

		removed := false
		repo := &mockGuildRepo{
			getGuildFn: func(ctx context.Context, gID string) (guild.Guild, []guild.Member, error) {
				return guild.Guild{ID: guildID, Name: "Knights", LeaderCharacterID: leaderID}, []guild.Member{
					{GuildID: guildID, CharacterID: applicantID, IsPending: true},
				}, nil
			},
			removeMemberFn: func(ctx context.Context, gID string, charID string) error {
				removed = true
				return nil
			},
		}

		svc, _ := guild.NewService(repo,
			guild.WithCharacterReader(charReader),
			guild.WithLetterSender(letterSender),
		)

		err := svc.RejectApplication(ctx, guildID, leaderID, applicantID)
		if err != nil {
			t.Fatalf("RejectApplication error: %v", err)
		}

		if !removed {
			t.Errorf("expected member to be removed")
		}
		if len(letterSender.letters) != 1 {
			t.Fatalf("expected 1 letter sent, got %d", len(letterSender.letters))
		}
		letter := letterSender.letters[0]
		if letter.RecipientID != applicantID {
			t.Errorf("expected letter recipient %q, got %q", applicantID, letter.RecipientID)
		}
		if !strings.Contains(letter.Content, "【＋不合格＋】") {
			t.Errorf("unexpected letter content: %q", letter.Content)
		}
	})
}

func TestService_BroadcastCallout(t *testing.T) {
	ctx := context.Background()
	guildID := "guild-1"
	member1 := "member-1"
	member2 := "member-2"
	pendingMember := "pending-1"

	t.Run("broadcast delivers letter to active members and awards +1 GP", func(t *testing.T) {
		letterSender := &mockLetterSender{}
		charReader := &mockCharReader{
			chars: map[string]corecharacter.Character{
				member1:       {ID: member1, Name: "Hero", Color: "#FFAA00"},
				member2:       {ID: member2, Name: "Mage", Color: "#00AAFF"},
				pendingMember: {ID: pendingMember, Name: "Newbie", Color: "#FFFFFF"},
			},
		}

		pointsAdded := int64(0)
		touched := false
		repo := &mockGuildRepo{
			getGuildFn: func(ctx context.Context, gID string) (guild.Guild, []guild.Member, error) {
				return guild.Guild{ID: guildID, Name: "Fellowship"}, []guild.Member{
					{GuildID: guildID, CharacterID: member1, IsPending: false},
					{GuildID: guildID, CharacterID: member2, IsPending: false},
					{GuildID: guildID, CharacterID: pendingMember, IsPending: true},
				}, nil
			},
			addPointsFn: func(ctx context.Context, gID string, pts int64) error {
				pointsAdded += pts
				return nil
			},
			touchActiveFn: func(ctx context.Context, gID string) error {
				touched = true
				return nil
			},
		}

		svc, _ := guild.NewService(repo,
			guild.WithCharacterReader(charReader),
			guild.WithLetterSender(letterSender),
		)

		err := svc.BroadcastCallout(ctx, guildID, member1, "今夜ギルド戦に参加できる人集まって！")
		if err != nil {
			t.Fatalf("BroadcastCallout error: %v", err)
		}

		if pointsAdded != 1 {
			t.Errorf("expected +1 GP, got %d", pointsAdded)
		}
		if !touched {
			t.Errorf("expected guild to be touched active")
		}

		// Letters sent to active members (member1 and member2), pendingMember skipped
		if len(letterSender.letters) != 2 {
			t.Fatalf("expected 2 letters sent, got %d", len(letterSender.letters))
		}
		for _, l := range letterSender.letters {
			if l.RecipientID == pendingMember {
				t.Errorf("pending member should not receive member broadcast letters")
			}
			if l.Content != "今夜ギルド戦に参加できる人集まって！" {
				t.Errorf("unexpected content: %q", l.Content)
			}
		}
	})

	t.Run("fails if sender is pending applicant", func(t *testing.T) {
		repo := &mockGuildRepo{
			getGuildFn: func(ctx context.Context, gID string) (guild.Guild, []guild.Member, error) {
				return guild.Guild{ID: guildID}, []guild.Member{
					{GuildID: guildID, CharacterID: pendingMember, IsPending: true},
				}, nil
			},
		}
		svc, _ := guild.NewService(repo)
		err := svc.BroadcastCallout(ctx, guildID, pendingMember, "こんにちは！")
		if !errors.Is(err, guild.ErrUnauthorized) {
			t.Errorf("expected ErrUnauthorized, got %v", err)
		}
	})

	t.Run("fails on empty message", func(t *testing.T) {
		svc, _ := guild.NewService(&mockGuildRepo{})
		err := svc.BroadcastCallout(ctx, guildID, member1, "   ")
		if !errors.Is(err, guild.ErrEmptyCalloutMessage) {
			t.Errorf("expected ErrEmptyCalloutMessage, got %v", err)
		}
	})
}

func TestService_Customization_MarkAndWallpaper(t *testing.T) {
	ctx := context.Background()
	guildID := "guild-1"
	leaderID := "leader-1"

	t.Run("ChangeMark success with 3000G fee", func(t *testing.T) {
		markUpdated := ""
		feePaid := 0
		repo := &mockGuildRepo{
			getGuildFn: func(ctx context.Context, gID string) (guild.Guild, []guild.Member, error) {
				return guild.Guild{ID: guildID, LeaderCharacterID: leaderID}, nil, nil
			},
			updateMarkFn: func(ctx context.Context, gID string, mark string, fee int, lID string) (corecharacter.Character, error) {
				markUpdated = mark
				feePaid = fee
				return corecharacter.Character{ID: lID, Money: 7000}, nil
			},
		}

		svc, _ := guild.NewService(repo)
		char, err := svc.ChangeMark(ctx, guildID, leaderID, "42")
		if err != nil {
			t.Fatalf("ChangeMark error: %v", err)
		}
		if markUpdated != "42" {
			t.Errorf("expected mark %q, got %q", "42", markUpdated)
		}
		if feePaid != guild.MarkChangeFee {
			t.Errorf("expected fee %d, got %d", guild.MarkChangeFee, feePaid)
		}
		if char.Money != 7000 {
			t.Errorf("expected remaining money 7000, got %d", char.Money)
		}
	})

	t.Run("ChangeWallpaper success with catalog price", func(t *testing.T) {
		bgimgUpdated := ""
		feePaid := 0
		repo := &mockGuildRepo{
			getGuildFn: func(ctx context.Context, gID string) (guild.Guild, []guild.Member, error) {
				return guild.Guild{ID: guildID, LeaderCharacterID: leaderID}, nil, nil
			},
			updateWallpaperFn: func(ctx context.Context, gID string, bgimg string, fee int, lID string) (corecharacter.Character, error) {
				bgimgUpdated = bgimg
				feePaid = fee
				return corecharacter.Character{ID: lID, Money: 4000}, nil
			},
		}

		svc, _ := guild.NewService(repo)
		char, err := svc.ChangeWallpaper(ctx, guildID, leaderID, "stage0")
		if err != nil {
			t.Fatalf("ChangeWallpaper error: %v", err)
		}
		if bgimgUpdated != "stage0.gif" {
			t.Errorf("expected bgimg %q, got %q", "stage0.gif", bgimgUpdated)
		}
		if feePaid != 6000 {
			t.Errorf("expected fee 6000, got %d", feePaid)
		}
		if char.Money != 4000 {
			t.Errorf("expected remaining money 4000, got %d", char.Money)
		}
	})

	t.Run("ChangeWallpaper fails on invalid catalog entry", func(t *testing.T) {
		svc, _ := guild.NewService(&mockGuildRepo{})
		_, err := svc.ChangeWallpaper(ctx, guildID, leaderID, "nonexistent_wallpaper")
		if !errors.Is(err, guild.ErrInvalidWallpaper) {
			t.Errorf("expected ErrInvalidWallpaper, got %v", err)
		}
	})
}

func TestService_DisbandInactiveGuilds(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)

	disbandedIDs := []string{}
	repo := &mockGuildRepo{
		listInactiveGuildsFn: func(ctx context.Context, cutoff time.Time, limit int) ([]guild.Guild, error) {
			expectedCutoff := now.Add(-guild.InactivityDisbandDuration)
			if !cutoff.Equal(expectedCutoff) {
				t.Errorf("expected cutoff %v, got %v", expectedCutoff, cutoff)
			}
			return []guild.Guild{
				{ID: "dead-guild-1", Name: "GhostGuild1"},
				{ID: "dead-guild-2", Name: "GhostGuild2"},
			}, nil
		},
		disbandGuildFn: func(ctx context.Context, gID string) error {
			disbandedIDs = append(disbandedIDs, gID)
			return nil
		},
	}

	svc, _ := guild.NewService(repo)
	count, list, err := svc.DisbandInactiveGuilds(ctx, now, 50)
	if err != nil {
		t.Fatalf("DisbandInactiveGuilds error: %v", err)
	}
	if count != 2 {
		t.Errorf("expected count 2, got %d", count)
	}
	if len(list) != 2 || list[0] != "dead-guild-1" || list[1] != "dead-guild-2" {
		t.Errorf("unexpected disbanded list: %v", list)
	}
	if len(disbandedIDs) != 2 {
		t.Errorf("expected 2 repository DisbandGuild calls, got %d", len(disbandedIDs))
	}
}
