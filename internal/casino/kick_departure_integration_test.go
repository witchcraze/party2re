package casino_test

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	valkeygo "github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/casino"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/testutil"
)

type kickWaitAdapter struct {
	timers   timer.Service
	failNext atomic.Bool
}

func (a *kickWaitAdapter) ImposeActionWaitUntil(ctx context.Context, characterID string, until time.Time) error {
	if a.failNext.Load() {
		return errDepartureFault
	}
	return a.timers.ImposeLockUntil(ctx, timer.CategoryActionWait, characterID, until)
}

func kickService(t *testing.T, db *sql.DB, client valkeygo.Client, evidence casino.DepartureRepository, wait casino.ActionWaitWriter) *casino.Service {
	t.Helper()
	svc := departureService(t, db, client, evidence)
	if wait != nil {
		svc.SetActionWait(wait)
	}
	return svc
}

func TestKickDepartureRecovery_InterruptionWindows(t *testing.T) {
	db := database.OpenTestDB(t)
	raw := openIsolatedTestValkey(t, 0)
	t.Cleanup(raw.Close)
	ctx := context.Background()
	realTimers := timer.NewService(raw)

	for _, window := range []string{"sql-rollback", "sql-lost-ack", "payload-rejected", "payload-lost-ack", "index", "mapping", "wait-write-failed", "acknowledgement"} {
		t.Run(window, func(t *testing.T) {
			f := departureFixture(t, ctx, db, raw)
			leader, target := f.actors[0], f.actors[1]
			t.Cleanup(func() {
				for _, id := range f.actors {
					if err := realTimers.ReleaseLock(context.Background(), timer.CategoryActionWait, id); err != nil {
						t.Error(err)
					}
				}
			})

			state := f.state(t, ctx)
			state.room.Status, state.room.Round = casino.RoomStatusWaiting, 0
			if err := f.rooms.UpdateRoomDetail(ctx, casino.RoomDetail{Room: state.room, Members: state.members}); err != nil {
				t.Fatal(err)
			}
			before := f.state(t, ctx)

			client := &departureWriteFault{Client: raw, roomID: f.roomID}
			evidence := departureEvidenceFault{DepartureRepository: f.accounts}
			waits := &kickWaitAdapter{timers: realTimers}
			switch window {
			case "sql-rollback":
				evidence.commitMode = "rollback"
			case "sql-lost-ack":
				evidence.commitMode = "lost-ack"
			case "payload-rejected":
				client.command = "SET"
			case "payload-lost-ack":
				client.command, client.lostAck = "SET", true
			case "index":
				client.command = "ZADD"
			case "mapping":
				client.command = "mapping"
			case "wait-write-failed":
				waits.failNext.Store(true)
			case "acknowledgement":
				evidence.failAck = true
			}
			client.enabled.Store(true)

			svc := kickService(t, db, client, evidence, waits)
			if _, err := svc.KickMember(ctx, f.roomID, leader, target); !errors.Is(err, errDepartureFault) || !errors.Is(err, casino.ErrDepartureRecovery) {
				t.Fatalf("interruption hidden: %v", err)
			}

			client.enabled.Store(false)
			waits.failNext.Store(false)

			d, err := f.accounts.FindDeparture(ctx, f.roomID)
			if err != nil {
				t.Fatal(err)
			}
			if window == "sql-rollback" {
				if d.Version != 0 {
					t.Fatalf("rollback retained evidence: %+v", d)
				}
				f.assertUnchanged(t, ctx, before)
				if rem, err := realTimers.GetRemainingLock(ctx, timer.CategoryActionWait, target); err != nil || rem != 0 {
					t.Fatalf("rollback wrote target wait: %v %v", rem, err)
				}
				return
			}

			if !d.Pending || !d.Kicked || d.LeaderID != leader || d.Member.CharacterID != target || d.WaitUntil.IsZero() {
				t.Fatalf("interruption lost durable kick evidence: %+v", d)
			}

			// Read-only GET must surface recovery without modifying wait or membership.
			observerClient := openIsolatedTestValkey(t, 0)
			t.Cleanup(observerClient.Close)
			observer := kickService(t, database.OpenTestDB(t), observerClient, nil, &kickWaitAdapter{timers: realTimers})
			for _, actor := range f.actors {
				ch, err := f.characters.FindByID(ctx, actor)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := observer.GetCharacterRoomView(ctx, ch.PlayerID, ch.ID); !errors.Is(err, casino.ErrDepartureRecovery) {
					t.Fatalf("GET hid pending departure: %v", err)
				}
				if _, err := observer.HasCharacterRoom(ctx, ch.ID); !errors.Is(err, casino.ErrDepartureRecovery) {
					t.Fatalf("activity hid pending departure: %v", err)
				}
			}
			if window == "wait-write-failed" {
				if rem, err := realTimers.GetRemainingLock(ctx, timer.CategoryActionWait, target); err != nil || rem != 0 {
					t.Fatalf("GET wrote target wait: %v %v", rem, err)
				}
			}

			// Valid later character mutations survive recovery, and kick adds 0 fatigue.
			if err := database.RunInTx(ctx, db, func(txCtx context.Context) error {
				ch, err := f.characters.FindByIDForUpdate(txCtx, target)
				if err != nil {
					return err
				}
				if err := ch.AddMoney(17); err != nil {
					return err
				}
				return f.characters.Update(txCtx, ch)
			}); err != nil {
				t.Fatal(err)
			}

			freshClient := openIsolatedTestValkey(t, 0)
			t.Cleanup(freshClient.Close)
			restarted := kickService(t, database.OpenTestDB(t), freshClient, nil, &kickWaitAdapter{timers: realTimers})
			for range 3 {
				if _, err := restarted.KickMember(ctx, f.roomID, leader, target); err != nil {
					t.Fatal(err)
				}
			}

			finalDep, err := f.accounts.FindDeparture(ctx, f.roomID)
			if err != nil || finalDep.Pending || !finalDep.Kicked || finalDep.Version != d.Version {
				t.Fatalf("recovery did not converge cleanly: %+v %v", finalDep, err)
			}
			for _, actor := range f.actors {
				if claim, err := f.accounts.FindPendingDeparture(ctx, actor); err != nil || claim != "" {
					t.Fatalf("retained claim for %s: %q %v", actor, claim, err)
				}
			}
			rem, err := realTimers.GetRemainingLock(ctx, timer.CategoryActionWait, target)
			if err != nil || rem <= 50*time.Second || rem > casino.KickActionWait {
				t.Fatalf("target kick wait=%v err=%v", rem, err)
			}
			if leaderRem, err := realTimers.GetRemainingLock(ctx, timer.CategoryActionWait, leader); err != nil || leaderRem != 0 {
				t.Fatalf("service KickMember wrote leader wait=%v err=%v", leaderRem, err)
			}
			after := f.state(t, ctx)
			if len(after.members) != 1 || after.members[0].CharacterID != leader || after.coins != before.coins || after.wins != before.wins {
				t.Fatalf("unexpected room state after recovery: %+v", after)
			}
			if _, err := f.rooms.GetCharacterRoom(ctx, target); !errors.Is(err, casino.ErrRoomNotFound) {
				t.Fatalf("kicked target mapping remained: %v", err)
			}
			targetChar, err := f.characters.FindByID(ctx, target)
			if err != nil || targetChar.Money != 17 || targetChar.Tired != 0 {
				t.Fatalf("target assets/fatigue corrupted: %+v %v", targetChar, err)
			}
			// Voluntary leave after a kick must still reject as ErrMemberNotFound.
			if err := restarted.LeaveRoom(ctx, f.roomID, target); !errors.Is(err, casino.ErrMemberNotFound) {
				t.Fatalf("LeaveRoom after kick returned %v, want ErrMemberNotFound", err)
			}
		})
	}
}

