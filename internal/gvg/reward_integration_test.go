package gvg_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"reflect"
	"testing"

	"github.com/valkey-io/valkey-go"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/gvg"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/testutil"
)

type rewardFixture struct {
	db        *sql.DB
	standings *database.GvGRepository
	rooms     *failingPublicationRepo
	detail    gvg.RoomDetail
	service   *gvg.Service
	battle    *mockBattleEngine
	client    valkey.Client
}

func newRewardFixture(t *testing.T, target, round int, outcome corebattle.Outcome) rewardFixture {
	t.Helper()
	if os.Getenv("PARTY2_DB_DSN") == "" || os.Getenv("PARTY2_VALKEY_ADDR") == "" {
		t.Skip("PARTY2_DB_DSN and PARTY2_VALKEY_ADDR are required")
	}
	ctx := context.Background()
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	standings, err := database.NewGvGRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	client, err := valkey.NewClient(valkey.ClientOption{InitAddress: []string{os.Getenv("PARTY2_VALKEY_ADDR")}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	roomRepo, err := gvg.NewValkeyRoomRepository(client)
	if err != nil {
		t.Fatal(err)
	}
	detail := gvg.RoomDetail{Room: gvg.GvGRoom{ID: id.New(), Round: round, Status: gvg.StatusInProgress, TargetWins: target, PrizePool: 3, GuildScores: map[string]int{}}}
	for i, color := range []string{"#FF0000", "#0000FF"} {
		g, char, err := database.CreateTestGuildWithLeader(ctx, db, fmt.Sprintf("Reward%d_%s", i, id.New()[:8]), 10000)
		if err != nil {
			t.Fatal(err)
		}
		detail.Members = append(detail.Members, gvg.GvGMember{RoomID: detail.Room.ID, CharacterID: char.ID, CharacterName: char.Name, GuildID: g.ID, GuildName: g.Name, GuildColor: color, HP: char.Stats.MaxHP, MaxHP: char.Stats.MaxHP})
	}
	detail.Room.LeaderCharacterID = detail.Members[0].CharacterID
	if err := roomRepo.SaveRoom(ctx, detail.Room, detail.Members); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := roomRepo.DeleteRoom(ctx, detail.Room.ID); err != nil {
			t.Error(err)
		}
	})
	chars, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	rooms := &failingPublicationRepo{RoomRepository: roomRepo}
	battle := &mockBattleEngine{outcome: outcome}
	service, err := gvg.NewService(rooms, standings, newMockGuildRepo(), chars, battle)
	if err != nil {
		t.Fatal(err)
	}
	return rewardFixture{db: db, standings: standings, rooms: rooms, detail: detail, service: service, battle: battle, client: client}
}

func (f rewardFixture) assertRewards(t *testing.T, winnerGP, loserGP int64, wins, draws int) {
	t.Helper()
	for i, member := range f.detail.Members {
		st, err := f.standings.GetOrCreateStanding(context.Background(), member.GuildID)
		if err != nil {
			t.Fatal(err)
		}
		gp, expectedWins, losses, medals := winnerGP, wins, 0, wins
		if i == 1 {
			gp, expectedWins, losses, medals = loserGP, 0, wins, 0
		}
		if st.VictoryPoints != gp || st.Wins != expectedWins || st.Losses != losses || st.Draws != draws || st.BronzeMedals != medals {
			t.Fatalf("guild %d: unexpected durable rewards: %+v", i, st)
		}
	}
}

