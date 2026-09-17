package gvg_test

import (
	"context"
	"errors"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/guild"
	"github.com/witchcraze/party2re/internal/gvg"
)

type mockFailingStandingRepo struct {
	*mockStandingRepo
	recordErr error
	addErr    error
}

func (m *mockFailingStandingRepo) RecordMatchSettlement(ctx context.Context, settlement gvg.MatchSettlement) error {
	if m.recordErr != nil {
		return m.recordErr
	}
	return m.mockStandingRepo.RecordMatchSettlement(ctx, settlement)
}

func (m *mockFailingStandingRepo) AddRoundWinGP(ctx context.Context, guildID string, gp int) error {
	if m.addErr != nil {
		return m.addErr
	}
	return m.mockStandingRepo.AddRoundWinGP(ctx, guildID, gp)
}

func setupSettlementTest(t *testing.T, standingRepo gvg.StandingRepository) (*gvg.Service, gvg.RoomRepository, string, string) {
	t.Helper()
	ctx := context.Background()

	charRepo := newMockCharRepo()
	guildRepo := newMockGuildRepo()
	roomRepo := gvg.NewMemoryRoomRepository()

	c1 := createTestCharacter("c1", "Player 1", 100, 0)
	c2 := createTestCharacter("c2", "Player 2", 100, 0)
	charRepo.characters[c1.ID] = c1
	charRepo.characters[c2.ID] = c2

	guildRepo.guilds["g1"] = guild.Guild{ID: "g1", Name: "Guild 1", Color: "#FF0000"}
	guildRepo.charGuilds["c1"] = "g1"
	guildRepo.guilds["g2"] = guild.Guild{ID: "g2", Name: "Guild 2", Color: "#0000FF"}
	guildRepo.charGuilds["c2"] = "g2"

	// Win outcome for leader guild (g1)
	battleEngine := &customGvGBattleEngine{
		result: corebattle.PartyBattleResult{
			Outcome: corebattle.OutcomeWin,
			Turns:   3,
			RemainingHP: map[string]int{
				"c1": 80,
				"c2": 0,
			},
			Logs: []corebattle.TurnLog{{Turn: 1, Message: "Round combat concluded"}},
		},
	}

	svc, err := gvg.NewService(roomRepo, standingRepo, guildRepo, charRepo, battleEngine)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	// Create room with TargetWins = 1 so round 1 concludes the match
	detail, err := svc.CreateRoom(ctx, "c1", gvg.CreateRoomRequest{
		Name:       "Settlement Test Room",
		MaxMembers: 2,
		TargetWins: 1,
	})
	if err != nil {
		t.Fatalf("CreateRoom failed: %v", err)
	}

	// Join c2 from g2
	_, err = svc.JoinRoom(ctx, "c2", detail.Room.ID, "#0000FF")
	if err != nil {
		t.Fatalf("JoinRoom failed: %v", err)
	}

	// Start match
	_, err = svc.StartMatch(ctx, "c1", detail.Room.ID)
	if err != nil {
		t.Fatalf("StartMatch failed: %v", err)
	}

	return svc, roomRepo, "c1", detail.Room.ID
}

func TestAdvanceRound_RecordMatchSettlement_ErrorPropagation(t *testing.T) {
	ctx := context.Background()
	expectedErr := errors.New("db deadlock on match settlement")

	failingStandings := &mockFailingStandingRepo{
		mockStandingRepo: newMockStandingRepo(),
		recordErr:        expectedErr,
	}

	svc, roomRepo, leaderID, roomID := setupSettlementTest(t, failingStandings)

	res, err := svc.AdvanceRound(ctx, leaderID, roomID)
	if err == nil {
		t.Fatalf("expected error from AdvanceRound when RecordMatchSettlement fails, got nil (res=%+v)", res)
	}

	if !errors.Is(err, expectedErr) {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}

	// Verify room in repo remains uncompleted
	detail, err := roomRepo.GetRoom(ctx, roomID)
	if err != nil {
		t.Fatalf("GetRoom failed: %v", err)
	}

	if detail.Room.Status == gvg.StatusCompleted {
		t.Errorf("expected Room.Status != StatusCompleted, got %s", detail.Room.Status)
	}
	if detail.Room.WinnerGuildID != "" {
		t.Errorf("expected WinnerGuildID to be empty, got %s", detail.Room.WinnerGuildID)
	}
}

