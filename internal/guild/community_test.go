package guild_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"golang.org/x/text/unicode/norm"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/guild"
	"github.com/witchcraze/party2re/internal/testutil"
	"github.com/witchcraze/party2re/internal/validation"
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
	letters      []sentLetterRecord
	sendLetterFn func(ctx context.Context, senderID, senderName, recipientID, recipientName, content, color string) error
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
	if m.sendLetterFn != nil {
		return m.sendLetterFn(ctx, senderID, senderName, recipientID, recipientName, content, color)
	}
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
					{GuildID: guildID, CharacterID: leaderID, Role: guild.RoleLeader, IsPending: false},
					{GuildID: guildID, CharacterID: applicantID, Role: guild.RoleMember, IsPending: false},
				}, nil
			},
		}
		svc, _ := guild.NewService(repo)
		err := svc.ApproveApplication(ctx, guildID, leaderID, applicantID, "隊長")
		if !errors.Is(err, guild.ErrMemberNotPending) {
			t.Errorf("expected ErrMemberNotPending, got %v", err)
		}
	})

	t.Run("persists NFC normalized role title for decomposed input", func(t *testing.T) {
		decomposed := strings.Repeat("e\u0301", 6)
		var approvedTitle string
		repo := &mockGuildRepo{
			getGuildFn: func(ctx context.Context, gID string) (guild.Guild, []guild.Member, error) {
				return guild.Guild{ID: guildID, Name: "Legendary", LeaderCharacterID: leaderID}, []guild.Member{
					{GuildID: guildID, CharacterID: leaderID, Role: guild.RoleLeader, IsPending: false},
					{GuildID: guildID, CharacterID: applicantID, Role: guild.RoleMember, IsPending: true, Title: guild.DefaultTitlePending},
				}, nil
			},
			approveMemberFn: func(ctx context.Context, gID string, charID string, title string) error {
				approvedTitle = title
				return nil
			},
			touchActiveFn: func(ctx context.Context, gID string) error {
				return nil
			},
		}

		svc, _ := guild.NewService(repo)
		err := svc.ApproveApplication(ctx, guildID, leaderID, applicantID, decomposed)
		if err != nil {
			t.Fatalf("ApproveApplication error: %v", err)
		}

		expected := norm.NFC.String(decomposed)
		if approvedTitle != expected {
			t.Errorf("approvedTitle = %q (width %d), want %q (width %d)",
				approvedTitle, guild.CalculateTitleWidth(approvedTitle),
				expected, guild.CalculateTitleWidth(expected))
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
					{GuildID: guildID, CharacterID: leaderID, Role: guild.RoleLeader},
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

	t.Run("broadcast delivers letter to all members including pending in roster order and awards +1 GP", func(t *testing.T) {
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
					{GuildID: guildID, CharacterID: pendingMember, IsPending: true},
					{GuildID: guildID, CharacterID: member2, IsPending: false},
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

		// Letters sent to all members in roster order (including pending applicant)
		if len(letterSender.letters) != 3 {
			t.Fatalf("expected 3 letters sent, got %d", len(letterSender.letters))
		}
		expectedRecipients := []string{member1, pendingMember, member2}
		for i, l := range letterSender.letters {
			if l.RecipientID != expectedRecipients[i] {
				t.Errorf("letter %d: expected recipient %q, got %q", i, expectedRecipients[i], l.RecipientID)
			}
			if l.Content != "今夜ギルド戦に参加できる人集まって！" {
				t.Errorf("unexpected content: %q", l.Content)
			}
		}
	})

	t.Run("fails if sender is pending applicant", func(t *testing.T) {
		letterSender := &mockLetterSender{}
		pointsAdded := int64(0)
		repo := &mockGuildRepo{
			getGuildFn: func(ctx context.Context, gID string) (guild.Guild, []guild.Member, error) {
				return guild.Guild{ID: guildID}, []guild.Member{
					{GuildID: guildID, CharacterID: pendingMember, IsPending: true},
				}, nil
			},
			addPointsFn: func(ctx context.Context, gID string, pts int64) error {
				pointsAdded += pts
				return nil
			},
		}
		svc, _ := guild.NewService(repo, guild.WithLetterSender(letterSender))
		err := svc.BroadcastCallout(ctx, guildID, pendingMember, "こんにちは！")
		if !errors.Is(err, guild.ErrUnauthorized) {
			t.Errorf("expected ErrUnauthorized, got %v", err)
		}
		if len(letterSender.letters) != 0 {
			t.Errorf("expected 0 letters sent by pending sender, got %d", len(letterSender.letters))
		}
		if pointsAdded != 0 {
			t.Errorf("expected 0 points added by pending sender, got %d", pointsAdded)
		}
	})

	t.Run("fails on empty message", func(t *testing.T) {
		svc, _ := guild.NewService(&mockGuildRepo{})
		err := svc.BroadcastCallout(ctx, guildID, member1, "   ")
		if !errors.Is(err, guild.ErrEmptyCalloutMessage) {
			t.Errorf("expected ErrEmptyCalloutMessage, got %v", err)
		}
	})

	t.Run("fails on message too long", func(t *testing.T) {
		svc, _ := guild.NewService(&mockGuildRepo{})
		err := svc.BroadcastCallout(ctx, guildID, member1, strings.Repeat("あ", guild.MaxNoticeLength+1))
		if !errors.Is(err, guild.ErrCalloutMessageTooLong) {
			t.Errorf("expected ErrCalloutMessageTooLong, got %v", err)
		}
	})

	t.Run("fails on control character", func(t *testing.T) {
		svc, _ := guild.NewService(&mockGuildRepo{})
		err := svc.BroadcastCallout(ctx, guildID, member1, "Hello\x00World")
		if !errors.Is(err, validation.ErrControlCharacter) {
			t.Errorf("expected ErrControlCharacter, got %v", err)
		}
	})

	t.Run("fails on zero-width space", func(t *testing.T) {
		svc, _ := guild.NewService(&mockGuildRepo{})
		err := svc.BroadcastCallout(ctx, guildID, member1, "Hello\u200BWorld")
		if !errors.Is(err, validation.ErrZeroWidth) {
			t.Errorf("expected ErrZeroWidth, got %v", err)
		}
	})

	t.Run("fails on bidi override", func(t *testing.T) {
		svc, _ := guild.NewService(&mockGuildRepo{})
		err := svc.BroadcastCallout(ctx, guildID, member1, "Hello\u202EWorld")
		if !errors.Is(err, validation.ErrBidiOverride) {
			t.Errorf("expected ErrBidiOverride, got %v", err)
		}
	})

	t.Run("fails on zalgo text", func(t *testing.T) {
		svc, _ := guild.NewService(&mockGuildRepo{})
		err := svc.BroadcastCallout(ctx, guildID, member1, "Z\u0300\u0301\u0302algo")
		if !errors.Is(err, validation.ErrZalgo) {
			t.Errorf("expected ErrZalgo, got %v", err)
		}
	})

	t.Run("delivers sanitized NFC message to members", func(t *testing.T) {
		letterSender := &mockLetterSender{}
		repo := &mockGuildRepo{
			getGuildFn: func(ctx context.Context, gID string) (guild.Guild, []guild.Member, error) {
				return guild.Guild{ID: guildID, Name: "騎士団"}, []guild.Member{
					{GuildID: guildID, CharacterID: member1, Role: guild.RoleMember},
				}, nil
			},
			addPointsFn: func(ctx context.Context, gID string, pts int64) error {
				return nil
			},
		}
		svc, _ := guild.NewService(repo, guild.WithLetterSender(letterSender))
		// "Ka\u0301llout" is NFD ("Ká" decomposed); should be normalized to NFC "Kállout"
		err := svc.BroadcastCallout(ctx, guildID, member1, "Ka\u0301llout")
		if err != nil {
			t.Fatalf("BroadcastCallout error: %v", err)
		}
		if len(letterSender.letters) != 1 {
			t.Fatalf("expected 1 letter sent, got %d", len(letterSender.letters))
		}
		if letterSender.letters[0].Content != "Kállout" {
			t.Errorf("expected NFC normalized 'Kállout', got %q", letterSender.letters[0].Content)
		}
	})

	t.Run("first recipient delivery failure returns error and skips point award and touch active", func(t *testing.T) {
		deliveryErr := errors.New("mail storage unavailable")
		letterSender := &mockLetterSender{
			sendLetterFn: func(ctx context.Context, senderID, senderName, recipientID, recipientName, content, color string) error {
				return deliveryErr
			},
		}
		pointsAdded := int64(0)
		touched := false
		repo := &mockGuildRepo{
			getGuildFn: func(ctx context.Context, gID string) (guild.Guild, []guild.Member, error) {
				return guild.Guild{ID: guildID, Name: "Fellowship"}, []guild.Member{
					{GuildID: guildID, CharacterID: member1, IsPending: false},
					{GuildID: guildID, CharacterID: member2, IsPending: false},
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

		svc, _ := guild.NewService(repo, guild.WithLetterSender(letterSender))
		err := svc.BroadcastCallout(ctx, guildID, member1, "こんにちは")
		if !errors.Is(err, deliveryErr) {
			t.Fatalf("expected error %v, got %v", deliveryErr, err)
		}
		if pointsAdded != 0 {
			t.Errorf("expected 0 points added on delivery failure, got %d", pointsAdded)
		}
		if touched {
			t.Errorf("expected touchActive not to be called on delivery failure")
		}
		if len(letterSender.letters) != 1 {
			t.Errorf("expected 1 delivery attempt before failure, got %d", len(letterSender.letters))
		}
	})

	t.Run("later recipient delivery failure returns error, preserves earlier delivery, skips point award", func(t *testing.T) {
		deliveryErr := errors.New("recipient mailbox full")
		letterSender := &mockLetterSender{
			sendLetterFn: func(ctx context.Context, senderID, senderName, recipientID, recipientName, content, color string) error {
				if recipientID == member2 {
					return deliveryErr
				}
				return nil
			},
		}
		pointsAdded := int64(0)
		touched := false
		repo := &mockGuildRepo{
			getGuildFn: func(ctx context.Context, gID string) (guild.Guild, []guild.Member, error) {
				return guild.Guild{ID: guildID, Name: "Fellowship"}, []guild.Member{
					{GuildID: guildID, CharacterID: member1, IsPending: false},
					{GuildID: guildID, CharacterID: member2, IsPending: false},
					{GuildID: guildID, CharacterID: "member-3", IsPending: false},
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

		svc, _ := guild.NewService(repo, guild.WithLetterSender(letterSender))
		err := svc.BroadcastCallout(ctx, guildID, member1, "こんにちは")
		if !errors.Is(err, deliveryErr) {
			t.Fatalf("expected error %v, got %v", deliveryErr, err)
		}
		if pointsAdded != 0 {
			t.Errorf("expected 0 points added on delivery failure, got %d", pointsAdded)
		}
		if touched {
			t.Errorf("expected touchActive not to be called on delivery failure")
		}
		if len(letterSender.letters) != 2 {
			t.Errorf("expected 2 delivery attempts before stopping, got %d", len(letterSender.letters))
		}
		if letterSender.letters[0].RecipientID != member1 || letterSender.letters[1].RecipientID != member2 {
			t.Errorf("unexpected delivery order: %+v", letterSender.letters)
		}
	})

	t.Run("pending recipient delivery failure returns error, preserves earlier delivery, skips point award", func(t *testing.T) {
		deliveryErr := errors.New("mail storage unavailable for pending recipient")
		letterSender := &mockLetterSender{
			sendLetterFn: func(ctx context.Context, senderID, senderName, recipientID, recipientName, content, color string) error {
				if recipientID == pendingMember {
					return deliveryErr
				}
				return nil
			},
		}
		pointsAdded := int64(0)
		touched := false
		repo := &mockGuildRepo{
			getGuildFn: func(ctx context.Context, gID string) (guild.Guild, []guild.Member, error) {
				return guild.Guild{ID: guildID, Name: "Fellowship"}, []guild.Member{
					{GuildID: guildID, CharacterID: member1, IsPending: false},
					{GuildID: guildID, CharacterID: pendingMember, IsPending: true},
					{GuildID: guildID, CharacterID: member2, IsPending: false},
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

		svc, _ := guild.NewService(repo, guild.WithLetterSender(letterSender))
		err := svc.BroadcastCallout(ctx, guildID, member1, "こんにちは")
		if !errors.Is(err, deliveryErr) {
			t.Fatalf("expected error %v, got %v", deliveryErr, err)
		}
		if pointsAdded != 0 {
			t.Errorf("expected 0 points added on delivery failure, got %d", pointsAdded)
		}
		if touched {
			t.Errorf("expected touchActive not to be called on delivery failure")
		}
		if len(letterSender.letters) != 2 {
			t.Errorf("expected 2 delivery attempts before stopping, got %d", len(letterSender.letters))
		}
		if len(letterSender.letters) >= 2 {
			if letterSender.letters[0].RecipientID != member1 || letterSender.letters[1].RecipientID != pendingMember {
				t.Errorf("unexpected delivery attempts: %+v", letterSender.letters)
			}
		}
	})
}

type postReviewFailLetter struct{}

func (postReviewFailLetter) SendLetter(context.Context, string, string, string, string, string, string) error {
	return errors.New("mail storage unavailable")
}

func TestPostReviewBroadcastFailure(t *testing.T) {
	points := 0
	repo := &mockGuildRepo{
		getGuildFn: func(context.Context, string) (guild.Guild, []guild.Member, error) {
			return guild.Guild{ID: "g"}, []guild.Member{
				{CharacterID: "leader", Role: guild.RoleLeader},
				{CharacterID: "member", Role: guild.RoleMember},
			}, nil
		},
		addPointsFn: func(context.Context, string, int64) error {
			points++
			return nil
		},
	}
	svc, err := guild.NewService(repo, guild.WithLetterSender(postReviewFailLetter{}))
	if err != nil {
		t.Fatal(err)
	}
	err = svc.BroadcastCallout(context.Background(), "g", "leader", "callout")
	if err == nil {
		t.Error("broadcast reports success after every letter delivery failed")
	}
	if points != 0 {
		t.Errorf("awarded activity points after failed delivery: %d", points)
	}
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

func TestService_RemoveCharacterFromGuild(t *testing.T) {
	ctx := context.Background()

	t.Run("empty id", func(t *testing.T) {
		svc, _ := guild.NewService(&mockGuildRepo{})
		if err := svc.RemoveCharacterFromGuild(ctx, "  "); err != nil {
			t.Errorf("expected nil error for empty ID, got %v", err)
		}
	})

	t.Run("not in guild", func(t *testing.T) {
		repo := &mockGuildRepo{
			getGuildByCharFn: func(ctx context.Context, characterID string) (guild.Guild, guild.Member, error) {
				return guild.Guild{}, guild.Member{}, guild.ErrCharacterNotInGuild
			},
		}
		svc, _ := guild.NewService(repo)
		if err := svc.RemoveCharacterFromGuild(ctx, "c1"); err != nil {
			t.Errorf("expected nil error when character not in guild, got %v", err)
		}
	})

	t.Run("sole member disbands guild and publishes news", func(t *testing.T) {
		var disbanded bool
		repo := &mockGuildRepo{
			getGuildByCharFn: func(ctx context.Context, characterID string) (guild.Guild, guild.Member, error) {
				return guild.Guild{ID: "g1", Name: "ひとりギルド"}, guild.Member{CharacterID: characterID}, nil
			},
			getGuildFn: func(ctx context.Context, guildID string) (guild.Guild, []guild.Member, error) {
				return guild.Guild{ID: "g1", Name: "ひとりギルド"}, []guild.Member{{CharacterID: "c1"}}, nil
			},
			disbandGuildFn: func(ctx context.Context, gID string) error {
				disbanded = true
				return nil
			},
		}
		news := &mockNewsPublisher{}
		svc, _ := guild.NewService(repo, guild.WithNewsPublisher(news))
		if err := svc.RemoveCharacterFromGuild(ctx, "c1"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !disbanded {
			t.Errorf("expected guild to be disbanded")
		}
		if len(news.calls) != 1 {
			t.Fatalf("expected 1 news publication, got %d", len(news.calls))
		}
		expectedTitle := "ギルド『ひとりギルド』が解散しました"
		if news.calls[0].Title != expectedTitle {
			t.Errorf("news Title = %s, want %s", news.calls[0].Title, expectedTitle)
		}
		if news.calls[0].Author != "System" {
			t.Errorf("news Author = %s, want System", news.calls[0].Author)
		}
	})

	t.Run("regular member removed", func(t *testing.T) {
		var removedID string
		repo := &mockGuildRepo{
			getGuildByCharFn: func(ctx context.Context, characterID string) (guild.Guild, guild.Member, error) {
				return guild.Guild{ID: "g1"}, guild.Member{CharacterID: characterID, Role: guild.RoleMember}, nil
			},
			getGuildFn: func(ctx context.Context, guildID string) (guild.Guild, []guild.Member, error) {
				return guild.Guild{ID: "g1"}, []guild.Member{
					{CharacterID: "leader", Role: guild.RoleLeader},
					{CharacterID: "c2", Role: guild.RoleMember},
				}, nil
			},
			removeMemberFn: func(ctx context.Context, guildID string, characterID string) error {
				removedID = characterID
				return nil
			},
		}
		svc, _ := guild.NewService(repo)
		if err := svc.RemoveCharacterFromGuild(ctx, "c2"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if removedID != "c2" {
			t.Errorf("expected c2 to be removed, got %v", removedID)
		}
	})

	t.Run("leader transfers leadership to successor with title", func(t *testing.T) {
		var transferredTo string
		repo := &mockGuildRepo{
			getGuildByCharFn: func(ctx context.Context, characterID string) (guild.Guild, guild.Member, error) {
				return guild.Guild{ID: "g1"}, guild.Member{CharacterID: characterID, Role: guild.RoleLeader}, nil
			},
			getGuildFn: func(ctx context.Context, guildID string) (guild.Guild, []guild.Member, error) {
				return guild.Guild{ID: "g1"}, []guild.Member{
					{CharacterID: "leader", Role: guild.RoleLeader},
					{CharacterID: "mem1", Role: guild.RoleMember, Title: "Member"},
					{CharacterID: "mem2", Role: guild.RoleMember, Title: "副ギルマス"}, // Sub-leader
				}, nil
			},
			transferLeadershipFn: func(ctx context.Context, guildID string, oldLeader, newLeader string) error {
				transferredTo = newLeader
				return nil
			},
			removeMemberFn: func(ctx context.Context, guildID string, characterID string) error {
				return nil
			},
		}
		svc, _ := guild.NewService(repo)
		if err := svc.RemoveCharacterFromGuild(ctx, "leader"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if transferredTo != "mem2" {
			t.Errorf("expected leadership transferred to mem2, got %v", transferredTo)
		}
	})

	t.Run("leader transfers leadership to first available member", func(t *testing.T) {
		var transferredTo string
		repo := &mockGuildRepo{
			getGuildByCharFn: func(ctx context.Context, characterID string) (guild.Guild, guild.Member, error) {
				return guild.Guild{ID: "g1"}, guild.Member{CharacterID: characterID, Role: guild.RoleLeader}, nil
			},
			getGuildFn: func(ctx context.Context, guildID string) (guild.Guild, []guild.Member, error) {
				return guild.Guild{ID: "g1"}, []guild.Member{
					{CharacterID: "leader", Role: guild.RoleLeader},
					{CharacterID: "mem1", Role: guild.RoleMember, Title: "Member"},
				}, nil
			},
			transferLeadershipFn: func(ctx context.Context, guildID string, oldLeader, newLeader string) error {
				transferredTo = newLeader
				return nil
			},
			removeMemberFn: func(ctx context.Context, guildID string, characterID string) error {
				return nil
			},
		}
		svc, _ := guild.NewService(repo)
		if err := svc.RemoveCharacterFromGuild(ctx, "leader"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if transferredTo != "mem1" {
			t.Errorf("expected leadership transferred to mem1, got %v", transferredTo)
		}
	})

	t.Run("leader leaves with only pending applicants transfers leadership to applicant and preserves guild", func(t *testing.T) {
		var disbanded bool
		var transferredTo string
		var removedChar string
		repo := &mockGuildRepo{
			getGuildByCharFn: func(ctx context.Context, characterID string) (guild.Guild, guild.Member, error) {
				return guild.Guild{ID: "g1", Name: "申請者のみギルド"}, guild.Member{CharacterID: characterID, Role: guild.RoleLeader}, nil
			},
			getGuildFn: func(ctx context.Context, guildID string) (guild.Guild, []guild.Member, error) {
				return guild.Guild{ID: "g1", Name: "申請者のみギルド"}, []guild.Member{
					{CharacterID: "leader", Role: guild.RoleLeader, IsPending: false},
					{CharacterID: "applicant1", Role: guild.RoleMember, Title: "参加申請中", IsPending: true},
				}, nil
			},
			transferLeadershipFn: func(ctx context.Context, guildID string, oldLeader, newLeader string) error {
				transferredTo = newLeader
				return nil
			},
			removeMemberFn: func(ctx context.Context, guildID string, characterID string) error {
				removedChar = characterID
				return nil
			},
			disbandGuildFn: func(ctx context.Context, gID string) error {
				disbanded = true
				return nil
			},
		}
		news := &mockNewsPublisher{}
		svc, _ := guild.NewService(repo, guild.WithNewsPublisher(news))
		if err := svc.RemoveCharacterFromGuild(ctx, "leader"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if transferredTo != "applicant1" {
			t.Errorf("expected leadership transferred to applicant1, got %v", transferredTo)
		}
		if disbanded {
			t.Errorf("expected guild to be preserved, but was disbanded")
		}
		if removedChar != "leader" {
			t.Errorf("expected leader to be removed, got %v", removedChar)
		}
		if len(news.calls) != 0 {
			t.Fatalf("expected 0 news publications, got %d", len(news.calls))
		}
	})

	t.Run("leader leaves with mixed active members and pending applicants transfers to earlier pending when no preferred title", func(t *testing.T) {
		var transferredTo string
		var removedChar string
		repo := &mockGuildRepo{
			getGuildByCharFn: func(ctx context.Context, characterID string) (guild.Guild, guild.Member, error) {
				return guild.Guild{ID: "g1"}, guild.Member{CharacterID: characterID, Role: guild.RoleLeader}, nil
			},
			getGuildFn: func(ctx context.Context, guildID string) (guild.Guild, []guild.Member, error) {
				return guild.Guild{ID: "g1"}, []guild.Member{
					{CharacterID: "leader", Role: guild.RoleLeader, IsPending: false},
					{CharacterID: "applicant_first", Role: guild.RoleMember, Title: "参加申請中", IsPending: true},
					{CharacterID: "mem1", Role: guild.RoleMember, Title: "一般", IsPending: false},
				}, nil
			},
			transferLeadershipFn: func(ctx context.Context, guildID string, oldLeader, newLeader string) error {
				transferredTo = newLeader
				return nil
			},
			removeMemberFn: func(ctx context.Context, guildID string, characterID string) error {
				removedChar = characterID
				return nil
			},
		}
		svc, _ := guild.NewService(repo)
		if err := svc.RemoveCharacterFromGuild(ctx, "leader"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if transferredTo != "applicant_first" {
			t.Errorf("expected leadership transferred to first remaining row applicant_first, got %v", transferredTo)
		}
		if removedChar != "leader" {
			t.Errorf("expected leader to be removed, got %v", removedChar)
		}
	})

	t.Run("leader leaves with mixed active members and pending applicants prefers row with preferred title regardless of pending", func(t *testing.T) {
		var transferredTo string
		repo := &mockGuildRepo{
			getGuildByCharFn: func(ctx context.Context, characterID string) (guild.Guild, guild.Member, error) {
				return guild.Guild{ID: "g1"}, guild.Member{CharacterID: characterID, Role: guild.RoleLeader}, nil
			},
			getGuildFn: func(ctx context.Context, guildID string) (guild.Guild, []guild.Member, error) {
				return guild.Guild{ID: "g1"}, []guild.Member{
					{CharacterID: "leader", Role: guild.RoleLeader, IsPending: false},
					{CharacterID: "applicant_first", Role: guild.RoleMember, Title: "参加申請中", IsPending: true},
					{CharacterID: "applicant_master", Role: guild.RoleMember, Title: "ギルマス志望", IsPending: true},
					{CharacterID: "mem1", Role: guild.RoleMember, Title: "一般", IsPending: false},
				}, nil
			},
			transferLeadershipFn: func(ctx context.Context, guildID string, oldLeader, newLeader string) error {
				transferredTo = newLeader
				return nil
			},
			removeMemberFn: func(ctx context.Context, guildID string, characterID string) error {
				return nil
			},
		}
		svc, _ := guild.NewService(repo)
		if err := svc.RemoveCharacterFromGuild(ctx, "leader"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if transferredTo != "applicant_master" {
			t.Errorf("expected leadership transferred to applicant_master with title, got %v", transferredTo)
		}
	})

	t.Run("leader leaves with multiple preferred title rows selects first matching in roster order", func(t *testing.T) {
		var transferredTo string
		repo := &mockGuildRepo{
			getGuildByCharFn: func(ctx context.Context, characterID string) (guild.Guild, guild.Member, error) {
				return guild.Guild{ID: "g1"}, guild.Member{CharacterID: characterID, Role: guild.RoleLeader}, nil
			},
			getGuildFn: func(ctx context.Context, guildID string) (guild.Guild, []guild.Member, error) {
				return guild.Guild{ID: "g1"}, []guild.Member{
					{CharacterID: "leader", Role: guild.RoleLeader, IsPending: false},
					{CharacterID: "applicant_sub", Role: guild.RoleMember, Title: "副ギルマス", IsPending: true},
					{CharacterID: "mem_sub", Role: guild.RoleMember, Title: "ギルマス代行", IsPending: false},
				}, nil
			},
			transferLeadershipFn: func(ctx context.Context, guildID string, oldLeader, newLeader string) error {
				transferredTo = newLeader
				return nil
			},
			removeMemberFn: func(ctx context.Context, guildID string, characterID string) error {
				return nil
			},
		}
		svc, _ := guild.NewService(repo)
		if err := svc.RemoveCharacterFromGuild(ctx, "leader"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if transferredTo != "applicant_sub" {
			t.Errorf("expected leadership transferred to first matching preferred title row applicant_sub, got %v", transferredTo)
		}
	})

	t.Run("leader leaves with multiple preferred title rows selects first active if active is first", func(t *testing.T) {
		var transferredTo string
		repo := &mockGuildRepo{
			getGuildByCharFn: func(ctx context.Context, characterID string) (guild.Guild, guild.Member, error) {
				return guild.Guild{ID: "g1"}, guild.Member{CharacterID: characterID, Role: guild.RoleLeader}, nil
			},
			getGuildFn: func(ctx context.Context, guildID string) (guild.Guild, []guild.Member, error) {
				return guild.Guild{ID: "g1"}, []guild.Member{
					{CharacterID: "leader", Role: guild.RoleLeader, IsPending: false},
					{CharacterID: "mem_sub", Role: guild.RoleMember, Title: "ギルマス代行", IsPending: false},
					{CharacterID: "applicant_sub", Role: guild.RoleMember, Title: "副ギルマス", IsPending: true},
				}, nil
			},
			transferLeadershipFn: func(ctx context.Context, guildID string, oldLeader, newLeader string) error {
				transferredTo = newLeader
				return nil
			},
			removeMemberFn: func(ctx context.Context, guildID string, characterID string) error {
				return nil
			},
		}
		svc, _ := guild.NewService(repo)
		if err := svc.RemoveCharacterFromGuild(ctx, "leader"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if transferredTo != "mem_sub" {
			t.Errorf("expected leadership transferred to mem_sub, got %v", transferredTo)
		}
	})

	t.Run("pending applicant departure does not disband guild when leader remains", func(t *testing.T) {
		var disbanded bool
		var removedChar string
		repo := &mockGuildRepo{
			getGuildByCharFn: func(ctx context.Context, characterID string) (guild.Guild, guild.Member, error) {
				return guild.Guild{ID: "g1"}, guild.Member{CharacterID: characterID, Role: guild.RoleMember, IsPending: true}, nil
			},
			getGuildFn: func(ctx context.Context, guildID string) (guild.Guild, []guild.Member, error) {
				return guild.Guild{ID: "g1"}, []guild.Member{
					{CharacterID: "leader", Role: guild.RoleLeader, IsPending: false},
					{CharacterID: "applicant1", Role: guild.RoleMember, Title: "参加申請中", IsPending: true},
				}, nil
			},
			removeMemberFn: func(ctx context.Context, guildID string, characterID string) error {
				removedChar = characterID
				return nil
			},
			disbandGuildFn: func(ctx context.Context, gID string) error {
				disbanded = true
				return nil
			},
		}
		svc, _ := guild.NewService(repo)
		if err := svc.RemoveCharacterFromGuild(ctx, "applicant1"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if disbanded {
			t.Errorf("expected guild not to be disbanded when applicant is removed and leader remains")
		}
		if removedChar != "applicant1" {
			t.Errorf("expected applicant1 to be removed, got %v", removedChar)
		}
	})

	t.Run("Leave: leader departure with only pending applicants transfers leadership to applicant and preserves guild", func(t *testing.T) {
		var disbanded bool
		var transferredTo string
		var removedChar string
		repo := &mockGuildRepo{
			getGuildFn: func(ctx context.Context, guildID string) (guild.Guild, []guild.Member, error) {
				return guild.Guild{ID: "g1", Name: "申請者のみギルド"}, []guild.Member{
					{CharacterID: "leader", Role: guild.RoleLeader, IsPending: false},
					{CharacterID: "applicant1", Role: guild.RoleMember, Title: "参加申請中", IsPending: true},
				}, nil
			},
			transferLeadershipFn: func(ctx context.Context, guildID string, oldLeader, newLeader string) error {
				transferredTo = newLeader
				return nil
			},
			removeMemberFn: func(ctx context.Context, guildID string, characterID string) error {
				removedChar = characterID
				return nil
			},
			disbandGuildFn: func(ctx context.Context, gID string) error {
				disbanded = true
				return nil
			},
		}
		news := &mockNewsPublisher{}
		svc, _ := guild.NewService(repo, guild.WithNewsPublisher(news))
		if err := svc.Leave(ctx, "g1", "leader"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if transferredTo != "applicant1" {
			t.Errorf("expected leadership transfer to applicant1, got %v", transferredTo)
		}
		if disbanded {
			t.Errorf("expected guild not to be disbanded when applicant remains")
		}
		if removedChar != "leader" {
			t.Errorf("expected leader to be removed, got %v", removedChar)
		}
		if len(news.calls) != 0 {
			t.Fatalf("expected 0 news publications, got %d", len(news.calls))
		}
	})

	t.Run("Leave: sole member departure disbands guild and publishes news", func(t *testing.T) {
		var disbanded bool
		repo := &mockGuildRepo{
			getGuildFn: func(ctx context.Context, guildID string) (guild.Guild, []guild.Member, error) {
				return guild.Guild{ID: "g1", Name: "ひとりギルド"}, []guild.Member{
					{CharacterID: "leader", Role: guild.RoleLeader, IsPending: false},
				}, nil
			},
			disbandGuildFn: func(ctx context.Context, gID string) error {
				disbanded = true
				return nil
			},
		}
		news := &mockNewsPublisher{}
		svc, _ := guild.NewService(repo, guild.WithNewsPublisher(news))
		if err := svc.Leave(ctx, "g1", "leader"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !disbanded {
			t.Errorf("expected sole member departure to disband guild")
		}
		if len(news.calls) != 1 {
			t.Fatalf("expected 1 news publication, got %d", len(news.calls))
		}
		expectedTitle := "ギルド『ひとりギルド』が解散しました"
		if news.calls[0].Title != expectedTitle {
			t.Errorf("news Title = %s, want %s", news.calls[0].Title, expectedTitle)
		}
	})

	t.Run("RemoveCharacterFromGuild: sole member departure disbands guild and publishes news", func(t *testing.T) {
		var disbanded bool
		repo := &mockGuildRepo{
			getGuildByCharFn: func(ctx context.Context, characterID string) (guild.Guild, guild.Member, error) {
				return guild.Guild{ID: "g1", Name: "ひとりギルド"}, guild.Member{CharacterID: characterID, Role: guild.RoleLeader}, nil
			},
			getGuildFn: func(ctx context.Context, guildID string) (guild.Guild, []guild.Member, error) {
				return guild.Guild{ID: "g1", Name: "ひとりギルド"}, []guild.Member{
					{CharacterID: "leader", Role: guild.RoleLeader, IsPending: false},
				}, nil
			},
			disbandGuildFn: func(ctx context.Context, gID string) error {
				disbanded = true
				return nil
			},
		}
		news := &mockNewsPublisher{}
		svc, _ := guild.NewService(repo, guild.WithNewsPublisher(news))
		if err := svc.RemoveCharacterFromGuild(ctx, "leader"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !disbanded {
			t.Errorf("expected sole member departure to disband guild")
		}
		if len(news.calls) != 1 {
			t.Fatalf("expected 1 news publication, got %d", len(news.calls))
		}
	})

	t.Run("leader departure: transfer leadership failure propagates without removing leader", func(t *testing.T) {
		var removed bool
		transferErr := errors.New("db error in transfer")
		repo := &mockGuildRepo{
			getGuildFn: func(ctx context.Context, guildID string) (guild.Guild, []guild.Member, error) {
				return guild.Guild{ID: "g1"}, []guild.Member{
					{CharacterID: "leader", Role: guild.RoleLeader},
					{CharacterID: "applicant1", Role: guild.RoleMember, IsPending: true},
				}, nil
			},
			transferLeadershipFn: func(ctx context.Context, guildID string, oldLeader, newLeader string) error {
				return transferErr
			},
			removeMemberFn: func(ctx context.Context, guildID string, characterID string) error {
				removed = true
				return nil
			},
		}
		svc, _ := guild.NewService(repo)
		err := svc.Leave(ctx, "g1", "leader")
		if !errors.Is(err, transferErr) {
			t.Fatalf("expected error %v, got %v", transferErr, err)
		}
		if removed {
			t.Errorf("leader should not be removed if transfer leadership fails")
		}
	})

	t.Run("leader departure: remove member failure propagates", func(t *testing.T) {
		removeErr := errors.New("db error in remove")
		repo := &mockGuildRepo{
			getGuildFn: func(ctx context.Context, guildID string) (guild.Guild, []guild.Member, error) {
				return guild.Guild{ID: "g1"}, []guild.Member{
					{CharacterID: "leader", Role: guild.RoleLeader},
					{CharacterID: "applicant1", Role: guild.RoleMember, IsPending: true},
				}, nil
			},
			transferLeadershipFn: func(ctx context.Context, guildID string, oldLeader, newLeader string) error {
				return nil
			},
			removeMemberFn: func(ctx context.Context, guildID string, characterID string) error {
				return removeErr
			},
		}
		svc, _ := guild.NewService(repo)
		err := svc.Leave(ctx, "g1", "leader")
		if !errors.Is(err, removeErr) {
			t.Fatalf("expected error %v, got %v", removeErr, err)
		}
	})

	t.Run("leader departure: disband failure propagates and suppresses news", func(t *testing.T) {
		disbandErr := errors.New("db error in disband")
		repo := &mockGuildRepo{
			getGuildFn: func(ctx context.Context, guildID string) (guild.Guild, []guild.Member, error) {
				return guild.Guild{ID: "g1", Name: "ひとりギルド"}, []guild.Member{
					{CharacterID: "leader", Role: guild.RoleLeader},
				}, nil
			},
			disbandGuildFn: func(ctx context.Context, gID string) error {
				return disbandErr
			},
		}
		news := &mockNewsPublisher{}
		svc, _ := guild.NewService(repo, guild.WithNewsPublisher(news))
		err := svc.Leave(ctx, "g1", "leader")
		if !errors.Is(err, disbandErr) {
			t.Fatalf("expected error %v, got %v", disbandErr, err)
		}
		if len(news.calls) != 0 {
			t.Fatalf("news should not be published if disband fails, got %d calls", len(news.calls))
		}
	})

	t.Run("concurrency: paired concurrent leave calls run without panic", func(t *testing.T) {
		repo := &mockGuildRepo{
			getGuildFn: func(ctx context.Context, guildID string) (guild.Guild, []guild.Member, error) {
				return guild.Guild{ID: "g1"}, []guild.Member{
					{CharacterID: "leader", Role: guild.RoleLeader, IsPending: false},
					{CharacterID: "applicant1", Role: guild.RoleMember, IsPending: true},
				}, nil
			},
			transferLeadershipFn: func(ctx context.Context, guildID string, oldLeader, newLeader string) error {
				return nil
			},
			removeMemberFn: func(ctx context.Context, guildID string, characterID string) error {
				return nil
			},
		}
		svc, _ := guild.NewService(repo)
		err1, err2 := testutil.RunRace2(
			func() error {
				return svc.Leave(ctx, "g1", "leader")
			},
			func() error {
				return svc.Leave(ctx, "g1", "applicant1")
			},
		)
		if err1 != nil {
			t.Errorf("unexpected error on leader leave: %v", err1)
		}
		if err2 != nil {
			t.Errorf("unexpected error on applicant leave: %v", err2)
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
		getGuildForUpdateFn: func(ctx context.Context, gID string) (guild.Guild, []guild.Member, error) {
			if gID == "dead-guild-1" {
				return guild.Guild{ID: "dead-guild-1", Name: "GhostGuild1", LastActiveAt: now.Add(-guild.InactivityDisbandDuration - time.Hour)}, nil, nil
			}
			if gID == "dead-guild-2" {
				return guild.Guild{ID: "dead-guild-2", Name: "GhostGuild2", LastActiveAt: now.Add(-guild.InactivityDisbandDuration - 2*time.Hour)}, nil, nil
			}
			return guild.Guild{}, nil, guild.ErrGuildNotFound
		},
		disbandGuildFn: func(ctx context.Context, gID string) error {
			disbandedIDs = append(disbandedIDs, gID)
			return nil
		},
	}

	news := &mockNewsPublisher{}
	svc, _ := guild.NewService(repo, guild.WithNewsPublisher(news))
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
	if len(news.calls) != 2 {
		t.Fatalf("expected 2 news publications, got %d", len(news.calls))
	}
	expectedTitle1 := "ギルド『GhostGuild1』が解散しました"
	expectedTitle2 := "ギルド『GhostGuild2』が解散しました"
	if news.calls[0].Title != expectedTitle1 || news.calls[1].Title != expectedTitle2 {
		t.Errorf("news titles = [%s, %s], want [%s, %s]", news.calls[0].Title, news.calls[1].Title, expectedTitle1, expectedTitle2)
	}
	if !news.calls[0].PublishedAt.Equal(now) || !news.calls[1].PublishedAt.Equal(now) {
		t.Errorf("news PublishedAt expected %v", now)
	}
}

func TestService_DisbandInactiveGuilds_ReactivatedGuildPreserved(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)

	disbandedIDs := []string{}
	repo := &mockGuildRepo{
		listInactiveGuildsFn: func(ctx context.Context, cutoff time.Time, limit int) ([]guild.Guild, error) {
			// Candidate extracted because it was inactive when scanned
			return []guild.Guild{
				{ID: "reactivated-guild-1", Name: "AliveGuild"},
			}, nil
		},
		getGuildForUpdateFn: func(ctx context.Context, gID string) (guild.Guild, []guild.Member, error) {
			// But rechecked under lock, guild was touched and is now active!
			return guild.Guild{
				ID:           "reactivated-guild-1",
				Name:         "AliveGuild",
				LastActiveAt: now, // recent activity >= cutoff
			}, nil, nil
		},
		disbandGuildFn: func(ctx context.Context, gID string) error {
			disbandedIDs = append(disbandedIDs, gID)
			return nil
		},
	}

	news := &mockNewsPublisher{}
	svc, _ := guild.NewService(repo, guild.WithNewsPublisher(news))
	count, list, err := svc.DisbandInactiveGuilds(ctx, now, 50)
	if err != nil {
		t.Fatalf("DisbandInactiveGuilds error: %v", err)
	}
	if count != 0 {
		t.Errorf("expected count 0 for reactivated guild, got %d", count)
	}
	if len(list) != 0 {
		t.Errorf("expected empty disbanded list, got %v", list)
	}
	if len(disbandedIDs) != 0 {
		t.Errorf("expected 0 DisbandGuild calls for reactivated guild, got %d", len(disbandedIDs))
	}
	if len(news.calls) != 0 {
		t.Errorf("expected 0 news publications for reactivated guild, got %d", len(news.calls))
	}
}

func TestService_DisbandInactiveGuilds_ConcurrentNotFound(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)

	disbandedIDs := []string{}
	repo := &mockGuildRepo{
		listInactiveGuildsFn: func(ctx context.Context, cutoff time.Time, limit int) ([]guild.Guild, error) {
			return []guild.Guild{
				{ID: "already-deleted-guild", Name: "GhostGuild"},
			}, nil
		},
		getGuildForUpdateFn: func(ctx context.Context, gID string) (guild.Guild, []guild.Member, error) {
			// Already deleted concurrently
			return guild.Guild{}, nil, guild.ErrGuildNotFound
		},
		disbandGuildFn: func(ctx context.Context, gID string) error {
			disbandedIDs = append(disbandedIDs, gID)
			return nil
		},
	}

	news := &mockNewsPublisher{}
	svc, _ := guild.NewService(repo, guild.WithNewsPublisher(news))
	count, list, err := svc.DisbandInactiveGuilds(ctx, now, 50)
	if err != nil {
		t.Fatalf("DisbandInactiveGuilds error: %v", err)
	}
	if count != 0 {
		t.Errorf("expected count 0 for already deleted guild, got %d", count)
	}
	if len(list) != 0 {
		t.Errorf("expected empty disbanded list, got %v", list)
	}
	if len(disbandedIDs) != 0 {
		t.Errorf("expected 0 DisbandGuild calls, got %d", len(disbandedIDs))
	}
	if len(news.calls) != 0 {
		t.Errorf("expected 0 news publications, got %d", len(news.calls))
	}
}

func TestService_DisbandInactiveGuilds_DisbandErrorPropagated(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)

	expectedErr := errors.New("db disk failure")
	repo := &mockGuildRepo{
		listInactiveGuildsFn: func(ctx context.Context, cutoff time.Time, limit int) ([]guild.Guild, error) {
			return []guild.Guild{
				{ID: "dead-guild-1", Name: "GhostGuild1"},
			}, nil
		},
		getGuildForUpdateFn: func(ctx context.Context, gID string) (guild.Guild, []guild.Member, error) {
			return guild.Guild{ID: "dead-guild-1", Name: "GhostGuild1", LastActiveAt: now.Add(-guild.InactivityDisbandDuration - time.Hour)}, nil, nil
		},
		disbandGuildFn: func(ctx context.Context, gID string) error {
			return expectedErr
		},
	}

	svc, _ := guild.NewService(repo)
	_, _, err := svc.DisbandInactiveGuilds(ctx, now, 50)
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected error %v, got %v", expectedErr, err)
	}
}
