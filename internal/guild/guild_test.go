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

type mockGuildRepo struct {
	createGuildFn        func(ctx context.Context, g guild.Guild, creator guild.Member, fee int) (guild.Guild, guild.Member, corecharacter.Character, error)
	getGuildFn           func(ctx context.Context, guildID string) (guild.Guild, []guild.Member, error)
	getGuildByCharFn     func(ctx context.Context, characterID string) (guild.Guild, guild.Member, error)
	listGuildsFn         func(ctx context.Context, offset, limit int) ([]guild.Guild, error)
	listInactiveGuildsFn func(ctx context.Context, cutoff time.Time, limit int) ([]guild.Guild, error)
	addMemberFn          func(ctx context.Context, member guild.Member) (guild.Member, error)
	removeMemberFn       func(ctx context.Context, guildID string, characterID string) error
	transferLeadershipFn func(ctx context.Context, guildID string, oldLeaderCharID string, newLeaderCharID string) error
	assignCustomRoleFn   func(ctx context.Context, guildID string, targetCharID string, title string) error
	approveMemberFn      func(ctx context.Context, guildID string, characterID string, title string) error
	updateNoticeFn       func(ctx context.Context, guildID string, notice string) error
	updateColorFn        func(ctx context.Context, guildID string, color string) error
	isColorTakenFn       func(ctx context.Context, color string, excludeGuildID string) (bool, error)
	addPointsFn          func(ctx context.Context, guildID string, points int64) error
	addGuildPointsFn     func(ctx context.Context, characterID string, points int) error
	updateMarkFn         func(ctx context.Context, guildID string, mark string, fee int, leaderID string) (corecharacter.Character, error)
	updateBgimgFn        func(ctx context.Context, guildID string, bgimg string) error
	updateWallpaperFn    func(ctx context.Context, guildID string, wallpaper string, fee int, leaderID string) (corecharacter.Character, error)
	touchActiveFn        func(ctx context.Context, guildID string) error
	disbandGuildFn       func(ctx context.Context, guildID string) error
	decayGuildPointsFn   func(ctx context.Context, factor float64) error
}

func (m *mockGuildRepo) CreateGuild(ctx context.Context, g guild.Guild, creator guild.Member, fee int) (guild.Guild, guild.Member, corecharacter.Character, error) {
	if m.createGuildFn != nil {
		return m.createGuildFn(ctx, g, creator, fee)
	}
	return g, creator, corecharacter.Character{ID: creator.CharacterID, Money: 10000 - fee}, nil
}

func (m *mockGuildRepo) GetGuild(ctx context.Context, guildID string) (guild.Guild, []guild.Member, error) {
	if m.getGuildFn != nil {
		return m.getGuildFn(ctx, guildID)
	}
	return guild.Guild{}, nil, guild.ErrGuildNotFound
}

func (m *mockGuildRepo) GetGuildByCharacter(ctx context.Context, characterID string) (guild.Guild, guild.Member, error) {
	if m.getGuildByCharFn != nil {
		return m.getGuildByCharFn(ctx, characterID)
	}
	return guild.Guild{}, guild.Member{}, guild.ErrCharacterNotInGuild
}

func (m *mockGuildRepo) ListGuilds(ctx context.Context, offset, limit int) ([]guild.Guild, error) {
	if m.listGuildsFn != nil {
		return m.listGuildsFn(ctx, offset, limit)
	}
	return nil, nil
}

func (m *mockGuildRepo) ListInactiveGuilds(ctx context.Context, cutoff time.Time, limit int) ([]guild.Guild, error) {
	if m.listInactiveGuildsFn != nil {
		return m.listInactiveGuildsFn(ctx, cutoff, limit)
	}
	return nil, nil
}

func (m *mockGuildRepo) AddMember(ctx context.Context, member guild.Member) (guild.Member, error) {
	if m.addMemberFn != nil {
		return m.addMemberFn(ctx, member)
	}
	return member, nil
}

func (m *mockGuildRepo) RemoveMember(ctx context.Context, guildID string, characterID string) error {
	if m.removeMemberFn != nil {
		return m.removeMemberFn(ctx, guildID, characterID)
	}
	return nil
}

func (m *mockGuildRepo) TransferLeadership(ctx context.Context, guildID string, oldLeaderCharID string, newLeaderCharID string) error {
	if m.transferLeadershipFn != nil {
		return m.transferLeadershipFn(ctx, guildID, oldLeaderCharID, newLeaderCharID)
	}
	return nil
}

func (m *mockGuildRepo) AssignCustomRole(ctx context.Context, guildID string, targetCharID string, title string) error {
	if m.assignCustomRoleFn != nil {
		return m.assignCustomRoleFn(ctx, guildID, targetCharID, title)
	}
	return nil
}

func (m *mockGuildRepo) ApproveMember(ctx context.Context, guildID string, characterID string, title string) error {
	if m.approveMemberFn != nil {
		return m.approveMemberFn(ctx, guildID, characterID, title)
	}
	return nil
}

