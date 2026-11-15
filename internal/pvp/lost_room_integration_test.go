package pvp_test

import (
	"context"
	"errors"
	"testing"

	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/id"
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
	if err != nil || last.Detail.Room.PrizePool != 0 || last.ForfeitedPool+wallets != 2000 || last.Detail.Room.Bet*len(last.Detail.Members)+wallets != 2000 {
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

func TestPvPCompletedRoomLeaveIntegration(t *testing.T) {
	f := newTransitionFixture(t)
	f.prepare(t, "settle")
	ctx := context.Background()
	outsider, err := database.CreateTestCharacterWithFunds(ctx, f.db, "O"+id.New()[:7], 1000)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := f.db.ExecContext(ctx, "DELETE FROM character_depots WHERE character_id = ?", outsider.ID); err != nil {
			t.Error(err)
		}
		if _, err := f.db.ExecContext(ctx, "DELETE FROM characters WHERE id = ?", outsider.ID); err != nil {
			t.Error(err)
		}
		if _, err := f.db.ExecContext(ctx, "DELETE FROM players WHERE id = ?", outsider.PlayerID); err != nil {
			t.Error(err)
		}
	})
	res, err := f.service.AdvanceRound(ctx, f.leader.ID, f.room.Room.ID)
	if err != nil || !res.MatchCompleted {
		t.Fatalf("expected completed match, got %+v err=%v", res, err)
	}
	if err := f.service.LeaveRoom(ctx, outsider.ID, f.room.Room.ID); !errors.Is(err, pvp.ErrCharacterNotInRoom) {
		t.Fatalf("expected ErrCharacterNotInRoom for unrelated actor in completed room, got %v", err)
	}
	if err := f.service.LeaveRoom(ctx, f.leader.ID, f.room.Room.ID); err != nil {
		t.Fatalf("leader leave completed room: %v", err)
	}
	if err := f.service.LeaveRoom(ctx, f.leader.ID, f.room.Room.ID); err != nil {
		t.Fatalf("idempotent leader leave completed room: %v", err)
	}
	rem, err := f.service.GetRoom(ctx, f.room.Room.ID)
	if err != nil || rem.Room.LeaderCharacterID != f.member.ID || rem.Room.LeaderName != f.member.Name || len(rem.Members) != 1 {
		t.Fatalf("expected member promoted in completed room, got %+v err=%v", rem, err)
	}
	if err := f.service.LeaveRoom(ctx, f.member.ID, f.room.Room.ID); err != nil {
		t.Fatalf("final leave completed room: %v", err)
	}
	if err := f.service.LeaveRoom(ctx, f.member.ID, f.room.Room.ID); err != nil {
		t.Fatalf("idempotent final leave completed room: %v", err)
	}
	if _, err := f.service.GetRoom(ctx, f.room.Room.ID); !errors.Is(err, pvp.ErrRoomNotFound) {
		t.Fatalf("expected completed room deleted after final exit, got %v", err)
	}
	f.assertFunds(t, 900, 900, 1, 0)
}

