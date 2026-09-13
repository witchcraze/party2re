package guild_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

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
	if g.Name != guildName || leaderM.Role != guild.RoleLeader || leaderM.Title != guild.DefaultTitleLeader {
		t.Errorf("unexpected guild/member: %+v, %+v", g, leaderM)
	}
	if g.Points != 0 || g.Color != guild.DefaultColor {
		t.Errorf("expected points 0, color #FFFFFF, got points %d, color %s", g.Points, g.Color)
	}

	// 2. Member1 joins
	m1, err := service.Join(ctx, g.ID, memberChar1.ID)
	if err != nil {
		t.Fatalf("service.Join failed: %v", err)
	}
	if m1.Role != guild.RoleMember || m1.Title != "" {
		t.Errorf("member1 role = %v, title = %q; want member, ''", m1.Role, m1.Title)
	}

	// 3. Leader assigns custom role title to Member1 (あたえる: 親衛隊長)
	if err := service.AssignCustomRole(ctx, g.ID, leaderChar.ID, memberChar1.ID, "親衛隊長"); err != nil {
		t.Fatalf("service.AssignCustomRole failed: %v", err)
	}
	_, m1Updated, err := service.GetByCharacter(ctx, memberChar1.ID)
	if err != nil || m1Updated.Title != "親衛隊長" {
		t.Fatalf("expected title '親衛隊長', got %q, err = %v", m1Updated.Title, err)
	}

	// 4. Leader updates guild color (からー)
	testColor := fmt.Sprintf("#%06X", (time.Now().UnixNano()/1000)%0xFFFFFF)
	if testColor == guild.DefaultColor || testColor == guild.NPCColor {
		testColor = "#FF3333"
	}
	if err := service.UpdateColor(ctx, g.ID, leaderChar.ID, testColor); err != nil {
		t.Fatalf("service.UpdateColor failed: %v", err)
	}
	detail, err := service.Get(ctx, g.ID)
	if err != nil || detail.Guild.Color != testColor {
		t.Fatalf("expected color %s, got %s, err = %v", testColor, detail.Guild.Color, err)
	}

	// 5. Member2 joins
	_, err = service.Join(ctx, g.ID, memberChar2.ID)
	if err != nil {
		t.Fatalf("service.Join (member2) failed: %v", err)
	}

	// 6. Leader updates notice
	if err := service.UpdateNotice(ctx, g.ID, leaderChar.ID, "Leader notice update"); err != nil {
		t.Fatalf("service.UpdateNotice failed: %v", err)
	}

	// 7. Leader kicks Member2
	if err := service.Kick(ctx, g.ID, leaderChar.ID, memberChar2.ID); err != nil {
		t.Fatalf("service.Kick failed: %v", err)
	}

	// 8. Add Guild Points dynamically
	if err := service.AddGuildPoints(ctx, memberChar1.ID, 100); err != nil {
		t.Fatalf("service.AddGuildPoints failed: %v", err)
	}
	gWithPoints, _, err := service.GetByCharacter(ctx, leaderChar.ID)
	if err != nil || gWithPoints.Points != 100 {
		t.Fatalf("expected points 100, got %d, err = %v", gWithPoints.Points, err)
	}

	// 9. Transfer leadership from Leader to Member1
	if err := service.TransferLeadership(ctx, g.ID, leaderChar.ID, memberChar1.ID); err != nil {
		t.Fatalf("service.TransferLeadership failed: %v", err)
	}

	// 10. Former leader leaves guild
	if err := service.Leave(ctx, g.ID, leaderChar.ID); err != nil {
		t.Fatalf("service.Leave failed: %v", err)
	}

	// 11. Sole leader (Member1) leaves -> disbands guild
	if err := service.Leave(ctx, g.ID, memberChar1.ID); err != nil {
		t.Fatalf("service.Leave (sole leader disband) failed: %v", err)
	}

	// 12. Verify guild is deleted
	if _, err := service.Get(ctx, g.ID); err == nil {
		t.Error("expected error getting disbanded guild, got nil")
	}
}
