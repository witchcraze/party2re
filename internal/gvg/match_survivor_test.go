package gvg_test

import (
	"context"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/guild"
	"github.com/witchcraze/party2re/internal/gvg"
)

type customGvGBattleEngine struct {
	result corebattle.PartyBattleResult
}

func (m *customGvGBattleEngine) ResolvePartyBattle(_ corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
	return m.result, nil
}

func TestGvG_3Guilds_ThirdGuildSurvives(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharRepo()
	guildRepo := newMockGuildRepo()
	standingRepo := newMockStandingRepo()
	roomRepo := gvg.NewMemoryRoomRepository()

	c1 := createTestCharacter("char-1", "RedLeader", 100, 0)
	c2 := createTestCharacter("char-2", "BlueMember", 100, 0)
	c3 := createTestCharacter("char-3", "GreenMember", 100, 0)
	charRepo.characters[c1.ID] = c1
	charRepo.characters[c2.ID] = c2
	charRepo.characters[c3.ID] = c3

	g1 := guild.Guild{ID: "guild-red", Name: "CrimsonKnights", Color: "#FF0000"}
	g2 := guild.Guild{ID: "guild-blue", Name: "AzureKnights", Color: "#0000FF"}
	g3 := guild.Guild{ID: "guild-green", Name: "VerdantKnights", Color: "#00FF00"}
	guildRepo.guilds[g1.ID] = g1
	guildRepo.guilds[g2.ID] = g2
	guildRepo.guilds[g3.ID] = g3
	guildRepo.charGuilds[c1.ID] = g1.ID
	guildRepo.charGuilds[c2.ID] = g2.ID
	guildRepo.charGuilds[c3.ID] = g3.ID

	// Red & Blue die, Green survives with 60 HP
	battleEngine := &customGvGBattleEngine{
		result: corebattle.PartyBattleResult{
			Outcome: corebattle.OutcomeDefeat,
			Turns:   5,
			RemainingHP: map[string]int{
				"char-1": 0,
				"char-2": 0,
				"char-3": 60,
			},
			Logs: []corebattle.TurnLog{{Turn: 1, Message: "Battle finished"}},
		},
	}

	svc, err := gvg.NewService(roomRepo, standingRepo, guildRepo, charRepo, battleEngine)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	detail, err := svc.CreateRoom(ctx, c1.ID, gvg.CreateRoomRequest{
		Name:       "Tri-Guild Skirmish",
		MaxMembers: 3,
		TargetWins: 2,
	})
	if err != nil {
		t.Fatalf("CreateRoom failed: %v", err)
	}

	_, _ = svc.JoinRoom(ctx, c2.ID, detail.Room.ID, "")
	_, _ = svc.JoinRoom(ctx, c3.ID, detail.Room.ID, "")
	_, err = svc.StartMatch(ctx, c1.ID, detail.Room.ID)
	if err != nil {
		t.Fatalf("StartMatch failed: %v", err)
	}

	res, err := svc.AdvanceRound(ctx, c1.ID, detail.Room.ID)
	if err != nil {
		t.Fatalf("AdvanceRound failed: %v", err)
	}

	if res.Outcome != "round_win" {
		t.Errorf("expected outcome round_win, got %s", res.Outcome)
	}
	if res.WinnerGuildID != g3.ID {
		t.Errorf("expected winner guild to be %s (Green, sole survivor), got %s", g3.ID, res.WinnerGuildID)
	}
	if res.WinnerGuildColor != "#00FF00" {
		t.Errorf("expected winner color #00FF00, got %s", res.WinnerGuildColor)
	}
	if res.GuildScores[g3.ID] != 1 {
		t.Errorf("expected Green guild score 1, got %d", res.GuildScores[g3.ID])
	}
	if res.GuildScores[g2.ID] != 0 {
		t.Errorf("expected Blue guild score 0, got %d", res.GuildScores[g2.ID])
	}
	if standingRepo.roundWins[g3.ID] != gvg.RoundWinGP {
		t.Errorf("expected Green guild to receive %d round win GP, got %d", gvg.RoundWinGP, standingRepo.roundWins[g3.ID])
	}
	if standingRepo.roundWins[g2.ID] != 0 {
		t.Errorf("expected Blue guild to receive 0 round win GP, got %d", standingRepo.roundWins[g2.ID])
	}
}