func TestPvPCompletedRoomLeavePartialFailureIntegration(t *testing.T) {
	ctx := context.Background()

	// 1. Remaining-member exit: SaveRoom fails -> character mapping stays intact so activity retry still works.
	t.Run("save-failure-preserves-retry", func(t *testing.T) {
		f := newTransitionFixture(t)
		f.prepare(t, "settle")
		if _, err := f.service.AdvanceRound(ctx, f.leader.ID, f.room.Room.ID); err != nil {
			t.Fatal(err)
		}
		f.rooms.fail = "save"
		f.rooms.armed.Store(true)
		if err := f.service.LeaveRoom(ctx, f.leader.ID, f.room.Room.ID); !errors.Is(err, errPublication) {
			t.Fatalf("expected SaveRoom failure, got %v", err)
		}
		// Both room and character mapping still show leader as member so activity retry works.
		cur, err := f.service.GetCharacterRoom(ctx, f.leader.ID)
		if err != nil || len(cur.Members) != 2 || cur.Room.LeaderCharacterID != f.leader.ID {
			t.Fatalf("expected intact membership for activity retry, got %+v err=%v", cur, err)
		}
		if err := f.service.LeaveRoom(ctx, f.leader.ID, f.room.Room.ID); err != nil {
			t.Fatalf("retry leave after SaveRoom failure: %v", err)
		}
		rem, err := f.service.GetRoom(ctx, f.room.Room.ID)
		if err != nil || len(rem.Members) != 1 || rem.Room.LeaderCharacterID != f.member.ID || rem.Room.LeaderName != f.member.Name {
			t.Fatalf("expected clean promotion after retry, got %+v err=%v", rem, err)
		}
	})

	// 2. Remaining-member exit: SaveRoom succeeds, DeleteCharacterRoom fails -> room is updated, GetCharacterRoom returns ErrRoomNotFound (no mismatch error), direct retry clears mapping.
	t.Run("delete-member-failure-after-save", func(t *testing.T) {
		f := newTransitionFixture(t)
		f.prepare(t, "settle")
		if _, err := f.service.AdvanceRound(ctx, f.leader.ID, f.room.Room.ID); err != nil {
			t.Fatal(err)
		}
		f.rooms.fail = "delete-member"
		f.rooms.armed.Store(true)
		if err := f.service.LeaveRoom(ctx, f.leader.ID, f.room.Room.ID); !errors.Is(err, errPublication) {
			t.Fatalf("expected DeleteCharacterRoom failure, got %v", err)
		}
		rem, err := f.service.GetRoom(ctx, f.room.Room.ID)
		if err != nil || len(rem.Members) != 1 || rem.Room.LeaderCharacterID != f.member.ID || rem.Room.LeaderName != f.member.Name {
			t.Fatalf("expected room already updated on DeleteCharacterRoom failure, got %+v err=%v", rem, err)
		}
		if _, err := f.service.GetCharacterRoom(ctx, f.leader.ID); !errors.Is(err, pvp.ErrRoomNotFound) {
			t.Fatalf("expected ErrRoomNotFound instead of stale membership mismatch, got %v", err)
		}
		if err := f.service.LeaveRoom(ctx, f.leader.ID, f.room.Room.ID); err != nil {
			t.Fatalf("retry leave to clear dangling character mapping: %v", err)
		}
		if err := f.service.LeaveRoom(ctx, f.member.ID, f.room.Room.ID); err != nil {
			t.Fatalf("member exit after leader partial recovery: %v", err)
		}
		if _, err := f.service.GetRoom(ctx, f.room.Room.ID); !errors.Is(err, pvp.ErrRoomNotFound) {
			t.Fatalf("expected room deleted after member exit, got %v", err)
		}
	})

	// 3. Final-member exit: DeleteRoom fails -> character mapping stays intact so activity retry still works.
	t.Run("delete-room-failure-preserves-retry", func(t *testing.T) {
		f := newTransitionFixture(t)
		f.prepare(t, "settle")
		if _, err := f.service.AdvanceRound(ctx, f.leader.ID, f.room.Room.ID); err != nil {
			t.Fatal(err)
		}
		if err := f.service.LeaveRoom(ctx, f.leader.ID, f.room.Room.ID); err != nil {
			t.Fatal(err)
		}
		f.rooms.fail = "delete-room"
		f.rooms.armed.Store(true)
		if err := f.service.LeaveRoom(ctx, f.member.ID, f.room.Room.ID); !errors.Is(err, errPublication) {
			t.Fatalf("expected DeleteRoom failure, got %v", err)
		}
		cur, err := f.service.GetCharacterRoom(ctx, f.member.ID)
		if err != nil || len(cur.Members) != 1 {
			t.Fatalf("expected final member mapping preserved for retry, got %+v err=%v", cur, err)
		}
		if err := f.service.LeaveRoom(ctx, f.member.ID, f.room.Room.ID); err != nil {
			t.Fatalf("retry final leave after DeleteRoom failure: %v", err)
		}
		if _, err := f.service.GetRoom(ctx, f.room.Room.ID); !errors.Is(err, pvp.ErrRoomNotFound) {
			t.Fatalf("expected room deleted on retry, got %v", err)
		}
		if _, err := f.service.GetCharacterRoom(ctx, f.member.ID); !errors.Is(err, pvp.ErrRoomNotFound) {
			t.Fatalf("expected character mapping deleted on retry, got %v", err)
		}
	})
}
