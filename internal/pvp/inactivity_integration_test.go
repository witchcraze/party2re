package pvp_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/valkey-io/valkey-go"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/pvp"
	"github.com/witchcraze/party2re/internal/testutil"
)

type faultTimer struct {
	timer.Service
	failOnID string
	armed    atomic.Bool
	err      error
}

func (t *faultTimer) ExtendLockUntil(ctx context.Context, category, targetID string, until time.Time) error {
	if t.armed.Load() && (t.failOnID == "" || t.failOnID == targetID) && t.armed.CompareAndSwap(true, false) {
		return t.err
	}
	return t.Service.ExtendLockUntil(ctx, category, targetID, until)
}

func cleanupSleepTimers(t *testing.T, timers timer.Service, ids ...string) {
	t.Helper()
	t.Cleanup(func() {
		for _, id := range ids {
			if err := timers.ReleaseLock(context.Background(), timer.CategorySleep, id); err != nil {
				t.Error(err)
			}
		}
	})
}

func pexpireAt(t *testing.T, client valkey.Client, key string) int64 {
	t.Helper()
	ms, err := client.Do(context.Background(), client.B().Pexpiretime().Key(key).Build()).AsInt64()
	if err != nil {
		t.Fatalf("PEXPIRETIME %s: %v", key, err)
	}
	return ms
}

