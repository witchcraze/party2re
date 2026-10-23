package casino_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/witchcraze/party2re/internal/casino"
)

func TestRoomPublication_AdapterParity(t *testing.T) {
	valkeyRooms, err := casino.NewValkeyRoomRepository(newInMemoryValkeyClient())
	if err != nil {
		t.Fatal(err)
	}
	for name, rooms := range map[string]casino.RoomRepository{"memory": casino.NewMemoryRoomRepository(), "valkey": valkeyRooms} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			room := casino.Room{ID: "room", PasswordHash: "secret", HasPassword: true, Status: casino.RoomStatusInProgress}
			member := casino.RoomMember{RoomID: room.ID, CharacterID: "leader", Card: 12}
			spectator := casino.RoomMember{RoomID: room.ID, CharacterID: "spectator", IsSpectator: true, Card: -1}
			loser := casino.RoomMember{RoomID: room.ID, CharacterID: "loser", Card: 0}
			if err := rooms.CreateRoom(ctx, room, member); err != nil {
				t.Fatal(err)
			}
			for _, m := range []casino.RoomMember{spectator, loser} {
				if err := rooms.AddMember(ctx, m); err != nil {
					t.Fatal(err)
				}
			}
			member.Action = "showdown"
			room.Pot = 10
			if err := rooms.UpdateRoomDetail(ctx, casino.RoomDetail{Room: room, Members: []casino.RoomMember{member, spectator}}); err != nil {
				t.Fatal(err)
			}
			got, err := rooms.GetRoom(ctx, room.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(*got, room) {
				t.Fatalf("room/password lost: %+v", got)
			}
			for _, want := range []casino.RoomMember{member, spectator} {
				got, err := rooms.GetMember(ctx, room.ID, want.CharacterID)
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(*got, want) {
					t.Fatalf("member changed: %+v want %+v", got, want)
				}
				if mapped, err := rooms.GetCharacterRoom(ctx, want.CharacterID); err != nil || mapped != room.ID {
					t.Fatalf("admission lost: %s %v", mapped, err)
				}
			}
			mapped, err := rooms.GetCharacterRoom(ctx, loser.CharacterID)
			if err != nil && !errors.Is(err, casino.ErrRoomNotFound) {
				t.Fatal(err)
			}
			if mapped != "" {
				t.Fatalf("eliminated actor retained: %s", mapped)
			}
		})
	}
}
