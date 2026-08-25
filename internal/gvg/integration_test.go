package gvg_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/guild"
	"github.com/witchcraze/party2re/internal/gvg"
)

func TestGvGIntegrationMatchFlow(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()

	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	guildRepo, err := database.NewGuildRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	gvgRepo, err := database.NewGvGRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	battleEngine := corebattle.Engine{}

	service, err := gvg.NewService(gvgRepo, guildRepo, charRepo, battleEngine)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Create Guild Alpha (3 members)
	leaderA, err := database.CreateTestCharacter(ctx, db, "GvG Leader Alpha")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE characters SET money = ? WHERE id = ?", 10000, leaderA.ID); err != nil {
		t.Fatal(err)
	}
	memA1, err := database.CreateTestCharacter(ctx, db, "GvG Mem Alpha 1")
	if err != nil {
		t.Fatal(err)
	}
	memA2, err := database.CreateTestCharacter(ctx, db, "GvG Mem Alpha 2")
	if err != nil {
		t.Fatal(err)
	}

	guildAID := fmt.Sprintf("g_alpha_%012x", time.Now().UnixNano())
	guildAName := fmt.Sprintf("Alpha_%d", time.Now().UnixNano()%1000000)
	gA := guild.Guild{
		ID:                guildAID,
		Name:              guildAName,
		LeaderCharacterID: leaderA.ID,
		Level:             1,
		Notice:            "Alpha Guild Notice",
	}
	memA := guild.Member{
		GuildID:     guildAID,
		CharacterID: leaderA.ID,
		Role:        guild.RoleLeader,
	}
	if _, _, _, err := guildRepo.CreateGuild(ctx, gA, memA, 5000); err != nil {
		t.Fatalf("create guild A: %v", err)
	}
	if _, err := guildRepo.AddMember(ctx, guild.Member{GuildID: guildAID, CharacterID: memA1.ID, Role: guild.RoleOfficer}); err != nil {
		t.Fatalf("add memA1: %v", err)
	}
	if _, err := guildRepo.AddMember(ctx, guild.Member{GuildID: guildAID, CharacterID: memA2.ID, Role: guild.RoleMember}); err != nil {
		t.Fatalf("add memA2: %v", err)
	}

	// 2. Create Guild Beta (2 members)
	leaderB, err := database.CreateTestCharacter(ctx, db, "GvG Leader Beta")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE characters SET money = ? WHERE id = ?", 10000, leaderB.ID); err != nil {
		t.Fatal(err)
	}
	memB1, err := database.CreateTestCharacter(ctx, db, "GvG Mem Beta 1")
	if err != nil {
		t.Fatal(err)
	}

	guildBID := fmt.Sprintf("g_beta_%012x", time.Now().UnixNano())
	guildBName := fmt.Sprintf("Beta_%d", time.Now().UnixNano()%1000000)
	gB := guild.Guild{
		ID:                guildBID,
		Name:              guildBName,
		LeaderCharacterID: leaderB.ID,
		Level:             1,
		Notice:            "Beta Guild Notice",
	}
	memB := guild.Member{
		GuildID:     guildBID,
		CharacterID: leaderB.ID,
		Role:        guild.RoleLeader,
	}
	if _, _, _, err := guildRepo.CreateGuild(ctx, gB, memB, 5000); err != nil {
		t.Fatalf("create guild B: %v", err)
	}
	if _, err := guildRepo.AddMember(ctx, guild.Member{GuildID: guildBID, CharacterID: memB1.ID, Role: guild.RoleMember}); err != nil {
		t.Fatalf("add memB1: %v", err)
	}

	// 3. Find Opponents
	opponents, err := service.FindOpponentGuilds(ctx, guildAID, 10)
	if err != nil {
		t.Fatalf("FindOpponentGuilds: %v", err)
	}
	if len(opponents) == 0 {
		t.Fatalf("expected opponents list")
	}

	// 4. Officer declares match against Guild Beta
	res, err := service.DeclareMatch(ctx, memA1.ID, guildBID)
	if err != nil {
		t.Fatalf("DeclareMatch: %v", err)
	}

	if res.Match.TotalRounds != 2 {
		t.Errorf("expected 2 rounds, got %d", res.Match.TotalRounds)
	}
	if res.Match.WinnerGuildID == "" && res.Match.ChallengerScore != res.Match.DefenderScore {
		t.Errorf("unexpected match result: %#v", res.Match)
	}

	// 5. Verify Standings
	standingA, err := service.GetStanding(ctx, guildAID)
	if err != nil {
		t.Fatalf("GetStanding A: %v", err)
	}
	standingB, err := service.GetStanding(ctx, guildBID)
	if err != nil {
		t.Fatalf("GetStanding B: %v", err)
	}

	if standingA.Rating == 1000 && standingB.Rating == 1000 && res.Match.ChallengerScore != res.Match.DefenderScore {
		t.Errorf("expected rating update from decisive match")
	}

	// 6. Verify Match History & Detail
	history, err := service.GetMatchHistory(ctx, guildAID, 5)
	if err != nil || len(history) == 0 {
		t.Fatalf("GetMatchHistory: %v", err)
	}

	detail, err := service.GetMatchDetail(ctx, res.Match.ID)
	if err != nil {
		t.Fatalf("GetMatchDetail: %v", err)
	}
	if len(detail.Rounds) != 2 {
		t.Errorf("expected 2 rounds in detail, got %d", len(detail.Rounds))
	}
}
