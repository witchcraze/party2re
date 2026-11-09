package casino_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"reflect"
	"slices"
	"testing"
	"time"

	valkeygo "github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/casino"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/testutil"
)

// Include mapping renewal failures without widening the existing game-action fault matrix.
type departureValkey struct {
	valkeygo.Client
	fault *publicationFault
}

func (c *departureValkey) Do(ctx context.Context, cmd valkeygo.Completed) valkeygo.ValkeyResult {
	args := cmd.Commands()
	if len(args) > 5 && (args[0] == "EVAL" || args[0] == "EVALSHA") {
		if err := c.fault.hit("valkey/renew"); err != nil {
			return valkeygo.NewErrorResult(err)
		}
	}
	return c.Client.Do(ctx, cmd)
}

func TestDeparturePublication_FailureBoundaries(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	raw := openIsolatedTestValkey(t, 0)
	t.Cleanup(raw.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	for _, scenario := range []string{"leader", "peer", "last-with-spectator", "last-alone", "spectator"} {
		t.Run(scenario, func(t *testing.T) {
			client := &departureValkey{Client: raw, fault: &publicationFault{}}
			f := newPublicationFixture(t, ctx, db, client, casino.GameTypeIndian, 500)
			client.fault = f.fault
			initial := f.state(t, ctx)
			initial.room.Status, initial.room.Round = casino.RoomStatusWaiting, 0
			initial.room.AllowSpectators = true
			actor := 0
			if scenario == "peer" || scenario == "spectator" {
				actor = 1
			}
			if scenario == "spectator" || scenario == "last-with-spectator" {
				for i := range initial.members {
					initial.members[i].IsSpectator = initial.members[i].CharacterID == f.actors[1]
				}
			}
			if scenario == "last-alone" {
				initial.members = slices.DeleteFunc(initial.members, func(m casino.RoomMember) bool { return m.CharacterID == f.actors[1] })
			}
			prepare := func(t *testing.T) {
				if err := f.rooms.UpdateRoom(ctx, initial.room); err != nil {
					t.Fatal(err)
				}
				f.restore(t, ctx, initial)
				for _, id := range f.actors {
					ch, err := f.characters.FindByID(ctx, id)
					if err != nil {
						t.Fatal(err)
					}
					ch.Tired = 7
					if err := f.characters.Update(ctx, ch); err != nil {
						t.Fatal(err)
					}
				}
				f.fault.resetCalls()
			}
			assertState := func(t *testing.T, applied bool) {
				t.Helper()
				after := f.state(t, ctx)
				want := initial
				want.members = slices.Clone(initial.members)
				if applied {
					want.members = slices.DeleteFunc(want.members, func(m casino.RoomMember) bool { return m.CharacterID == f.actors[actor] })
					if scenario == "last-alone" || scenario == "last-with-spectator" {
						want.room.Status = casino.RoomStatusDisbanded
					} else if scenario == "leader" {
						want.room.LeaderCharacterID = f.actors[1]
					}
					want.room.UpdatedAt = after.room.UpdatedAt
				}
				if !reflect.DeepEqual(after, want) {
					t.Fatalf("applied=%v: persisted state=%+v, want %+v", applied, after, want)
				}
				for i, id := range f.actors {
					ch, err := f.characters.FindByID(ctx, id)
					if err != nil {
						t.Fatal(err)
					}
					wantTired := 7
					if applied && i == actor && scenario != "spectator" {
						wantTired++
					}
					if ch.Tired != wantTired {
						t.Fatalf("actor %d tired=%d, want %d", i, ch.Tired, wantTired)
					}
					view, err := f.svc.GetCharacterRoomView(ctx, ch.PlayerID, ch.ID)
					if err != nil {
						t.Fatal(err)
					}
					admitted := slices.ContainsFunc(want.members, func(m casino.RoomMember) bool { return m.CharacterID == id }) && want.room.Status != casino.RoomStatusDisbanded
					if (view != nil) != admitted {
						t.Fatalf("actor %d activity=%+v, admitted=%v", i, view, admitted)
					}
					if view != nil && view.Room.LeaderCharacterID != want.room.LeaderCharacterID {
						t.Fatalf("activity retained departed leader: %+v", view)
					}
				}
			}
			prepare(t)
			if err := f.svc.LeaveRoom(ctx, f.roomID, f.actors[actor]); err != nil {
				t.Fatal(err)
			}
			calls := f.fault.getCalls()
			assertState(t, true)
			payload := slices.Index(calls, "valkey/SET")
			if payload < 0 || slices.Index(calls[payload+1:], "valkey/SET") >= 0 {
				t.Fatalf("departure must publish exactly once: %v", calls)
			}
			for fail, call := range calls {
				t.Run(publicationFailureName(fail, call), func(t *testing.T) {
					prepare(t)
					f.fault.failAt = fail + 1
					leaveErr := f.svc.LeaveRoom(ctx, f.roomID, f.actors[actor])
					f.fault.failAt = 0
					if !errors.Is(leaveErr, errPublicationFault) || errors.Is(leaveErr, casino.ErrSecondarySync) != (fail > payload) {
						t.Fatalf("failure at %s misclassified: %v", call, leaveErr)
					}
					assertState(t, fail > payload)
					if fail <= payload {
						// Confirm rejection before explicitly attempting a new departure.
						if err := f.svc.LeaveRoom(ctx, f.roomID, f.actors[actor]); err != nil {
							t.Fatal(err)
						}
					}
					for recovery := 0; recovery < 2; recovery++ {
						assertState(t, true)
						if err := f.svc.LeaveRoom(ctx, f.roomID, f.actors[actor]); !errors.Is(err, casino.ErrMemberNotFound) {
							t.Fatalf("already departed retry=%v", err)
						}
					}
					assertState(t, true)
				})
			}
		})
	}
}

func TestDeparturePublication_ConcurrentLeaves(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	raw := openIsolatedTestValkey(t, 0)
	t.Cleanup(raw.Close)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	f := newPublicationFixture(t, ctx, db, raw, casino.GameTypeIndian, 500)
	before := f.state(t, ctx)
	errA, errB := testutil.RunRace2(
		func() error { return f.svc.LeaveRoom(ctx, f.roomID, f.actors[0]) },
		func() error { return f.svc.LeaveRoom(ctx, f.roomID, f.actors[0]) },
	)
	if !((errA == nil && errors.Is(errB, casino.ErrMemberNotFound)) || (errB == nil && errors.Is(errA, casino.ErrMemberNotFound))) {
		t.Fatalf("concurrent departures: %v, %v", errA, errB)
	}
	after := f.state(t, ctx)
	ch, err := f.characters.FindByID(ctx, f.actors[0])
	if err != nil {
		t.Fatal(err)
	}
	if ch.Tired != 1 || after.coins != before.coins || after.room.LeaderCharacterID != f.actors[1] || len(after.members) != 1 {
		t.Fatalf("departure not applied exactly once: tired=%d state=%+v", ch.Tired, after)
	}
}

type departureFinalizationFailure struct {
	db  *sql.DB
	err error
}

func (p departureFinalizationFailure) RunInTx(ctx context.Context, fn func(context.Context) error) error {
	return database.RunInTx(ctx, p.db, func(txCtx context.Context) error {
		if err := fn(txCtx); err != nil {
			return err
		}
		// Deterministic rollback after payload publication; actual COMMIT ambiguity belongs to #1997.
		return p.err
	})
}

func TestDeparturePublication_PostPublicationRollback(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	raw := openIsolatedTestValkey(t, 0)
	t.Cleanup(raw.Close)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	for _, secondary := range []bool{false, true} {
		t.Run(map[bool]string{false: "payload-only", true: "payload-and-secondary-error"}[secondary], func(t *testing.T) {
			client := &faultValkeyClient{Client: raw}
			f := newPublicationFixture(t, ctx, db, client, casino.GameTypeIndian, 500)
			finalizationErr := errors.New("injected SQL finalization failure")
			svc, err := casino.NewService(f.accounts, casino.WithRoomRepository(f.rooms), casino.WithCharacterRepository(f.characters),
				casino.WithTransactionProvider(departureFinalizationFailure{db: db, err: finalizationErr}))
			if err != nil {
				t.Fatal(err)
			}
			client.failZadd.Store(secondary)
			leaveErr := svc.LeaveRoom(ctx, f.roomID, f.actors[0])
			client.failZadd.Store(false)
			if !errors.Is(leaveErr, finalizationErr) || errors.Is(leaveErr, casino.ErrSecondarySync) != secondary {
				t.Fatalf("lost transaction/publication errors: %v", leaveErr)
			}
			after := f.state(t, ctx)
			ch, err := f.characters.FindByID(ctx, f.actors[0])
			if err != nil {
				t.Fatal(err)
			}
			if ch.Tired != 0 || after.room.LeaderCharacterID != f.actors[1] || len(after.members) != 1 {
				t.Fatalf("post-publication rollback boundary hidden: tired=%d state=%+v", ch.Tired, after)
			}
		})
	}
}