func (m *mockGuildRepo) UpdateNotice(ctx context.Context, guildID string, notice string) error {
	if m.updateNoticeFn != nil {
		return m.updateNoticeFn(ctx, guildID, notice)
	}
	return nil
}

func (m *mockGuildRepo) UpdateColor(ctx context.Context, guildID string, color string) error {
	if m.updateColorFn != nil {
		return m.updateColorFn(ctx, guildID, color)
	}
	return nil
}

func (m *mockGuildRepo) IsColorTaken(ctx context.Context, color string, excludeGuildID string) (bool, error) {
	if m.isColorTakenFn != nil {
		return m.isColorTakenFn(ctx, color, excludeGuildID)
	}
	return false, nil
}

func (m *mockGuildRepo) AddPoints(ctx context.Context, guildID string, points int64) error {
	if m.addPointsFn != nil {
		return m.addPointsFn(ctx, guildID, points)
	}
	return nil
}

func (m *mockGuildRepo) AddGuildPoints(ctx context.Context, characterID string, points int) error {
	if m.addGuildPointsFn != nil {
		return m.addGuildPointsFn(ctx, characterID, points)
	}
	return nil
}

func (m *mockGuildRepo) UpdateMark(ctx context.Context, guildID string, mark string, fee int, leaderID string) (corecharacter.Character, error) {
	if m.updateMarkFn != nil {
		return m.updateMarkFn(ctx, guildID, mark, fee, leaderID)
	}
	return corecharacter.Character{ID: leaderID, Money: 10000 - fee}, nil
}

func (m *mockGuildRepo) UpdateBgimg(ctx context.Context, guildID string, bgimg string) error {
	if m.updateBgimgFn != nil {
		return m.updateBgimgFn(ctx, guildID, bgimg)
	}
	return nil
}

func (m *mockGuildRepo) UpdateWallpaper(ctx context.Context, guildID string, wallpaper string, fee int, leaderID string) (corecharacter.Character, error) {
	if m.updateWallpaperFn != nil {
		return m.updateWallpaperFn(ctx, guildID, wallpaper, fee, leaderID)
	}
	return corecharacter.Character{ID: leaderID, Money: 10000 - fee}, nil
}

func (m *mockGuildRepo) TouchActive(ctx context.Context, guildID string) error {
	if m.touchActiveFn != nil {
		return m.touchActiveFn(ctx, guildID)
	}
	return nil
}

func (m *mockGuildRepo) DisbandGuild(ctx context.Context, guildID string) error {
	if m.disbandGuildFn != nil {
		return m.disbandGuildFn(ctx, guildID)
	}
	return nil
}

func (m *mockGuildRepo) DecayGuildPoints(ctx context.Context, factor float64) error {
	if m.decayGuildPointsFn != nil {
		return m.decayGuildPointsFn(ctx, factor)
	}
	return nil
}

// -------------------------------------------------------------------
// Domain Unit Tests
// -------------------------------------------------------------------

func TestRole_Valid(t *testing.T) {
	tests := []struct {
		role  guild.Role
		valid bool
	}{
		{guild.RoleLeader, true},
		{guild.RoleMember, true},
		{"officer", false}, // Purged fictional role
		{"admin", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := tt.role.Valid(); got != tt.valid {
			t.Errorf("Role(%q).Valid() = %v, want %v", tt.role, got, tt.valid)
		}
	}
}

func TestCalculateTitleWidth(t *testing.T) {
	tests := []struct {
		title string
		want  int
	}{
		{"隊長", 4},
		{"親衛隊長", 8},
		{"一二三四五六", 12},
		{"一二三四五六七", 14},
		{"Captain", 7},
		{"123456789012", 12},
		{"1234567890123", 13},
	}
	for _, tt := range tests {
		got := guild.CalculateTitleWidth(tt.title)
		if got != tt.want {
			t.Errorf("CalculateTitleWidth(%q) = %d, want %d", tt.title, got, tt.want)
		}
	}
}

func TestValidateRoleTitle(t *testing.T) {
	tests := []struct {
		name    string
		title   string
		wantErr error
	}{
		{"Valid kanji title", "親衛隊長", nil},
		{"Valid 6 full-width kanji", "一二三四五六", nil},
		{"Valid ASCII title", "Captain", nil},
		{"Valid 12 half-width chars", "123456789012", nil},
		{"Empty title", "", guild.ErrInvalidRoleTitle},
		{"Contains half-width space", "隊長 副隊長", guild.ErrInvalidRoleTitle},
		{"Contains full-width space", "隊長　副隊長", guild.ErrInvalidRoleTitle},
		{"Contains comma", "隊長,副隊長", guild.ErrInvalidRoleTitle},
		{"Contains at sign", "隊長@本部", guild.ErrInvalidRoleTitle},
		{"Contains full-width at sign", "隊長＠本部", guild.ErrInvalidRoleTitle},
		{"Reserved title 参加申請中", "参加申請中", guild.ErrReservedRoleTitle},
		{"Reserved title ギルマス", "ギルマス", guild.ErrReservedRoleTitle},
		{"Too long full-width (7 chars = 14 width)", "一二三四五六七", guild.ErrRoleTitleTooLong},
		{"Too long ASCII (13 chars = 13 width)", "1234567890123", guild.ErrRoleTitleTooLong},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := guild.ValidateRoleTitle(tt.title)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("ValidateRoleTitle(%q) = %v, want %v", tt.title, err, tt.wantErr)
			}
		})
	}
}

