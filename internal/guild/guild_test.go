package guild_test

import (
	"context"
	"errors"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/guild"
)

type mockGuildRepo struct {
	createGuildFn        func(ctx context.Context, g guild.Guild, creator guild.Member, fee int) (guild.Guild, guild.Member, corecharacter.Character, error)
	getGuildFn           func(ctx context.Context, guildID string) (guild.Guild, []guild.Member, error)
	getGuildByCharFn     func(ctx context.Context, characterID string) (guild.Guild, guild.Member, error)
	listGuildsFn         func(ctx context.Context, offset, limit int) ([]guild.Guild, error)
	addMemberFn          func(ctx context.Context, member guild.Member) (guild.Member, error)
	removeMemberFn       func(ctx context.Context, guildID string, characterID string) error
	transferLeadershipFn func(ctx context.Context, guildID string, oldLeaderCharID string, newLeaderCharID string) error
	updateMemberRoleFn   func(ctx context.Context, guildID string, targetCharID string, newRole guild.Role) error
	updateNoticeFn       func(ctx context.Context, guildID string, notice string) error
	donateFn             func(ctx context.Context, guildID string, characterID string, amount int) (guild.Guild, guild.Member, corecharacter.Character, error)
	disbandGuildFn       func(ctx context.Context, guildID string) error
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

func (m *mockGuildRepo) UpdateMemberRole(ctx context.Context, guildID string, targetCharID string, newRole guild.Role) error {
	if m.updateMemberRoleFn != nil {
		return m.updateMemberRoleFn(ctx, guildID, targetCharID, newRole)
	}
	return nil
}

func (m *mockGuildRepo) UpdateNotice(ctx context.Context, guildID string, notice string) error {
	if m.updateNoticeFn != nil {
		return m.updateNoticeFn(ctx, guildID, notice)
	}
	return nil
}

func (m *mockGuildRepo) Donate(ctx context.Context, guildID string, characterID string, amount int) (guild.Guild, guild.Member, corecharacter.Character, error) {
	if m.donateFn != nil {
		return m.donateFn(ctx, guildID, characterID, amount)
	}
	return guild.Guild{ID: guildID}, guild.Member{GuildID: guildID, CharacterID: characterID}, corecharacter.Character{ID: characterID}, nil
}

func (m *mockGuildRepo) DisbandGuild(ctx context.Context, guildID string) error {
	if m.disbandGuildFn != nil {
		return m.disbandGuildFn(ctx, guildID)
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
		{guild.RoleOfficer, true},
		{guild.RoleMember, true},
		{"admin", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := tt.role.Valid(); got != tt.valid {
			t.Errorf("Role(%q).Valid() = %v, want %v", tt.role, got, tt.valid)
		}
	}
}

func TestGuild_Capacity(t *testing.T) {
	tests := []struct {
		level    int
		capacity int
	}{
		{0, 10},
		{1, 10},
		{2, 12},
		{5, 18},
		{10, 28},
	}
	for _, tt := range tests {
		g := guild.Guild{Level: tt.level}
		if got := g.Capacity(); got != tt.capacity {
			t.Errorf("Guild{Level: %d}.Capacity() = %d, want %d", tt.level, got, tt.capacity)
		}
	}
}

func TestGuild_CalculateLevel(t *testing.T) {
	tests := []struct {
		exp   int64
		level int
	}{
		{0, 1},
		{9999, 1},
		{10000, 2},
		{39999, 2},
		{40000, 3},
		{810000, 10},
		{99999999, 10}, // Capped at MaxLevel
	}
	for _, tt := range tests {
		if got := guild.CalculateLevel(tt.exp); got != tt.level {
			t.Errorf("CalculateLevel(%d) = %d, want %d", tt.exp, got, tt.level)
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

	t.Run("Empty or too long guild name", func(t *testing.T) {
		svc, _ := guild.NewService(&mockGuildRepo{})
		if _, _, _, err := svc.Create(ctx, "char1", ""); !errors.Is(err, guild.ErrInvalidGuildName) {
			t.Errorf("err = %v, want %v", err, guild.ErrInvalidGuildName)
		}
		longName := "This Guild Name Exceeds Thirty Two Characters Easily"
		if _, _, _, err := svc.Create(ctx, "char1", longName); !errors.Is(err, guild.ErrInvalidGuildName) {
			t.Errorf("err = %v, want %v", err, guild.ErrInvalidGuildName)
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

	t.Run("Success", func(t *testing.T) {
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
		if g.Name != "Knights" || m.Role != guild.RoleLeader || m.CharacterID != "char1" {
			t.Errorf("created guild = %+v, member = %+v", createdGuild, createdMember)
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

	t.Run("Guild full", func(t *testing.T) {
		members := make([]guild.Member, 10)
		repo := &mockGuildRepo{
			getGuildFn: func(_ context.Context, guildID string) (guild.Guild, []guild.Member, error) {
				return guild.Guild{ID: guildID, Level: 1}, members, nil
			},
		}
		svc, _ := guild.NewService(repo)
		_, err := svc.Join(ctx, "g1", "char1")
		if !errors.Is(err, guild.ErrGuildFull) {
			t.Errorf("err = %v, want %v", err, guild.ErrGuildFull)
		}
	})

	t.Run("Success", func(t *testing.T) {
		repo := &mockGuildRepo{
			getGuildFn: func(_ context.Context, guildID string) (guild.Guild, []guild.Member, error) {
				return guild.Guild{ID: guildID, Level: 1}, []guild.Member{{CharacterID: "c0", Role: guild.RoleLeader}}, nil
			},
		}
		svc, _ := guild.NewService(repo)
		m, err := svc.Join(ctx, "g1", "char1")
		if err != nil {
			t.Fatalf("Join() unexpected error: %v", err)
		}
		if m.Role != guild.RoleMember || m.CharacterID != "char1" {
			t.Errorf("joined member = %+v", m)
		}
	})
}

func TestService_Leave(t *testing.T) {
	ctx := context.Background()

	t.Run("Leader cannot leave with other members", func(t *testing.T) {
		repo := &mockGuildRepo{
			getGuildFn: func(_ context.Context, guildID string) (guild.Guild, []guild.Member, error) {
				return guild.Guild{ID: guildID}, []guild.Member{
					{CharacterID: "leader1", Role: guild.RoleLeader},
					{CharacterID: "member1", Role: guild.RoleMember},
				}, nil
			},
		}
		svc, _ := guild.NewService(repo)
		err := svc.Leave(ctx, "g1", "leader1")
		if !errors.Is(err, guild.ErrLeaderCannotLeaveWithMembers) {
			t.Errorf("err = %v, want %v", err, guild.ErrLeaderCannotLeaveWithMembers)
		}
	})

	t.Run("Sole leader leaving disbands guild", func(t *testing.T) {
		disbanded := false
		repo := &mockGuildRepo{
			getGuildFn: func(_ context.Context, guildID string) (guild.Guild, []guild.Member, error) {
				return guild.Guild{ID: guildID}, []guild.Member{
					{CharacterID: "leader1", Role: guild.RoleLeader},
				}, nil
			},
			disbandGuildFn: func(_ context.Context, guildID string) error {
				disbanded = true
				return nil
			},
		}
		svc, _ := guild.NewService(repo)
		if err := svc.Leave(ctx, "g1", "leader1"); err != nil {
			t.Fatalf("Leave() unexpected error: %v", err)
		}
		if !disbanded {
			t.Error("expected sole leader leave to trigger disband")
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
		{CharacterID: "officer1", Role: guild.RoleOfficer},
		{CharacterID: "officer2", Role: guild.RoleOfficer},
		{CharacterID: "member1", Role: guild.RoleMember},
	}
	repo := &mockGuildRepo{
		getGuildFn: func(_ context.Context, guildID string) (guild.Guild, []guild.Member, error) {
			return guild.Guild{ID: guildID}, members, nil
		},
	}
	svc, _ := guild.NewService(repo)

	t.Run("Cannot kick leader", func(t *testing.T) {
		err := svc.Kick(ctx, "g1", "officer1", "leader1")
		if !errors.Is(err, guild.ErrCannotKickLeader) {
			t.Errorf("err = %v, want %v", err, guild.ErrCannotKickLeader)
		}
	})

	t.Run("Officer cannot kick another officer", func(t *testing.T) {
		err := svc.Kick(ctx, "g1", "officer1", "officer2")
		if !errors.Is(err, guild.ErrCannotKickEqualOrHigherRole) {
			t.Errorf("err = %v, want %v", err, guild.ErrCannotKickEqualOrHigherRole)
		}
	})

	t.Run("Member cannot kick anyone", func(t *testing.T) {
		err := svc.Kick(ctx, "g1", "member1", "officer1")
		if !errors.Is(err, guild.ErrUnauthorized) {
			t.Errorf("err = %v, want %v", err, guild.ErrUnauthorized)
		}
	})

	t.Run("Officer can kick member", func(t *testing.T) {
		if err := svc.Kick(ctx, "g1", "officer1", "member1"); err != nil {
			t.Errorf("Kick() unexpected error: %v", err)
		}
	})

	t.Run("Leader can kick officer", func(t *testing.T) {
		if err := svc.Kick(ctx, "g1", "leader1", "officer1"); err != nil {
			t.Errorf("Kick() unexpected error: %v", err)
		}
	})
}

func TestService_TransferLeadership(t *testing.T) {
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

func TestService_UpdateRole(t *testing.T) {
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

	t.Run("Cannot set RoleLeader directly", func(t *testing.T) {
		err := svc.UpdateRole(ctx, "g1", "leader1", "member1", guild.RoleLeader)
		if !errors.Is(err, guild.ErrInvalidRole) {
			t.Errorf("err = %v, want %v", err, guild.ErrInvalidRole)
		}
	})

	t.Run("Unauthorized when non-leader tries to update role", func(t *testing.T) {
		err := svc.UpdateRole(ctx, "g1", "member1", "member1", guild.RoleOfficer)
		if !errors.Is(err, guild.ErrUnauthorized) {
			t.Errorf("err = %v, want %v", err, guild.ErrUnauthorized)
		}
	})

	t.Run("Success promote to officer", func(t *testing.T) {
		updated := false
		repo.updateMemberRoleFn = func(_ context.Context, guildID, target string, r guild.Role) error {
			if target == "member1" && r == guild.RoleOfficer {
				updated = true
			}
			return nil
		}
		if err := svc.UpdateRole(ctx, "g1", "leader1", "member1", guild.RoleOfficer); err != nil {
			t.Fatalf("UpdateRole() unexpected error: %v", err)
		}
		if !updated {
			t.Error("expected update member role to be called")
		}
	})
}

func TestService_Donate(t *testing.T) {
	ctx := context.Background()

	repo := &mockGuildRepo{
		getGuildByCharFn: func(_ context.Context, charID string) (guild.Guild, guild.Member, error) {
			return guild.Guild{ID: "g1", Level: 1, Exp: 5000, Gold: 1000}, guild.Member{GuildID: "g1", CharacterID: charID}, nil
		},
	}
	svc, _ := guild.NewService(repo)

	t.Run("Invalid donation amount", func(t *testing.T) {
		_, _, _, err := svc.Donate(ctx, "g1", "char1", 0)
		if !errors.Is(err, guild.ErrInvalidDonationAmount) {
			t.Errorf("err = %v, want %v", err, guild.ErrInvalidDonationAmount)
		}
	})

	t.Run("Success with donation delegation", func(t *testing.T) {
		var passedAmount int
		repo.donateFn = func(_ context.Context, gID, cID string, amount int) (guild.Guild, guild.Member, corecharacter.Character, error) {
			passedAmount = amount
			return guild.Guild{ID: gID, Level: 2, Exp: 10000}, guild.Member{GuildID: gID, CharacterID: cID}, corecharacter.Character{ID: cID}, nil
		}

		g, _, _, err := svc.Donate(ctx, "g1", "char1", 5000)
		if err != nil {
			t.Fatalf("Donate() unexpected error: %v", err)
		}
		if passedAmount != 5000 || g.Level != 2 || g.Exp != 10000 {
			t.Errorf("got amount %d, level %d, exp %d; want 5000, 2, 10000", passedAmount, g.Level, g.Exp)
		}
	})
}
