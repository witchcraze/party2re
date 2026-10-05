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
		Points:            0,
		Notice:            "Welcome to the guild",
		Color:             guild.DefaultColor,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	creatorMember := guild.Member{
		GuildID:     guildID,
		CharacterID: leaderChar.ID,
		Role:        guild.RoleLeader,
		Title:       guild.DefaultTitleLeader,
		JoinedAt:    now,
	}

	// 2. Create Guild (fee 5000)
	createdG, createdM, updatedLeader, err := guildRepo.CreateGuild(ctx, g, creatorMember, 5000)
	if err != nil {
		t.Fatalf("CreateGuild failed: %v", err)
	}
	if createdG.ID != guildID || createdM.Role != guild.RoleLeader || createdM.Title != guild.DefaultTitleLeader {
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
		Points:            0,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	dupM := guild.Member{
		GuildID:     dupGuildID,
		CharacterID: memberChar.ID,
		Role:        guild.RoleLeader,
		Title:       guild.DefaultTitleLeader,
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
		GuildID:     guildID,
		CharacterID: memberChar.ID,
		Role:        guild.RoleMember,
		Title:       "",
		JoinedAt:    now,
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
		Title:       "",
		JoinedAt:    now,
	}); !errors.Is(err, guild.ErrCharacterAlreadyInGuild) {
		t.Errorf("duplicate member err = %v, want %v", err, guild.ErrCharacterAlreadyInGuild)
	}

	// 6. Assign Custom Role Title
	if err := guildRepo.AssignCustomRole(ctx, guildID, memberChar.ID, "親衛隊長"); err != nil {
		t.Fatalf("AssignCustomRole failed: %v", err)
	}
	_, updatedM, err := guildRepo.GetGuildByCharacter(ctx, memberChar.ID)
	if err != nil || updatedM.Title != "親衛隊長" {
		t.Errorf("updated member title = %v, err = %v", updatedM.Title, err)
	}

	// 7. Update Notice
	if err := guildRepo.UpdateNotice(ctx, guildID, "Updated Guild Notice!"); err != nil {
		t.Fatalf("UpdateNotice failed: %v", err)
	}
	gNotice, _, _ := guildRepo.GetGuild(ctx, guildID)
	if gNotice.Notice != "Updated Guild Notice!" {
		t.Errorf("notice = %q, want %q", gNotice.Notice, "Updated Guild Notice!")
	}

	// 8. Add Guild Points
	if err := guildRepo.AddPoints(ctx, guildID, 500); err != nil {
		t.Fatalf("AddPoints failed: %v", err)
	}
	if err := guildRepo.AddGuildPoints(ctx, memberChar.ID, 200); err != nil {
		t.Fatalf("AddGuildPoints failed: %v", err)
	}
	gPoints, _, _ := guildRepo.GetGuild(ctx, guildID)
	if gPoints.Points != 700 {
		t.Errorf("expected points 700, got %d", gPoints.Points)
	}

	// 9. Update Color and verify uniqueness check
	testColor := fmt.Sprintf("#%06X", (time.Now().UnixNano()/1000)%0xFFFFFF)
	if err := guildRepo.UpdateColor(ctx, guildID, testColor); err != nil {
		t.Fatalf("UpdateColor failed: %v", err)
	}
	taken, err := guildRepo.IsColorTaken(ctx, testColor, "")
	if err != nil || !taken {
		t.Fatalf("IsColorTaken expected true, got %v, err %v", taken, err)
	}
	takenExcludeSelf, err := guildRepo.IsColorTaken(ctx, testColor, guildID)
	if err != nil || takenExcludeSelf {
		t.Fatalf("IsColorTaken excluding self expected false, got %v, err %v", takenExcludeSelf, err)
	}

	// 10. Transfer Leadership
	if err := guildRepo.TransferLeadership(ctx, guildID, leaderChar.ID, memberChar.ID); err != nil {
		t.Fatalf("TransferLeadership failed: %v", err)
	}
	gAfterTransfer, membersAfterTransfer, _ := guildRepo.GetGuild(ctx, guildID)
	if gAfterTransfer.LeaderCharacterID != memberChar.ID {
		t.Errorf("leader = %q, want %q", gAfterTransfer.LeaderCharacterID, memberChar.ID)
	}
	for _, m := range membersAfterTransfer {
		if m.CharacterID == memberChar.ID && (m.Role != guild.RoleLeader || m.Title != guild.DefaultTitleLeader) {
			t.Errorf("new leader role/title = %v / %v, want leader / ギルマス", m.Role, m.Title)
		}
		if m.CharacterID == leaderChar.ID && (m.Role != guild.RoleMember || m.Title != "") {
			t.Errorf("former leader role/title = %v / %v, want member / ''", m.Role, m.Title)
		}
	}

	// 11. List Guilds (ordered by points DESC)
	guildList, err := guildRepo.ListGuilds(ctx, 0, 10)
	if err != nil {
		t.Fatalf("ListGuilds failed: %v", err)
	}
	if len(guildList) == 0 {
		t.Error("expected non-empty guild list")
	}

	// 12. Remove Member
	if err := guildRepo.RemoveMember(ctx, guildID, leaderChar.ID); err != nil {
		t.Fatalf("RemoveMember failed: %v", err)
	}
	if _, _, err := guildRepo.GetGuildByCharacter(ctx, leaderChar.ID); !errors.Is(err, guild.ErrCharacterNotInGuild) {
		t.Errorf("removed member err = %v, want %v", err, guild.ErrCharacterNotInGuild)
	}

	// 13. Disband Guild
	if err := guildRepo.DisbandGuild(ctx, guildID); err != nil {
		t.Fatalf("DisbandGuild failed: %v", err)
	}
	if _, _, err := guildRepo.GetGuild(ctx, guildID); !errors.Is(err, guild.ErrGuildNotFound) {
		t.Errorf("disbanded guild err = %v, want %v", err, guild.ErrGuildNotFound)
	}
}