func TestGvGRewardPublicationIntegration(t *testing.T) {
	for _, tc := range []struct {
		name           string
		target, round  int
		outcome        corebattle.Outcome
		gp, opponentGP int64
		wins, draws    int
	}{
		{"intermediate", 2, 1, corebattle.OutcomeWin, 3, 0, 0, 0},
		{"final", 1, 1, corebattle.OutcomeWin, 10, 4, 1, 0},
		{"final_draw", 3, gvg.MaxRounds, corebattle.OutcomeDraw, 4, 4, 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newRewardFixture(t, tc.target, tc.round, tc.outcome)
			ctx := context.Background()
			f.rooms.fail = true
			if _, err := f.service.AdvanceRound(ctx, f.detail.Room.LeaderCharacterID, f.detail.Room.ID); !errors.Is(err, errRoomPublication) {
				t.Fatalf("publication: %v", err)
			}
			f.assertRewards(t, tc.gp, tc.opponentGP, tc.wins, tc.draws)
			saved, found, err := f.standings.FindRoundTransition(ctx, f.detail.Room.ID, tc.round)
			if err != nil || !found {
				t.Fatalf("durable transition: found=%v err=%v", found, err)
			}
			f.battle.outcome = corebattle.OutcomeDefeat
			f.rooms.fail = false
			// Construct another service/repository to rule out an in-process retry guard.
			reloaded, err := database.NewGvGRepository(f.db)
			if err != nil {
				t.Fatal(err)
			}
			chars, err := database.NewCharacterRepository(f.db)
			if err != nil {
				t.Fatal(err)
			}
			retry, err := gvg.NewService(f.rooms, reloaded, newMockGuildRepo(), chars, f.battle)
			if err != nil {
				t.Fatal(err)
			}
			result, err := retry.AdvanceRound(ctx, f.detail.Room.LeaderCharacterID, f.detail.Room.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(result, saved.Result) {
				t.Fatalf("recovery changed outcome: %+v / %+v", result, saved.Result)
			}
			room, err := f.rooms.GetRoom(ctx, f.detail.Room.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(room, saved.Detail) {
				t.Fatalf("recovered room differs: %+v / %+v", room, saved.Detail)
			}
			f.assertRewards(t, tc.gp, tc.opponentGP, tc.wins, tc.draws)
		})
	}
}

func TestGvGRewardConcurrentRetryIntegration(t *testing.T) {
	f := newRewardFixture(t, 1, 1, corebattle.OutcomeWin)
	ctx := context.Background()
	f.rooms.fail = true
	// Every service retry still sees round 1 because publication keeps failing.
	stress := testutil.RunConcurrentStressTest(t, testutil.GetStressConfig(), func(_, _ int) error {
		_, err := f.service.AdvanceRound(ctx, f.detail.Room.LeaderCharacterID, f.detail.Room.ID)
		if errors.Is(err, errRoomPublication) {
			return nil
		}
		return fmt.Errorf("expected failed publication: %v", err)
	})
	if stress.Successes != stress.TotalOps {
		t.Fatalf("retry stress: %+v", stress)
	}
	f.assertRewards(t, 10, 4, 1, 0)
	saved, found, err := f.standings.FindRoundTransition(ctx, f.detail.Room.ID, 1)
	if err != nil || !found {
		t.Fatalf("transition: %v %v", found, err)
	}
	// Competing SQL claims must also be safe independently of the Valkey lock.
	sqlRoomIDs := []string{id.New(), id.New()}
	stress = testutil.RunConcurrentStressTest(t, testutil.GetStressConfig(), func(worker, _ int) error {
		contender := saved
		contender.Detail.Room.ID = sqlRoomIDs[worker%2]
		settlement := *saved.Settlement
		settlement.GuildIDs = append([]string(nil), settlement.GuildIDs...)
		if worker%2 == 0 {
			settlement.GuildIDs[0], settlement.GuildIDs[1] = settlement.GuildIDs[1], settlement.GuildIDs[0]
		}
		contender.Settlement = &settlement
		_, err := f.standings.CommitRoundTransition(ctx, contender.Detail.Room.ID, 1, contender)
		return err
	})
	if stress.Successes != stress.TotalOps {
		t.Fatalf("SQL retry stress: %+v", stress)
	}
	f.assertRewards(t, 30, 12, 3, 0)
}

func TestGvGRewardSuccessiveRoundsIntegration(t *testing.T) {
	f := newRewardFixture(t, 2, 1, corebattle.OutcomeWin)
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		if _, err := f.service.AdvanceRound(ctx, f.detail.Room.LeaderCharacterID, f.detail.Room.ID); err != nil {
			t.Fatal(err)
		}
	}
	f.assertRewards(t, 13, 4, 1, 0)
	// A snapshot older than the latest durable round recovers the terminal state.
	if err := f.rooms.SaveRoom(ctx, f.detail.Room, f.detail.Members); err != nil {
		t.Fatal(err)
	}
	result, err := f.service.AdvanceRound(ctx, f.detail.Room.LeaderCharacterID, f.detail.Room.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !result.MatchCompleted || result.GuildScores[f.detail.Members[0].GuildID] != 2 {
		t.Fatalf("stale recovery: %+v", result)
	}
	f.assertRewards(t, 13, 4, 1, 0)
}

func TestGvGRewardCleanupFailureIntegration(t *testing.T) {
	f := newRewardFixture(t, 1, 1, corebattle.OutcomeWin)
	ctx := context.Background()
	if _, err := f.service.AdvanceRound(ctx, f.detail.Room.LeaderCharacterID, f.detail.Room.ID); err != nil {
		t.Fatal(err)
	}
	f.rooms.failDelete = true
	if err := f.service.LeaveRoom(ctx, f.detail.Room.LeaderCharacterID, f.detail.Room.ID); !errors.Is(err, errRoomPublication) {
		t.Fatalf("cleanup: %v", err)
	}
	if _, err := f.service.AdvanceRound(ctx, f.detail.Room.LeaderCharacterID, f.detail.Room.ID); !errors.Is(err, gvg.ErrMatchNotInProgress) {
		t.Fatalf("completed room retry: %v", err)
	}
	f.rooms.failDelete = false
	if err := f.service.LeaveRoom(ctx, f.detail.Room.LeaderCharacterID, f.detail.Room.ID); err != nil {
		t.Fatal(err)
	}
	committed, found, err := f.standings.FindRoundTransition(ctx, f.detail.Room.ID, 1)
	if err != nil || !found {
		t.Fatalf("cleanup lost durable guard: %v %v", found, err)
	}
	// A stale contender cannot settle the same match using a later round identity.
	if _, err := f.standings.CommitRoundTransition(ctx, f.detail.Room.ID, 2, committed); err != nil {
		t.Fatal(err)
	}
	f.assertRewards(t, 10, 4, 1, 0)
}

func TestGvGRewardSettlementFailureIntegration(t *testing.T) {
	f := newRewardFixture(t, 1, 1, corebattle.OutcomeWin)
	ctx := context.Background()
	winner := f.detail.Members[0].GuildID
	if _, err := f.standings.GetOrCreateStanding(ctx, winner); err != nil {
		t.Fatal(err)
	}
	// Overflow the SQL INT win counter after the round GP update, during settlement.
	if _, err := f.db.ExecContext(ctx, "UPDATE gvg_standings SET wins = 2147483647 WHERE guild_id = ?", winner); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.AdvanceRound(ctx, f.detail.Room.LeaderCharacterID, f.detail.Room.ID); err == nil {
		t.Fatal("expected settlement overflow")
	}
	st, err := f.standings.GetOrCreateStanding(ctx, winner)
	if err != nil {
		t.Fatal(err)
	}
	if st.VictoryPoints != 0 || st.BronzeMedals != 0 || st.Wins != 2147483647 {
		t.Fatalf("partial reward survived rollback: %+v", st)
	}
	if _, found, err := f.standings.FindRoundTransition(ctx, f.detail.Room.ID, 1); err != nil || found {
		t.Fatalf("failed settlement claimed the round: %v %v", found, err)
	}
	if _, err := f.db.ExecContext(ctx, "UPDATE gvg_standings SET wins = 0 WHERE guild_id = ?", winner); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.AdvanceRound(ctx, f.detail.Room.LeaderCharacterID, f.detail.Room.ID); err != nil {
		t.Fatal(err)
	}
	f.assertRewards(t, 10, 4, 1, 0)
}

func TestGvGRewardRollbackIntegration(t *testing.T) {
	f := newRewardFixture(t, 1, 1, corebattle.OutcomeWin)
	ctx := context.Background()
	rollback := errors.New("force outer rollback")
	transition := gvg.RoundTransition{
		Detail: f.detail,
		Result: gvg.RoundResolution{WinnerGuildID: f.detail.Members[0].GuildID, MatchCompleted: true},
		Settlement: &gvg.MatchSettlement{
			WinnerGuildID: f.detail.Members[0].GuildID, WinnerPrizeGP: 3,
			GuildIDs:      []string{f.detail.Members[0].GuildID, f.detail.Members[1].GuildID},
			ParticipantGP: map[string]int{f.detail.Members[0].GuildID: 4, f.detail.Members[1].GuildID: 4},
		},
	}
	err := database.RunInTx(ctx, f.db, func(txCtx context.Context) error {
		if _, err := f.standings.CommitRoundTransition(txCtx, f.detail.Room.ID, 1, transition); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	f.assertRewards(t, 0, 0, 0, 0)
	if _, found, err := f.standings.FindRoundTransition(ctx, f.detail.Room.ID, 1); err != nil || found {
		t.Fatalf("rolled back guard remains: %v %v", found, err)
	}
	// SQL rollback must leave the claim reusable.
	if _, err := f.service.AdvanceRound(ctx, f.detail.Room.LeaderCharacterID, f.detail.Room.ID); err != nil {
		t.Fatal(err)
	}
	f.assertRewards(t, 10, 4, 1, 0)
}
