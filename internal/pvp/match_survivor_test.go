package pvp_test

import (
	"context"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/pvp"
)

func TestColosseum_3Teams_ThirdTeamSurvives(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharRepo()
	roomRepo := pvp.NewMemoryRoomRepository()

	c1 := createTestCharacter("char-1", "RedLeader", 500)
	c2 := createTestCharacter("char-2", "BlueMember", 500)
	c3 := createTestCharacter("char-3", "GreenMember", 500)
	charRepo.add(c1)
	charRepo.add(c2)
	charRepo.add(c3)

	// Custom battle engine where Red & Blue die, Green survives with 75 HP
	battleEngine := mockBattleEngine{
		result: corebattle.PartyBattleResult{
			Outcome: corebattle.OutcomeDefeat,
			Turns:   4,
			RemainingHP: map[string]int{
				"char-1": 0,
				"char-2": 0,
				"char-3": 75,
			},
			Logs: []corebattle.TurnLog{{Turn: 1, Message: "Combat finished"}},
		},
	}
	svc, err := pvp.NewService(roomRepo, charRepo, battleEngine)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	detail, err := svc.CreateRoom(ctx, c1.ID, pvp.CreateRoomRequest{
		Name:       "3-Way Battle",
		Bet:        50,
		MaxMembers: 3,
		TargetWins: 2,
	})
	if err != nil {
		t.Fatalf("CreateRoom failed: %v", err)
	}

	_, _ = svc.JoinRoom(ctx, c2.ID, detail.Room.ID, "")
	_, _ = svc.JoinRoom(ctx, c3.ID, detail.Room.ID, "")
	_, _ = svc.SelectTeam(ctx, c1.ID, detail.Room.ID, pvp.ColorRed)
	_, _ = svc.SelectTeam(ctx, c2.ID, detail.Room.ID, pvp.ColorBlue)
	_, _ = svc.SelectTeam(ctx, c3.ID, detail.Room.ID, pvp.ColorGreen)
	_, err = svc.StartMatch(ctx, c1.ID, detail.Room.ID)
	if err != nil {
		t.Fatalf("StartMatch failed: %v", err)
	}

	// Advance Round 1: Green survived, Red and Blue died
	res, err := svc.AdvanceRound(ctx, c1.ID, detail.Room.ID)
	if err != nil {
		t.Fatalf("AdvanceRound failed: %v", err)
	}

	if res.Outcome != "round_win" {
		t.Errorf("expected outcome round_win, got %s", res.Outcome)
	}
	if res.WinnerTeam != pvp.ColorGreen {
		t.Errorf("expected winner team to be %s (Green, sole survivor), got %s", pvp.ColorGreen, res.WinnerTeam)
	}
	if res.TeamScores[pvp.ColorGreen] != 1 {
		t.Errorf("expected Green score 1, got %d", res.TeamScores[pvp.ColorGreen])
	}
	if res.TeamScores[pvp.ColorBlue] != 0 {
		t.Errorf("expected Blue score 0, got %d", res.TeamScores[pvp.ColorBlue])
	}
	if res.TeamScores[pvp.ColorRed] != 0 {
		t.Errorf("expected Red score 0, got %d", res.TeamScores[pvp.ColorRed])
	}
}

func TestColosseum_AllTeamsFell_Draw(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharRepo()
	roomRepo := pvp.NewMemoryRoomRepository()

	c1 := createTestCharacter("char-1", "RedLeader", 500)
	c2 := createTestCharacter("char-2", "BlueMember", 500)
	charRepo.add(c1)
	charRepo.add(c2)

	// Custom battle engine where everyone falls (RemainingHP all 0)
	battleEngine := mockBattleEngine{
		result: corebattle.PartyBattleResult{
			Outcome: corebattle.OutcomeDraw,
			Turns:   3,
			RemainingHP: map[string]int{
				"char-1": 0,
				"char-2": 0,
			},
			Logs: []corebattle.TurnLog{{Turn: 1, Message: "Mutual annihilation"}},
		},
	}
	svc, err := pvp.NewService(roomRepo, charRepo, battleEngine)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	detail, err := svc.CreateRoom(ctx, c1.ID, pvp.CreateRoomRequest{
		Name:       "Mutual Kill Room",
		Bet:        50,
		MaxMembers: 2,
		TargetWins: 2,
	})
	if err != nil {
		t.Fatalf("CreateRoom failed: %v", err)
	}

	_, _ = svc.JoinRoom(ctx, c2.ID, detail.Room.ID, "")
	_, _ = svc.SelectTeam(ctx, c1.ID, detail.Room.ID, pvp.ColorRed)
	_, _ = svc.SelectTeam(ctx, c2.ID, detail.Room.ID, pvp.ColorBlue)
	_, _ = svc.StartMatch(ctx, c1.ID, detail.Room.ID)

	res, err := svc.AdvanceRound(ctx, c1.ID, detail.Room.ID)
	if err != nil {
		t.Fatalf("AdvanceRound failed: %v", err)
	}

	if res.Outcome != "draw" {
		t.Errorf("expected outcome draw, got %s", res.Outcome)
	}
	if res.WinnerTeam != "" {
		t.Errorf("expected no winner team on mutual wipeout, got %s", res.WinnerTeam)
	}
	if res.TeamScores[pvp.ColorRed] != 0 || res.TeamScores[pvp.ColorBlue] != 0 {
		t.Errorf("expected 0 scores on draw, got %+v", res.TeamScores)
	}
}
