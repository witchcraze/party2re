package casino_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/casino"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/testutil"
	"github.com/witchcraze/party2re/internal/testutil/valkeytest"
)

func TestProjectionStorageFailures(t *testing.T) {
	ctx := context.Background()
	failure := errors.New("store unavailable")
	for _, stage := range []string{"prune", "list", "room", "missing-prune", "expiry-index", "expiry-read", "delete-character", "delete-room", "delete-index"} {
		t.Run(stage, func(t *testing.T) {
			raw, err := json.Marshal(casino.RoomDetail{Room: casino.Room{ID: "room", UpdatedAt: time.Now().Add(-time.Hour)}, Members: []casino.RoomMember{{CharacterID: "member"}}})
			if err != nil {
				t.Fatal(err)
			}
			client := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(_ context.Context, cmd valkey.Completed) valkey.ValkeyResult {
				args := cmd.Commands()
				if (stage == "prune" && args[0] == "ZREMRANGEBYSCORE") || (stage == "list" && args[0] == "ZREVRANGE") || (stage == "room" && args[0] == "GET") || (stage == "missing-prune" && args[0] == "ZREM") || (stage == "expiry-index" && args[0] == "ZRANGEBYSCORE") || (stage == "expiry-read" && args[0] == "GET") || (stage == "delete-character" && args[0] == "DEL" && args[1] == casino.DefaultCharacterKeyPrefix+"member") || (stage == "delete-room" && args[0] == "DEL" && args[1] == casino.DefaultRoomKeyPrefix+"room") || (stage == "delete-index" && args[0] == "ZREM") {
					return valkeytest.MakeErrorResult(failure)
				}
				switch args[0] {
				case "ZREVRANGE", "ZRANGEBYSCORE":
					return valkeytest.MakeStringSliceResult([]string{"room"})
				case "GET":
					if stage == "missing-prune" {
						return valkeytest.MakeNilResult()
					}
					return valkeytest.MakeStringResult(string(raw))
				default:
					return valkeytest.MakeOKResult()
				}
			}))
			repo, err := casino.NewValkeyRoomRepository(client)
			if err != nil {
				t.Fatal(err)
			}
			if stage == "prune" || stage == "list" || stage == "room" || stage == "missing-prune" {
				_, err = repo.ListActiveRooms(ctx)
			} else {
				_, err = repo.PurgeIdleRooms(ctx, time.Now().Add(-casino.IdleRoomTimeout))
			}
			if !errors.Is(err, failure) {
				t.Fatalf("required error lost: %v", err)
			}
		})
	}
}

func TestExpiryRechecksActivityUnderLock(t *testing.T) {
	ctx := context.Background()
	base := newInMemoryValkeyClient()
	enumerated := make(chan struct{})
	renewed := make(chan struct{})
	client := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		result := base.Do(ctx, cmd)
		if cmd.Commands()[0] == "ZRANGEBYSCORE" {
			close(enumerated)
			select {
			case <-renewed:
			case <-ctx.Done():
				return valkeytest.MakeErrorResult(ctx.Err())
			}
		}
		return result
	}))
	repo, err := casino.NewValkeyRoomRepository(client)
	if err != nil {
		t.Fatal(err)
	}
	room := casino.Room{ID: "room", UpdatedAt: time.Now().Add(-time.Hour), Pot: 123}
	member := casino.RoomMember{RoomID: "room", CharacterID: "member", Card: 7}
	if err := repo.CreateRoom(ctx, room, member); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	errs := testutil.RunRace(
		func() error { _, err := repo.PurgeIdleRooms(ctx, time.Now().Add(-casino.IdleRoomTimeout)); return err },
		func() error {
			defer close(renewed)
			select {
			case <-enumerated:
			case <-ctx.Done():
				return ctx.Err()
			}
			room.UpdatedAt = time.Now()
			return repo.UpdateRoom(ctx, room)
		},
	)
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	got, err := repo.GetRoom(ctx, "room")
	if err != nil {
		t.Fatal(err)
	}
	members, err := repo.ListMembers(ctx, "room")
	if err != nil {
		t.Fatal(err)
	}
	if got.Pot != 123 || len(members) != 1 || members[0].Card != 7 {
		t.Fatalf("renewed room lost live state: %+v %+v", got, members)
	}
}

type failedEnrichment struct {
	casino.CharacterRepository
	failure error
}

func (r failedEnrichment) FindByID(context.Context, string) (corecharacter.Character, error) {
	return corecharacter.Character{}, r.failure
}

func TestProjectionRequiredEnrichmentFailure(t *testing.T) {
	ctx := context.Background()
	failure := errors.New("name unavailable")
	repo, err := casino.NewValkeyRoomRepository(newInMemoryValkeyClient(), casino.WithValkeyCharacterRepository(failedEnrichment{failure: failure}))
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateRoom(ctx, casino.Room{ID: "room"}, casino.RoomMember{RoomID: "room", CharacterID: "member"}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ListMembers(ctx, "room"); !errors.Is(err, failure) {
		t.Fatalf("enrichment error lost: %v", err)
	}
	if _, err := repo.GetMember(ctx, "room", "member"); !errors.Is(err, failure) {
		t.Fatalf("enrichment error lost: %v", err)
	}
	// Lobby needs only counts; no unnecessary private-name enrichment.
	if _, err := repo.ListActiveRooms(ctx); err != nil {
		t.Fatal(err)
	}
}