func TestKickDepartureRecovery_PreservesLongerWaitAndSkipsExpired(t *testing.T) {
	db := database.OpenTestDB(t)
	raw := openIsolatedTestValkey(t, 0)
	t.Cleanup(raw.Close)
	ctx := context.Background()
	realTimers := timer.NewService(raw)

	t.Run("preserves longer active wait", func(t *testing.T) {
		f := departureFixture(t, ctx, db, raw)
		leader, target := f.actors[0], f.actors[1]
		t.Cleanup(func() {
			if err := realTimers.ReleaseLock(context.Background(), timer.CategoryActionWait, target); err != nil {
				t.Errorf("cleanup ReleaseLock action_wait %s: %v", target, err)
			}
		})

		state := f.state(t, ctx)
		state.room.Status, state.room.Round = casino.RoomStatusWaiting, 0
		if err := f.rooms.UpdateRoomDetail(ctx, casino.RoomDetail{Room: state.room, Members: state.members}); err != nil {
			t.Fatal(err)
		}

		longerUntil := time.Now().Add(3 * time.Minute).Truncate(time.Millisecond)
		if err := realTimers.SetLockUntil(ctx, timer.CategoryActionWait, target, longerUntil); err != nil {
			t.Fatal(err)
		}
		waits := &kickWaitAdapter{timers: realTimers}
		waits.failNext.Store(true)
		svc := kickService(t, db, raw, nil, waits)
		if _, err := svc.KickMember(ctx, f.roomID, leader, target); !errors.Is(err, casino.ErrDepartureRecovery) {
			t.Fatalf("expected ErrDepartureRecovery, got %v", err)
		}
		waits.failNext.Store(false)
		if err := svc.RecoverCharacterDeparture(ctx, target); err != nil {
			t.Fatal(err)
		}
		rem, err := realTimers.GetRemainingLock(ctx, timer.CategoryActionWait, target)
		if err != nil || rem <= 2*time.Minute {
			t.Fatalf("recovery shortened existing longer wait: %v %v", rem, err)
		}
	})

	t.Run("expired wait deadline creates no timer on recovery", func(t *testing.T) {
		f := departureFixture(t, ctx, db, raw)
		leader, target := f.actors[0], f.actors[1]
		t.Cleanup(func() {
			if err := realTimers.ReleaseLock(context.Background(), timer.CategoryActionWait, target); err != nil {
				t.Errorf("cleanup ReleaseLock action_wait %s: %v", target, err)
			}
		})

		state := f.state(t, ctx)
		state.room.Status, state.room.Round = casino.RoomStatusWaiting, 0
		if err := f.rooms.UpdateRoomDetail(ctx, casino.RoomDetail{Room: state.room, Members: state.members}); err != nil {
			t.Fatal(err)
		}

		waits := &kickWaitAdapter{timers: realTimers}
		waits.failNext.Store(true)
		svc := kickService(t, db, raw, nil, waits)
		if _, err := svc.KickMember(ctx, f.roomID, leader, target); !errors.Is(err, casino.ErrDepartureRecovery) {
			t.Fatalf("expected ErrDepartureRecovery, got %v", err)
		}

		// Force the recorded WaitUntil into the past to simulate >60s passing before recovery.
		if _, err := db.ExecContext(ctx,
			`UPDATE casino_departures SET departure_json = JSON_SET(departure_json, '$.WaitUntil', ?) WHERE room_id = ?`,
			time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano), f.roomID); err != nil {
			t.Fatal(err)
		}

		waits.failNext.Store(false)
		if err := svc.RecoverCharacterDeparture(ctx, target); err != nil {
			t.Fatal(err)
		}
		if rem, err := realTimers.GetRemainingLock(ctx, timer.CategoryActionWait, target); err != nil || rem != 0 {
			t.Fatalf("expired kick wait revived timer: %v %v", rem, err)
		}
		if claim, err := f.accounts.FindPendingDeparture(ctx, target); err != nil || claim != "" {
			t.Fatalf("pending claim remained after expired recovery: %q %v", claim, err)
		}
	})
}

