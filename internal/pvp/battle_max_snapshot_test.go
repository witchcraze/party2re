package pvp_test

import (
	"context"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/pvp"
)

type boostedBuilder struct{}

func (boostedBuilder) BuildParticipantWithCurrentHP(_ context.Context, id string, hp int) (corebattle.Participant, error) {
	return corebattle.Participant{ID: id, HP: hp, MaxHP: 100, MaxMP: 20}, nil
}

func (boostedBuilder) BuildBattleMaxima(_ context.Context, char corecharacter.Character) (int, int, error) {
	if char.ID == "char-1" {
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

func TestColosseum_JoinSnapshotsBattleLineMaxHP(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharRepo()
	engine := &snapshotEngine{}
	svc, err := pvp.NewService(pvp.NewMemoryRoomRepository(), charRepo, engine,
		pvp.WithTransitionRepository(pvp.NewMemoryTransitionRepository()), pvp.WithParticipantBuilder(boostedBuilder{}))
	if err != nil {
		t.Fatal(err)
	}
	c1, c2 := createTestCharacter("char-1", "Alice", 500), createTestCharacter("char-2", "Bob", 500)
	charRepo.add(c1)
	charRepo.add(c2)
	detail, err := svc.CreateRoom(ctx, c1.ID, pvp.CreateRoomRequest{Name: "Duel", Bet: 10, MaxMembers: 2, TargetWins: 1})
	if err != nil {
		t.Fatal(err)
	}
	if detail, err = svc.JoinRoom(ctx, c2.ID, detail.Room.ID, ""); err != nil {
		t.Fatal(err)
	}
	roomID := detail.Room.ID
	if _, err := svc.SelectTeam(ctx, c1.ID, roomID, pvp.ColorRed); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SelectTeam(ctx, c2.ID, roomID, pvp.ColorBlue); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.StartMatch(ctx, c1.ID, roomID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AdvanceRound(ctx, c1.ID, roomID); err != nil {
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
