package pvp_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/pvp"
	"github.com/witchcraze/party2re/internal/testutil"
)

// ownedSweepStore keeps the sweep on this test's rooms; the shared DB may hold other tests' rooms.
type ownedSweepStore struct {
	pvp.TransitionRepository
	owned map[string]bool
}

func (s ownedSweepStore) ListOverdueRooms(ctx context.Context, cutoff time.Time, limit int) ([]string, error) {
	ids, err := s.TransitionRepository.ListOverdueRooms(ctx, cutoff, 1000)
	if err != nil {
		return nil, err
	}
	var owned []string
	for _, id := range ids {
		if s.owned[id] && len(owned) < limit {
			owned = append(owned, id)
		}
	}
	return owned, nil
}

func newSweepFixture(t *testing.T) (*transitionFixture, *atomic.Int64, *faultTimer, time.Time) {
	t.Helper()
	f := newTransitionFixture(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	timers := &faultTimer{Service: timer.NewService(f.client), err: errors.New("sleep timer unavailable")}
	cleanupSleepTimers(t, timers.Service, f.leader.ID, f.member.ID)
	clock := &atomic.Int64{}
	clock.Store(now.UnixNano())
	svc, err := pvp.NewService(f.rooms, f.chars, f.battle,
		pvp.WithTransactionProvider(database.NewTransactionProvider(f.db)),
		pvp.WithTransitionRepository(ownedSweepStore{TransitionRepository: f.store, owned: f.roomIDs}),
		pvp.WithPrizeRemitter(newSQLPrizeRemitter(t, f.db)),
		pvp.WithTimerService(timers),
		pvp.WithClock(func() time.Time { return time.Unix(0, clock.Load()).UTC() }),
	)
	if err != nil {
		t.Fatal(err)
	}
	f.service = svc
	room, err := svc.CreateRoom(ctx, f.leader.ID, pvp.CreateRoomRequest{Name: "SweepIdle", Bet: 100, MaxMembers: 2})
	if err != nil {
		t.Fatal(err)
	}
	f.room = room
	f.roomIDs[room.Room.ID] = true
	if _, err := svc.JoinRoom(ctx, f.member.ID, room.Room.ID, ""); err != nil {
		t.Fatal(err)
	}
	return f, clock, timers, now
}

func assertSweptTimeout(t *testing.T, f *transitionFixture, timeoutAt time.Time) {
	t.Helper()
	ctx := context.Background()
	assertPvPForfeited(t, f)
	tr, err := f.store.FindTransition(ctx, f.room.Room.ID)
	wantUntil := timeoutAt.Add(pvp.InactivitySleepDuration)
	if err != nil || tr.Pending || tr.Kind != "timeout" || tr.Detail.Room.Status != pvp.StatusDisbanded || !tr.PenaltyUntil.Equal(wantUntil) {
		t.Fatalf("unexpected timeout transition: %+v %v", tr, err)
	}
	for _, actor := range []string{f.leader.ID, f.member.ID} {
		c, err := f.chars.FindByID(ctx, actor)
		if err != nil || !c.PendingWake {
			t.Fatalf("actor %s missing PendingWake: %v", actor, err)
		}
		if exp := pexpireAt(t, f.client, timer.PrefixTimer+timer.CategorySleep+":"+actor); exp != wantUntil.UnixMilli() {
			t.Fatalf("actor %s sleep deadline=%d, want %d", actor, exp, wantUntil.UnixMilli())
		}
		if claim, err := f.service.GetCharacterClaim(ctx, actor); err != nil || claim != "" {
			t.Fatalf("actor %s retained claim: %q %v", actor, claim, err)
		}
	}
}

// The sweep stands in for legacy's quest-list cleanup (quest.cgi:80-91): no member acts.
func TestPvPSweepInactiveRoomsIntegration(t *testing.T) {
	f, clock, _, now := newSweepFixture(t)
	ctx := context.Background()

	clock.Store(now.Add(pvp.InactivityTimeout).UnixNano())
	if err := f.service.SweepInactiveRooms(ctx); err != nil {
		t.Fatal(err)
	}
	if tr, err := f.store.FindTransition(ctx, f.room.Room.ID); err != nil || tr.Kind == "timeout" {
		t.Fatalf("room timed out at the inclusive 1800s boundary: %+v %v", tr, err)
	}

	timeoutAt := now.Add(pvp.InactivityTimeout + time.Second)
	clock.Store(timeoutAt.UnixNano())
	if err := f.service.SweepInactiveRooms(ctx); err != nil {
		t.Fatal(err)
	}
	assertSweptTimeout(t, f, timeoutAt)
	if _, err := f.service.GetRoom(ctx, f.room.Room.ID); !errors.Is(err, pvp.ErrRoomNotFound) {
		t.Fatalf("swept room still published: %v", err)
	}

	before, _ := f.store.FindTransition(ctx, f.room.Room.ID)
	clock.Store(timeoutAt.Add(time.Minute).UnixNano())
	if err := f.service.SweepInactiveRooms(ctx); err != nil {
		t.Fatal(err)
	}
	if after, err := f.store.FindTransition(ctx, f.room.Room.ID); err != nil || after.Version != before.Version {
		t.Fatalf("repeated sweep changed the room: %d -> %+v %v", before.Version, after, err)
	}
	assertSweptTimeout(t, f, timeoutAt)
}

func TestPvPSweepRacesLiveMutationsIntegration(t *testing.T) {
	f, clock, _, now := newSweepFixture(t)
	ctx := context.Background()
	roomID := f.room.Room.ID
	colors := []string{pvp.ColorRed, pvp.ColorBlue}

	// Live room: concurrent sweeps never time out a room that activity keeps renewing.
	active := now.Add(pvp.InactivityTimeout - 100*time.Second)
	clock.Store(active.UnixNano())
	live := testutil.RunConcurrentStressTest(t, testutil.GetStressConfig(), func(worker, op int) error {
		switch worker % 3 {
		case 0:
			return f.service.SweepInactiveRooms(ctx)
		case 1:
			_, err := f.service.SelectTeam(ctx, f.member.ID, roomID, colors[op%2])
			return err
		default:
			_, err := f.service.SelectTeam(ctx, f.leader.ID, roomID, colors[(op+1)%2])
			return err
		}
	})
	if live.Failures != 0 || live.Successes == 0 {
		t.Fatalf("live race failed: %+v", live)
	}
	if tr, err := f.store.FindTransition(ctx, roomID); err != nil || tr.Kind == "timeout" || !tr.LastActivityAt.Equal(active) {
		t.Fatalf("live room was timed out or lost activity: %+v %v", tr, err)
	}

	// Overdue room: the sweep and every mutating path race; exactly one timeout applies.
	timeoutAt := active.Add(pvp.InactivityTimeout + time.Second)
	clock.Store(timeoutAt.UnixNano())
	overdue := testutil.RunConcurrentStressTest(t, testutil.GetStressConfig(), func(worker, op int) error {
		var err error
		switch worker % 5 {
		case 0:
			err = f.service.SweepInactiveRooms(ctx)
		case 1:
			_, err = f.service.SelectTeam(ctx, f.member.ID, roomID, colors[op%2])
		case 2:
			_, err = f.service.StartMatch(ctx, f.leader.ID, roomID)
		case 3:
			_, err = f.service.AdvanceRound(ctx, f.leader.ID, roomID)
		default:
			_, err = f.service.LeaveRoom(ctx, f.member.ID, roomID)
		}
		if errors.Is(err, pvp.ErrRoomNotFound) || errors.Is(err, pvp.ErrRoomInactive) {
			return nil
		}
		return err
	})
	if overdue.Failures != 0 || overdue.Successes == 0 {
		t.Fatalf("overdue race failed: %+v", overdue)
	}
	assertSweptTimeout(t, f, timeoutAt)
}

// A failed sweep publication stays pending with its fixed deadline; the next tick completes it.
func TestPvPSweepRetriesFailedPublicationIntegration(t *testing.T) {
	f, clock, timers, now := newSweepFixture(t)
	ctx := context.Background()
	timeoutAt := now.Add(pvp.InactivityTimeout + time.Second)
	clock.Store(timeoutAt.UnixNano())
	timers.failOnID = f.member.ID
	timers.armed.Store(true)
	if err := f.service.SweepInactiveRooms(ctx); err == nil {
		t.Fatal("sweep hid the sleep-timer failure")
	}
	if tr, err := f.store.FindTransition(ctx, f.room.Room.ID); err != nil || !tr.Pending || tr.Kind != "timeout" {
		t.Fatalf("failed publication not left pending: %+v %v", tr, err)
	}

	clock.Store(timeoutAt.Add(10 * time.Second).UnixNano())
	if err := f.service.SweepInactiveRooms(ctx); err != nil {
		t.Fatal(err)
	}
	assertSweptTimeout(t, f, timeoutAt)
}
