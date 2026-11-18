package gvg_test

import (
	"context"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/guild"
	"github.com/witchcraze/party2re/internal/gvg"
)

// boostedBuilder reports a battle maximum above the stored one, as a held 113 does.
type boostedBuilder struct{}

func (boostedBuilder) BuildParticipantWithCurrentHP(_ context.Context, id string, hp int) (corebattle.Participant, error) {
	return corebattle.Participant{ID: id, HP: hp, MaxHP: 100, MaxMP: 20}, nil
}

func (boostedBuilder) BuildBattleMaxima(_ context.Context, char corecharacter.Character) (int, int, error) {
	if char.ID == "c1" {
		return 200, 20, nil
	}
	return 100, 120, nil
}

type snapshotEngine struct {
	mockBattleEngine
	request corebattle.PartyBattleRequest
}

func (e *snapshotEngine) ResolvePartyBattle(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
	e.request = req
	return e.mockBattleEngine.ResolvePartyBattle(req)
}

func TestJoinSnapshotsBattleLineMaxHP(t *testing.T) {
	ctx := context.Background()
	guildRepo, charRepo := newMockGuildRepo(), newMockCharRepo()
	engine := &snapshotEngine{mockBattleEngine: mockBattleEngine{outcome: corebattle.OutcomeWin}}
	svc, err := gvg.NewService(gvg.NewMemoryRoomRepository(), newMockStandingRepo(), guildRepo, charRepo,
		engine, gvg.WithParticipantBuilder(boostedBuilder{}))
	if err != nil {
		t.Fatal(err)
	}
	charRepo.characters["c1"] = createTestCharacter("c1", "Leader", 100, 0)
	charRepo.characters["c2"] = createTestCharacter("c2", "Joiner", 100, 0)
	guildRepo.guilds["g1"] = guild.Guild{ID: "g1", Name: "Crimson", Color: "#FF3333"}
	guildRepo.guilds["g2"] = guild.Guild{ID: "g2", Name: "Azure", Color: "#33CCFF"}
	guildRepo.charGuilds["c1"], guildRepo.charGuilds["c2"] = "g1", "g2"

	detail, err := svc.CreateRoom(ctx, "c1", gvg.CreateRoomRequest{Name: "GvG", MaxMembers: 2, TargetWins: 1})
	if err != nil {
		t.Fatal(err)
	}
	if detail, err = svc.JoinRoom(ctx, "c2", detail.Room.ID, ""); err != nil {
		t.Fatal(err)
	}
	roomID := detail.Room.ID
	if _, err := svc.StartMatch(ctx, "c1", roomID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AdvanceRound(ctx, "c1", roomID, 1); err != nil {
		t.Fatal(err)
	}
	if len(engine.request.Allies) != 1 || len(engine.request.Enemies) != 1 {
		t.Fatalf("expected both participants in the battle: %+v", engine.request)
	}
	leader, joiner := engine.request.Allies[0], engine.request.Enemies[0]
	if leader.HP != 200 || leader.MaxHP != 200 || leader.MaxMP != 20 ||
		joiner.HP != 100 || joiner.MaxHP != 100 || joiner.MaxMP != 120 {
		t.Fatalf("battle did not use saved maxima: leader %+v, joiner %+v", leader, joiner)
	}
}
