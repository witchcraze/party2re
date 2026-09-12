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
	roomRepo := gvg.NewMemoryRoomRepository()

	service, err := gvg.NewService(roomRepo, gvgRepo, guildRepo, charRepo, battleEngine)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Create Guild Alpha (Crimson)
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

	guildAID := fmt.Sprintf("g_alpha_%012x", time.Now().UnixNano())
	guildAName := fmt.Sprintf("Alpha_%d", time.Now().UnixNano()%1000000)
	gA := guild.Guild{
		ID:                guildAID,
		Name:              guildAName,
		LeaderCharacterID: leaderA.ID,
		Level:             1,
		Color:             "#FF3333", // Red
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

	// 2. Create Guild Beta (Blue)
	leaderB, err := database.CreateTestCharacter(ctx, db, "GvG Leader Beta")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE characters SET money = ? WHERE id = ?", 10000, leaderB.ID); err != nil {
		t.Fatal(err)
	}

	guildBID := fmt.Sprintf("g_beta_%012x", time.Now().UnixNano())
	guildBName := fmt.Sprintf("Beta_%d", time.Now().UnixNano()%1000000)
	gB := guild.Guild{
		ID:                guildBID,
		Name:              guildBName,
		LeaderCharacterID: leaderB.ID,
		Level:             1,
		Color:             "#6666FF", // Blue
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

	// 3. Leader A creates GvG Room (seeds 2 GP)
	createdRoom, err := service.CreateRoom(ctx, leaderA.ID, gvg.CreateRoomRequest{
		Name:       "Live GvG Tournament",
		MaxMembers: 4,
		TargetWins: 1,
	})
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if createdRoom.Room.PrizePool != 2 {
		t.Fatalf("expected 2 GP prize pool on create, got %d", createdRoom.Room.PrizePool)
	}

	// 4. Leader B joins GvG Room (adds 1 GP -> 3 GP)
	joinedRoom, err := service.JoinRoom(ctx, leaderB.ID, createdRoom.Room.ID, "")
	if err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	if joinedRoom.Room.PrizePool != 3 {
		t.Fatalf("expected 3 GP prize pool on join, got %d", joinedRoom.Room.PrizePool)
	}

	// 5. Start Match (@かいし)
	startedRoom, err := service.StartMatch(ctx, leaderA.ID, createdRoom.Room.ID)
	if err != nil {
		t.Fatalf("StartMatch: %v", err)
	}
	if startedRoom.Room.Status != gvg.StatusInProgress || startedRoom.Room.Round != 1 {
		t.Fatalf("expected round 1 in_progress, got status=%s round=%d", startedRoom.Room.Status, startedRoom.Room.Round)
	}

	// 6. Advance Round
	roundRes, err := service.AdvanceRound(ctx, leaderA.ID, createdRoom.Room.ID)
	if err != nil {
		t.Fatalf("AdvanceRound: %v", err)
	}

	if !roundRes.MatchCompleted {
		t.Fatalf("expected match completed on 1 target win, got %#v", roundRes)
	}

	// 7. Verify Durable Standings in MariaDB
	stA, err := service.GetStanding(ctx, guildAID)
	if err != nil {
		t.Fatalf("GetStanding A: %v", err)
	}
	stB, err := service.GetStanding(ctx, guildBID)
	if err != nil {
		t.Fatalf("GetStanding B: %v", err)
	}

	if stA.Wins == 1 {
		if stA.BronzeMedals != 1 {
			t.Errorf("expected 1 bronze medal for winner guild A, got %d", stA.BronzeMedals)
		}
		if stA.VictoryPoints < 3 {
			t.Errorf("expected GP >= 3 for winner guild A, got %d", stA.VictoryPoints)
		}
		if stB.Losses != 1 {
			t.Errorf("expected 1 loss for loser guild B, got %d", stB.Losses)
		}
	} else if stB.Wins == 1 {
		if stB.BronzeMedals != 1 {
			t.Errorf("expected 1 bronze medal for winner guild B, got %d", stB.BronzeMedals)
		}
		if stA.Losses != 1 {
			t.Errorf("expected 1 loss for loser guild A, got %d", stA.Losses)
		}
	}

	// 8. Verify Leaderboard
	leaderboard, err := service.GetLeaderboard(ctx, 10)
	if err != nil {
		t.Fatalf("GetLeaderboard: %v", err)
	}
	if len(leaderboard) < 2 {
		t.Errorf("expected at least 2 guilds in leaderboard, got %d", len(leaderboard))
	}
}