func TestKickDepartureRecovery_RejectsStaleKickAfterNewAdmission(t *testing.T) {
	db := database.OpenTestDB(t)
	raw := openIsolatedTestValkey(t, 0)
	t.Cleanup(raw.Close)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	realTimers := timer.NewService(raw)

	source := departureFixture(t, ctx, db, raw)
	target := departureFixture(t, ctx, db, raw)
	for _, f := range []*publicationFixture{source, target} {
		state := f.state(t, ctx)
		state.room.Status, state.room.Round, state.room.MaxPlayers = casino.RoomStatusWaiting, 0, 4
		if err := f.rooms.UpdateRoomDetail(ctx, casino.RoomDetail{Room: state.room, Members: state.members}); err != nil {
			t.Fatal(err)
		}
	}
	leader, mover := source.actors[0], source.actors[1]
	t.Cleanup(func() {
		if err := realTimers.ReleaseLock(context.Background(), timer.CategoryActionWait, mover); err != nil {
			t.Errorf("cleanup ReleaseLock action_wait %s: %v", mover, err)
		}
	})

	pause := pausedDepartureCommit{DepartureRepository: source.accounts, staged: make(chan struct{}), resume: make(chan struct{})}
	kicker := kickService(t, db, raw, pause, &kickWaitAdapter{timers: realTimers})
	joiner := kickService(t, database.OpenTestDB(t), raw, nil, &kickWaitAdapter{timers: realTimers})

	kickErr, joinErr := testutil.RunRace2(
		func() error {
			_, err := kicker.KickMember(ctx, source.roomID, leader, mover)
			return err
		},
		func() error {
			defer close(pause.resume)
			select {
			case <-pause.staged:
			case <-ctx.Done():
				return ctx.Err()
			}
			keys := []string{casino.DefaultRoomKeyPrefix + source.roomID}
			for _, actor := range source.actors {
				keys = append(keys, casino.DefaultCharacterKeyPrefix+actor)
			}
			if err := raw.Do(ctx, raw.B().Del().Key(keys...).Build()).Error(); err != nil {
				return err
			}
			_, err := joiner.JoinRoom(ctx, target.roomID, mover, "", 0)
			return err
		},
	)
	if joinErr != nil || !errors.Is(kickErr, casino.ErrDepartureRecovery) {
		t.Fatalf("new admission must win over stale kick: join=%v kick=%v", joinErr, kickErr)
	}
	if d, err := source.accounts.FindDeparture(ctx, source.roomID); err != nil || d.Version != 0 {
		t.Fatalf("stale kick committed evidence: %+v %v", d, err)
	}
	if claim, err := source.accounts.FindPendingDeparture(ctx, mover); err != nil || claim != "" {
		t.Fatalf("new admission stranded by claim: %q %v", claim, err)
	}
	if rem, err := realTimers.GetRemainingLock(ctx, timer.CategoryActionWait, mover); err != nil || rem != 0 {
		t.Fatalf("rejected stale kick imposed wait: %v %v", rem, err)
	}
	ch, err := source.characters.FindByID(ctx, mover)
	if err != nil {
		t.Fatal(err)
	}
	if view, err := joiner.GetCharacterRoomView(ctx, ch.PlayerID, mover); err != nil || view == nil || view.Room.ID != target.roomID {
		t.Fatalf("new admission overwritten: %+v %v", view, err)
	}
}