func TestGuildRepository_ConcurrentPoints(t *testing.T) {
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

	leaderChar, err := CreateTestCharacter(ctx, db, "Points Leader")
	if err != nil {
		t.Fatalf("failed to create leader char: %v", err)
	}

	guildID := id.New()
	g, m, _, err := guildRepo.CreateGuild(ctx, guild.Guild{
		ID:                guildID,
		Name:              fmt.Sprintf("Concurrent Points %s", guildID),
		LeaderCharacterID: leaderChar.ID,
		Points:            0,
		Notice:            "",
		Color:             guild.DefaultColor,
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	}, guild.Member{
		GuildID:     "",
		CharacterID: leaderChar.ID,
		Role:        guild.RoleLeader,
		Title:       guild.DefaultTitleLeader,
		JoinedAt:    time.Now().UTC(),
	}, 0)
	if err != nil {
		t.Fatalf("failed to create guild: %v", err)
	}
	_ = m

	// Create 4 members, each adding 500 GP concurrently
	const numMembers = 4
	const pointsAmount = 500
	var memberIDs []string
	for i := 0; i < numMembers; i++ {
		memChar, err := CreateTestCharacter(ctx, db, fmt.Sprintf("Member %d", i+1))
		if err != nil {
			t.Fatalf("failed to create member char %d: %v", i+1, err)
		}
		_ = charRepo.Update(ctx, memChar)

		_, err = guildRepo.AddMember(ctx, guild.Member{
			GuildID:     g.ID,
			CharacterID: memChar.ID,
			Role:        guild.RoleMember,
			Title:       "",
			JoinedAt:    time.Now().UTC(),
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
			errs <- guildRepo.AddGuildPoints(ctx, charID, pointsAmount)
		}(mID)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Errorf("AddGuildPoints returned error: %v", err)
		}
	}

	finalGuild, _, err := guildRepo.GetGuild(ctx, g.ID)
	if err != nil {
		t.Fatalf("GetGuild failed: %v", err)
	}

	expectedPoints := int64(numMembers * pointsAmount)
	if finalGuild.Points != expectedPoints {
		t.Errorf("final guild points = %d, want %d", finalGuild.Points, expectedPoints)
	}
}