func TestPvPActivityTTLsAndEvictionProtectionIntegration(t *testing.T) {
	f := newTransitionFixture(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	timers := timer.NewService(f.client)
	cleanupSleepTimers(t, timers, f.leader.ID, f.member.ID)

	svc, err := pvp.NewService(f.rooms, f.chars, f.battle,
		pvp.WithTransactionProvider(database.NewTransactionProvider(f.db)),
		pvp.WithTransitionRepository(f.store),
		pvp.WithPrizeRemitter(newSQLPrizeRemitter(t, f.db)),
		pvp.WithTimerService(timers),
		pvp.WithClock(func() time.Time { return now }),
	)
	if err != nil {
		t.Fatal(err)
	}
	f.service = svc

	room, err := svc.CreateRoom(ctx, f.leader.ID, pvp.CreateRoomRequest{Name: "TTLParity", Bet: 100, MaxMembers: 2})
	if err != nil {
		t.Fatal(err)
	}
	f.room = room
	f.roomIDs[room.Room.ID] = true

	// JoinRoom 10m later must NOT extend room/mapping TTL past CreateRoom + 1800s.
	now = now.Add(10 * time.Minute)
	if _, err := svc.JoinRoom(ctx, f.member.ID, room.Room.ID, ""); err != nil {
		t.Fatal(err)
	}
	roomTTL, err := f.client.Do(ctx, f.client.B().Ttl().Key(pvp.DefaultRoomKeyPrefix+room.Room.ID).Build()).AsInt64()
	if err != nil || roomTTL <= 1000 || roomTTL > 1250 {
		t.Fatalf("JoinRoom extended room TTL unexpectedly: %d (%v)", roomTTL, err)
	}
	memberMapTTL, err := f.client.Do(ctx, f.client.B().Ttl().Key(pvp.DefaultCharacterKeyPrefix+f.member.ID).Build()).AsInt64()
	if err != nil || memberMapTTL <= 1000 || memberMapTTL > 1250 {
		t.Fatalf("JoinRoom extended mapping TTL unexpectedly: %d (%v)", memberMapTTL, err)
	}

	// SelectTeam refreshes SQL last_activity_at and both room & reverse-mapping TTLs.
	now = now.Add(15 * time.Minute) // T0 + 25m
	if _, err := svc.SelectTeam(ctx, f.leader.ID, room.Room.ID, pvp.ColorRed); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SelectTeam(ctx, f.member.ID, room.Room.ID, pvp.ColorBlue); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.StartMatch(ctx, f.leader.ID, room.Room.ID); err != nil {
		t.Fatal(err)
	}
	refreshedRoomTTL, err := f.client.Do(ctx, f.client.B().Ttl().Key(pvp.DefaultRoomKeyPrefix+room.Room.ID).Build()).AsInt64()
	refreshedMapTTL, err2 := f.client.Do(ctx, f.client.B().Ttl().Key(pvp.DefaultCharacterKeyPrefix+f.member.ID).Build()).AsInt64()
	if err != nil || err2 != nil || refreshedRoomTTL < 1700 || refreshedMapTTL < 1700 {
		t.Fatalf("StartMatch did not renew room/mapping TTLs: room=%d map=%d (%v, %v)", refreshedRoomTTL, refreshedMapTTL, err, err2)
	}

	// Arbitrary Valkey eviction at T0+35m (only 10m after StartMatch at T0+25m) must NOT trigger inactivity sleep!
	now = now.Add(10 * time.Minute) // T0 + 35m, StartMatch + 10m
	losePvPRoom(t, f)
	if err := svc.LeaveRoom(ctx, f.leader.ID, room.Room.ID); err != nil {
		t.Fatalf("LeaveRoom after eviction: %v", err)
	}
	tr, err := f.store.FindTransition(ctx, room.Room.ID)
	if err != nil || tr.Kind != "abandon" {
		t.Fatalf("expected abandon (not timeout) because StartMatch updated SQL last_activity_at: %+v %v", tr, err)
	}
	for _, actor := range []string{f.leader.ID, f.member.ID} {
		c, _ := f.chars.FindByID(ctx, actor)
		if c.PendingWake {
			t.Fatalf("actor %s should not have PendingWake on eviction", actor)
		}
		if locked, _ := timers.IsLocked(ctx, timer.CategorySleep, actor); locked {
			t.Fatalf("actor %s should not have sleep lock on eviction", actor)
		}
	}
}

func TestPvPInactivityCleanupConcurrentIntegration(t *testing.T) {
	f := newTransitionFixture(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	timers := timer.NewService(f.client)
	cleanupSleepTimers(t, timers, f.leader.ID, f.member.ID)

	var clock atomic.Int64
	clock.Store(now.UnixNano())
	newSvc := func() *pvp.Service {
		svc, err := pvp.NewService(f.rooms, f.chars, f.battle,
			pvp.WithTransactionProvider(database.NewTransactionProvider(f.db)),
			pvp.WithTransitionRepository(f.store),
			pvp.WithPrizeRemitter(newSQLPrizeRemitter(t, f.db)),
			pvp.WithTimerService(timers),
			pvp.WithClock(func() time.Time { return time.Unix(0, clock.Load()).UTC() }),
		)
		if err != nil {
			t.Fatal(err)
		}
		return svc
	}
	f.service = newSvc()

	room, err := f.service.CreateRoom(ctx, f.leader.ID, pvp.CreateRoomRequest{Name: "ConcurrentIdle", Bet: 100, MaxMembers: 2})
	if err != nil {
		t.Fatal(err)
	}
	f.room = room
	f.roomIDs[room.Room.ID] = true
	if _, err := f.service.JoinRoom(ctx, f.member.ID, room.Room.ID, ""); err != nil {
		t.Fatal(err)
	}

	// Simulate room expiration in Valkey after 1801s of inactivity.
	timeoutAt := now.Add(pvp.InactivityTimeout + time.Second)
	clock.Store(timeoutAt.UnixNano())
	losePvPRoom(t, f)

	result := testutil.RunConcurrentStressTest(t, testutil.GetStressConfig(), func(worker, op int) error {
		actor := f.leader.ID
		if worker%2 == 0 {
			actor = f.member.ID
		}
		return f.service.LeaveRoom(ctx, actor, room.Room.ID)
	})
	if result.Failures != 0 || result.Successes == 0 {
		t.Fatalf("concurrent inactivity cleanup failed: %+v", result)
	}

	assertPvPForfeited(t, f)
	tr, err := f.store.FindTransition(ctx, room.Room.ID)
	wantUntil := timeoutAt.Add(pvp.InactivitySleepDuration)
	if err != nil || tr.Pending || tr.Kind != "timeout" || tr.Detail.Room.Status != pvp.StatusDisbanded || !tr.PenaltyUntil.Equal(wantUntil) {
		t.Fatalf("unexpected timeout transition: %+v %v", tr, err)
	}
	for _, actor := range []string{f.leader.ID, f.member.ID} {
		c, _ := f.chars.FindByID(ctx, actor)
		if !c.PendingWake {
			t.Fatalf("actor %s missing PendingWake", actor)
		}
		exp := pexpireAt(t, f.client, timer.PrefixTimer+timer.CategorySleep+":"+actor)
		if exp != wantUntil.UnixMilli() {
			t.Fatalf("actor %s sleep deadline=%d, want %d", actor, exp, wantUntil.UnixMilli())
		}
		if claim, err := f.service.GetCharacterClaim(ctx, actor); err != nil || claim != "" {
			t.Fatalf("actor %s retained claim: %q %v", actor, claim, err)
		}
	}

	// Valid newer membership after recovery is never erased by delayed old room retry.
	if _, err := f.db.ExecContext(ctx, "UPDATE characters SET pending_wake = FALSE WHERE id = ?", f.leader.ID); err != nil {
		t.Fatal(err)
	}
	if err := timers.ReleaseLock(ctx, timer.CategorySleep, f.leader.ID); err != nil {
		t.Fatal(err)
	}
	newRoom, err := f.service.CreateRoom(ctx, f.leader.ID, pvp.CreateRoomRequest{Name: "Fresh", Bet: 100})
	if err != nil {
		t.Fatalf("CreateRoom after wakeup: %v", err)
	}
	f.roomIDs[newRoom.Room.ID] = true
	if err := f.service.LeaveRoom(ctx, f.leader.ID, room.Room.ID); err != nil {
		t.Fatalf("delayed old LeaveRoom retry: %v", err)
	}
	if cur, err := f.service.GetCharacterRoom(ctx, f.leader.ID); err != nil || cur.Room.ID != newRoom.Room.ID {
		t.Fatalf("old retry erased newer membership: %+v %v", cur, err)
	}
}

func TestPvPInactivityInterruptedCleanupIntegration(t *testing.T) {
	f := newTransitionFixture(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	baseTimer := timer.NewService(f.client)
	cleanupSleepTimers(t, baseTimer, f.leader.ID, f.member.ID)

	timerErr := errors.New("injected sleep timer error")
	ft := &faultTimer{Service: baseTimer, failOnID: f.member.ID, err: timerErr}
	svc, err := pvp.NewService(f.rooms, f.chars, f.battle,
		pvp.WithTransactionProvider(database.NewTransactionProvider(f.db)),
		pvp.WithTransitionRepository(f.store),
		pvp.WithPrizeRemitter(newSQLPrizeRemitter(t, f.db)),
		pvp.WithTimerService(ft),
		pvp.WithClock(func() time.Time { return now }),
	)
	if err != nil {
		t.Fatal(err)
	}
	f.service = svc

	room, err := svc.CreateRoom(ctx, f.leader.ID, pvp.CreateRoomRequest{Name: "Interrupted", Bet: 100, MaxMembers: 2})
	if err != nil {
		t.Fatal(err)
	}
	f.room = room
	f.roomIDs[room.Room.ID] = true
	if _, err := svc.JoinRoom(ctx, f.member.ID, room.Room.ID, ""); err != nil {
		t.Fatal(err)
	}

	// Preserve existing longer sleep on leader, fail timer write on member.
	longer := now.Add(3 * time.Hour).Truncate(time.Millisecond)
	if err := baseTimer.SetLockUntil(ctx, timer.CategorySleep, f.leader.ID, longer); err != nil {
		t.Fatal(err)
	}

	timeoutAt := now.Add(pvp.InactivityTimeout + time.Second)
	now = timeoutAt
	ft.armed.Store(true)
	if err := svc.LeaveRoom(ctx, f.leader.ID, room.Room.ID); !errors.Is(err, timerErr) {
		t.Fatalf("expected injected timer failure, got %v", err)
	}
	if claim, err := svc.GetCharacterClaim(ctx, f.member.ID); err != nil || claim != room.Room.ID {
		t.Fatalf("claim released before timer write succeeded: %q %v", claim, err)
	}

	// Retry 10 minutes later on a fresh service instance: preserves original PenaltyUntil and leader's longer sleep.
	now = timeoutAt.Add(10 * time.Minute)
	restarted, err := pvp.NewService(f.rooms, f.chars, &transitionBattle{outcome: corebattle.OutcomeWin},
		pvp.WithTransactionProvider(database.NewTransactionProvider(f.db)),
		pvp.WithTransitionRepository(f.store),
		pvp.WithPrizeRemitter(newSQLPrizeRemitter(t, f.db)),
		pvp.WithTimerService(baseTimer),
		pvp.WithClock(func() time.Time { return now }),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := restarted.LeaveRoom(ctx, f.member.ID, room.Room.ID); err != nil {
		t.Fatalf("retry LeaveRoom: %v", err)
	}
	wantUntil := timeoutAt.Add(pvp.InactivitySleepDuration)
	if exp := pexpireAt(t, f.client, timer.PrefixTimer+timer.CategorySleep+":"+f.member.ID); exp != wantUntil.UnixMilli() {
		t.Fatalf("member sleep=%d, want %d (must not reset on retry)", exp, wantUntil.UnixMilli())
	}
	if exp := pexpireAt(t, f.client, timer.PrefixTimer+timer.CategorySleep+":"+f.leader.ID); exp != longer.UnixMilli() {
		t.Fatalf("leader longer sleep=%d, want %d", exp, longer.UnixMilli())
	}
}
