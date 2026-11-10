package casino_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	valkeygo "github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/casino"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/testutil"
)

var errDepartureFault = errors.New("injected departure interruption")

type departureEvidenceFault struct {
	casino.DepartureRepository
	commitMode string
	failAck    bool
}

func (r departureEvidenceFault) CommitDeparture(ctx context.Context, d casino.Departure, mutate func(context.Context) error) error {
	if r.commitMode == "expired" {
		d.Detail.Room.UpdatedAt = time.Now().Add(-casino.DefaultLobbyTTL - time.Minute)
	}
	if r.commitMode == "rollback" {
		return r.DepartureRepository.CommitDeparture(ctx, d, func(ctx context.Context) error {
			if err := mutate(ctx); err != nil {
				return err
			}
			return errDepartureFault
		})
	}
	if err := r.DepartureRepository.CommitDeparture(ctx, d, mutate); err != nil {
		return err
	}
	if r.commitMode == "lost-ack" || r.commitMode == "expired" {
		return errDepartureFault
	}
	return nil
}

func (r departureEvidenceFault) AcknowledgeDeparture(ctx context.Context, roomID string, version int) error {
	if r.failAck {
		return errDepartureFault
	}
	return r.DepartureRepository.AcknowledgeDeparture(ctx, roomID, version)
}

type departureWriteFault struct {
	valkeygo.Client
	roomID  string
	command string
	lostAck bool
	enabled atomic.Bool
}

func (c *departureWriteFault) Do(ctx context.Context, cmd valkeygo.Completed) valkeygo.ValkeyResult {
	args := cmd.Commands()
	match := len(args) > 1 && ((c.command == "SET" && args[0] == "SET" && args[1] == casino.DefaultRoomKeyPrefix+c.roomID) ||
		(c.command == "ZADD" && args[0] == "ZADD") || (c.command == "ZREM" && args[0] == "ZREM") ||
		(c.command == "mapping" && (args[0] == "EVAL" || args[0] == "EVALSHA") && len(args) > 3 && strings.HasPrefix(args[3], casino.DefaultCharacterKeyPrefix)))
	if match && c.enabled.Load() {
		if c.lostAck {
			if result := c.Client.Do(ctx, cmd); result.Error() != nil {
				return result
			}
		}
		return valkeygo.NewErrorResult(errDepartureFault)
	}
	return c.Client.Do(ctx, cmd)
}