func TestGuildRepository_DecayGuildPoints(t *testing.T) {
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

	createGuildWithPoints := func(nameSuffix string, initialPoints int64) guild.Guild {
		char, err := CreateTestCharacter(ctx, db, "Char_"+nameSuffix)
		if err != nil {
			t.Fatal(err)
		}
		gID := fmt.Sprintf("g_decay_%s_%08x", nameSuffix, time.Now().UnixNano()%100000000)
		gName := fmt.Sprintf("DecayGuild_%s_%d", nameSuffix, time.Now().UnixNano()%1000000)
		now := time.Now().UTC()
		g := guild.Guild{
			ID:                gID,
			Name:              gName,
			LeaderCharacterID: char.ID,
			Points:            0,
			Notice:            "Test Notice",
			Color:             guild.DefaultColor,
			Mark:              guild.DefaultMark,
			LastActiveAt:      now,
			CreatedAt:         now,
			UpdatedAt:         now,
		}
		mem := guild.Member{
			GuildID:     gID,
			CharacterID: char.ID,
			Role:        guild.RoleLeader,
			Title:       guild.DefaultTitleLeader,
			JoinedAt:    now,
		}
		createdGuild, _, _, err := guildRepo.CreateGuild(ctx, g, mem, 0)
		if err != nil {
			t.Fatalf("failed to create guild %s: %v", nameSuffix, err)
		}
		// If initialPoints > 0, set points directly since CreateGuild sets 0
		if initialPoints > 0 {
			if err := guildRepo.AddPoints(ctx, createdGuild.ID, initialPoints); err != nil {
				t.Fatalf("failed to set initial points: %v", err)
			}
		}
		return createdGuild
	}

	g100 := createGuildWithPoints("100", 100)
	g9 := createGuildWithPoints("9", 9)
	g1 := createGuildWithPoints("1", 1)
	g0 := createGuildWithPoints("0", 0)

	// Execute decay with legacy factor 0.8
	if err := guildRepo.DecayGuildPoints(ctx, 0.8); err != nil {
		t.Fatalf("DecayGuildPoints failed: %v", err)
	}

	g100After, _, err := guildRepo.GetGuild(ctx, g100.ID)
	if err != nil {
		t.Fatalf("failed to get g100: %v", err)
	}
	if g100After.Points != 80 {
		t.Errorf("g100 points = %d, want 80", g100After.Points)
	}

	g9After, _, err := guildRepo.GetGuild(ctx, g9.ID)
	if err != nil {
		t.Fatalf("failed to get g9: %v", err)
	}
	if g9After.Points != 7 {
		t.Errorf("g9 points = %d, want 7", g9After.Points)
	}

	g1After, _, err := guildRepo.GetGuild(ctx, g1.ID)
	if err != nil {
		t.Fatalf("failed to get g1: %v", err)
	}
	if g1After.Points != 0 {
		t.Errorf("g1 points = %d, want 0", g1After.Points)
	}

	g0After, _, err := guildRepo.GetGuild(ctx, g0.ID)
	if err != nil {
		t.Fatalf("failed to get g0: %v", err)
	}
	if g0After.Points != 0 {
		t.Errorf("g0 points = %d, want 0", g0After.Points)
	}

	// Successive decay run
	if err := guildRepo.DecayGuildPoints(ctx, 0.8); err != nil {
		t.Fatalf("second DecayGuildPoints failed: %v", err)
	}
	g100Second, _, _ := guildRepo.GetGuild(ctx, g100.ID)
	if g100Second.Points != 64 {
		t.Errorf("g100 points after 2nd decay = %d, want 64", g100Second.Points)
	}
	g9Second, _, _ := guildRepo.GetGuild(ctx, g9.ID)
	if g9Second.Points != 5 {
		t.Errorf("g9 points after 2nd decay = %d, want 5", g9Second.Points)
	}
	g1Second, _, _ := guildRepo.GetGuild(ctx, g1.ID)
	if g1Second.Points != 0 {
		t.Errorf("g1 points after 2nd decay = %d, want 0", g1Second.Points)
	}
}

func TestGuildRepository_TransferLeadership_PendingApplicant(t *testing.T) {
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

	guildName := fmt.Sprintf("SuccG_%08x", time.Now().UnixNano()%0xFFFFFFFF)
	gCreated, leaderChar, err := CreateTestGuildWithLeader(ctx, db, guildName, 10000)
	if err != nil {
		t.Fatalf("CreateTestGuildWithLeader failed: %v", err)
	}
	applicantChar, err := CreateTestCharacter(ctx, db, "PendingSuccApp")
	if err != nil {
		t.Fatal(err)
	}
	guildID := gCreated.ID
	now := time.Now().UTC().Truncate(time.Second)

	// Add pending applicant
	_, err = guildRepo.AddMember(ctx, guild.Member{
		GuildID:     guildID,
		CharacterID: applicantChar.ID,
		Role:        guild.RoleMember,
		Title:       "参加申請中",
		IsPending:   true,
		JoinedAt:    now.Add(time.Second),
	})
	if err != nil {
		t.Fatalf("AddMember failed: %v", err)
	}

	// Transfer leadership to pending applicant
	if err := guildRepo.TransferLeadership(ctx, guildID, leaderChar.ID, applicantChar.ID); err != nil {
		t.Fatalf("TransferLeadership failed: %v", err)
	}

	gAfter, membersAfter, err := guildRepo.GetGuild(ctx, guildID)
	if err != nil {
		t.Fatalf("GetGuild failed: %v", err)
	}
	if gAfter.LeaderCharacterID != applicantChar.ID {
		t.Errorf("leader = %q, want %q", gAfter.LeaderCharacterID, applicantChar.ID)
	}

	for _, m := range membersAfter {
		if m.CharacterID == applicantChar.ID {
			if m.Role != guild.RoleLeader {
				t.Errorf("successor role = %v, want %v", m.Role, guild.RoleLeader)
			}
			if m.Title != guild.DefaultTitleLeader {
				t.Errorf("successor title = %q, want %q", m.Title, guild.DefaultTitleLeader)
			}
			if m.IsPending {
				t.Errorf("successor IsPending = true, want false")
			}
		}
		if m.CharacterID == leaderChar.ID {
			if m.Role != guild.RoleMember {
				t.Errorf("former leader role = %v, want %v", m.Role, guild.RoleMember)
			}
			if m.Title != "" {
				t.Errorf("former leader title = %q, want empty", m.Title)
			}
		}
	}
}

