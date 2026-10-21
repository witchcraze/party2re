package pvp_test

import (
	"context"
	"errors"
	"testing"

	"github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/pvp"
)

func TestPvPRejectsStaleFinancialSnapshotIntegration(t *testing.T) {
	f := newTransitionFixture(t)
	f.prepare(t, "leave")
	ctx := context.Background()
	stale, err := f.rooms.GetRoom(ctx, f.room.Room.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.act(t, "leave"); err != nil {
		t.Fatal(err)
	}
	current, err := f.rooms.GetRoom(ctx, f.room.Room.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a delayed older publisher after its lease expires.
	if err := f.rooms.SaveRoom(ctx, stale.Room, stale.Members); err != nil {
		t.Fatal(err)
	}
	if err := f.act(t, "leave"); !errors.Is(err, pvp.ErrTransitionConflict) {
		t.Fatalf("accepted stale refund: %v", err)
	}
	f.assertFunds(t, 900, 1000, 0, 100)
	if err := f.rooms.SaveRoom(ctx, current.Room, current.Members); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.JoinRoom(ctx, f.member.ID, f.room.Room.ID, "secret"); err != nil {
		t.Fatal(err)
	}
	if err := f.act(t, "leave"); err != nil {
		t.Fatal(err)
	}
	f.assertFunds(t, 900, 1000, 0, 100)
}

type pvpCommandFailure struct {
	valkey.Client
	command, key, wrongTypeKey string
}

func (c *pvpCommandFailure) Do(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
	args := cmd.Commands()
	if args[0] == c.command && args[1] == c.key {
		// Real server rejection without corrupting a shared room/index key.
		return c.Client.Do(ctx, c.B().Zadd().Key(c.wrongTypeKey).ScoreMember().ScoreMember(1, "fault").Build())
	}
	return c.Client.Do(ctx, cmd)
}

func TestPvPValkeyCommandRejectionIntegration(t *testing.T) {
	for _, command := range []string{"SET", "ZADD", "DEL"} {
		t.Run(command, func(t *testing.T) {
			f := newTransitionFixture(t)
			kind := "settle"
			if command == "DEL" {
				kind = "disband"
			}
			f.prepare(t, kind)
			ctx := context.Background()
			wrongTypeKey := pvp.DefaultRoomKeyPrefix + f.room.Room.ID + ":fault"
			if err := f.client.Do(ctx, f.client.B().Set().Key(wrongTypeKey).Value("wrongtype").Ex(pvp.DefaultLobbyTTL).Build()).Error(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := f.client.Do(ctx, f.client.B().Del().Key(wrongTypeKey).Build()).Error(); err != nil {
					t.Error(err)
				}
			})
			key := pvp.DefaultRoomKeyPrefix + f.room.Room.ID
			if command == "ZADD" {
				key = pvp.DefaultRoomsActiveIndexKey
			}
			client := &pvpCommandFailure{Client: f.client, command: command, key: key, wrongTypeKey: wrongTypeKey}
			rooms, err := pvp.NewValkeyRoomRepository(client)
			if err != nil {
				t.Fatal(err)
			}
			f.rooms.RoomRepository = rooms
			if err := f.act(t, kind); err == nil {
				t.Fatal("expected Valkey rejection")
			}
			healthy, err := pvp.NewValkeyRoomRepository(f.client)
			if err != nil {
				t.Fatal(err)
			}
			f.rooms.RoomRepository = healthy
			f.restart(t)
			if err := f.act(t, kind); err != nil {
				t.Fatal(err)
			}
			if kind == "settle" {
				f.assertFunds(t, 1100, 900, 1, 0)
			} else {
				f.assertFunds(t, 1000, 1000, 0, 0)
			}
		})
	}
}