func openDepartureDB(t *testing.T) *sql.DB {
	t.Helper()
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func departureService(t *testing.T, db *sql.DB, client valkeygo.Client, evidence casino.DepartureRepository) *casino.Service {
	t.Helper()
	accounts, err := database.NewCasinoRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	characters, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	rooms, err := casino.NewValkeyRoomRepository(client, casino.WithValkeyCharacterRepository(characters))
	if err != nil {
		t.Fatal(err)
	}
	if evidence == nil {
		evidence = accounts
	}
	svc, err := casino.NewService(accounts, casino.WithRoomRepository(rooms), casino.WithCharacterRepository(characters),
		casino.WithTransactionProvider(database.NewTransactionProvider(db)), casino.WithDepartureRepository(evidence))
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func departureFixture(t *testing.T, ctx context.Context, db *sql.DB, client valkeygo.Client) *publicationFixture {
	t.Helper()
	f := newPublicationFixture(t, ctx, db, client, casino.GameTypeIndian, 500)
	t.Cleanup(func() {
		for _, table := range []string{"casino_departure_claims", "casino_departures"} {
			if _, err := db.ExecContext(context.Background(), "DELETE FROM "+table+" WHERE room_id = ?", f.roomID); err != nil {
				t.Error(err)
			}
		}
	})
	return f
}

func assertDeparture(t *testing.T, ctx context.Context, f *publicationFixture, actor int, wantTired int) casino.Departure {
	t.Helper()
	d, err := f.accounts.FindDeparture(ctx, f.roomID)
	if err != nil {
		t.Fatal(err)
	}
	if d.Version < 1 || d.Pending || d.Member.CharacterID != f.actors[actor] || d.Member.JoinedAt.IsZero() {
		t.Fatalf("invalid terminal receipt: %+v", d)
	}
	for i, actorID := range f.actors {
		ch, err := f.characters.FindByID(ctx, actorID)
		if err != nil {
			t.Fatal(err)
		}
		want := 0
		if i == actor {
			want = wantTired
		}
		if ch.Tired != want {
			t.Fatalf("actor %d tired=%d, want %d", i, ch.Tired, want)
		}
		if claim, err := f.accounts.FindPendingDeparture(ctx, actorID); err != nil || claim != "" {
			t.Fatalf("retained claim %q: %v", claim, err)
		}
	}
	return d
}

func TestDepartureRecovery_InterruptionWindows(t *testing.T) {
	db := openDepartureDB(t)
	raw := openIsolatedTestValkey(t, 0)
	t.Cleanup(raw.Close)
	ctx := context.Background()
	for _, window := range []string{"sql-rollback", "sql-lost-ack", "payload-rejected", "payload-lost-ack", "index", "mapping", "acknowledgement"} {
		t.Run(window, func(t *testing.T) {
			f := departureFixture(t, ctx, db, raw)
			before := f.state(t, ctx)
			client := &departureWriteFault{Client: raw, roomID: f.roomID}
			evidence := departureEvidenceFault{DepartureRepository: f.accounts}
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
			case "acknowledgement":
				evidence.failAck = true
			}
			client.enabled.Store(true)
			svc := departureService(t, db, client, evidence)
			if err := svc.LeaveRoom(ctx, f.roomID, f.actors[0]); !errors.Is(err, errDepartureFault) || !errors.Is(err, casino.ErrDepartureRecovery) {
				t.Fatalf("interruption hidden: %v", err)
			}
			client.enabled.Store(false)
			d, err := f.accounts.FindDeparture(ctx, f.roomID)
			if err != nil {
				t.Fatal(err)
			}
			if window == "sql-rollback" {
				if d.Version != 0 {
					t.Fatalf("rollback retained evidence: %+v", d)
				}
				f.assertUnchanged(t, ctx, before)
			} else {
				if !d.Pending {
					t.Fatal("interruption lost durable work")
				}
				ch, err := f.characters.FindByID(ctx, f.actors[0])
				if err != nil || ch.Tired != 1 {
					t.Fatalf("fatigue not committed: %+v %v", ch, err)
				}
				// Independent SQL/Valkey handles inspect pending work without repairing it.
				observerClient := openIsolatedTestValkey(t, 0)
				t.Cleanup(observerClient.Close)
				observer := departureService(t, openDepartureDB(t), observerClient, nil)
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
				if _, err := observer.GetCharacterRoomView(ctx, "unowned", f.actors[0]); !errors.Is(err, casino.ErrRoomViewForbidden) {
					t.Fatalf("ownership bypass: %v", err)
				}
				afterRead, err := f.accounts.FindDeparture(ctx, f.roomID)
				if err != nil || !afterRead.Pending {
					t.Fatalf("GET repaired pending work: %+v %v", afterRead, err)
				}
			}
			// A valid later character mutation must survive recovery.
			if err := database.RunInTx(ctx, db, func(txCtx context.Context) error {
				ch, err := f.characters.FindByIDForUpdate(txCtx, f.actors[0])
				if err != nil {
					return err
				}
				ch.AddTired(9)
				if err := ch.AddMoney(17); err != nil {
					return err
				}
				return f.characters.Update(txCtx, ch)
			}); err != nil {
				t.Fatal(err)
			}
			freshClient := openIsolatedTestValkey(t, 0)
			t.Cleanup(freshClient.Close)
			restarted := departureService(t, openDepartureDB(t), freshClient, nil)
			for range 3 {
				if err := restarted.LeaveRoom(ctx, f.roomID, f.actors[0]); err != nil {
					t.Fatal(err)
				}
			}
			assertDeparture(t, ctx, f, 0, 10)
			after := f.state(t, ctx)
			ch, err := f.characters.FindByID(ctx, f.actors[0])
			if err != nil || ch.Money != 17 || after.coins != before.coins || after.wins != before.wins || len(after.members) != 1 || after.room.LeaderCharacterID != f.actors[1] {
				t.Fatalf("recovery corrupted assets/roster: ch=%+v state=%+v err=%v", ch, after, err)
			}
			for _, actor := range f.actors {
				ch, err := f.characters.FindByID(ctx, actor)
				if err != nil {
					t.Fatal(err)
				}
				view, err := restarted.GetCharacterRoomView(ctx, ch.PlayerID, actor)
				if err != nil || (view != nil) != (actor == f.actors[1]) {
					t.Fatalf("owned activity disagrees: %+v %v", view, err)
				}
			}
		})
	}
}

func TestDepartureRecovery_RolesAndDisband(t *testing.T) {
	db := openDepartureDB(t)
	raw := openIsolatedTestValkey(t, 0)
	t.Cleanup(raw.Close)
	ctx := context.Background()
	for _, scenario := range []string{"leader", "peer", "spectator", "last-alone", "last-with-spectator"} {
		for _, interrupted := range []bool{false, true} {
			t.Run(scenario+map[bool]string{false: "/healthy", true: "/interrupted"}[interrupted], func(t *testing.T) {
				f := departureFixture(t, ctx, db, raw)
				state := f.state(t, ctx)
				state.room.Status, state.room.Round = casino.RoomStatusWaiting, 0
				state.room.AllowSpectators = true
				actor := 0
				if scenario == "peer" || scenario == "spectator" {
					actor = 1
				}
				if scenario == "spectator" || scenario == "last-with-spectator" {
					for i := range state.members {
						state.members[i].IsSpectator = state.members[i].CharacterID == f.actors[1]
					}
				}
				if scenario == "last-alone" {
					state.members = slices.DeleteFunc(state.members, func(m casino.RoomMember) bool { return m.CharacterID == f.actors[1] })
				}
				if err := f.rooms.UpdateRoomDetail(ctx, casino.RoomDetail{Room: state.room, Members: state.members}); err != nil {
					t.Fatal(err)
				}
				client := &departureWriteFault{Client: raw, roomID: f.roomID, command: "mapping"}
				client.enabled.Store(interrupted)
				svc := departureService(t, db, client, nil)
				err := svc.LeaveRoom(ctx, f.roomID, f.actors[actor])
				if interrupted != errors.Is(err, errDepartureFault) {
					t.Fatalf("unexpected outcome: %v", err)
				}
				client.enabled.Store(false)
				for range 2 {
					if err := svc.LeaveRoom(ctx, f.roomID, f.actors[actor]); err != nil {
						t.Fatal(err)
					}
				}
				tired := 1
				if scenario == "spectator" {
					tired = 0
				}
				assertDeparture(t, ctx, f, actor, tired)
				after := f.state(t, ctx)
				if slices.ContainsFunc(after.members, func(m casino.RoomMember) bool { return m.CharacterID == f.actors[actor] }) {
					t.Fatal("departed member retained")
				}
				last := strings.HasPrefix(scenario, "last-")
				if (after.room.Status == casino.RoomStatusDisbanded) != last {
					t.Fatalf("status=%s", after.room.Status)
				}
				if scenario == "leader" && after.room.LeaderCharacterID != f.actors[1] {
					t.Fatal("leader not transferred")
				}
				if after.coins != state.coins || after.wins != state.wins {
					t.Fatal("departure changed coins/wins")
				}
				for _, id := range f.actors {
					ch, err := f.characters.FindByID(ctx, id)
					if err != nil {
						t.Fatal(err)
					}
					view, err := svc.GetCharacterRoomView(ctx, ch.PlayerID, id)
					want := !last && id != f.actors[actor]
					if err != nil || (view != nil) != want {
						t.Fatalf("activity=%+v want=%v err=%v", view, want, err)
					}
				}
			})
		}
	}
}

func TestDepartureRecovery_ConcurrentDuplicateAndRejoin(t *testing.T) {
	db := openDepartureDB(t)
	raw := openIsolatedTestValkey(t, 0)
	t.Cleanup(raw.Close)
	ctx := context.Background()
	f := departureFixture(t, ctx, db, raw)
	before := f.state(t, ctx)
	client := openIsolatedTestValkey(t, 0)
	t.Cleanup(client.Close)
	other := departureService(t, openDepartureDB(t), client, nil)
	errs := testutil.RunRace(
		func() error { return f.svc.LeaveRoom(ctx, f.roomID, f.actors[0]) },
		func() error { return other.LeaveRoom(ctx, f.roomID, f.actors[0]) },
	)
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	first := assertDeparture(t, ctx, f, 0, 1)
	if _, err := other.JoinRoom(ctx, f.roomID, f.actors[0], "", 1); err != nil {
		t.Fatal(err)
	}
	if err := other.LeaveRoom(ctx, f.roomID, f.actors[0]); err != nil {
		t.Fatal(err)
	}
	second := assertDeparture(t, ctx, f, 0, 2)
	if second.Version != first.Version+1 || second.Member.JoinedAt.Equal(first.Member.JoinedAt) {
		t.Fatal("new admission reused an old identity")
	}
	if after := f.state(t, ctx); after.coins != before.coins || after.wins != before.wins {
		t.Fatal("duplicate departure changed assets")
	}
}

func TestDepartureRecovery_LostValkeyAndExpiry(t *testing.T) {
	db := openDepartureDB(t)
	raw := openIsolatedTestValkey(t, 0)
	t.Cleanup(raw.Close)
	ctx := context.Background()
	for _, expired := range []bool{false, true} {
		t.Run(map[bool]string{false: "lost-live-state", true: "expired-state"}[expired], func(t *testing.T) {
			f := departureFixture(t, ctx, db, raw)
			room, err := f.rooms.GetRoom(ctx, f.roomID)
			if err != nil {
				t.Fatal(err)
			}
			room.HasPassword, room.PasswordHash = true, "private-hash"
			if err := f.rooms.UpdateRoom(ctx, *room); err != nil {
				t.Fatal(err)
			}
			mode := "lost-ack"
			if expired {
				mode = "expired"
			}
			svc := departureService(t, db, raw, departureEvidenceFault{DepartureRepository: f.accounts, commitMode: mode})
			if err := svc.LeaveRoom(ctx, f.roomID, f.actors[0]); !errors.Is(err, errDepartureFault) {
				t.Fatal(err)
			}
			// Lose only this fixture's live state, preserving SQL and unrelated rooms.
			if err := f.rooms.DeleteRoom(ctx, f.roomID); err != nil {
				t.Fatal(err)
			}
			fresh := openIsolatedTestValkey(t, 0)
			t.Cleanup(fresh.Close)
			restarted := departureService(t, openDepartureDB(t), fresh, nil)
			peer, err := f.characters.FindByID(ctx, f.actors[1])
			if err != nil {
				t.Fatal(err)
			}
			if _, err := restarted.GetCharacterRoomView(ctx, peer.PlayerID, peer.ID); !errors.Is(err, casino.ErrDepartureRecovery) {
				t.Fatalf("loss hid pending work: %v", err)
			}
			// Even with no reverse mapping, a new admission must first finish the old departure.
			if _, err := restarted.JoinRoom(ctx, f.roomID, peer.ID, "incorrect", 0); expired != errors.Is(err, casino.ErrRoomNotFound) || (!expired && !errors.Is(err, casino.ErrInvalidPassword)) {
				t.Fatalf("command failed to recover lost room: %v", err)
			}
			for range 2 {
				if err := restarted.LeaveRoom(ctx, f.roomID, f.actors[0]); err != nil {
					t.Fatal(err)
				}
			}
			d := assertDeparture(t, ctx, f, 0, 1)
			saved, err := f.rooms.GetRoom(ctx, f.roomID)
			if expired {
				if !errors.Is(err, casino.ErrRoomNotFound) {
					t.Fatalf("expired room revived: %+v %v", saved, err)
				}
				for _, actor := range f.actors {
					if _, err := f.rooms.GetCharacterRoom(ctx, actor); !errors.Is(err, casino.ErrRoomNotFound) {
						t.Fatalf("expired mapping restored: %v", err)
					}
				}
			} else {
				if err != nil || saved.PasswordHash != "private-hash" || saved.LeaderCharacterID != peer.ID || saved.DepartureVersion != d.Version {
					t.Fatalf("restored room lost facts: %+v %v", saved, err)
				}
				view, err := restarted.GetCharacterRoomView(ctx, peer.PlayerID, peer.ID)
				if err != nil || view == nil {
					t.Fatalf("lost mapping not restored: %+v %v", view, err)
				}
				ttl, err := raw.Do(ctx, raw.B().Pttl().Key(casino.DefaultRoomKeyPrefix+f.roomID).Build()).ToInt64()
				if err != nil || ttl > time.Until(d.Detail.Room.UpdatedAt.Add(casino.DefaultLobbyTTL)).Milliseconds()+100 {
					t.Fatalf("recovery extended deadline: ttl=%d %v", ttl, err)
				}
			}
		})
	}
}

func TestDepartureRecovery_PersistentFailureDoesNotBlockOtherRooms(t *testing.T) {
	db := openDepartureDB(t)
	raw := openIsolatedTestValkey(t, 0)
	t.Cleanup(raw.Close)
	ctx := context.Background()
	f := departureFixture(t, ctx, db, raw)
	other := departureFixture(t, ctx, db, raw)
	client := &departureWriteFault{Client: raw, roomID: f.roomID, command: "SET"}
	client.enabled.Store(true)
	svc := departureService(t, db, client, nil)
	for range 2 {
		if err := svc.LeaveRoom(ctx, f.roomID, f.actors[0]); !errors.Is(err, casino.ErrDepartureRecovery) {
			t.Fatal(err)
		}
	}
	if err := svc.LeaveRoom(ctx, other.roomID, other.actors[0]); err != nil {
		t.Fatal(err)
	}
	assertDeparture(t, ctx, other, 0, 1)
	ch, err := f.characters.FindByID(ctx, f.actors[0])
	if err != nil || ch.Tired != 1 {
		t.Fatalf("persistent failures duplicated fatigue: %+v %v", ch, err)
	}
	d, err := f.accounts.FindDeparture(ctx, f.roomID)
	if err != nil || !d.Pending {
		t.Fatalf("persistent failure discarded evidence: %+v %v", d, err)
	}
	client.enabled.Store(false)
	if err := svc.LeaveRoom(ctx, f.roomID, f.actors[0]); err != nil {
		t.Fatal(err)
	}
	assertDeparture(t, ctx, f, 0, 1)
}

func TestDepartureRecovery_RejectsAmbientTransactionAndStaleRoom(t *testing.T) {
	db := openDepartureDB(t)
	raw := openIsolatedTestValkey(t, 0)
	t.Cleanup(raw.Close)
	ctx := context.Background()
	f := departureFixture(t, ctx, db, raw)
	before := f.state(t, ctx)
	if err := database.RunInTx(ctx, db, func(txCtx context.Context) error {
		return f.svc.LeaveRoom(txCtx, f.roomID, f.actors[0])
	}); !errors.Is(err, casino.ErrDepartureRecovery) {
		t.Fatalf("published within ambient transaction: %v", err)
	}
	f.assertUnchanged(t, ctx, before)
	if err := f.svc.LeaveRoom(ctx, f.roomID, f.actors[0]); err != nil {
		t.Fatal(err)
	}
	assertDeparture(t, ctx, f, 0, 1)
	// An older cached admission cannot claim the same fatigue again.
	if err := f.rooms.UpdateRoomDetail(ctx, casino.RoomDetail{Room: before.room, Members: before.members}); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.LeaveRoom(ctx, f.roomID, f.actors[0]); !errors.Is(err, casino.ErrDepartureRecovery) {
		t.Fatalf("stale admission accepted: %v", err)
	}
	assertDeparture(t, ctx, f, 0, 1)
}