func TestValidateColorFormat(t *testing.T) {
	tests := []struct {
		color   string
		want    string
		wantErr error
	}{
		{"#FFFFFF", "#FFFFFF", nil},
		{"#ffffff", "#FFFFFF", nil},
		{"#ff3333", "#FF3333", nil},
		{"#33CCFF", "#33CCFF", nil},
		{"invalid", "", guild.ErrInvalidColorFormat},
		{"#FFF", "", guild.ErrInvalidColorFormat},
		{"#GGGGGG", "", guild.ErrInvalidColorFormat},
		{"", "", guild.ErrInvalidColorFormat},
	}
	for _, tt := range tests {
		got, err := guild.ValidateColorFormat(tt.color)
		if !errors.Is(err, tt.wantErr) {
			t.Errorf("ValidateColorFormat(%q) error = %v, want %v", tt.color, err, tt.wantErr)
		}
		if err == nil && got != tt.want {
			t.Errorf("ValidateColorFormat(%q) = %q, want %q", tt.color, got, tt.want)
		}
	}
}

func TestService_Create_Validation(t *testing.T) {
	ctx := context.Background()

	t.Run("Empty character ID", func(t *testing.T) {
		svc, _ := guild.NewService(&mockGuildRepo{})
		_, _, _, err := svc.Create(ctx, "", "Knights")
		if !errors.Is(err, guild.ErrCharacterNotFound) {
			t.Errorf("err = %v, want %v", err, guild.ErrCharacterNotFound)
		}
	})

	t.Run("Invalid guild names rejected", func(t *testing.T) {
		svc, _ := guild.NewService(&mockGuildRepo{})
		invalidNames := []struct {
			name  string
			input string
		}{
			{"empty", ""},
			{"whitespace only", "   "},
			{"leading ascii space", " Knights"},
			{"trailing ascii space", "Knights "},
			{"internal ascii space", "Knights of Round"},
			{"japanese fullwidth space", "騎士団　本部"},
			{"tab character", "Knights\tRound"},
			{"newline character", "Knights\nRound"},
			{"too long name (33 runes)", strings.Repeat("A", 33)},
			{"prohibited comma", "Knights,Round"},
			{"prohibited semicolon", "Knights;Round"},
			{"prohibited double quote", `"Knights"`},
			{"prohibited single quote", "'Knights'"},
			{"prohibited ampersand", "Knights&Round"},
			{"prohibited less than", "<Knights>"},
			{"prohibited greater than", "Knights>Round"},
			{"prohibited at sign", "@Knights"},
			{"prohibited fullwidth at sign", "＠Knights"},
			{"prohibited backslash", `Knights\Round`},
			{"prohibited slash", "Knights/Round"},
			{"control character C0", "Knights\x00Round"},
			{"control character C1", "Knights\u0080Round"},
			{"zero-width space", "Knights\u200BRound"},
			{"bidi override", "Knights\u202ERound"},
			{"zalgo text", "Z\u0300\u0301\u0302algo"},
		}

		for _, tc := range invalidNames {
			t.Run(tc.name, func(t *testing.T) {
				if _, _, _, err := svc.Create(ctx, "char1", tc.input); !errors.Is(err, guild.ErrInvalidGuildName) {
					t.Errorf("Create(%q) err = %v, want %v", tc.input, err, guild.ErrInvalidGuildName)
				}
			})
		}
	})

	t.Run("Success normalizes guild name with NFC", func(t *testing.T) {
		var createdGuild guild.Guild
		repo := &mockGuildRepo{
			createGuildFn: func(_ context.Context, g guild.Guild, m guild.Member, fee int) (guild.Guild, guild.Member, corecharacter.Character, error) {
				createdGuild = g
				return g, m, corecharacter.Character{ID: m.CharacterID, Money: 5000}, nil
			},
		}
		svc, _ := guild.NewService(repo)
		// Decomposed unicode "Ka\u0301" -> NFC "Ká"
		g, _, _, err := svc.Create(ctx, "char1", "Ka\u0301")
		if err != nil {
			t.Fatalf("Create() unexpected error: %v", err)
		}
		if g.Name != "Ká" || createdGuild.Name != "Ká" {
			t.Errorf("expected NFC normalized name %q, got %q (repo: %q)", "Ká", g.Name, createdGuild.Name)
		}
	})

	t.Run("Character already in a guild", func(t *testing.T) {
		repo := &mockGuildRepo{
			getGuildByCharFn: func(_ context.Context, charID string) (guild.Guild, guild.Member, error) {
				return guild.Guild{ID: "g1"}, guild.Member{GuildID: "g1", CharacterID: charID}, nil
			},
		}
		svc, _ := guild.NewService(repo)
		_, _, _, err := svc.Create(ctx, "char1", "Knights")
		if !errors.Is(err, guild.ErrCharacterAlreadyInGuild) {
			t.Errorf("err = %v, want %v", err, guild.ErrCharacterAlreadyInGuild)
		}
	})

	t.Run("Success initializes Points 0 and Leader Title ギルマス", func(t *testing.T) {
		var createdGuild guild.Guild
		var createdMember guild.Member
		repo := &mockGuildRepo{
			createGuildFn: func(_ context.Context, g guild.Guild, m guild.Member, fee int) (guild.Guild, guild.Member, corecharacter.Character, error) {
				createdGuild = g
				createdMember = m
				return g, m, corecharacter.Character{ID: m.CharacterID, Money: 5000}, nil
			},
		}
		svc, _ := guild.NewService(repo)
		g, m, _, err := svc.Create(ctx, "char1", "Knights")
		if err != nil {
			t.Fatalf("Create() unexpected error: %v", err)
		}
		if g.Name != "Knights" || m.Role != guild.RoleLeader || m.Title != guild.DefaultTitleLeader || m.CharacterID != "char1" {
			t.Errorf("created guild = %+v, member = %+v", createdGuild, createdMember)
		}
		if g.Points != 0 || g.Color != guild.DefaultColor {
			t.Errorf("expected Points 0, Color #FFFFFF, got %+v", g)
		}
	})
}

