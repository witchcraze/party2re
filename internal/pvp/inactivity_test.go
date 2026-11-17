package pvp_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/pvp"
)

type failingTimer struct {
	timer.Service
	failOnID  string
	failCalls atomic.Int32
	failErr   error
}

func (f *failingTimer) ExtendLockUntil(ctx context.Context, category, targetID string, until time.Time) error {
	if f.failErr != nil && (f.failOnID == "" || f.failOnID == targetID) {
		if f.failCalls.Add(1) == 1 {
			return f.failErr
		}
	}
	return f.Service.ExtendLockUntil(ctx, category, targetID, until)
}

type failingReadRooms struct {
	pvp.RoomRepository
	err error
}

func (r failingReadRooms) GetRoom(context.Context, string) (pvp.RoomDetail, error) {
	return pvp.RoomDetail{}, r.err
}

func newClockedService(t *testing.T, outcome corebattle.Outcome, start time.Time) (*pvp.Service, *mockCharRepo, *pvp.MemoryRoomRepository, *pvp.MemoryTransitionRepository, timer.Service, *time.Time) {
	t.Helper()
	now := start
	charRepo := newMockCharRepo()
	roomRepo := pvp.NewMemoryRoomRepository()
	transRepo := pvp.NewMemoryTransitionRepository()
	timers := timer.NewService(nil)
	svc, err := pvp.NewService(
		roomRepo,
		charRepo,
		mockBattleEngine{result: corebattle.PartyBattleResult{Outcome: outcome, Turns: 1}},
		pvp.WithTransitionRepository(transRepo),
		pvp.WithPrizeRemitter(charRepo),
		pvp.WithTimerService(timers),
		pvp.WithClock(func() time.Time { return now }),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc, charRepo, roomRepo, transRepo, timers, &now
}

func TestPvPInactivity_ClockBoundariesAndRefreshingEvents(t *testing.T) {
	ctx := context.Background()
	t0 := time.Now().UTC().Truncate(time.Second)
	svc, chars, _, trans, timers, now := newClockedService(t, corebattle.OutcomeWin, t0)

	alice := createTestCharacter("char-1", "Alice", 500)
	bob := createTestCharacter("char-2", "Bob", 500)
	chars.add(alice)
	chars.add(bob)

	room, err := svc.CreateRoom(ctx, alice.ID, pvp.CreateRoomRequest{Name: "Clock", Bet: 100, MaxMembers: 2, TargetWins: 2})
	if err != nil {
		t.Fatal(err)
	}

	// JoinRoom does NOT refresh LastActivityAt (legacy quest.cgi:add_member writes quest.cgi, not room log.cgi).
	*now = t0.Add(10 * time.Minute)
	if _, err := svc.JoinRoom(ctx, bob.ID, room.Room.ID, ""); err != nil {
		t.Fatal(err)
	}
	tr, err := trans.FindTransition(ctx, room.Room.ID)
	if err != nil || !tr.LastActivityAt.Equal(t0) {
		t.Fatalf("JoinRoom changed LastActivityAt: %s want %s (err=%v)", tr.LastActivityAt, t0, err)
	}

	// GETs never refresh LastActivityAt.
	*now = t0.Add(20 * time.Minute)
	if _, err := svc.GetRoom(ctx, room.Room.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ListRooms(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetCharacterRoom(ctx, alice.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetCharacterClaim(ctx, alice.ID); err != nil {
		t.Fatal(err)
	}
	tr, _ = trans.FindTransition(ctx, room.Room.ID)
	if !tr.LastActivityAt.Equal(t0) {
		t.Fatalf("GET refreshed LastActivityAt: %s want %s", tr.LastActivityAt, t0)
	}

	// SelectTeam refreshes LastActivityAt.
	*now = t0.Add(25 * time.Minute)
	if _, err := svc.SelectTeam(ctx, alice.ID, room.Room.ID, pvp.ColorRed); err != nil {
		t.Fatal(err)
	}
	tr, _ = trans.FindTransition(ctx, room.Room.ID)
	if !tr.LastActivityAt.Equal(*now) {
		t.Fatalf("SelectTeam did not refresh LastActivityAt: %s want %s", tr.LastActivityAt, *now)
	}

	// At exactly +1800s from SelectTeam, room is NOT inactive yet (strictly > 1800s).
	*now = now.Add(pvp.InactivityTimeout)
	if _, err := svc.SelectTeam(ctx, bob.ID, room.Room.ID, pvp.ColorBlue); err != nil {
		t.Fatalf("expected active at exact 1800s boundary, got %v", err)
	}

	// StartMatch and non-final AdvanceRound refresh LastActivityAt.
	*now = now.Add(5 * time.Minute)
	if _, err := svc.StartMatch(ctx, alice.ID, room.Room.ID); err != nil {
		t.Fatal(err)
	}
	tr, _ = trans.FindTransition(ctx, room.Room.ID)
	if !tr.LastActivityAt.Equal(*now) {
		t.Fatalf("StartMatch did not refresh LastActivityAt: %s want %s", tr.LastActivityAt, *now)
	}

	*now = now.Add(5 * time.Minute)
	if _, err := svc.AdvanceRound(ctx, alice.ID, room.Room.ID); err != nil {
		t.Fatal(err)
	}
	lastAct := *now
	tr, _ = trans.FindTransition(ctx, room.Room.ID)
	if !tr.LastActivityAt.Equal(lastAct) {
		t.Fatalf("AdvanceRound did not refresh LastActivityAt: %s want %s", tr.LastActivityAt, lastAct)
	}

	// At lastAct + 1801s, LeaveRoom triggers inactivity cleanup for both participants.
	*now = lastAct.Add(pvp.InactivityTimeout + time.Second)
	if err := svc.LeaveRoom(ctx, bob.ID, room.Room.ID); err != nil {
		t.Fatalf("LeaveRoom timeout cleanup: %v", err)
	}
	// Idempotent second LeaveRoom does not error or re-penalize.
	if err := svc.LeaveRoom(ctx, alice.ID, room.Room.ID); err != nil {
		t.Fatalf("second LeaveRoom after timeout: %v", err)
	}

	tr, _ = trans.FindTransition(ctx, room.Room.ID)
	if tr.Kind != "timeout" || tr.Detail.Room.Status != pvp.StatusDisbanded || tr.ForfeitedPool != 200 || tr.Pending {
		t.Fatalf("unexpected timeout transition: %+v", tr)
	}
	for _, id := range []string{alice.ID, bob.ID} {
		c, _ := chars.FindByID(ctx, id)
		if c.Money != 400 || !c.PendingWake {
			t.Fatalf("actor %s money=%d pendingWake=%v; want 400/true", id, c.Money, c.PendingWake)
		}
		if locked, err := timers.IsLocked(ctx, timer.CategorySleep, id); err != nil || !locked {
			t.Fatalf("actor %s missing sleep lock: locked=%v err=%v", id, locked, err)
		}
		if claim, err := svc.GetCharacterClaim(ctx, id); err != nil || claim != "" {
			t.Fatalf("actor %s retained claim: %q %v", id, claim, err)
		}
	}
}

func TestPvPInactivity_CompletedRoomTimeoutAndEvictionDistinction(t *testing.T) {
	ctx := context.Background()
	t0 := time.Now().UTC().Truncate(time.Second)

	t.Run("completed room inactivity penalizes only remaining members", func(t *testing.T) {
		svc, chars, _, trans, timers, now := newClockedService(t, corebattle.OutcomeWin, t0)
		alice := createTestCharacter("char-1", "Alice", 500)
		bob := createTestCharacter("char-2", "Bob", 500)
		chars.add(alice)
		chars.add(bob)

		room, _ := svc.CreateRoom(ctx, alice.ID, pvp.CreateRoomRequest{Name: "CompletedIdle", Bet: 100, TargetWins: 1})
		_, _ = svc.JoinRoom(ctx, bob.ID, room.Room.ID, "")
		_, _ = svc.SelectTeam(ctx, alice.ID, room.Room.ID, pvp.ColorRed)
		_, _ = svc.SelectTeam(ctx, bob.ID, room.Room.ID, pvp.ColorBlue)
		_, _ = svc.StartMatch(ctx, alice.ID, room.Room.ID)
		res, err := svc.AdvanceRound(ctx, alice.ID, room.Room.ID)
		if err != nil || !res.MatchCompleted {
			t.Fatalf("AdvanceRound: %+v %v", res, err)
		}

		// Alice leaves right away: no sleep penalty for Alice.
		*now = t0.Add(time.Minute)
		if err := svc.LeaveRoom(ctx, alice.ID, room.Room.ID); err != nil {
			t.Fatal(err)
		}
		aliceAfter, _ := chars.FindByID(ctx, alice.ID)
		if aliceAfter.PendingWake {
			t.Fatal("departed Alice should not have PendingWake")
		}

		// Bob stays past 1800s after Alice's leave -> timeout applies only to Bob.
		*now = now.Add(pvp.InactivityTimeout + time.Second)
		if err := svc.LeaveRoom(ctx, bob.ID, room.Room.ID); err != nil {
			t.Fatal(err)
		}
		tr, _ := trans.FindTransition(ctx, room.Room.ID)
		if tr.Kind != "timeout" || tr.ForfeitedPool != 0 || len(tr.RemovedIDs) != 1 || tr.RemovedIDs[0] != bob.ID {
			t.Fatalf("unexpected completed timeout transition: %+v", tr)
		}
		aliceFinal, _ := chars.FindByID(ctx, alice.ID)
		bobFinal, _ := chars.FindByID(ctx, bob.ID)
		if aliceFinal.PendingWake || !bobFinal.PendingWake {
			t.Fatalf("PendingWake alice=%v bob=%v; want false/true", aliceFinal.PendingWake, bobFinal.PendingWake)
		}
		if locked, _ := timers.IsLocked(ctx, timer.CategorySleep, alice.ID); locked {
			t.Fatal("Alice should not be sleeping")
		}
		if locked, _ := timers.IsLocked(ctx, timer.CategorySleep, bob.ID); !locked {
			t.Fatal("Bob should be sleeping")
		}
	})

	t.Run("arbitrary eviction before 1800s adds no sleep", func(t *testing.T) {
		svc, chars, rooms, trans, timers, now := newClockedService(t, corebattle.OutcomeWin, t0)
		alice := createTestCharacter("char-1", "Alice", 500)
		chars.add(alice)
		room, _ := svc.CreateRoom(ctx, alice.ID, pvp.CreateRoomRequest{Name: "Evicted", Bet: 100})
		*now = t0.Add(5 * time.Minute)
		if err := rooms.DeleteRoom(ctx, room.Room.ID); err != nil {
			t.Fatal(err)
		}
		if err := svc.LeaveRoom(ctx, alice.ID, room.Room.ID); err != nil {
			t.Fatal(err)
		}
		tr, _ := trans.FindTransition(ctx, room.Room.ID)
		if tr.Kind != "abandon" || tr.ForfeitedPool != 100 {
			t.Fatalf("expected abandon without timeout, got %+v", tr)
		}
		c, _ := chars.FindByID(ctx, alice.ID)
		if c.PendingWake {
			t.Fatal("missing-key recovery must not set PendingWake")
		}
		if locked, _ := timers.IsLocked(ctx, timer.CategorySleep, alice.ID); locked {
			t.Fatal("missing-key recovery must not set sleep lock")
		}
	})
}

func TestPvPInactivity_ConnectivityErrorAndInterruptedRetry(t *testing.T) {
	ctx := context.Background()
	t0 := time.Now().UTC().Truncate(time.Second)

	t.Run("connectivity failure does not become inactivity penalty", func(t *testing.T) {
		now := t0
		chars := newMockCharRepo()
		alice := createTestCharacter("char-1", "Alice", 500)
		chars.add(alice)
		rooms := pvp.NewMemoryRoomRepository()
		trans := pvp.NewMemoryTransitionRepository()
		timers := timer.NewService(nil)
		storageErr := errors.New("valkey unavailable")

		svc, _ := pvp.NewService(rooms, chars, mockBattleEngine{}, pvp.WithTransitionRepository(trans), pvp.WithTimerService(timers), pvp.WithClock(func() time.Time { return now }))
		room, _ := svc.CreateRoom(ctx, alice.ID, pvp.CreateRoomRequest{Name: "ConnErr", Bet: 100})

		now = t0.Add(pvp.InactivityTimeout + time.Minute)
		downSvc, _ := pvp.NewService(failingReadRooms{RoomRepository: rooms, err: storageErr}, chars, mockBattleEngine{}, pvp.WithTransitionRepository(trans), pvp.WithTimerService(timers), pvp.WithClock(func() time.Time { return now }))
		if err := downSvc.LeaveRoom(ctx, alice.ID, room.Room.ID); !errors.Is(err, storageErr) {
			t.Fatalf("expected storageErr, got %v", err)
		}
		tr, _ := trans.FindTransition(ctx, room.Room.ID)
		if tr.Kind != "create" || tr.Pending {
			t.Fatalf("connectivity failure mutated transition: %+v", tr)
		}
		c, _ := chars.FindByID(ctx, alice.ID)
		if c.PendingWake {
			t.Fatal("connectivity failure set PendingWake")
		}
	})

	t.Run("timer failure preserves fixed deadline and retry does not extend", func(t *testing.T) {
		now := t0
		chars := newMockCharRepo()
		alice := createTestCharacter("char-1", "Alice", 500)
		bob := createTestCharacter("char-2", "Bob", 500)
		chars.add(alice)
		chars.add(bob)
		rooms := pvp.NewMemoryRoomRepository()
		trans := pvp.NewMemoryTransitionRepository()
		baseTimer := timer.NewService(nil)
		timerErr := errors.New("timer down")
		ft := &failingTimer{Service: baseTimer, failOnID: bob.ID, failErr: timerErr}

		svc, _ := pvp.NewService(rooms, chars, mockBattleEngine{}, pvp.WithTransitionRepository(trans), pvp.WithTimerService(ft), pvp.WithClock(func() time.Time { return now }))
		room, _ := svc.CreateRoom(ctx, alice.ID, pvp.CreateRoomRequest{Name: "Retry", Bet: 100})
		_, _ = svc.JoinRoom(ctx, bob.ID, room.Room.ID, "")

		timeoutAt := t0.Add(pvp.InactivityTimeout + time.Second)
		now = timeoutAt
		if err := svc.LeaveRoom(ctx, alice.ID, room.Room.ID); !errors.Is(err, timerErr) {
			t.Fatalf("expected timerErr, got %v", err)
		}
		until, pending, err := svc.GetPendingSleepPenalty(ctx, bob.ID)
		wantUntil := timeoutAt.Add(pvp.InactivitySleepDuration)
		if err != nil || !pending || !until.Equal(wantUntil) {
			t.Fatalf("pending penalty=(%s, %v, %v), want (%s, true, nil)", until, pending, err, wantUntil)
		}

		// Retry 10 minutes later: uses saved PenaltyUntil without adding another 3600s.
		now = timeoutAt.Add(10 * time.Minute)
		if err := svc.LeaveRoom(ctx, bob.ID, room.Room.ID); err != nil {
			t.Fatalf("retry LeaveRoom: %v", err)
		}
		_, pendingAfter, err := svc.GetPendingSleepPenalty(ctx, bob.ID)
		if err != nil || pendingAfter {
			t.Fatalf("expected pending cleared after retry, got %v %v", pendingAfter, err)
		}
		tr, _ := trans.FindTransition(ctx, room.Room.ID)
		if !tr.PenaltyUntil.Equal(wantUntil) || tr.Pending {
			t.Fatalf("transition after retry: %+v", tr)
		}
		if _, err := svc.CreateRoom(ctx, alice.ID, pvp.CreateRoomRequest{Name: "Blocked", Bet: 100}); !errors.Is(err, pvp.ErrCharacterSleeping) {
			t.Fatalf("expected ErrCharacterSleeping on CreateRoom while sleeping, got %v", err)
		}
	})
}

func TestPvPInactivity_LazyCleanupOnOtherMutations(t *testing.T) {
	ctx := context.Background()
	t0 := time.Now().UTC().Truncate(time.Second)

	for _, op := range []string{"select", "start", "advance", "join", "create"} {
		t.Run(op, func(t *testing.T) {
			svc, chars, _, trans, timers, now := newClockedService(t, corebattle.OutcomeWin, t0)
			alice := createTestCharacter("char-1", "Alice", 500)
			bob := createTestCharacter("char-2", "Bob", 500)
			charlie := createTestCharacter("char-3", "Charlie", 500)
			chars.add(alice)
			chars.add(bob)
			chars.add(charlie)

			room, _ := svc.CreateRoom(ctx, alice.ID, pvp.CreateRoomRequest{Name: "Lazy-" + op, Bet: 100, MaxMembers: 3})
			if op == "start" || op == "advance" {
				_, _ = svc.JoinRoom(ctx, bob.ID, room.Room.ID, "")
				_, _ = svc.SelectTeam(ctx, alice.ID, room.Room.ID, pvp.ColorRed)
				_, _ = svc.SelectTeam(ctx, bob.ID, room.Room.ID, pvp.ColorBlue)
			}
			if op == "advance" {
				_, _ = svc.StartMatch(ctx, alice.ID, room.Room.ID)
			}

			*now = t0.Add(pvp.InactivityTimeout + time.Second)
			var err error
			switch op {
			case "select":
				_, err = svc.SelectTeam(ctx, alice.ID, room.Room.ID, pvp.ColorRed)
			case "start":
				_, err = svc.StartMatch(ctx, alice.ID, room.Room.ID)
			case "advance":
				_, err = svc.AdvanceRound(ctx, alice.ID, room.Room.ID)
			case "join":
				_, err = svc.JoinRoom(ctx, charlie.ID, room.Room.ID, "")
			case "create":
				_, err = svc.CreateRoom(ctx, alice.ID, pvp.CreateRoomRequest{Name: "New", Bet: 100})
			}

			wantErr := pvp.ErrRoomNotFound
			if op == "create" {
				wantErr = pvp.ErrCharacterSleeping
			}
			if !errors.Is(err, wantErr) {
				t.Fatalf("%s err=%v, want %v", op, err, wantErr)
			}
			tr, _ := trans.FindTransition(ctx, room.Room.ID)
			if tr.Kind != "timeout" || tr.Detail.Room.Status != pvp.StatusDisbanded {
				t.Fatalf("expected timeout transition, got %+v", tr)
			}
			if locked, _ := timers.IsLocked(ctx, timer.CategorySleep, alice.ID); !locked {
				t.Fatal("Alice should be sleeping after lazy cleanup")
			}
			if op == "join" {
				charlieChar, _ := chars.FindByID(ctx, charlie.ID)
				if charlieChar.Money != 500 || charlieChar.PendingWake {
					t.Fatalf("non-member Charlie affected by room timeout: %+v", charlieChar)
				}
			}
		})
	}
}

func TestPvPInactivity_LongerSleepAndPendingLeaveBeforeTimeout(t *testing.T) {
	ctx := context.Background()

	t.Run("preserves existing longer sleep", func(t *testing.T) {
		t0 := time.Now().UTC().Truncate(time.Second)
		svc, chars, _, _, timers, now := newClockedService(t, corebattle.OutcomeWin, t0)
		alice := createTestCharacter("char-1", "Alice", 500)
		chars.add(alice)
		room, _ := svc.CreateRoom(ctx, alice.ID, pvp.CreateRoomRequest{Name: "LongSleep", Bet: 100})

		longer := time.Now().UTC().Add(2 * time.Hour)
		if err := timers.SetLockUntil(ctx, timer.CategorySleep, alice.ID, longer); err != nil {
			t.Fatal(err)
		}
		*now = t0.Add(pvp.InactivityTimeout + time.Second)
		if err := svc.LeaveRoom(ctx, alice.ID, room.Room.ID); err != nil {
			t.Fatal(err)
		}
		rem, err := timers.GetRemainingLock(ctx, timer.CategorySleep, alice.ID)
		if err != nil || rem <= pvp.InactivitySleepDuration+5*time.Minute {
			t.Fatalf("longer sleep was shortened: rem=%s err=%v", rem, err)
		}
	})

	t.Run("pending non-timeout leave resolves before timing out remaining members", func(t *testing.T) {
		t0 := time.Now().UTC().Truncate(time.Second)
		now := t0
		chars := newMockCharRepo()
		alice := createTestCharacter("char-1", "Alice", 500)
		bob := createTestCharacter("char-2", "Bob", 500)
		chars.add(alice)
		chars.add(bob)
		rooms := pvp.NewMemoryRoomRepository()
		trans := pvp.NewMemoryTransitionRepository()
		timers := timer.NewService(nil)

		svc, _ := pvp.NewService(rooms, chars, mockBattleEngine{}, pvp.WithTransitionRepository(trans), pvp.WithTimerService(timers), pvp.WithClock(func() time.Time { return now }))
		room, _ := svc.CreateRoom(ctx, alice.ID, pvp.CreateRoomRequest{Name: "PendingLeave", Bet: 100})
		_, _ = svc.JoinRoom(ctx, bob.ID, room.Room.ID, "")

		// Simulate Bob committing LeaveRoom (refunded) at T0+5m with failed Valkey publication.
		now = t0.Add(5 * time.Minute)
		failErr := errors.New("save failed")
		faultRooms := &failingDeleteRooms{RoomRepository: rooms, failSave: failErr}
		faultSvc, _ := pvp.NewService(faultRooms, chars, mockBattleEngine{}, pvp.WithTransitionRepository(trans), pvp.WithTimerService(timers), pvp.WithClock(func() time.Time { return now }))
		if err := faultSvc.LeaveRoom(ctx, bob.ID, room.Room.ID); !errors.Is(err, failErr) {
			t.Fatalf("expected failErr, got %v", err)
		}

		// After T0+5m+1801s, Alice triggers LeaveRoom -> Bob's pending leave finishes without penalty, Alice times out.
		now = now.Add(pvp.InactivityTimeout + time.Second)
		if err := svc.LeaveRoom(ctx, alice.ID, room.Room.ID); err != nil {
			t.Fatalf("LeaveRoom: %v", err)
		}
		bobAfter, _ := chars.FindByID(ctx, bob.ID)
		aliceAfter, _ := chars.FindByID(ctx, alice.ID)
		if bobAfter.Money != 500 || bobAfter.PendingWake {
			t.Fatalf("Bob should be refunded without PendingWake: %+v", bobAfter)
		}
		if locked, _ := timers.IsLocked(ctx, timer.CategorySleep, bob.ID); locked {
			t.Fatal("departed Bob must not receive sleep penalty")
		}
		if aliceAfter.Money != 400 || !aliceAfter.PendingWake {
			t.Fatalf("Alice should forfeit bet and have PendingWake: %+v", aliceAfter)
		}
		if locked, _ := timers.IsLocked(ctx, timer.CategorySleep, alice.ID); !locked {
			t.Fatal("remaining Alice should receive sleep lock")
		}
	})

	t.Run("retry after penalty deadline expired does not recreate sleep timer", func(t *testing.T) {
		t0 := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Second)
		now := t0
		chars := newMockCharRepo()
		alice := createTestCharacter("char-1", "Alice", 500)
		chars.add(alice)
		rooms := pvp.NewMemoryRoomRepository()
		trans := pvp.NewMemoryTransitionRepository()
		baseTimer := timer.NewService(nil)
		timerErr := errors.New("timer down")
		ft := &failingTimer{Service: baseTimer, failErr: timerErr}

		svc, _ := pvp.NewService(rooms, chars, mockBattleEngine{}, pvp.WithTransitionRepository(trans), pvp.WithTimerService(ft), pvp.WithClock(func() time.Time { return now }))
		room, _ := svc.CreateRoom(ctx, alice.ID, pvp.CreateRoomRequest{Name: "ExpiredPenalty", Bet: 100})
		now = t0.Add(pvp.InactivityTimeout + time.Second) // still in past (> 90m ago), so PenaltyUntil < time.Now()
		if err := svc.LeaveRoom(ctx, alice.ID, room.Room.ID); !errors.Is(err, timerErr) {
			t.Fatalf("expected timerErr, got %v", err)
		}
		if err := svc.LeaveRoom(ctx, alice.ID, room.Room.ID); err != nil {
			t.Fatalf("retry after expiry: %v", err)
		}
		if locked, _ := baseTimer.IsLocked(ctx, timer.CategorySleep, alice.ID); locked {
			t.Fatal("expired penalty deadline must not recreate sleep timer")
		}
	})
}

type failingDeleteRooms struct {
	pvp.RoomRepository
	failSave error
}

func (r *failingDeleteRooms) SaveRoom(ctx context.Context, room pvp.ColosseumRoom, members []pvp.RoomMember) error {
	if r.failSave != nil {
		err := r.failSave
		r.failSave = nil
		return err
	}
	return r.RoomRepository.SaveRoom(ctx, room, members)
}
