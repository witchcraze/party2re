package database

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/guild"
	"github.com/witchcraze/party2re/internal/id"
)

func TestGuildRepository_ApplicationsAndApprovals(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	guildRepo, err := NewGuildRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	leaderChar, err := CreateTestCharacter(ctx, db, "LeaderApp")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = db.ExecContext(ctx, "UPDATE characters SET money = 10000 WHERE id = ?", leaderChar.ID)

	appChar1, err := CreateTestCharacter(ctx, db, "Applicant1")
	if err != nil {
		t.Fatal(err)
	}

	appChar2, err := CreateTestCharacter(ctx, db, "Applicant2")
	if err != nil {
		t.Fatal(err)
	}

	guildID := id.New()
	g, _, _, err := guildRepo.CreateGuild(ctx, guild.Guild{
		ID:                guildID,
		Name:              fmt.Sprintf("AppGuild_%s", guildID[:8]),
		LeaderCharacterID: leaderChar.ID,
		Points:            0,
		Notice:            "Test Notice",
		Color:             guild.DefaultColor,
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	}, guild.Member{
		GuildID:     guildID,
		CharacterID: leaderChar.ID,
		Role:        guild.RoleLeader,
		Title:       guild.DefaultTitleLeader,
		JoinedAt:    time.Now().UTC(),
	}, 0)
	if err != nil {
		t.Fatalf("CreateGuild failed: %v", err)
	}

	// 1. Applicant 1 applies
	_, err = guildRepo.AddMember(ctx, guild.Member{
		GuildID:     g.ID,
		CharacterID: appChar1.ID,
		Role:        guild.RoleMember,
		Title:       guild.DefaultTitlePending,
		IsPending:   true,
		JoinedAt:    time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("AddMember (applicant 1) failed: %v", err)
	}

	findMember := func(members []guild.Member, charID string) *guild.Member {
		for i := range members {
			if members[i].CharacterID == charID {
				return &members[i]
			}
		}
		return nil
	}

	// Verify applicant 1 is pending
	_, members, err := guildRepo.GetGuild(ctx, g.ID)
	if err != nil {
		t.Fatalf("GetGuild failed: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(members))
	}
	m1 := findMember(members, appChar1.ID)
	if m1 == nil || !m1.IsPending || m1.Title != guild.DefaultTitlePending {
		t.Errorf("expected applicant 1 to be pending with %q, got %+v",
			guild.DefaultTitlePending, m1)
	}

	// 2. Leader approves Applicant 1 with role title
	err = guildRepo.ApproveMember(ctx, g.ID, appChar1.ID, "親衛隊長")
	if err != nil {
		t.Fatalf("ApproveMember failed: %v", err)
	}

	// Verify Applicant 1 is now active
	_, members, err = guildRepo.GetGuild(ctx, g.ID)
	if err != nil {
		t.Fatalf("GetGuild failed: %v", err)
	}
	m1 = findMember(members, appChar1.ID)
	if m1 == nil || m1.IsPending || m1.Title != "親衛隊長" {
		t.Errorf("expected applicant 1 to be approved with %q, got %+v",
			"親衛隊長", m1)
	}

	// Re-approving active member should fail with ErrMemberNotPending
	err = guildRepo.ApproveMember(ctx, g.ID, appChar1.ID, "副隊長")
	if !errors.Is(err, guild.ErrMemberNotPending) {
		t.Errorf("expected ErrMemberNotPending on already approved member, got %v", err)
	}

	// 3. Applicant 2 applies and is rejected (removed)
	_, err = guildRepo.AddMember(ctx, guild.Member{
		GuildID:     g.ID,
		CharacterID: appChar2.ID,
		Role:        guild.RoleMember,
		Title:       guild.DefaultTitlePending,
		IsPending:   true,
		JoinedAt:    time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("AddMember (applicant 2) failed: %v", err)
	}

	err = guildRepo.RemoveMember(ctx, g.ID, appChar2.ID)
	if err != nil {
		t.Fatalf("RemoveMember (reject) failed: %v", err)
	}

	// Verify applicant 2 is no longer in guild
	_, _, err = guildRepo.GetGuildByCharacter(ctx, appChar2.ID)
	if !errors.Is(err, guild.ErrCharacterNotInGuild) {
		t.Errorf("expected ErrCharacterNotInGuild after rejection, got %v", err)
	}
}

func TestGuildRepository_CustomizationMarkAndWallpaper(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	guildRepo, err := NewGuildRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	leaderChar, err := CreateTestCharacter(ctx, db, "LeaderCust")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = db.ExecContext(ctx, "UPDATE characters SET money = 10000 WHERE id = ?", leaderChar.ID)

	guildID := id.New()
	g, _, _, err := guildRepo.CreateGuild(ctx, guild.Guild{
		ID:                guildID,
		Name:              fmt.Sprintf("CustGuild_%s", guildID[:8]),
		LeaderCharacterID: leaderChar.ID,
		Points:            0,
		Notice:            "",
		Color:             guild.DefaultColor,
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	}, guild.Member{
		GuildID:     guildID,
		CharacterID: leaderChar.ID,
		Role:        guild.RoleLeader,
		Title:       guild.DefaultTitleLeader,
		JoinedAt:    time.Now().UTC(),
	}, 0)
	if err != nil {
		t.Fatalf("CreateGuild failed: %v", err)
	}

	// 1. Update Mark (3,000G fee)
	updatedChar, err := guildRepo.UpdateMark(ctx, g.ID, "42", 3000, leaderChar.ID)
	if err != nil {
		t.Fatalf("UpdateMark failed: %v", err)
	}
	if updatedChar.Money != 7000 {
		t.Errorf("expected leader money 7000, got %d", updatedChar.Money)
	}

	fetchedGuild, _, err := guildRepo.GetGuild(ctx, g.ID)
	if err != nil {
		t.Fatalf("GetGuild failed: %v", err)
	}
	if fetchedGuild.Mark != "42" {
		t.Errorf("expected guild mark %q, got %q", "42", fetchedGuild.Mark)
	}

	// 2. Update Wallpaper (6,000G fee)
	updatedChar, err = guildRepo.UpdateWallpaper(ctx, g.ID, "stage0.gif", 6000, leaderChar.ID)
	if err != nil {
		t.Fatalf("UpdateWallpaper failed: %v", err)
	}
	if updatedChar.Money != 1000 {
		t.Errorf("expected leader money 1000, got %d", updatedChar.Money)
	}

	fetchedGuild, _, err = guildRepo.GetGuild(ctx, g.ID)
	if err != nil {
		t.Fatalf("GetGuild failed: %v", err)
	}
	if fetchedGuild.Bgimg != "stage0.gif" {
		t.Errorf("expected guild bgimg %q, got %q", "stage0.gif", fetchedGuild.Bgimg)
	}

	// 3. Insufficient funds test
	_, err = guildRepo.UpdateMark(ctx, g.ID, "99", 3000, leaderChar.ID)
	if !errors.Is(err, guild.ErrInsufficientFunds) {
		t.Errorf("expected ErrInsufficientFunds, got %v", err)
	}
}

func TestGuildRepository_InactivityScanAndDisband(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	guildRepo, err := NewGuildRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	leaderChar, err := CreateTestCharacter(ctx, db, "LeaderInactive")
	if err != nil {
		t.Fatal(err)
	}

	guildID := id.New()
	g, _, _, err := guildRepo.CreateGuild(ctx, guild.Guild{
		ID:                guildID,
		Name:              fmt.Sprintf("InactiveGuild_%s", guildID[:8]),
		LeaderCharacterID: leaderChar.ID,
		Points:            0,
		Notice:            "",
		Color:             guild.DefaultColor,
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	}, guild.Member{
		GuildID:     guildID,
		CharacterID: leaderChar.ID,
		Role:        guild.RoleLeader,
		Title:       guild.DefaultTitleLeader,
		JoinedAt:    time.Now().UTC(),
	}, 0)
	if err != nil {
		t.Fatalf("CreateGuild failed: %v", err)
	}

	// Manually set last_active_at to 25 days ago
	oldTime := time.Now().UTC().Add(-25 * 24 * time.Hour)
	_, err = db.ExecContext(ctx, "UPDATE guilds SET last_active_at = ? WHERE id = ?", oldTime, g.ID)
	if err != nil {
		t.Fatalf("failed to update last_active_at: %v", err)
	}

	// Scan with cutoff 20 days ago -> should find our guild
	cutoff := time.Now().UTC().Add(-20 * 24 * time.Hour)
	inactive, err := guildRepo.ListInactiveGuilds(ctx, cutoff, 100)
	if err != nil {
		t.Fatalf("ListInactiveGuilds failed: %v", err)
	}
	found := false
	for _, in := range inactive {
		if in.ID == g.ID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected guild %s to be listed as inactive", g.ID)
	}

	// TouchActive updates last_active_at to now
	err = guildRepo.TouchActive(ctx, g.ID)
	if err != nil {
		t.Fatalf("TouchActive failed: %v", err)
	}

	// Now scan again -> should not find our guild
	inactive, err = guildRepo.ListInactiveGuilds(ctx, cutoff, 100)
	if err != nil {
		t.Fatalf("ListInactiveGuilds after touch failed: %v", err)
	}
	for _, in := range inactive {
		if in.ID == g.ID {
			t.Errorf("guild %s should no longer be inactive after TouchActive", g.ID)
		}
	}

	// DisbandGuild cleans up
	err = guildRepo.DisbandGuild(ctx, g.ID)
	if err != nil {
		t.Fatalf("DisbandGuild failed: %v", err)
	}

	_, _, err = guildRepo.GetGuild(ctx, g.ID)
	if !errors.Is(err, guild.ErrGuildNotFound) {
		t.Errorf("expected ErrGuildNotFound after DisbandGuild, got %v", err)
	}
}
