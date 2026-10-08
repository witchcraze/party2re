package casino_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/casino"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

func TestCharacterRoomObservationUsesActualAdmission(t *testing.T) {
	ctx := context.Background()
	repo := casino.NewMemoryRoomRepository()
	chars := &inMemoryCharRepo{chars: map[string]corecharacter.Character{"actor": {ID: "actor", PlayerID: "owner"}, "spectator": {ID: "spectator", PlayerID: "owner"}, "other": {ID: "other", PlayerID: "another"}}}
	svc, err := casino.NewService(newMockPrizeCasinoRepo(), casino.WithRoomRepository(repo), casino.WithCharacterRepository(chars))
	if err != nil {
		t.Fatal(err)
	}
	if got, err := svc.GetCharacterRoomView(ctx, "owner", "actor"); err != nil || got != nil {
		t.Fatalf("absent: %+v %v", got, err)
	}
	room := casino.Room{ID: "room", Status: casino.RoomStatusInProgress, GameType: casino.GameTypeIndian, Round: 1, UpdatedAt: time.Now(), PasswordHash: "secret"}
	if err := repo.CreateRoom(ctx, room, casino.RoomMember{RoomID: "room", CharacterID: "actor", Card: 3}); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddMember(ctx, casino.RoomMember{RoomID: "room", CharacterID: "spectator", IsSpectator: true, Card: -1}); err != nil {
		t.Fatal(err)
	}
	for _, actor := range []string{"actor", "spectator"} {
		view, err := svc.GetCharacterRoomView(ctx, "owner", actor)
		if err != nil || view == nil || view.Room.ID != "room" {
			t.Fatalf("admitted: %+v %v", view, err)
		}
		for _, member := range view.Members {
			if actor == "actor" && member.CharacterID == actor && member.Card != -1 {
				t.Fatal("own indian card leaked")
			}
		}
	}
	if got, err := svc.GetCharacterRoomView(ctx, "owner", "other"); got != nil || !errors.Is(err, casino.ErrRoomViewForbidden) {
		t.Fatalf("unowned: %+v %v", got, err)
	}
	if after, err := repo.GetRoom(ctx, "room"); err != nil || !reflect.DeepEqual(room, *after) {
		t.Fatalf("GET advanced/renewed room: %+v %v", after, err)
	}
}

type failedActivityIndex struct {
	casino.RoomRepository
	err   error
	calls int
}

func (r *failedActivityIndex) GetCharacterRoom(context.Context, string) (string, error) {
	r.calls++
	return "", r.err
}

func TestCharacterRoomObservationPropagatesIndexFailure(t *testing.T) {
	want := errors.New("activity index failed")
	repo := &failedActivityIndex{RoomRepository: casino.NewMemoryRoomRepository(), err: want}
	chars := &inMemoryCharRepo{chars: map[string]corecharacter.Character{"actor": {ID: "actor", PlayerID: "owner"}}}
	svc, err := casino.NewService(newMockPrizeCasinoRepo(), casino.WithRoomRepository(repo), casino.WithCharacterRepository(chars))
	if err != nil {
		t.Fatal(err)
	}
	if got, err := svc.GetCharacterRoomView(context.Background(), "other", "actor"); got != nil || !errors.Is(err, casino.ErrRoomViewForbidden) || repo.calls != 0 {
		t.Fatalf("read index before ownership: %+v %v calls=%d", got, err, repo.calls)
	}
	if got, err := svc.GetCharacterRoomView(context.Background(), "owner", "actor"); got != nil || !errors.Is(err, want) || repo.calls != 1 {
		t.Fatalf("hidden index failure: %+v %v", got, err)
	}
}
