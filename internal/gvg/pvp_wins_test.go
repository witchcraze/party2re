package gvg_test

import (
	"context"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/guild"
	"github.com/witchcraze/party2re/internal/gvg"
)

type scriptedGvGBattleEngine struct {
	results []corebattle.PartyBattleResult
	idx     int
}

func (e *scriptedGvGBattleEngine) ResolvePartyBattle(_ corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
	res := e.results[e.idx]
	e.idx++
	return res, nil
}

func TestGvG_StrongOpponentDefeatIncrementsPvPWinsPerRound(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharRepo()
	guildRepo := newMockGuildRepo()
	standingRepo := newMockStandingRepo()
	standingRepo.chars = charRepo
	roomRepo := gvg.NewMemoryRoomRepository()

	c1 := createTestCharacter("char-1", "LeaderRed", 100, 0)
	c2 := createTestCharacter("char-2", "StrongBlue", 100, 0)
	c3 := createTestCharacter("char-3", "WeakBlue", 100, 0)
	c3.Stats.MaxHP, c3.Stats.HP = 20, 20
	c3.Stats.MaxMP, c3.Stats.MP = 0, 0
	c3.Stats.Attack, c3.Stats.Defense, c3.Stats.Agility = 0, 0, 0
	for _, c := range []corecharacter.Character{c1, c2, c3} {
		charRepo.characters[c.ID] = c
	}

	guildRepo.guilds["g1"] = guild.Guild{ID: "g1", Name: "Crimson", Color: "#FF0000"}
	guildRepo.guilds["g2"] = guild.Guild{ID: "g2", Name: "Azure", Color: "#0000FF"}
	guildRepo.charGuilds[c1.ID] = "g1"
	guildRepo.charGuilds[c2.ID] = "g2"
	guildRepo.charGuilds[c3.ID] = "g2"

	engine := &scriptedGvGBattleEngine{
		results: []corebattle.PartyBattleResult{
			{
				// Round 1: ends in draw, but c1 defeated StrongBlue earlier in combat.
				Outcome:     corebattle.OutcomeDraw,
				Turns:       3,
				RemainingHP: map[string]int{c1.ID: 50, c2.ID: 0, c3.ID: 50},
				Defeats:     []corebattle.PartyBattleDefeat{{ActorID: c1.ID, TargetID: c2.ID}},
			},
			{
				// Round 2: c1 defeats WeakBlue (ignored) and StrongBlue (+1).
				Outcome:     corebattle.OutcomeWin,
				Turns:       4,
				RemainingHP: map[string]int{c1.ID: 70, c2.ID: 0, c3.ID: 0},
				Defeats: []corebattle.PartyBattleDefeat{
					{ActorID: c1.ID, TargetID: c3.ID},
					{ActorID: c1.ID, TargetID: c2.ID},
				},
			},
		},
	}

	svc, err := gvg.NewService(roomRepo, standingRepo, guildRepo, charRepo, engine)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}
	detail, _ := svc.CreateRoom(ctx, c1.ID, gvg.CreateRoomRequest{Name: "GvG KillP", MaxMembers: 4, TargetWins: 1})
	_, _ = svc.JoinRoom(ctx, c2.ID, detail.Room.ID, "")
	_, _ = svc.JoinRoom(ctx, c3.ID, detail.Room.ID, "")
	_, _ = svc.StartMatch(ctx, c1.ID, detail.Room.ID)

	if _, err := svc.AdvanceRound(ctx, c1.ID, detail.Room.ID, 1); err != nil {
		t.Fatalf("round 1 failed: %v", err)
	}
	if got := charRepo.characters[c1.ID].PvPWins; got != 1 {
		t.Fatalf("expected immediate PvPWins=1 after GvG round 1 strong defeat, got %d", got)
	}

	res2, err := svc.AdvanceRound(ctx, c1.ID, detail.Room.ID, 2)
	if err != nil || !res2.MatchCompleted {
		t.Fatalf("round 2 failed: res=%+v err=%v", res2, err)
	}
	if got := charRepo.characters[c1.ID].PvPWins; got != 2 {
		t.Fatalf("expected cumulative PvPWins=2 after GvG round 2 (excluding weak opponent), got %d", got)
	}
	if got := charRepo.characters[c2.ID].PvPWins; got != 0 {
		t.Fatalf("expected StrongBlue PvPWins=0, got %d", got)
	}
}
