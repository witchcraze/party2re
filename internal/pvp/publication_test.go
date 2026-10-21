package pvp_test

import (
	"context"
	"errors"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/pvp"
)

type failingPublicationRooms struct {
	pvp.RoomRepository
	failSave bool
}

func (r *failingPublicationRooms) SaveRoom(ctx context.Context, room pvp.ColosseumRoom, members []pvp.RoomMember) error {
	if r.failSave {
		return errors.New("publication failed")
	}
	return r.RoomRepository.SaveRoom(ctx, room, members)
}

func TestCreatePublicationRetryPreservesBet(t *testing.T) {
	ctx := context.Background()
	chars := newMockCharRepo()
	chars.add(createTestCharacter("leader", "Leader", 1000))
	rooms := &failingPublicationRooms{RoomRepository: pvp.NewMemoryRoomRepository(), failSave: true}
	svc, err := pvp.NewService(rooms, chars, mockBattleEngine{result: corebattle.PartyBattleResult{Outcome: corebattle.OutcomeWin}}, pvp.WithTransitionRepository(pvp.NewMemoryTransitionRepository()))
	if err != nil {
		t.Fatal(err)
	}
	req := pvp.CreateRoomRequest{Name: "Retry", Bet: 100}
	if _, err := svc.CreateRoom(ctx, "leader", req); err == nil {
		t.Fatal("expected publication error")
	}
	rooms.failSave = false
	room, err := svc.CreateRoom(ctx, "leader", req)
	if err != nil {
		t.Fatal(err)
	}
	char, err := chars.FindByID(ctx, "leader")
	if err != nil {
		t.Fatal(err)
	}
	if char.Money != 900 || room.Room.PrizePool != 100 {
		t.Fatalf("money=%d pool=%d; expected a single 100G bet", char.Money, room.Room.PrizePool)
	}
}
