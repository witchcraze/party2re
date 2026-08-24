package guild_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/guild"
)

func TestGuildServiceDatabaseIntegration(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	guildRepo, err := database.NewGuildRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	service, err := guild.NewService(guildRepo)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// Create test characters
	leaderChar, err := database.CreateTestCharacter(ctx, db, "SvcGuildLeader")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE characters SET money = ? WHERE id = ?", 20000, leaderChar.ID); err != nil {
		t.Fatal(err)
	}

	memberChar1, err := database.CreateTestCharacter(ctx, db, "SvcGuildMember1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE characters SET money = ? WHERE id = ?", 15000, memberChar1.ID); err != nil {
		t.Fatal(err)
	}

	memberChar2, err := database.CreateTestCharacter(ctx, db, "SvcGuildMember2")
	if err != nil {
		t.Fatal(err)
	}

	guildName := fmt.Sprintf("Integ_%s", leaderChar.ID[:8])

	// 1. Create Guild
	g, leaderM, _, err := service.Create(ctx, leaderChar.ID, guildName)
	if err != nil {
		t.Fatalf("service.Create failed: %v", err)
	}
	if g.Name != guildName || leaderM.Role != guild.RoleLeader {
		t.Errorf("unexpected guild/member: %+v, %+v", g, leaderM)
	}

	// 2. Member1 joins
	m1, err := service.Join(ctx, g.ID, memberChar1.ID)
	if err != nil {
		t.Fatalf("service.Join failed: %v", err)
	}
	if m1.Role != guild.RoleMember {
		t.Errorf("member1 role = %v, want member", m1.Role)
	}

	// 3. Leader promotes Member1 to Officer
	if err := service.UpdateRole(ctx, g.ID, leaderChar.ID, memberChar1.ID, guild.RoleOfficer); err != nil {
		t.Fatalf("service.UpdateRole failed: %v", err)
	}

	// 4. Member2 joins
	_, err = service.Join(ctx, g.ID, memberChar2.ID)
	if err != nil {
		t.Fatalf("service.Join (member2) failed: %v", err)
	}

	// 5. Member1 (Officer) updates notice
	if err := service.UpdateNotice(ctx, g.ID, memberChar1.ID, "Officer notice update"); err != nil {
		t.Fatalf("service.UpdateNotice failed: %v", err)
	}

	// 6. Member1 (Officer) kicks Member2
	if err := service.Kick(ctx, g.ID, memberChar1.ID, memberChar2.ID); err != nil {
		t.Fatalf("service.Kick failed: %v", err)
	}

	// 7. Member1 donates 10000 gold -> triggers level up to Level 2
	donatedG, _, _, err := service.Donate(ctx, g.ID, memberChar1.ID, 10000)
	if err != nil {
		t.Fatalf("service.Donate failed: %v", err)
	}
	if donatedG.Level != 2 || donatedG.Exp != 10000 {
		t.Errorf("donated guild level = %d, exp = %d; want level 2, exp 10000", donatedG.Level, donatedG.Exp)
	}

	// 8. Transfer leadership from Leader to Member1
	if err := service.TransferLeadership(ctx, g.ID, leaderChar.ID, memberChar1.ID); err != nil {
		t.Fatalf("service.TransferLeadership failed: %v", err)
	}

	// 9. Former leader leaves guild
	if err := service.Leave(ctx, g.ID, leaderChar.ID); err != nil {
		t.Fatalf("service.Leave failed: %v", err)
	}

	// 10. Sole leader (Member1) leaves -> disbands guild
	if err := service.Leave(ctx, g.ID, memberChar1.ID); err != nil {
		t.Fatalf("service.Leave (sole leader disband) failed: %v", err)
	}

	// 11. Verify guild is deleted
	if _, err := service.Get(ctx, g.ID); err == nil {
		t.Error("expected error getting disbanded guild, got nil")
	}
}