func TestAdvanceRound_AddRoundWinGP_ErrorPropagation(t *testing.T) {
	ctx := context.Background()
	expectedErr := errors.New("db timeout on round win gp")

	failingStandings := &mockFailingStandingRepo{
		mockStandingRepo: newMockStandingRepo(),
		addErr:           expectedErr,
	}

	svc, roomRepo, leaderID, roomID := setupSettlementTest(t, failingStandings)

	res, err := svc.AdvanceRound(ctx, leaderID, roomID)
	if err == nil {
		t.Fatalf("expected error from AdvanceRound when AddRoundWinGP fails, got nil (res=%+v)", res)
	}

	if !errors.Is(err, expectedErr) {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}

	// Verify room in repo was not saved with completed status or modified round
	detail, err := roomRepo.GetRoom(ctx, roomID)
	if err != nil {
		t.Fatalf("GetRoom failed: %v", err)
	}

	if detail.Room.Status == gvg.StatusCompleted {
		t.Errorf("expected Room.Status != StatusCompleted, got %s", detail.Room.Status)
	}
	if detail.Room.GuildScores["g1"] != 0 {
		t.Errorf("expected GuildScores to remain unsaved, got %d", detail.Room.GuildScores["g1"])
	}
}

func TestAdvanceRound_RecordMatchSettlement_Draw_ErrorPropagation(t *testing.T) {
	ctx := context.Background()
	expectedErr := errors.New("db deadlock on draw settlement")

	failingStandings := &mockFailingStandingRepo{
		mockStandingRepo: newMockStandingRepo(),
		recordErr:        expectedErr,
	}

	charRepo := newMockCharRepo()
	guildRepo := newMockGuildRepo()
	roomRepo := gvg.NewMemoryRoomRepository()

	c1 := createTestCharacter("c1", "Player 1", 100, 0)
	c2 := createTestCharacter("c2", "Player 2", 100, 0)
	charRepo.characters[c1.ID] = c1
	charRepo.characters[c2.ID] = c2

	guildRepo.guilds["g1"] = guild.Guild{ID: "g1", Name: "Guild 1", Color: "#FF0000"}
	guildRepo.charGuilds["c1"] = "g1"
	guildRepo.guilds["g2"] = guild.Guild{ID: "g2", Name: "Guild 2", Color: "#0000FF"}
	guildRepo.charGuilds["c2"] = "g2"

	// Draw battle outcome (both die)
	battleEngine := &customGvGBattleEngine{
		result: corebattle.PartyBattleResult{
			Outcome: corebattle.OutcomeDraw,
			Turns:   5,
			RemainingHP: map[string]int{
				"c1": 0,
				"c2": 0,
			},
			Logs: []corebattle.TurnLog{{Turn: 1, Message: "Round draw"}},
		},
	}

	svc, err := gvg.NewService(roomRepo, failingStandings, guildRepo, charRepo, battleEngine)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	detail, err := svc.CreateRoom(ctx, "c1", gvg.CreateRoomRequest{
		Name:       "Draw Test Room",
		MaxMembers: 2,
		TargetWins: 3,
	})
	if err != nil {
		t.Fatalf("CreateRoom failed: %v", err)
	}

	_, err = svc.JoinRoom(ctx, "c2", detail.Room.ID, "#0000FF")
	if err != nil {
		t.Fatalf("JoinRoom failed: %v", err)
	}

	_, err = svc.StartMatch(ctx, "c1", detail.Room.ID)
	if err != nil {
		t.Fatalf("StartMatch failed: %v", err)
	}

	// Fast forward room to round 10
	roomDetail, err := roomRepo.GetRoom(ctx, detail.Room.ID)
	if err != nil {
		t.Fatalf("GetRoom failed: %v", err)
	}
	roomDetail.Room.Round = gvg.MaxRounds
	if err := roomRepo.SaveRoom(ctx, roomDetail.Room, roomDetail.Members); err != nil {
		t.Fatalf("SaveRoom failed: %v", err)
	}

	res, err := svc.AdvanceRound(ctx, "c1", detail.Room.ID)
	if err == nil {
		t.Fatalf("expected error from AdvanceRound when draw RecordMatchSettlement fails, got nil (res=%+v)", res)
	}

	if !errors.Is(err, expectedErr) {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}

	// Verify room status was not saved as completed
	savedDetail, err := roomRepo.GetRoom(ctx, detail.Room.ID)
	if err != nil {
		t.Fatalf("GetRoom failed: %v", err)
	}

	if savedDetail.Room.Status == gvg.StatusCompleted {
		t.Errorf("expected Room.Status != StatusCompleted, got %s", savedDetail.Room.Status)
	}
}