func TestKickDepartureRecovery_ConcurrentKickAndRejoin(t *testing.T) {
	db := database.OpenTestDB(t)
	raw := openIsolatedTestValkey(t, 0)
	t.Cleanup(raw.Close)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	realTimers := timer.NewService(raw)

	f := departureFixture(t, ctx, db, raw)
	otherRoom := departureFixture(t, ctx, db, raw)
	for _, fix := range []*publicationFixture{f, otherRoom} {
		state := fix.state(t, ctx)
		state.room.Status, state.room.Round, state.room.MaxPlayers = casino.RoomStatusWaiting, 0, 4
		if err := fix.rooms.UpdateRoomDetail(ctx, casino.RoomDetail{Room: state.room, Members: state.members}); err != nil {
			t.Fatal(err)
		}
	}
	leader, peer := f.actors[0], f.actors[1]
	t.Cleanup(func() {
		if err := realTimers.ReleaseLock(context.Background(), timer.CategoryActionWait, peer); err != nil {
			t.Errorf("cleanup ReleaseLock action_wait %s: %v", peer, err)
		}
	})

	c1, c2 := openIsolatedTestValkey(t, 0), openIsolatedTestValkey(t, 0)
	t.Cleanup(c1.Close)
	t.Cleanup(c2.Close)
	svc1 := kickService(t, database.OpenTestDB(t), c1, nil, &kickWaitAdapter{timers: realTimers})
	svc2 := kickService(t, database.OpenTestDB(t), c2, nil, &kickWaitAdapter{timers: realTimers})

	var joinErr error
	errs := testutil.RunRace(
		func() error {
			_, err := svc1.KickMember(ctx, f.roomID, leader, peer)
			return err
		},
		func() error {
			_, err := svc2.KickMember(ctx, f.roomID, leader, peer)
			return err
		},
		func() error {
			_, joinErr = svc2.JoinRoom(ctx, otherRoom.roomID, peer, "", 0)
			if errors.Is(joinErr, casino.ErrAlreadyInRoom) || errors.Is(joinErr, casino.ErrDepartureRecovery) {
				return nil
			}
			return joinErr
		},
	)
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if joinErr != nil && !errors.Is(joinErr, casino.ErrAlreadyInRoom) && !errors.Is(joinErr, casino.ErrDepartureRecovery) {
		t.Fatalf("unexpected join error during concurrent kick: %v", joinErr)
	}
	if joinErr != nil {
		if _, err := otherRoom.rooms.GetMember(ctx, otherRoom.roomID, peer); !errors.Is(err, casino.ErrMemberNotFound) {
			t.Fatalf("rejected join leaked membership in otherRoom: %v", err)
		}
	}
	d, err := f.accounts.FindDeparture(ctx, f.roomID)
	if err != nil || d.Pending || !d.Kicked || d.Version != 1 {
		t.Fatalf("unexpected departure state after concurrent kick: %+v %v", d, err)
	}
	if rem, err := realTimers.GetRemainingLock(ctx, timer.CategoryActionWait, peer); err != nil || rem <= 50*time.Second {
		t.Fatalf("peer missing kick wait after concurrent kick: %v %v", rem, err)
	}
	if slices.ContainsFunc(f.state(t, ctx).members, func(m casino.RoomMember) bool { return m.CharacterID == peer }) {
		t.Fatal("kicked peer remained in room")
	}
	if claim, err := f.accounts.FindPendingDeparture(ctx, peer); err != nil || claim != "" {
		t.Fatalf("pending claim left behind: %q %v", claim, err)
	}
}