func TestService_Join(t *testing.T) {
	ctx := context.Background()

	t.Run("Character already in guild", func(t *testing.T) {
		repo := &mockGuildRepo{
			getGuildByCharFn: func(_ context.Context, charID string) (guild.Guild, guild.Member, error) {
				return guild.Guild{ID: "g1"}, guild.Member{GuildID: "g1", CharacterID: charID}, nil
			},
		}
		svc, _ := guild.NewService(repo)
		_, err := svc.Join(ctx, "g2", "char1")
		if !errors.Is(err, guild.ErrCharacterAlreadyInGuild) {
			t.Errorf("err = %v, want %v", err, guild.ErrCharacterAlreadyInGuild)
		}
	})

	t.Run("Success without capacity limits", func(t *testing.T) {
		repo := &mockGuildRepo{
			getGuildFn: func(_ context.Context, guildID string) (guild.Guild, []guild.Member, error) {
				return guild.Guild{ID: guildID, Points: 100}, []guild.Member{{CharacterID: "c0", Role: guild.RoleLeader}}, nil
			},
		}
		svc, _ := guild.NewService(repo)
		m, err := svc.Join(ctx, "g1", "char1")
		if err != nil {
			t.Fatalf("Join() unexpected error: %v", err)
		}
		if m.Role != guild.RoleMember || m.CharacterID != "char1" || m.Title != "" {
			t.Errorf("joined member = %+v", m)
		}
	})
}

type mockNewsPublisher struct {
	calls []struct {
		Category    string
		Title       string
		Content     string
		Author      string
		PublishedAt time.Time
	}
}

func (m *mockNewsPublisher) PublishNews(ctx context.Context, category, title, content, author string, publishedAt time.Time) error {
	m.calls = append(m.calls, struct {
		Category    string
		Title       string
		Content     string
		Author      string
		PublishedAt time.Time
	}{
		Category:    category,
		Title:       title,
		Content:     content,
		Author:      author,
		PublishedAt: publishedAt,
	})
	return nil
}

