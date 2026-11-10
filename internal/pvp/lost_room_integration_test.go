package pvp_test

import (
	"context"
	"errors"
	"testing"

	"github.com/witchcraze/party2re/internal/pvp"
	"github.com/witchcraze/party2re/internal/testutil"
)

func losePvPRoom(t *testing.T, f *transitionFixture) {
	t.Helper()
	// Expiry/eviction can remove the room while its reverse mappings survive.
	if err := f.client.Do(context.Background(), f.client.B().Del().Key(pvp.DefaultRoomKeyPrefix+f.room.Room.ID).Build()).Error(); err != nil {
		t.Fatal(err)
	}
}

func assertPvPForfeited(t *testing.T, f *transitionFixture) {
	t.Helper()
	ctx := context.Background()
	wallets := 0
	for _, actor := range []string{f.leader.ID, f.member.ID} {
		char, err := f.chars.FindByID(ctx, actor)
		if err != nil || char.Money != 900 || char.PvPWins != 0 {
			t.Fatalf("recovery changed wallet/victories: %+v %v", char, err)
		}
		wallets += char.Money
	}
	last, err := f.store.FindTransition(ctx, f.room.Room.ID)
	if err != nil || last.Detail.Room.PrizePool != 0 || last.Detail.Room.Bet*len(last.Detail.Members)+wallets != 2000 {
		t.Fatalf("wallets plus forfeiture do not conserve assets: %+v %v", last, err)
	}
}

func TestPvPLostRoomRecoveryConcurrentIntegration(t *testing.T) {
	for _, phase := range []string{"recruiting", "in_progress"} {
		t.Run(phase, func(t *testing.T) {
			f := newTransitionFixture(t)
			kind := "leave"
			if phase == "in_progress" {
				kind = "settle"
			}
			f.prepare(t, kind)
			ctx := context.Background()
			before, err := f.store.FindTransition(ctx, f.room.Room.ID)
			if err != nil {
				t.Fatal(err)
			}
			losePvPRoom(t, f)
			f.restart(t)
			result := testutil.RunConcurrentStressTest(t, testutil.GetStressConfig(), func(worker, op int) error {
				actor := f.leader.ID
				if worker%2 == 0 {
					actor = f.member.ID
				}
				return f.service.LeaveRoom(ctx, actor, f.room.Room.ID)
			})
			if result.Failures != 0 || result.Successes == 0 {
				t.Fatalf("recovery: %+v", result)
			}
			assertPvPForfeited(t, f)
			after, err := f.store.FindTransition(ctx, f.room.Room.ID)
			if err != nil || after.Pending || after.Version != before.Version+1 || after.Kind != "abandon" || after.Detail.Room.Status != pvp.StatusDisbanded || len(after.RemovedIDs) != 2 {
				t.Fatalf("terminal recovery evidence: %+v %v", after, err)
			}
			// Wallets plus the forfeited escrow still account for the original gold.
			if after.Detail.Room.Bet*len(after.Detail.Members) != before.Detail.Room.PrizePool {
				t.Fatal("forfeiture lost escrow evidence")
			}
			for _, actor := range []string{f.leader.ID, f.member.ID} {
				if claim, err := f.service.GetCharacterClaim(ctx, actor); err != nil || claim != "" {
					t.Fatalf("claim remains: %s %v", claim, err)
				}
				if _, err := f.service.GetCharacterRoom(ctx, actor); !errors.Is(err, pvp.ErrRoomNotFound) {
					t.Fatalf("live membership remains: %v", err)
				}
			}
			oldRoom := f.room.Room.ID
			if err := f.create(t); err != nil {
				t.Fatalf("re-entry: %v", err)
			}
			if err := f.service.LeaveRoom(ctx, f.leader.ID, oldRoom); err != nil {
				t.Fatal(err)
			}
			if room, err := f.service.GetCharacterRoom(ctx, f.leader.ID); err != nil || room.Room.ID != f.room.Room.ID {
				t.Fatalf("old retry erased new membership: %+v %v", room, err)
			}
		})
	}
}

func TestPvPLostRoomRecoveryPublicationRetryIntegration(t *testing.T) {
	f := newTransitionFixture(t)
	f.prepare(t, "leave")
	ctx := context.Background()
	losePvPRoom(t, f)
	f.rooms.fail = "delete-member"
	f.rooms.armed.Store(true)
	if err := f.service.LeaveRoom(ctx, f.member.ID, f.room.Room.ID); !errors.Is(err, errPublication) {
		t.Fatalf("missing publication failure: %v", err)
	}
	assertPvPForfeited(t, f)
	if claim, err := f.service.GetCharacterClaim(ctx, f.member.ID); err != nil || claim != f.room.Room.ID {
		t.Fatalf("claim released before publication: %s %v", claim, err)
	}
	f.restart(t)
	if err := f.service.LeaveRoom(ctx, f.leader.ID, f.room.Room.ID); err != nil {
		t.Fatal(err)
	}
	if claim, err := f.service.GetCharacterClaim(ctx, f.member.ID); err != nil || claim != "" {
		t.Fatalf("retry retained claim: %s %v", claim, err)
	}
	assertPvPForfeited(t, f)
}

func TestPvPLostRoomRecoversPendingPublicationBeforeAbandoningIntegration(t *testing.T) {
	f := newTransitionFixture(t)
	f.prepare(t, "leave")
	ctx := context.Background()
	f.rooms.fail = "save"
	f.rooms.armed.Store(true)
	if err := f.service.LeaveRoom(ctx, f.member.ID, f.room.Room.ID); !errors.Is(err, errPublication) {
		t.Fatal(err)
	}
	losePvPRoom(t, f)
	f.restart(t)
	if err := f.service.LeaveRoom(ctx, f.leader.ID, f.room.Room.ID); err != nil {
		t.Fatal(err)
	}
	f.assertFunds(t, 1000, 1000, 0, 0)
	last, err := f.store.FindTransition(ctx, f.room.Room.ID)
	if err != nil || last.Kind != "disband" {
		t.Fatalf("pending publication mistaken for room loss: %+v %v", last, err)
	}
}

type unavailablePvPRooms struct {
	pvp.RoomRepository
	err error
}

func (r unavailablePvPRooms) GetRoom(context.Context, string) (pvp.RoomDetail, error) {
	return pvp.RoomDetail{}, r.err
}

func TestPvPLostRoomRecoveryRejectsUnownedAndUnavailableIntegration(t *testing.T) {
	f := newTransitionFixture(t)
	if err := f.create(t); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	losePvPRoom(t, f)
	if err := f.service.LeaveRoom(ctx, f.member.ID, f.room.Room.ID); !errors.Is(err, pvp.ErrCharacterNotInRoom) {
		t.Fatalf("unowned recovery: %v", err)
	}
	before, err := f.store.FindTransition(ctx, f.room.Room.ID)
	if err != nil {
		t.Fatal(err)
	}
	storageErr := errors.New("Valkey unavailable")
	f.rooms.RoomRepository = unavailablePvPRooms{RoomRepository: f.rooms.RoomRepository, err: storageErr}
	if err := f.service.LeaveRoom(ctx, f.leader.ID, f.room.Room.ID); !errors.Is(err, storageErr) {
		t.Fatalf("storage failure treated as loss: %v", err)
	}
	after, err := f.store.FindTransition(ctx, f.room.Room.ID)
	if err != nil || after.Version != before.Version || after.Pending {
		t.Fatalf("rejected recovery changed claim: %+v %v", after, err)
	}
	f.assertFunds(t, 900, 1000, 0, 100)
}