func TestGvG_AllGuildsFell_Draw(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharRepo()
	guildRepo := newMockGuildRepo()
	standingRepo := newMockStandingRepo()
	roomRepo := gvg.NewMemoryRoomRepository()

	c1 := createTestCharacter("char-1", "RedLeader", 100, 0)
	c2 := createTestCharacter("char-2", "BlueMember", 100, 0)
	charRepo.characters[c1.ID] = c1
	charRepo.characters[c2.ID] = c2

	g1 := guild.Guild{ID: "guild-red", Name: "CrimsonKnights", Color: "#FF0000"}
	g2 := guild.Guild{ID: "guild-blue", Name: "AzureKnights", Color: "#0000FF"}
	guildRepo.guilds[g1.ID] = g1
	guildRepo.guilds[g2.ID] = g2
	guildRepo.charGuilds[c1.ID] = g1.ID
	guildRepo.charGuilds[c2.ID] = g2.ID

	// All fallen
	battleEngine := &customGvGBattleEngine{
		result: corebattle.PartyBattleResult{
			Outcome: corebattle.OutcomeDraw,
			Turns:   3,
			RemainingHP: map[string]int{
				"char-1": 0,
				"char-2": 0,
			},
			Logs: []corebattle.TurnLog{{Turn: 1, Message: "Mutual wipeout"}},
		},
	}

	svc, err := gvg.NewService(roomRepo, standingRepo, guildRepo, charRepo, battleEngine)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	detail, err := svc.CreateRoom(ctx, c1.ID, gvg.CreateRoomRequest{
		Name:       "Draw Skirmish",
		MaxMembers: 2,
		TargetWins: 2,
	})
	if err != nil {
		t.Fatalf("CreateRoom failed: %v", err)
	}

	_, _ = svc.JoinRoom(ctx, c2.ID, detail.Room.ID, "")
	_, err = svc.StartMatch(ctx, c1.ID, detail.Room.ID)
	if err != nil {
		t.Fatalf("StartMatch failed: %v", err)
	}

	res, err := svc.AdvanceRound(ctx, c1.ID, detail.Room.ID)
	if err != nil {
		t.Fatalf("AdvanceRound failed: %v", err)
	}

	if res.Outcome != "draw" {
		t.Errorf("expected outcome draw, got %s", res.Outcome)
	}
	if res.WinnerGuildID != "" {
		t.Errorf("expected no winner guild on draw, got %s", res.WinnerGuildID)
	}
	if res.GuildScores[g1.ID] != 0 || res.GuildScores[g2.ID] != 0 {
		t.Errorf("expected 0 scores on draw, got %+v", res.GuildScores)
	}
	if standingRepo.roundWins[g1.ID] != 0 || standingRepo.roundWins[g2.ID] != 0 {
		t.Errorf("expected 0 round win GP on draw, got %+v", standingRepo.roundWins)
	}
}

func TestGvG_3Guilds_RealBattleEngine_LeaderFallsEarly(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharRepo()
	guildRepo := newMockGuildRepo()
	standingRepo := newMockStandingRepo()
	roomRepo := gvg.NewMemoryRoomRepository()

	c1 := createTestCharacter("char-1", "RedLeader", 100, 0)
	c1.Stats.HP = 1
	c1.Stats.MaxHP = 1
	c1.Stats.Attack = 5
	c1.Stats.Defense = 0
	c1.Stats.Agility = 1

	c2 := createTestCharacter("char-2", "BlueMember", 100, 0)
	c2.Stats.HP = 100
	c2.Stats.MaxHP = 100
	c2.Stats.Attack = 30
	c2.Stats.Defense = 10
	c2.Stats.Agility = 20

	c3 := createTestCharacter("char-3", "GreenMember", 100, 0)
	c3.Stats.HP = 80
	c3.Stats.MaxHP = 80
	c3.Stats.Attack = 25
	c3.Stats.Defense = 10
	c3.Stats.Agility = 15

	charRepo.characters[c1.ID] = c1
	charRepo.characters[c2.ID] = c2
	charRepo.characters[c3.ID] = c3

	g1 := guild.Guild{ID: "guild-red", Name: "CrimsonKnights", Color: "#FF0000"}
	g2 := guild.Guild{ID: "guild-blue", Name: "AzureKnights", Color: "#0000FF"}
	g3 := guild.Guild{ID: "guild-green", Name: "VerdantKnights", Color: "#00FF00"}
	guildRepo.guilds[g1.ID] = g1
	guildRepo.guilds[g2.ID] = g2
	guildRepo.guilds[g3.ID] = g3
	guildRepo.charGuilds[c1.ID] = g1.ID
	guildRepo.charGuilds[c2.ID] = g2.ID
	guildRepo.charGuilds[c3.ID] = g3.ID

	svc, err := gvg.NewService(roomRepo, standingRepo, guildRepo, charRepo, corebattle.Engine{})
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	detail, err := svc.CreateRoom(ctx, c1.ID, gvg.CreateRoomRequest{
		Name:       "Real Engine Tri-Guild",
		MaxMembers: 3,
		TargetWins: 2,
	})
	if err != nil {
		t.Fatalf("CreateRoom failed: %v", err)
	}

	_, _ = svc.JoinRoom(ctx, c2.ID, detail.Room.ID, "")
	_, _ = svc.JoinRoom(ctx, c3.ID, detail.Room.ID, "")
	_, err = svc.StartMatch(ctx, c1.ID, detail.Room.ID)
	if err != nil {
		t.Fatalf("StartMatch failed: %v", err)
	}

	res, err := svc.AdvanceRound(ctx, c1.ID, detail.Room.ID)
	if err != nil {
		t.Fatalf("AdvanceRound failed: %v", err)
	}

	if res.Outcome != "round_win" {
		t.Fatalf("expected outcome round_win, got %s", res.Outcome)
	}
	if res.WinnerGuildID != g2.ID && res.WinnerGuildID != g3.ID {
		t.Errorf("expected winner to be Blue or Green guild, got %s", res.WinnerGuildID)
	}
	if res.GuildScores[g1.ID] != 0 {
		t.Errorf("expected Red guild to have 0 score, got %d", res.GuildScores[g1.ID])
	}
	if res.GuildScores[res.WinnerGuildID] != 1 {
		t.Errorf("expected winner %s to have score 1, got %d", res.WinnerGuildID, res.GuildScores[res.WinnerGuildID])
	}
}