func TestService_Leave(t *testing.T) {
	ctx := context.Background()

	t.Run("Leader leaves with other members transfers leadership (title candidate preferred)", func(t *testing.T) {
		var transferredTo string
		var removedChar string
		repo := &mockGuildRepo{
			getGuildFn: func(_ context.Context, guildID string) (guild.Guild, []guild.Member, error) {
				return guild.Guild{ID: guildID, Name: "TestGuild"}, []guild.Member{
					{CharacterID: "leader1", Role: guild.RoleLeader},
					{CharacterID: "member1", Role: guild.RoleMember, Title: "一般"},
					{CharacterID: "member2", Role: guild.RoleMember, Title: "ギルマス代行"},
				}, nil
			},
			transferLeadershipFn: func(_ context.Context, guildID, oldLeader, newLeader string) error {
				transferredTo = newLeader
				return nil
			},
			removeMemberFn: func(_ context.Context, guildID, charID string) error {
				removedChar = charID
				return nil
			},
		}
		svc, _ := guild.NewService(repo)
		if err := svc.Leave(ctx, "g1", "leader1"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if transferredTo != "member2" {
			t.Errorf("transferredTo = %s, want member2 (title candidate)", transferredTo)
		}
		if removedChar != "leader1" {
			t.Errorf("removedChar = %s, want leader1", removedChar)
		}
	})

	t.Run("Leader leaves with other members transfers leadership (next member fallback)", func(t *testing.T) {
		var transferredTo string
		var removedChar string
		repo := &mockGuildRepo{
			getGuildFn: func(_ context.Context, guildID string) (guild.Guild, []guild.Member, error) {
				return guild.Guild{ID: guildID, Name: "TestGuild"}, []guild.Member{
					{CharacterID: "leader1", Role: guild.RoleLeader},
					{CharacterID: "member1", Role: guild.RoleMember, Title: "一般"},
				}, nil
			},
			transferLeadershipFn: func(_ context.Context, guildID, oldLeader, newLeader string) error {
				transferredTo = newLeader
				return nil
			},
			removeMemberFn: func(_ context.Context, guildID, charID string) error {
				removedChar = charID
				return nil
			},
		}
		svc, _ := guild.NewService(repo)
		if err := svc.Leave(ctx, "g1", "leader1"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if transferredTo != "member1" {
			t.Errorf("transferredTo = %s, want member1", transferredTo)
		}
		if removedChar != "leader1" {
			t.Errorf("removedChar = %s, want leader1", removedChar)
		}
	})

	t.Run("Sole leader leaving disbands guild and publishes news", func(t *testing.T) {
		disbanded := false
		repo := &mockGuildRepo{
			getGuildFn: func(_ context.Context, guildID string) (guild.Guild, []guild.Member, error) {
				return guild.Guild{ID: guildID, Name: "孤独なギルド"}, []guild.Member{
					{CharacterID: "leader1", Role: guild.RoleLeader},
				}, nil
			},
			disbandGuildFn: func(_ context.Context, guildID string) error {
				disbanded = true
				return nil
			},
		}
		news := &mockNewsPublisher{}
		svc, _ := guild.NewService(repo, guild.WithNewsPublisher(news))
		if err := svc.Leave(ctx, "g1", "leader1"); err != nil {
			t.Fatalf("Leave() unexpected error: %v", err)
		}
		if !disbanded {
			t.Error("expected sole leader leave to trigger disband")
		}
		if len(news.calls) != 1 {
			t.Fatalf("expected 1 news publication, got %d", len(news.calls))
		}
		call := news.calls[0]
		if call.Category != "guild" {
			t.Errorf("news Category = %s, want guild", call.Category)
		}
		expectedMsg := "ギルド『孤独なギルド』が解散しました"
		if call.Title != expectedMsg || call.Content != expectedMsg {
			t.Errorf("news Title/Content = %s, want %s", call.Title, expectedMsg)
		}
		if call.Author != "System" {
			t.Errorf("news Author = %s, want System", call.Author)
		}
	})

	t.Run("Regular member leaves", func(t *testing.T) {
		removed := false
		repo := &mockGuildRepo{
			getGuildFn: func(_ context.Context, guildID string) (guild.Guild, []guild.Member, error) {
				return guild.Guild{ID: guildID}, []guild.Member{
					{CharacterID: "leader1", Role: guild.RoleLeader},
					{CharacterID: "member1", Role: guild.RoleMember},
				}, nil
			},
			removeMemberFn: func(_ context.Context, guildID, charID string) error {
				if charID == "member1" {
					removed = true
				}
				return nil
			},
		}
		svc, _ := guild.NewService(repo)
		if err := svc.Leave(ctx, "g1", "member1"); err != nil {
			t.Fatalf("Leave() unexpected error: %v", err)
		}
		if !removed {
			t.Error("expected member to be removed")
		}
	})
}

func TestService_Kick(t *testing.T) {
	ctx := context.Background()

	members := []guild.Member{
		{CharacterID: "leader1", Role: guild.RoleLeader},
		{CharacterID: "member1", Role: guild.RoleMember},
	}
	repo := &mockGuildRepo{
		getGuildFn: func(_ context.Context, guildID string) (guild.Guild, []guild.Member, error) {
			return guild.Guild{ID: guildID}, members, nil
		},
	}
	svc, _ := guild.NewService(repo)

	t.Run("Cannot kick leader", func(t *testing.T) {
		err := svc.Kick(ctx, "g1", "leader1", "leader1")
		if err == nil {
			t.Error("expected error kicking leader, got nil")
		}
	})

	t.Run("Member cannot kick anyone", func(t *testing.T) {
		err := svc.Kick(ctx, "g1", "member1", "leader1")
		if !errors.Is(err, guild.ErrUnauthorized) {
			t.Errorf("err = %v, want %v", err, guild.ErrUnauthorized)
		}
	})

	t.Run("Leader can kick member", func(t *testing.T) {
		kicked := false
		repo.removeMemberFn = func(_ context.Context, guildID, charID string) error {
			if charID == "member1" {
				kicked = true
			}
			return nil
		}
		if err := svc.Kick(ctx, "g1", "leader1", "member1"); err != nil {
			t.Errorf("Kick() unexpected error: %v", err)
		}
		if !kicked {
			t.Error("expected member to be kicked")
		}
	})
}

func TestService_TransferLeadership(t *testing.T) {
	ctx := context.Background()
	members := []guild.Member{
		{CharacterID: "leader1", Role: guild.RoleLeader, Title: guild.DefaultTitleLeader},
		{CharacterID: "member1", Role: guild.RoleMember, Title: "親衛隊長"},
	}
	repo := &mockGuildRepo{
		getGuildFn: func(_ context.Context, guildID string) (guild.Guild, []guild.Member, error) {
			return guild.Guild{ID: guildID}, members, nil
		},
	}
	svc, _ := guild.NewService(repo)

	t.Run("Unauthorized if not leader", func(t *testing.T) {
		err := svc.TransferLeadership(ctx, "g1", "member1", "leader1")
		if !errors.Is(err, guild.ErrUnauthorized) {
			t.Errorf("err = %v, want %v", err, guild.ErrUnauthorized)
		}
	})

	t.Run("Target not member", func(t *testing.T) {
		err := svc.TransferLeadership(ctx, "g1", "leader1", "outsider")
		if !errors.Is(err, guild.ErrTargetNotMember) {
			t.Errorf("err = %v, want %v", err, guild.ErrTargetNotMember)
		}
	})

	t.Run("Success", func(t *testing.T) {
		transferred := false
		repo.transferLeadershipFn = func(_ context.Context, guildID, oldL, newL string) error {
			if oldL == "leader1" && newL == "member1" {
				transferred = true
			}
			return nil
		}
		if err := svc.TransferLeadership(ctx, "g1", "leader1", "member1"); err != nil {
			t.Fatalf("TransferLeadership() unexpected error: %v", err)
		}
		if !transferred {
			t.Error("expected leadership transfer to be called")
		}
	})
}

func TestService_AssignCustomRole(t *testing.T) {
	ctx := context.Background()
	members := []guild.Member{
		{CharacterID: "leader1", Role: guild.RoleLeader, Title: guild.DefaultTitleLeader},
		{CharacterID: "member1", Role: guild.RoleMember, Title: ""},
	}
	repo := &mockGuildRepo{
		getGuildFn: func(_ context.Context, guildID string) (guild.Guild, []guild.Member, error) {
			return guild.Guild{ID: guildID}, members, nil
		},
	}
	svc, _ := guild.NewService(repo)

	t.Run("Unauthorized when non-leader tries to assign custom role", func(t *testing.T) {
		err := svc.AssignCustomRole(ctx, "g1", "member1", "member1", "隊長")
		if !errors.Is(err, guild.ErrUnauthorized) {
			t.Errorf("err = %v, want %v", err, guild.ErrUnauthorized)
		}
	})

	t.Run("Cannot assign custom role to leader", func(t *testing.T) {
		err := svc.AssignCustomRole(ctx, "g1", "leader1", "leader1", "大将軍")
		if !errors.Is(err, guild.ErrCannotAssignToLeader) {
			t.Errorf("err = %v, want %v", err, guild.ErrCannotAssignToLeader)
		}
	})

	t.Run("Target not member", func(t *testing.T) {
		err := svc.AssignCustomRole(ctx, "g1", "leader1", "stranger", "親衛隊")
		if !errors.Is(err, guild.ErrTargetNotMember) {
			t.Errorf("err = %v, want %v", err, guild.ErrTargetNotMember)
		}
	})

	t.Run("Invalid title validations", func(t *testing.T) {
		if err := svc.AssignCustomRole(ctx, "g1", "leader1", "member1", ""); !errors.Is(err, guild.ErrInvalidRoleTitle) {
			t.Errorf("expected ErrInvalidRoleTitle, got %v", err)
		}
		if err := svc.AssignCustomRole(ctx, "g1", "leader1", "member1", "ギルマス"); !errors.Is(err, guild.ErrReservedRoleTitle) {
			t.Errorf("expected ErrReservedRoleTitle, got %v", err)
		}
		if err := svc.AssignCustomRole(ctx, "g1", "leader1", "member1", "参加申請中"); !errors.Is(err, guild.ErrReservedRoleTitle) {
			t.Errorf("expected ErrReservedRoleTitle, got %v", err)
		}
		if err := svc.AssignCustomRole(ctx, "g1", "leader1", "member1", "一二三四五六七"); !errors.Is(err, guild.ErrRoleTitleTooLong) {
			t.Errorf("expected ErrRoleTitleTooLong, got %v", err)
		}
	})

	t.Run("Success assign custom title (up to 6 full-width characters)", func(t *testing.T) {
		var assignedTitle string
		repo.assignCustomRoleFn = func(_ context.Context, guildID, targetCharID, title string) error {
			if targetCharID == "member1" {
				assignedTitle = title
			}
			return nil
		}
		if err := svc.AssignCustomRole(ctx, "g1", "leader1", "member1", "親衛隊長"); err != nil {
			t.Fatalf("AssignCustomRole failed: %v", err)
		}
		if assignedTitle != "親衛隊長" {
			t.Errorf("assignedTitle = %q, want '親衛隊長'", assignedTitle)
		}
	})
}

func TestService_UpdateColor(t *testing.T) {
	ctx := context.Background()
	members := []guild.Member{
		{CharacterID: "leader1", Role: guild.RoleLeader},
		{CharacterID: "member1", Role: guild.RoleMember},
	}
	repo := &mockGuildRepo{
		getGuildFn: func(_ context.Context, guildID string) (guild.Guild, []guild.Member, error) {
			return guild.Guild{ID: guildID, Color: "#FFFFFF"}, members, nil
		},
	}
	svc, _ := guild.NewService(repo)

	t.Run("Unauthorized when non-leader changes color", func(t *testing.T) {
		err := svc.UpdateColor(ctx, "g1", "member1", "#33CCFF")
		if !errors.Is(err, guild.ErrUnauthorized) {
			t.Errorf("err = %v, want %v", err, guild.ErrUnauthorized)
		}
	})

	t.Run("Invalid color format", func(t *testing.T) {
		err := svc.UpdateColor(ctx, "g1", "leader1", "blue")
		if !errors.Is(err, guild.ErrInvalidColorFormat) {
			t.Errorf("err = %v, want %v", err, guild.ErrInvalidColorFormat)
		}
	})

	t.Run("NPC color prohibited", func(t *testing.T) {
		err := svc.UpdateColor(ctx, "g1", "leader1", guild.NPCColor)
		if !errors.Is(err, guild.ErrColorTaken) {
			t.Errorf("err = %v, want %v", err, guild.ErrColorTaken)
		}
	})

	t.Run("Duplicate color taken by another guild", func(t *testing.T) {
		repo.isColorTakenFn = func(_ context.Context, color, excludeGuildID string) (bool, error) {
			if color == "#FF3333" {
				return true, nil
			}
			return false, nil
		}
		err := svc.UpdateColor(ctx, "g1", "leader1", "#FF3333")
		if !errors.Is(err, guild.ErrColorTaken) {
			t.Errorf("err = %v, want %v", err, guild.ErrColorTaken)
		}
	})

	t.Run("Success with unique hex color", func(t *testing.T) {
		var updatedColor string
		repo.isColorTakenFn = func(_ context.Context, color, excludeGuildID string) (bool, error) {
			return false, nil
		}
		repo.updateColorFn = func(_ context.Context, guildID, color string) error {
			updatedColor = color
			return nil
		}
		if err := svc.UpdateColor(ctx, "g1", "leader1", "#33ccff"); err != nil {
			t.Fatalf("UpdateColor failed: %v", err)
		}
		if updatedColor != "#33CCFF" {
			t.Errorf("updatedColor = %q, want '#33CCFF'", updatedColor)
		}
	})

	t.Run("Default white color allows multiple guilds", func(t *testing.T) {
		var updatedColor string
		repo.updateColorFn = func(_ context.Context, guildID, color string) error {
			updatedColor = color
			return nil
		}
		if err := svc.UpdateColor(ctx, "g1", "leader1", "#FFFFFF"); err != nil {
			t.Fatalf("UpdateColor failed: %v", err)
		}
		if updatedColor != "#FFFFFF" {
			t.Errorf("updatedColor = %q, want '#FFFFFF'", updatedColor)
		}
	})
}

func TestService_GuildPoints(t *testing.T) {
	ctx := context.Background()
	repo := &mockGuildRepo{}
	svc, _ := guild.NewService(repo)

	t.Run("AddPoints delegates directly", func(t *testing.T) {
		var addedPoints int64
		repo.addPointsFn = func(_ context.Context, guildID string, points int64) error {
			addedPoints = points
			return nil
		}
		if err := svc.AddPoints(ctx, "g1", 100); err != nil {
			t.Fatalf("AddPoints failed: %v", err)
		}
		if addedPoints != 100 {
			t.Errorf("addedPoints = %d, want 100", addedPoints)
		}
	})

	t.Run("AddGuildPoints delegates with character ID", func(t *testing.T) {
		var charID string
		var pointsAdded int
		repo.addGuildPointsFn = func(_ context.Context, cID string, p int) error {
			charID = cID
			pointsAdded = p
			return nil
		}
		if err := svc.AddGuildPoints(ctx, "char1", 50); err != nil {
			t.Fatalf("AddGuildPoints failed: %v", err)
		}
		if charID != "char1" || pointsAdded != 50 {
			t.Errorf("got charID %q, points %d; want 'char1', 50", charID, pointsAdded)
		}
	})
}

func TestService_GetByCharacter(t *testing.T) {
	ctx := context.Background()

	repo := &mockGuildRepo{
		getGuildByCharFn: func(_ context.Context, charID string) (guild.Guild, guild.Member, error) {
			if charID == "char1" {
				return guild.Guild{ID: "g1", Name: "MyGuild"}, guild.Member{GuildID: "g1", CharacterID: charID, Role: guild.RoleLeader, Title: guild.DefaultTitleLeader}, nil
			}
			return guild.Guild{}, guild.Member{}, guild.ErrCharacterNotInGuild
		},
	}
	svc, _ := guild.NewService(repo)

	t.Run("empty character ID", func(t *testing.T) {
		_, _, err := svc.GetByCharacter(ctx, "")
		if !errors.Is(err, guild.ErrCharacterNotFound) {
			t.Errorf("got %v, want %v", err, guild.ErrCharacterNotFound)
		}
	})

	t.Run("found character", func(t *testing.T) {
		g, m, err := svc.GetByCharacter(ctx, "char1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if g.ID != "g1" || m.CharacterID != "char1" {
			t.Errorf("unexpected guild/member: %+v, %+v", g, m)
		}
	})
}

func TestService_Disband(t *testing.T) {
	ctx := context.Background()

	var disbandedID string
	repo := &mockGuildRepo{
		getGuildByCharFn: func(_ context.Context, charID string) (guild.Guild, guild.Member, error) {
			switch charID {
			case "leader":
				return guild.Guild{ID: "g1", Name: "解散ギルド"}, guild.Member{GuildID: "g1", CharacterID: "leader", Role: guild.RoleLeader}, nil
			case "member":
				return guild.Guild{ID: "g1", Name: "解散ギルド"}, guild.Member{GuildID: "g1", CharacterID: "member", Role: guild.RoleMember}, nil
			default:
				return guild.Guild{}, guild.Member{}, guild.ErrCharacterNotInGuild
			}
		},
		disbandGuildFn: func(_ context.Context, guildID string) error {
			disbandedID = guildID
			return nil
		},
	}
	svc, _ := guild.NewService(repo)

	t.Run("empty guild ID", func(t *testing.T) {
		err := svc.Disband(ctx, "", "leader")
		if !errors.Is(err, guild.ErrInvalidGuildID) {
			t.Errorf("got %v, want %v", err, guild.ErrInvalidGuildID)
		}
	})

	t.Run("empty leader char ID", func(t *testing.T) {
		err := svc.Disband(ctx, "g1", "")
		if !errors.Is(err, guild.ErrCharacterNotFound) {
			t.Errorf("got %v, want %v", err, guild.ErrCharacterNotFound)
		}
	})

	t.Run("non-leader unauthorized", func(t *testing.T) {
		err := svc.Disband(ctx, "g1", "member")
		if !errors.Is(err, guild.ErrUnauthorized) {
			t.Errorf("got %v, want %v", err, guild.ErrUnauthorized)
		}
	})

	t.Run("successful disband publishes news", func(t *testing.T) {
		news := &mockNewsPublisher{}
		svcWithNews, _ := guild.NewService(repo, guild.WithNewsPublisher(news))
		err := svcWithNews.Disband(ctx, "g1", "leader")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if disbandedID != "g1" {
			t.Errorf("disbandedID = %s, want g1", disbandedID)
		}
		if len(news.calls) != 1 {
			t.Fatalf("expected 1 news publication, got %d", len(news.calls))
		}
		if news.calls[0].Category != "guild" {
			t.Errorf("news Category = %s, want guild", news.calls[0].Category)
		}
		expectedTitle := "ギルド『解散ギルド』が解散しました"
		if news.calls[0].Title != expectedTitle {
			t.Errorf("news Title = %s, want %s", news.calls[0].Title, expectedTitle)
		}
		if news.calls[0].Content != expectedTitle {
			t.Errorf("news Content = %s, want %s", news.calls[0].Content, expectedTitle)
		}
		if news.calls[0].Author != "System" {
			t.Errorf("news Author = %s, want System", news.calls[0].Author)
		}
	})
}

func TestValidateGuildName(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantErr    error
		wantOutput string
	}{
		{"valid ascii name", "Knights", nil, "Knights"},
		{"valid japanese name", "勇者のギルド", nil, "勇者のギルド"},
		{"valid max 32 runes", strings.Repeat("あ", 32), nil, strings.Repeat("あ", 32)},
		{"nfc normalized", "Ka\u0301", nil, "Ká"},
		{"exceeds 32 runes", strings.Repeat("あ", 33), guild.ErrInvalidGuildName, ""},
		{"empty name", "", guild.ErrInvalidGuildName, ""},
		{"whitespace only", "   ", guild.ErrInvalidGuildName, ""},
		{"leading space", " Knights", guild.ErrInvalidGuildName, ""},
		{"trailing space", "Knights ", guild.ErrInvalidGuildName, ""},
		{"internal space", "Knights of Round", guild.ErrInvalidGuildName, ""},
		{"japanese fullwidth space", "騎士団　本部", guild.ErrInvalidGuildName, ""},
		{"prohibited comma", "Knights,Round", guild.ErrInvalidGuildName, ""},
		{"prohibited at sign", "@Knights", guild.ErrInvalidGuildName, ""},
		{"prohibited fullwidth at sign", "＠Knights", guild.ErrInvalidGuildName, ""},
		{"control character", "Knights\x00", guild.ErrInvalidGuildName, ""},
		{"zero-width space", "Knights\u200B", guild.ErrInvalidGuildName, ""},
		{"bidi override", "Knights\u202E", guild.ErrInvalidGuildName, ""},
		{"zalgo text", "Z\u0300\u0301\u0302algo", guild.ErrInvalidGuildName, ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := guild.ValidateGuildName(tc.input)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("ValidateGuildName(%q) err = %v; want %v", tc.input, err, tc.wantErr)
				}
			} else {
				if err != nil {
					t.Fatalf("ValidateGuildName(%q) unexpected err = %v", tc.input, err)
				}
				if got != tc.wantOutput {
					t.Errorf("ValidateGuildName(%q) = %q; want %q", tc.input, got, tc.wantOutput)
				}
			}
		})
	}
}
