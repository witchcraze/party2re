package database_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/guild"
	"github.com/witchcraze/party2re/internal/gvg"
)

func TestGvGRepository(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	guildRepo, err := database.NewGuildRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	gvgRepo, err := database.NewGvGRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Create Guild A
	charA, err := database.CreateTestCharacter(ctx, db, "GvG Leader A")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE characters SET money = ? WHERE id = ?", 10000, charA.ID); err != nil {
		t.Fatal(err)
	}
	guildA_ID := fmt.Sprintf("ga_%016x", time.Now().UnixNano())
	guildA_Name := fmt.Sprintf("GuildA_%d", time.Now().UnixNano()%1000000)
	gA := guild.Guild{
		ID:                guildA_ID,
		Name:              guildA_Name,
		LeaderCharacterID: charA.ID,
		Points:            0,
		Color:             "#FF3333",
		Notice:            "Guild A",
	}
	memA := guild.Member{
		GuildID:     guildA_ID,
		CharacterID: charA.ID,
		Role:        guild.RoleLeader,
	}
	if _, _, _, err := guildRepo.CreateGuild(ctx, gA, memA, 5000); err != nil {
		t.Fatalf("create guild A: %v", err)
	}

	// 2. Create Guild B
	charB, err := database.CreateTestCharacter(ctx, db, "GvG Leader B")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE characters SET money = ? WHERE id = ?", 10000, charB.ID); err != nil {
		t.Fatal(err)
	}
	guildB_ID := fmt.Sprintf("gb_%016x", time.Now().UnixNano())
	guildB_Name := fmt.Sprintf("GuildB_%d", time.Now().UnixNano()%1000000)
	gB := guild.Guild{
		ID:                guildB_ID,
		Name:              guildB_Name,
		LeaderCharacterID: charB.ID,
		Points:            0,
		Color:             "#6666FF",
		Notice:            "Guild B",
	}
	memB := guild.Member{
		GuildID:     guildB_ID,
		CharacterID: charB.ID,
		Role:        guild.RoleLeader,
	}
	if _, _, _, err := guildRepo.CreateGuild(ctx, gB, memB, 5000); err != nil {
		t.Fatalf("create guild B: %v", err)
	}

	// 3. Test GetOrCreateStanding
	stA, err := gvgRepo.GetOrCreateStanding(ctx, guildA_ID)
	if err != nil {
		t.Fatalf("GetOrCreateStanding guild A: %v", err)
	}
	if stA.Wins != 0 || stA.BronzeMedals != 0 || stA.VictoryPoints != 0 {
		t.Errorf("unexpected initial standing A: %#v", stA)
	}

	// 4. Test AddRoundWinGP
	if err := gvgRepo.AddRoundWinGP(ctx, guildA_ID, 3); err != nil {
		t.Fatalf("AddRoundWinGP: %v", err)
	}
	stA_afterRound, err := gvgRepo.GetOrCreateStanding(ctx, guildA_ID)
	if err != nil {
		t.Fatalf("GetOrCreateStanding after round win: %v", err)
	}
	if stA_afterRound.VictoryPoints != 3 {
		t.Errorf("expected 3 GP after round win, got %d", stA_afterRound.VictoryPoints)
	}

	// 5. Test RecordMatchSettlement (Win / Loss)
	settlement := gvg.MatchSettlement{
		WinnerGuildID: guildA_ID,
		WinnerPrizeGP: 3, // 3 GP room prize pool
		GuildIDs:      []string{guildA_ID, guildB_ID},
		IsDraw:        false,
		ParticipantGP: map[string]int{
			guildA_ID: 4, // 1 participant * 4 GP
			guildB_ID: 4, // 1 participant * 4 GP
		},
	}
	if err := gvgRepo.RecordMatchSettlement(ctx, settlement); err != nil {
		t.Fatalf("RecordMatchSettlement: %v", err)
	}

	updatedA, err := gvgRepo.GetOrCreateStanding(ctx, guildA_ID)
	if err != nil {
		t.Fatalf("GetOrCreateStanding A after settlement: %v", err)
	}
	// 3 GP (from round) + 3 GP (prize pool) + 4 GP (participant) = 10 GP
	if updatedA.Wins != 1 || updatedA.BronzeMedals != 1 || updatedA.VictoryPoints != 10 {
		t.Errorf("unexpected updated standing A: %#v", updatedA)
	}

	updatedB, err := gvgRepo.GetOrCreateStanding(ctx, guildB_ID)
	if err != nil {
		t.Fatalf("GetOrCreateStanding B after settlement: %v", err)
	}
	// 4 GP (participant), 1 loss
	if updatedB.Losses != 1 || updatedB.VictoryPoints != 4 {
		t.Errorf("unexpected updated standing B: %#v", updatedB)
	}

	// 6. Test Draw Settlement
	drawSettlement := gvg.MatchSettlement{
		WinnerGuildID: "",
		WinnerPrizeGP: 0,
		GuildIDs:      []string{guildA_ID, guildB_ID},
		IsDraw:        true,
		ParticipantGP: map[string]int{
			guildA_ID: 4,
			guildB_ID: 4,
		},
	}
	if err := gvgRepo.RecordMatchSettlement(ctx, drawSettlement); err != nil {
		t.Fatalf("RecordMatchSettlement (draw): %v", err)
	}

	afterDrawA, err := gvgRepo.GetOrCreateStanding(ctx, guildA_ID)
	if err != nil || afterDrawA.Draws != 1 || afterDrawA.VictoryPoints != 14 {
		t.Errorf("unexpected standing A after draw: %#v, err=%v", afterDrawA, err)
	}

	// 7. Verify Leaderboard
	leaderboard, err := gvgRepo.GetLeaderboard(ctx, 10)
	if err != nil || len(leaderboard) < 2 {
		t.Fatalf("GetLeaderboard failed: len=%d, err=%v", len(leaderboard), err)
	}
}
