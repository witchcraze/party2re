package database

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/guild"
	"github.com/witchcraze/party2re/internal/id"
)

func TestGuildRepositoryLifecycle(t *testing.T) {
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

	// 1. Create test characters
	leaderChar, err := CreateTestCharacter(ctx, db, "GuildLeader")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE characters SET money = ? WHERE id = ?", 10000, leaderChar.ID); err != nil {
		t.Fatal(err)
	}

	memberChar, err := CreateTestCharacter(ctx, db, "GuildMember")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE characters SET money = ? WHERE id = ?", 5000, memberChar.ID); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	guildID := fmt.Sprintf("g_%016x", time.Now().UnixNano())
	guildName := fmt.Sprintf("Knights_%d", time.Now().UnixNano()%1000000)

	g := guild.Guild{
		ID:                guildID,
		Name:              guildName,
		LeaderCharacterID: leaderChar.ID,
		Level:             1,
		Exp:               0,
		Gold:              0,
		Notice:            "Welcome to the guild",
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	creatorMember := guild.Member{
		GuildID:          guildID,
		CharacterID:      leaderChar.ID,
		Role:             guild.RoleLeader,
		JoinedAt:         now,
		TotalDonatedGold: 0,
	}

	// 2. Create Guild (fee 5000)
	createdG, createdM, updatedLeader, err := guildRepo.CreateGuild(ctx, g, creatorMember, 5000)
	if err != nil {
		t.Fatalf("CreateGuild failed: %v", err)
	}
	if createdG.ID != guildID || createdM.Role != guild.RoleLeader {
		t.Errorf("unexpected created guild/member: %+v, %+v", createdG, createdM)
	}
	if updatedLeader.Money != 5000 {
		t.Errorf("leader remaining money = %d, want 5000", updatedLeader.Money)
	}

	// 3. Duplicate guild name should fail
	dupGuildID := fmt.Sprintf("dup_%016x", time.Now().UnixNano())
	dupG := guild.Guild{
		ID:                dupGuildID,
		Name:              guildName,
		LeaderCharacterID: memberChar.ID,
		Level:             1,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	dupM := guild.Member{
		GuildID:     dupGuildID,
		CharacterID: memberChar.ID,
		Role:        guild.RoleLeader,
		JoinedAt:    now,
	}
	if _, _, _, err := guildRepo.CreateGuild(ctx, dupG, dupM, 5000); !errors.Is(err, guild.ErrGuildNameTaken) {
		t.Errorf("duplicate name err = %v, want %v", err, guild.ErrGuildNameTaken)
	}

	// 4. GetGuild & GetGuildByCharacter
	fetchedG, members, err := guildRepo.GetGuild(ctx, guildID)
	if err != nil {
		t.Fatalf("GetGuild failed: %v", err)
	}
	if fetchedG.Name != guildName || len(members) != 1 {
		t.Errorf("fetched guild = %+v, members len = %d", fetchedG, len(members))
	}

	charGuild, charM, err := guildRepo.GetGuildByCharacter(ctx, leaderChar.ID)
	if err != nil {
		t.Fatalf("GetGuildByCharacter failed: %v", err)
	}
	if charGuild.ID != guildID || charM.Role != guild.RoleLeader {
		t.Errorf("char guild = %+v, member = %+v", charGuild, charM)
	}

	// 5. Add Member
	newMember := guild.Member{
		GuildID:          guildID,
		CharacterID:      memberChar.ID,
		Role:             guild.RoleMember,
		JoinedAt:         now,
		TotalDonatedGold: 0,
	}
	addedM, err := guildRepo.AddMember(ctx, newMember)
	if err != nil {
		t.Fatalf("AddMember failed: %v", err)
	}
	if addedM.CharacterID != memberChar.ID || addedM.Role != guild.RoleMember {
		t.Errorf("added member = %+v", addedM)
	}

	// Adding same member again to another guild must fail (enforcing unique membership)
	anotherGuildID := fmt.Sprintf("oth_%016x", time.Now().UnixNano())
	if _, err := guildRepo.AddMember(ctx, guild.Member{
		GuildID:     anotherGuildID,
		CharacterID: memberChar.ID,
		Role:        guild.RoleMember,
		JoinedAt:    now,
	}); !errors.Is(err, guild.ErrCharacterAlreadyInGuild) {
		t.Errorf("duplicate member err = %v, want %v", err, guild.ErrCharacterAlreadyInGuild)
	}

	// 6. Update Member Role
	if err := guildRepo.UpdateMemberRole(ctx, guildID, memberChar.ID, guild.RoleOfficer); err != nil {
		t.Fatalf("UpdateMemberRole failed: %v", err)
	}
	_, updatedM, err := guildRepo.GetGuildByCharacter(ctx, memberChar.ID)
	if err != nil || updatedM.Role != guild.RoleOfficer {
		t.Errorf("updated member role = %v, err = %v", updatedM.Role, err)
	}

	// 7. Update Notice
	if err := guildRepo.UpdateNotice(ctx, guildID, "Updated Guild Notice!"); err != nil {
		t.Fatalf("UpdateNotice failed: %v", err)
	}
	gNotice, _, _ := guildRepo.GetGuild(ctx, guildID)
	if gNotice.Notice != "Updated Guild Notice!" {
		t.Errorf("notice = %q, want %q", gNotice.Notice, "Updated Guild Notice!")
	}

	// 8. Donate Gold
	donatedG, donatedM, updatedDonor, err := guildRepo.Donate(ctx, guildID, memberChar.ID, 2000)
	if err != nil {
		t.Fatalf("Donate failed: %v", err)
	}
	if donatedG.Gold != 2000 || donatedG.Exp != 2000 || donatedM.TotalDonatedGold != 2000 {
		t.Errorf("unexpected donation state: guild = %+v, member = %+v", donatedG, donatedM)
	}
	if updatedDonor.Money != 3000 {
		t.Errorf("donor remaining money = %d, want 3000", updatedDonor.Money)
	}

	// 9. Transfer Leadership
	if err := guildRepo.TransferLeadership(ctx, guildID, leaderChar.ID, memberChar.ID); err != nil {
		t.Fatalf("TransferLeadership failed: %v", err)
	}
	gAfterTransfer, membersAfterTransfer, _ := guildRepo.GetGuild(ctx, guildID)
	if gAfterTransfer.LeaderCharacterID != memberChar.ID {
		t.Errorf("leader = %q, want %q", gAfterTransfer.LeaderCharacterID, memberChar.ID)
	}
	for _, m := range membersAfterTransfer {
		if m.CharacterID == memberChar.ID && m.Role != guild.RoleLeader {
			t.Errorf("new leader role = %v, want leader", m.Role)
		}
		if m.CharacterID == leaderChar.ID && m.Role != guild.RoleOfficer {
			t.Errorf("former leader role = %v, want officer", m.Role)
		}
	}

	// 10. List Guilds
	guildList, err := guildRepo.ListGuilds(ctx, 0, 10)
	if err != nil {
		t.Fatalf("ListGuilds failed: %v", err)
	}
	if len(guildList) == 0 {
		t.Error("expected non-empty guild list")
	}

	// 11. Remove Member
	if err := guildRepo.RemoveMember(ctx, guildID, leaderChar.ID); err != nil {
		t.Fatalf("RemoveMember failed: %v", err)
	}
	if _, _, err := guildRepo.GetGuildByCharacter(ctx, leaderChar.ID); !errors.Is(err, guild.ErrCharacterNotInGuild) {
		t.Errorf("removed member err = %v, want %v", err, guild.ErrCharacterNotInGuild)
	}

	// 12. Disband Guild
	if err := guildRepo.DisbandGuild(ctx, guildID); err != nil {
		t.Fatalf("DisbandGuild failed: %v", err)
	}
	if _, _, err := guildRepo.GetGuild(ctx, guildID); !errors.Is(err, guild.ErrGuildNotFound) {
		t.Errorf("disbanded guild err = %v, want %v", err, guild.ErrGuildNotFound)
	}
}

func TestGuildRepository_ConcurrentDonation(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := OpenFromEnvironment()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	guildRepo, err := NewGuildRepository(db)
	if err != nil {
		t.Fatalf("failed to create guild repo: %v", err)
	}

	charRepo, err := NewCharacterRepository(db)
	if err != nil {
		t.Fatalf("failed to create character repo: %v", err)
	}

	leaderChar, err := CreateTestCharacter(ctx, db, "Donate Leader")
	if err != nil {
		t.Fatalf("failed to create leader char: %v", err)
	}
	leaderChar.Money = 10000
	_ = charRepo.Update(ctx, leaderChar)

	guildID := id.New()
	g, m, _, err := guildRepo.CreateGuild(ctx, guild.Guild{
		ID:                guildID,
		Name:              fmt.Sprintf("Concurrent Donators %s", guildID),
		LeaderCharacterID: leaderChar.ID,
		Level:             1,
		Exp:               0,
		Gold:              0,
		Notice:            "",
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	}, guild.Member{
		GuildID:          "",
		CharacterID:      leaderChar.ID,
		Role:             guild.RoleLeader,
		JoinedAt:         time.Now().UTC(),
		TotalDonatedGold: 0,
	}, 0)
	if err != nil {
		t.Fatalf("failed to create guild: %v", err)
	}
	_ = m

	// Create 4 members, each donating 500G concurrently
	const numMembers = 4
	const donationAmount = 500
	var memberIDs []string
	for i := 0; i < numMembers; i++ {
		memChar, err := CreateTestCharacter(ctx, db, fmt.Sprintf("Donor %d", i+1))
		if err != nil {
			t.Fatalf("failed to create member char %d: %v", i+1, err)
		}
		memChar.Money = 1000
		_ = charRepo.Update(ctx, memChar)

		_, err = guildRepo.AddMember(ctx, guild.Member{
			GuildID:          g.ID,
			CharacterID:      memChar.ID,
			Role:             guild.RoleMember,
			JoinedAt:         time.Now().UTC(),
			TotalDonatedGold: 0,
		})
		if err != nil {
			t.Fatalf("failed to add member %d: %v", i+1, err)
		}
		memberIDs = append(memberIDs, memChar.ID)
	}

	var wg sync.WaitGroup
	errs := make(chan error, numMembers)
	for _, mID := range memberIDs {
		wg.Add(1)
		go func(charID string) {
			defer wg.Done()
			_, _, _, err := guildRepo.Donate(ctx, g.ID, charID, donationAmount)
			errs <- err
		}(mID)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Errorf("donation returned error: %v", err)
		}
	}

	finalGuild, _, err := guildRepo.GetGuild(ctx, g.ID)
	if err != nil {
		t.Fatalf("GetGuild failed: %v", err)
	}

	expectedExp := int64(numMembers * donationAmount)
	expectedGold := int64(numMembers * donationAmount)
	if finalGuild.Exp != expectedExp {
		t.Errorf("final guild exp = %d, want %d", finalGuild.Exp, expectedExp)
	}
	if finalGuild.Gold != expectedGold {
		t.Errorf("final guild gold = %d, want %d", finalGuild.Gold, expectedGold)
	}
}
