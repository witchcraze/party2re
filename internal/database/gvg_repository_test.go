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
		Level:             1,
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
		Level:             1,
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
	if stA.Rating != 1000 || stA.Wins != 0 || stA.BronzeMedals != 0 {
		t.Errorf("unexpected initial standing A: %#v", stA)
	}

	// 4. Test FindOpponentGuilds
	opponents, err := gvgRepo.FindOpponentGuilds(ctx, guildA_ID, 10)
	if err != nil {
		t.Fatalf("FindOpponentGuilds: %v", err)
	}
	if len(opponents) == 0 {
		t.Fatalf("expected at least 1 opponent guild")
	}
	for _, opp := range opponents {
		if opp.GuildID == guildA_ID {
			t.Errorf("FindOpponentGuilds returned self: %s", opp.GuildID)
		}
		if opp.GuildName == "" || opp.Level <= 0 {
			t.Errorf("invalid opponent candidate: %#v", opp)
		}
	}

	// 5. Test RecordMatchAndUpdateStandings
	matchID := fmt.Sprintf("gvg_test_%d", time.Now().UnixNano())
	roundID := fmt.Sprintf("%s_r1", matchID)
	matchRecord := gvg.MatchRecord{
		ID:                     matchID,
		ChallengerGuildID:      guildA_ID,
		DefenderGuildID:        guildB_ID,
		WinnerGuildID:          guildA_ID,
		ChallengerScore:        1,
		DefenderScore:          0,
		TotalRounds:            1,
		ChallengerRatingBefore: 1000,
		ChallengerRatingAfter:  1016,
		DefenderRatingBefore:   1000,
		DefenderRatingAfter:    984,
		Rounds: []gvg.MatchRound{
			{
				ID:                      roundID,
				MatchID:                 matchID,
				RoundIndex:              1,
				ChallengerCharacterID:   charA.ID,
				ChallengerCharacterName: charA.Name,
				DefenderCharacterID:     charB.ID,
				DefenderCharacterName:   charB.Name,
				WinnerCharacterID:       charA.ID,
				Turns:                   4,
				CreatedAt:               time.Now(),
			},
		},
		CreatedAt: time.Now(),
	}

	memberRewards := map[string]gvg.MemberReward{
		charA.ID: {Experience: 50, Gold: 100},
		charB.ID: {Experience: 15, Gold: 20},
	}

	err = gvgRepo.RecordMatchAndUpdateStandings(
		ctx,
		matchRecord,
		16, -16,
		100, 20,
		10, 1,
		true, false,
		memberRewards,
	)
	if err != nil {
		t.Fatalf("RecordMatchAndUpdateStandings failed: %v", err)
	}

	// 6. Verify Updated Standings
	updatedA, err := gvgRepo.GetOrCreateStanding(ctx, guildA_ID)
	if err != nil || updatedA.Rating != 1016 || updatedA.Wins != 1 || updatedA.BronzeMedals != 1 || updatedA.VictoryPoints != 10 {
		t.Errorf("unexpected updated standing A: %#v, err=%v", updatedA, err)
	}

	updatedB, err := gvgRepo.GetOrCreateStanding(ctx, guildB_ID)
	if err != nil || updatedB.Rating != 984 || updatedB.Losses != 1 || updatedB.VictoryPoints != 1 {
		t.Errorf("unexpected updated standing B: %#v, err=%v", updatedB, err)
	}

	// 7. Verify Match Detail
	detail, err := gvgRepo.GetMatchDetail(ctx, matchID)
	if err != nil {
		t.Fatalf("GetMatchDetail failed: %v", err)
	}
	if detail.WinnerGuildID != guildA_ID || len(detail.Rounds) != 1 {
		t.Errorf("unexpected match detail: %#v", detail)
	}

	// 8. Verify Match History
	history, err := gvgRepo.GetMatchHistory(ctx, guildA_ID, 10)
	if err != nil || len(history) == 0 {
		t.Fatalf("GetMatchHistory failed: %v", err)
	}

	// 9. Verify Leaderboard
	leaderboard, err := gvgRepo.GetLeaderboard(ctx, 10)
	if err != nil || len(leaderboard) == 0 {
		t.Fatalf("GetLeaderboard failed: %v", err)
	}
}