func TestGuildRepository_TransferLeadership_InvalidSuccessor(t *testing.T) {
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

	guildName := fmt.Sprintf("InvSuccG_%08x", time.Now().UnixNano()%0xFFFFFFFF)
	gCreated, leaderChar, err := CreateTestGuildWithLeader(ctx, db, guildName, 10000)
	if err != nil {
		t.Fatalf("CreateTestGuildWithLeader failed: %v", err)
	}

	// Create a real character who is NOT in the guild
	nonMemberChar, err := CreateTestCharacter(ctx, db, "NonMemberSucc")
	if err != nil {
		t.Fatal(err)
	}

	// Attempt transfer to a non-member successor
	err = guildRepo.TransferLeadership(ctx, gCreated.ID, leaderChar.ID, nonMemberChar.ID)
	if !errors.Is(err, guild.ErrTargetNotMember) {
		t.Fatalf("expected ErrTargetNotMember, got %v", err)
	}

	// Verify no partial leadership change occurred
	gAfter, membersAfter, err := guildRepo.GetGuild(ctx, gCreated.ID)
	if err != nil {
		t.Fatalf("GetGuild failed: %v", err)
	}
	if gAfter.LeaderCharacterID != leaderChar.ID {
		t.Errorf("leader = %q, want %q (should remain unchanged)", gAfter.LeaderCharacterID, leaderChar.ID)
	}
	if len(membersAfter) != 1 || membersAfter[0].CharacterID != leaderChar.ID || membersAfter[0].Role != guild.RoleLeader {
		t.Errorf("leader membership modified on failed transfer: %+v", membersAfter)
	}
}

func TestGuildRepository_TransferLeadership_StaleLeader(t *testing.T) {
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

	guildName := fmt.Sprintf("StaleLdrG_%08x", time.Now().UnixNano()%0xFFFFFFFF)
	gCreated, leaderChar, err := CreateTestGuildWithLeader(ctx, db, guildName, 10000)
	if err != nil {
		t.Fatalf("CreateTestGuildWithLeader failed: %v", err)
	}
	memberChar, err := CreateTestCharacter(ctx, db, "MemberForStale")
	if err != nil {
		t.Fatal(err)
	}
	_, err = guildRepo.AddMember(ctx, guild.Member{
		GuildID:     gCreated.ID,
		CharacterID: memberChar.ID,
		Role:        guild.RoleMember,
		Title:       "",
		JoinedAt:    time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}

	// Attempt transfer with incorrect/stale oldLeaderID
	staleLeaderID := "stale_char_" + id.New()[:8]
	err = guildRepo.TransferLeadership(ctx, gCreated.ID, staleLeaderID, memberChar.ID)
	if !errors.Is(err, guild.ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}

	// Verify no partial leadership change occurred
	gAfter, _, err := guildRepo.GetGuild(ctx, gCreated.ID)
	if err != nil {
		t.Fatalf("GetGuild failed: %v", err)
	}
	if gAfter.LeaderCharacterID != leaderChar.ID {
		t.Errorf("leader = %q, want %q (should remain unchanged)", gAfter.LeaderCharacterID, leaderChar.ID)
	}
}

func TestGuildRepository_GetGuildForUpdate(t *testing.T) {
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

	guildName := fmt.Sprintf("LockG_%08x", time.Now().UnixNano()%0xFFFFFFFF)
	gCreated, leaderChar, err := CreateTestGuildWithLeader(ctx, db, guildName, 10000)
	if err != nil {
		t.Fatalf("CreateTestGuildWithLeader failed: %v", err)
	}

	// Inside RunInTx, acquire Rank 7 lock
	err = RunInTx(ctx, db, func(txCtx context.Context) error {
		gLocked, members, err := guildRepo.GetGuildForUpdate(txCtx, gCreated.ID)
		if err != nil {
			return err
		}
		if gLocked.ID != gCreated.ID {
			t.Errorf("locked guild ID = %s, want %s", gLocked.ID, gCreated.ID)
		}
		if len(members) != 1 || members[0].CharacterID != leaderChar.ID {
			t.Errorf("unexpected members: %+v", members)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("GetGuildForUpdate in tx failed: %v", err)
	}

	// Non-existent guild returns ErrGuildNotFound
	_, _, err = guildRepo.GetGuildForUpdate(ctx, "nonexistent_guild_id")
	if !errors.Is(err, guild.ErrGuildNotFound) {
		t.Fatalf("expected ErrGuildNotFound, got %v", err)
	}
}
