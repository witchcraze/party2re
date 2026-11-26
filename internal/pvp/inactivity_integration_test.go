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
	if _, err := svc.LeaveRoom(ctx, f.leader.ID, room.Room.ID); err != nil {
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
		_, err := f.service.LeaveRoom(ctx, actor, room.Room.ID)
		return err
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
	if _, err := f.service.LeaveRoom(ctx, f.leader.ID, room.Room.ID); err != nil {
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
	if _, err := svc.LeaveRoom(ctx, f.leader.ID, room.Room.ID); !errors.Is(err, timerErr) {
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
	if _, err := restarted.LeaveRoom(ctx, f.member.ID, room.Room.ID); err != nil {
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

type scriptDefeatBattle struct {
	defeats []corebattle.PartyBattleDefeat
}

func (b scriptDefeatBattle) ResolvePartyBattle(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
	rem := make(map[string]int, len(req.Allies)+len(req.Enemies))
	for _, p := range append(req.Allies, req.Enemies...) {
		rem[p.ID] = max(1, p.HP)
	}
	return corebattle.PartyBattleResult{
		Outcome:     corebattle.OutcomeDraw,
		Turns:       2,
		RemainingHP: rem,
		Defeats:     b.defeats,
	}, nil
}

func TestPvPInactivity_PendingKillsExpiredRoomAndRetryIntegration(t *testing.T) {
	f := newTransitionFixture(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	baseTimer := timer.NewService(f.client)
	cleanupSleepTimers(t, baseTimer, f.leader.ID, f.member.ID)

	timerErr := errors.New("injected sleep timer error")
	ft := &faultTimer{Service: baseTimer, failOnID: f.member.ID, err: timerErr}
	engine := scriptDefeatBattle{defeats: []corebattle.PartyBattleDefeat{{ActorID: f.leader.ID, TargetID: f.member.ID}}}

	svc, err := pvp.NewService(f.rooms, f.chars, engine,
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

	room, err := svc.CreateRoom(ctx, f.leader.ID, pvp.CreateRoomRequest{Name: "ExpiredKillP", Bet: 100, MaxMembers: 2, TargetWins: 2})
	if err != nil {
		t.Fatal(err)
	}
	f.room = room
	f.roomIDs[room.Room.ID] = true
	if _, err := svc.JoinRoom(ctx, f.member.ID, room.Room.ID, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SelectTeam(ctx, f.leader.ID, room.Room.ID, pvp.ColorRed); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SelectTeam(ctx, f.member.ID, room.Room.ID, pvp.ColorBlue); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.StartMatch(ctx, f.leader.ID, room.Room.ID); err != nil {
		t.Fatal(err)
	}

	// Non-final round 1: leader scores kill_p without changing FinancialVersion.
	res1, err := svc.AdvanceRound(ctx, f.leader.ID, room.Room.ID)
	if err != nil || res1.MatchCompleted {
		t.Fatalf("round 1 failed: res=%+v err=%v", res1, err)
	}
	if midLeader, _ := f.chars.FindByID(ctx, f.leader.ID); midLeader.PvPWins != 0 {
		t.Fatalf("expected PvPWins=0 mid-match before timeout/settlement, got %d", midLeader.PvPWins)
	}

	// Simulate Valkey room expiration after 30 minutes of inactivity.
	timeoutAt := now.Add(pvp.InactivityTimeout + time.Second)
	now = timeoutAt
	losePvPRoom(t, f)

	// Arm timer failure after SQL timeout commit, then retry on a fresh service instance.
	ft.armed.Store(true)
	if _, err := svc.LeaveRoom(ctx, f.leader.ID, room.Room.ID); !errors.Is(err, timerErr) {
		t.Fatalf("expected injected timer failure, got %v", err)
	}
	restarted, err := pvp.NewService(f.rooms, f.chars, engine,
		pvp.WithTransactionProvider(database.NewTransactionProvider(f.db)),
		pvp.WithTransitionRepository(f.store),
		pvp.WithPrizeRemitter(newSQLPrizeRemitter(t, f.db)),
		pvp.WithTimerService(baseTimer),
		pvp.WithClock(func() time.Time { return timeoutAt.Add(time.Minute) }),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.LeaveRoom(ctx, f.member.ID, room.Room.ID); err != nil {
		t.Fatalf("retry LeaveRoom: %v", err)
	}
	// Repeating cleanup/leave must never double-increment PvPWins.
	if _, err := restarted.LeaveRoom(ctx, f.leader.ID, room.Room.ID); err != nil && !errors.Is(err, pvp.ErrRoomNotFound) && !errors.Is(err, pvp.ErrCharacterNotInRoom) {
		t.Fatalf("second retry LeaveRoom unexpected error: %v", err)
	}

	gotLeader, err := f.chars.FindByID(ctx, f.leader.ID)
	if err != nil || gotLeader.PvPWins != 1 || !gotLeader.PendingWake {
		t.Fatalf("leader after timeout recovery = %+v (want PvPWins=1, PendingWake=true), err=%v", gotLeader, err)
	}
	gotMember, err := f.chars.FindByID(ctx, f.member.ID)
	if err != nil || gotMember.PvPWins != 0 || !gotMember.PendingWake {
		t.Fatalf("member after timeout recovery = %+v (want PvPWins=0, PendingWake=true), err=%v", gotMember, err)
	}
}

type afterCommitTransitionRepo struct {
	pvp.TransitionRepository
	onCommit func()
}

func (r *afterCommitTransitionRepo) CommitTransition(ctx context.Context, t pvp.Transition, mutate func(context.Context) error) error {
	if err := r.TransitionRepository.CommitTransition(ctx, t, mutate); err != nil {
		return err
	}
	if r.onCommit != nil {
		r.onCommit()
	}
	return nil
}

func TestPvPJoinDeadlineBoundariesIntegration(t *testing.T) {
	ctx := context.Background()

	newClocked := func(t *testing.T, start time.Time) (*transitionFixture, timer.Service, *atomic.Int64) {
		t.Helper()
		f := newTransitionFixture(t)
		timers := timer.NewService(f.client)
		cleanupSleepTimers(t, timers, f.leader.ID, f.member.ID)
		var clock atomic.Int64
		clock.Store(start.UnixNano())
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
		f.service = svc
		return f, timers, &clock
	}

	for _, tc := range []struct {
		name      string
		offset    time.Duration
		wantAllow bool
	}{
		{name: "sub-second remaining TTL", offset: pvp.InactivityTimeout - 500*time.Millisecond, wantAllow: true},
		{name: "exact 1800s deadline", offset: pvp.InactivityTimeout, wantAllow: true},
		{name: "just after 1800s deadline", offset: pvp.InactivityTimeout + time.Millisecond, wantAllow: false},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t0 := time.Now().UTC().Truncate(time.Second)
			f, timers, clock := newClocked(t, t0)

			room, err := f.service.CreateRoom(ctx, f.leader.ID, pvp.CreateRoomRequest{Name: "Boundary", Bet: 100, MaxMembers: 2})
			if err != nil {
				t.Fatal(err)
			}
			f.room = room
			f.roomIDs[room.Room.ID] = true

			at := t0.Add(tc.offset)
			clock.Store(at.UnixNano())

			joined, err := f.service.JoinRoom(ctx, f.member.ID, room.Room.ID, "")
			if tc.wantAllow {
				if err != nil {
					t.Fatalf("expected JoinRoom to succeed at offset %s, got %v", tc.offset, err)
				}
				if len(joined.Members) != 2 || joined.Room.PrizePool != 200 {
					t.Fatalf("unexpected joined room: %+v", joined)
				}
				live, err := f.service.GetRoom(ctx, room.Room.ID)
				if err != nil || len(live.Members) != 2 {
					t.Fatalf("GetRoom after join: %+v err=%v", live, err)
				}
				if mapped, err := f.rooms.GetCharacterRoom(ctx, f.member.ID); err != nil || mapped != room.Room.ID {
					t.Fatalf("GetCharacterRoom(member) = %q, err=%v", mapped, err)
				}
				tr, err := f.store.FindTransition(ctx, room.Room.ID)
				if err != nil || tr.Pending || !tr.LastActivityAt.Equal(t0) {
					t.Fatalf("JoinRoom altered activity or left pending: %+v err=%v", tr, err)
				}
				f.assertConservedFunds(t, 900, 900, 0, 200, 0)
				for _, actor := range []string{f.leader.ID, f.member.ID} {
					if claim, err := f.service.GetCharacterClaim(ctx, actor); err != nil || claim != room.Room.ID {
						t.Fatalf("actor %s claim=%q err=%v", actor, claim, err)
					}
					if locked, err := timers.IsLocked(ctx, timer.CategorySleep, actor); err != nil || locked {
						t.Fatalf("actor %s should not sleep: locked=%v err=%v", actor, locked, err)
					}
				}
				pttl, err := f.client.Do(ctx, f.client.B().Pttl().Key(pvp.DefaultRoomKeyPrefix+room.Room.ID).Build()).AsInt64()
				if err != nil || pttl <= 0 || pttl > 1500 {
					t.Fatalf("expected positive bounded PTTL (<=1500ms) near deadline, got %d err=%v", pttl, err)
				}
				return
			}

			if !errors.Is(err, pvp.ErrRoomNotFound) {
				t.Fatalf("expected ErrRoomNotFound after deadline, got %v", err)
			}
			// Failed fresh join leaves applicant wallet, claim and sleep unchanged.
			f.assertConservedFunds(t, 900, 1000, 0, 0, 100)
			if claim, err := f.service.GetCharacterClaim(ctx, f.member.ID); err != nil || claim != "" {
				t.Fatalf("rejected applicant retained claim %q err=%v", claim, err)
			}
			if locked, _ := timers.IsLocked(ctx, timer.CategorySleep, f.member.ID); locked {
				t.Fatal("rejected applicant must not receive sleep penalty")
			}
			// Leader (the actual roster) received genuine timeout penalty.
			gotLeader, _ := f.chars.FindByID(ctx, f.leader.ID)
			if !gotLeader.PendingWake {
				t.Fatal("leader missing PendingWake on timeout")
			}
			if exp := pexpireAt(t, f.client, timer.PrefixTimer+timer.CategorySleep+":"+f.leader.ID); exp != at.Add(pvp.InactivitySleepDuration).UnixMilli() {
				t.Fatalf("leader sleep=%d want %d", exp, at.Add(pvp.InactivitySleepDuration).UnixMilli())
			}
			if claim, err := f.service.GetCharacterClaim(ctx, f.leader.ID); err != nil || claim != "" {
				t.Fatalf("leader retained claim %q err=%v", claim, err)
			}
			if _, err := f.service.GetRoom(ctx, room.Room.ID); !errors.Is(err, pvp.ErrRoomNotFound) {
				t.Fatalf("expected Valkey room deleted, got %v", err)
			}
		})
	}

	t.Run("connectivity error at deadline does not trigger timeout", func(t *testing.T) {
		t0 := time.Now().UTC().Truncate(time.Second)
		f, timers, clock := newClocked(t, t0)

		room, err := f.service.CreateRoom(ctx, f.leader.ID, pvp.CreateRoomRequest{Name: "ConnBoundary", Bet: 100, MaxMembers: 2})
		if err != nil {
			t.Fatal(err)
		}
		f.room = room
		f.roomIDs[room.Room.ID] = true

		clock.Store(t0.Add(pvp.InactivityTimeout + time.Second).UnixNano())
		storageErr := errors.New("valkey unavailable")
		f.rooms.RoomRepository = unavailablePvPRooms{RoomRepository: f.rooms.RoomRepository, err: storageErr}

		if _, err := f.service.JoinRoom(ctx, f.member.ID, room.Room.ID, ""); !errors.Is(err, storageErr) {
			t.Fatalf("expected storageErr, got %v", err)
		}
		tr, err := f.store.FindTransition(ctx, room.Room.ID)
		if err != nil || tr.Kind != "create" || tr.Pending || tr.ForfeitedPool != 0 {
			t.Fatalf("connectivity failure mutated transition: %+v err=%v", tr, err)
		}
		f.assertConservedFunds(t, 900, 1000, 0, 100, 0)
		if locked, _ := timers.IsLocked(ctx, timer.CategorySleep, f.leader.ID); locked {
			t.Fatal("connectivity error must not impose sleep lock")
		}
	})
}

func TestPvPJoinCrossingDeadlineAndRecoveryIntegration(t *testing.T) {
	ctx := context.Background()

	t.Run("healthy pre-expiry join contrast", func(t *testing.T) {
		t0 := time.Now().UTC().Truncate(time.Second)
		f := newTransitionFixture(t)
		timers := timer.NewService(f.client)
		cleanupSleepTimers(t, timers, f.leader.ID, f.member.ID)
		now := t0
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
		room, err := svc.CreateRoom(ctx, f.leader.ID, pvp.CreateRoomRequest{Name: "HealthyJoin", Bet: 100, MaxMembers: 2})
		if err != nil {
			t.Fatal(err)
		}
		f.room = room
		f.roomIDs[room.Room.ID] = true

		now = t0.Add(10 * time.Minute)
		joined, err := svc.JoinRoom(ctx, f.member.ID, room.Room.ID, "")
		if err != nil || len(joined.Members) != 2 {
			t.Fatalf("healthy join failed: %+v err=%v", joined, err)
		}
		tr, err := f.store.FindTransition(ctx, room.Room.ID)
		if err != nil || tr.Pending || !tr.LastActivityAt.Equal(t0) {
			t.Fatalf("healthy join must stay activity-neutral: %+v err=%v", tr, err)
		}
		if mapped, err := f.rooms.GetCharacterRoom(ctx, f.member.ID); err != nil || mapped != room.Room.ID {
			t.Fatalf("member reverse mapping missing: %q err=%v", mapped, err)
		}
		f.assertConservedFunds(t, 900, 900, 0, 200, 0)
	})

	t.Run("join committed before expiry with publication crossing deadline", func(t *testing.T) {
		t0 := time.Now().UTC().Truncate(time.Second)
		f := newTransitionFixture(t)
		timers := timer.NewService(f.client)
		cleanupSleepTimers(t, timers, f.leader.ID, f.member.ID)
		var clock atomic.Int64
		clock.Store(t0.UnixNano())

		timeoutAt := t0.Add(pvp.InactivityTimeout + time.Second)
		armed := atomic.Bool{}
		wrappedStore := &afterCommitTransitionRepo{
			TransitionRepository: f.store,
			onCommit: func() {
				if armed.CompareAndSwap(true, false) {
					clock.Store(timeoutAt.UnixNano())
				}
			},
		}
		svc, err := pvp.NewService(f.rooms, f.chars, f.battle,
			pvp.WithTransactionProvider(database.NewTransactionProvider(f.db)),
			pvp.WithTransitionRepository(wrappedStore),
			pvp.WithPrizeRemitter(newSQLPrizeRemitter(t, f.db)),
			pvp.WithTimerService(timers),
			pvp.WithClock(func() time.Time { return time.Unix(0, clock.Load()).UTC() }),
		)
		if err != nil {
			t.Fatal(err)
		}
		room, err := svc.CreateRoom(ctx, f.leader.ID, pvp.CreateRoomRequest{Name: "CrossPub", Bet: 100, MaxMembers: 2})
		if err != nil {
			t.Fatal(err)
		}
		f.room = room
		f.roomIDs[room.Room.ID] = true

		// Join admitted at T+1799s; clock crosses T+1801s right after SQL commit before publishTransition.
		clock.Store(t0.Add(pvp.InactivityTimeout - time.Second).UnixNano())
		armed.Store(true)
		if _, err := svc.JoinRoom(ctx, f.member.ID, room.Room.ID, ""); !errors.Is(err, pvp.ErrRoomInactive) {
			t.Fatalf("expected ErrRoomInactive when publication crosses deadline, got %v", err)
		}

		tr, err := f.store.FindTransition(ctx, room.Room.ID)
		if err != nil || tr.Pending || tr.Kind != "timeout" || tr.Detail.Room.Status != pvp.StatusDisbanded || !tr.PenaltyUntil.Equal(timeoutAt.Add(pvp.InactivitySleepDuration)) {
			t.Fatalf("expected explicit timeout transition, got %+v err=%v", tr, err)
		}
		f.assertConservedFunds(t, 900, 900, 0, 0, 200)
		for _, actor := range []string{f.leader.ID, f.member.ID} {
			c, _ := f.chars.FindByID(ctx, actor)
			if !c.PendingWake {
				t.Fatalf("actor %s missing PendingWake", actor)
			}
			if exp := pexpireAt(t, f.client, timer.PrefixTimer+timer.CategorySleep+":"+actor); exp != timeoutAt.Add(pvp.InactivitySleepDuration).UnixMilli() {
				t.Fatalf("actor %s sleep=%d want %d", actor, exp, timeoutAt.Add(pvp.InactivitySleepDuration).UnixMilli())
			}
			if claim, err := svc.GetCharacterClaim(ctx, actor); err != nil || claim != "" {
				t.Fatalf("actor %s retained claim %q err=%v", actor, claim, err)
			}
			if _, err := f.rooms.GetCharacterRoom(ctx, actor); !errors.Is(err, pvp.ErrRoomNotFound) {
				t.Fatalf("actor %s retained reverse mapping: %v", actor, err)
			}
		}
		if _, err := svc.GetRoom(ctx, room.Room.ID); !errors.Is(err, pvp.ErrRoomNotFound) {
			t.Fatalf("expected live room deleted, got %v", err)
		}
	})

	for _, fault := range []string{"save", "set"} {
		fault := fault
		t.Run("failed join publication recovered across deadline/"+fault, func(t *testing.T) {
			t0 := time.Now().UTC().Truncate(time.Second)
			f := newTransitionFixture(t)
			baseTimer := timer.NewService(f.client)
			cleanupSleepTimers(t, baseTimer, f.leader.ID, f.member.ID)
			timerErr := errors.New("injected sleep timer error")
			ft := &faultTimer{Service: baseTimer, failOnID: f.member.ID, err: timerErr}

			var clock atomic.Int64
			clock.Store(t0.UnixNano())
			newSvc := func(tm pvp.TimerService) *pvp.Service {
				s, err := pvp.NewService(f.rooms, f.chars, f.battle,
					pvp.WithTransactionProvider(database.NewTransactionProvider(f.db)),
					pvp.WithTransitionRepository(f.store),
					pvp.WithPrizeRemitter(newSQLPrizeRemitter(t, f.db)),
					pvp.WithTimerService(tm),
					pvp.WithClock(func() time.Time { return time.Unix(0, clock.Load()).UTC() }),
				)
				if err != nil {
					t.Fatal(err)
				}
				return s
			}
			svc := newSvc(ft)

			room, err := svc.CreateRoom(ctx, f.leader.ID, pvp.CreateRoomRequest{Name: "RecoverJoin-" + fault, Bet: 100, MaxMembers: 2})
			if err != nil {
				t.Fatal(err)
			}
			f.room = room
			f.roomIDs[room.Room.ID] = true

			clock.Store(t0.Add(pvp.InactivityTimeout - time.Second).UnixNano())
			f.rooms.fail = fault
			f.rooms.armed.Store(true)
			if _, err := svc.JoinRoom(ctx, f.member.ID, room.Room.ID, ""); !errors.Is(err, errPublication) {
				t.Fatalf("expected errPublication, got %v", err)
			}

			// Before recovery: SQL holds pending join, claims for both, full escrow, no sleep penalty.
			beforeTr, err := f.store.FindTransition(ctx, room.Room.ID)
			if err != nil || !beforeTr.Pending || beforeTr.Kind != "join" || !beforeTr.LastActivityAt.Equal(t0) || len(beforeTr.Detail.Members) != 2 {
				t.Fatalf("unexpected pending join before recovery: %+v err=%v", beforeTr, err)
			}
			f.assertConservedFunds(t, 900, 900, 0, 200, 0)
			for _, actor := range []string{f.leader.ID, f.member.ID} {
				if claim, err := svc.GetCharacterClaim(ctx, actor); err != nil || claim != room.Room.ID {
					t.Fatalf("actor %s missing claim before recovery: %q err=%v", actor, claim, err)
				}
			}
			if _, err := f.rooms.GetCharacterRoom(ctx, f.member.ID); !errors.Is(err, pvp.ErrRoomNotFound) {
				t.Fatalf("member reverse mapping should be absent before recovery, got %v", err)
			}

			// Cross deadline and arm timer fault to verify intermediate pending timeout outcome.
			timeoutAt := t0.Add(pvp.InactivityTimeout + time.Second)
			clock.Store(timeoutAt.UnixNano())
			ft.armed.Store(true)
			if _, err := svc.LeaveRoom(ctx, f.member.ID, room.Room.ID); !errors.Is(err, timerErr) {
				t.Fatalf("expected timerErr during timeout recovery, got %v", err)
			}
			midTr, err := f.store.FindTransition(ctx, room.Room.ID)
			wantUntil := timeoutAt.Add(pvp.InactivitySleepDuration)
			if err != nil || !midTr.Pending || midTr.Kind != "timeout" || midTr.Detail.Room.Status != pvp.StatusDisbanded || midTr.ForfeitedPool != 200 || !midTr.PenaltyUntil.Equal(wantUntil) {
				t.Fatalf("unexpected pending timeout transition: %+v err=%v", midTr, err)
			}
			if until, pending, err := svc.GetPendingSleepPenalty(ctx, f.member.ID); err != nil || !pending || !until.Equal(wantUntil) {
				t.Fatalf("GetPendingSleepPenalty=(%s, %v, %v), want (%s, true, nil)", until, pending, err, wantUntil)
			}
			if claim, err := svc.GetCharacterClaim(ctx, f.member.ID); err != nil || claim != room.Room.ID {
				t.Fatalf("claim released before timer completed: %q err=%v", claim, err)
			}

			// Complete recovery on fresh service instance 5 minutes later; JoinRoom observes sleeping penalty.
			clock.Store(timeoutAt.Add(5 * time.Minute).UnixNano())
			restarted := newSvc(baseTimer)
			if _, err := restarted.JoinRoom(ctx, f.member.ID, room.Room.ID, ""); !errors.Is(err, pvp.ErrCharacterSleeping) {
				t.Fatalf("expected ErrCharacterSleeping on recovery retry, got %v", err)
			}

			afterTr, err := f.store.FindTransition(ctx, room.Room.ID)
			if err != nil || afterTr.Pending || afterTr.Kind != "timeout" || !afterTr.PenaltyUntil.Equal(wantUntil) {
				t.Fatalf("unexpected final transition: %+v err=%v", afterTr, err)
			}
			f.assertConservedFunds(t, 900, 900, 0, 0, 200)
			for _, actor := range []string{f.leader.ID, f.member.ID} {
				c, _ := f.chars.FindByID(ctx, actor)
				if !c.PendingWake {
					t.Fatalf("actor %s missing PendingWake after recovery", actor)
				}
				if exp := pexpireAt(t, f.client, timer.PrefixTimer+timer.CategorySleep+":"+actor); exp != wantUntil.UnixMilli() {
					t.Fatalf("actor %s sleep=%d want %d", actor, exp, wantUntil.UnixMilli())
				}
				if claim, err := restarted.GetCharacterClaim(ctx, actor); err != nil || claim != "" {
					t.Fatalf("actor %s retained claim after recovery: %q err=%v", actor, claim, err)
				}
				if _, err := f.rooms.GetCharacterRoom(ctx, actor); !errors.Is(err, pvp.ErrRoomNotFound) {
					t.Fatalf("actor %s retained reverse mapping after recovery: %v", actor, err)
				}
			}
			if _, err := restarted.GetRoom(ctx, room.Room.ID); !errors.Is(err, pvp.ErrRoomNotFound) {
				t.Fatalf("expected room deleted after recovery, got %v", err)
			}
		})
	}
}
