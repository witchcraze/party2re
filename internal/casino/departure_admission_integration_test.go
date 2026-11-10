package casino_test

import (
	"context"
	"database/sql"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/witchcraze/party2re/internal/casino"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/testutil"
)

// Pause after the preflight returned no pending claim, before admission starts.
type pausedDeparturePreflight struct {
	casino.DepartureRepository
	actor   string
	checked chan struct{}
	resume  chan struct{}
	paused  atomic.Bool
	db      *sql.DB
}

func (r *pausedDeparturePreflight) FindPendingDeparture(ctx context.Context, actor string) (string, error) {
	roomID, err := r.DepartureRepository.FindPendingDeparture(ctx, actor)
	if actor == r.actor && database.TxFromContext(ctx) != nil {
		// An independent transaction proves the admission keeps the character locked.
		probe := database.RunInTx(context.Background(), r.db, func(probeCtx context.Context) error {
			_, err := database.ExecutorFromContext(probeCtx, r.db).ExecContext(probeCtx, "SELECT id FROM characters WHERE id = ? FOR UPDATE NOWAIT", actor)
			return err
		})
		var busy *mysql.MySQLError
		if !errors.As(probe, &busy) || busy.Number != 1205 {
			return "", errors.New("admission must hold the character SQL lock through its final claim check")
		}
	}
	if err == nil && roomID == "" && actor == r.actor && r.paused.CompareAndSwap(false, true) {
		close(r.checked)
		select {
		case <-r.resume:
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	return roomID, err
}

func TestDepartureRecovery_AdmissionAfterEmptyPreflight(t *testing.T) {
	db := openDepartureDB(t)
	raw := openIsolatedTestValkey(t, 0)
	t.Cleanup(raw.Close)
	for _, role := range []string{"departing-participant", "disbanded-spectator"} {
		for _, admission := range []string{"join", "spectate", "create"} {
			t.Run(role+"/"+admission, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
				defer cancel()
				source := departureFixture(t, ctx, db, raw)
				target := departureFixture(t, ctx, db, raw)
				state := target.state(t, ctx)
				state.room.MaxPlayers, state.room.AllowSpectators = 4, true
				if err := target.rooms.UpdateRoomDetail(ctx, casino.RoomDetail{Room: state.room, Members: state.members}); err != nil {
					t.Fatal(err)
				}
				mover := source.actors[0]
				if role == "disbanded-spectator" {
					state := source.state(t, ctx)
					for i := range state.members {
						state.members[i].IsSpectator = state.members[i].CharacterID == source.actors[1]
					}
					if err := source.rooms.UpdateRoomDetail(ctx, casino.RoomDetail{Room: state.room, Members: state.members}); err != nil {
						t.Fatal(err)
					}
					mover = source.actors[1]
				}
				beforeSource, beforeTarget := source.state(t, ctx), target.state(t, ctx)
				preflight := &pausedDeparturePreflight{DepartureRepository: source.accounts, actor: mover, checked: make(chan struct{}), resume: make(chan struct{}), db: db}
				joiner := departureService(t, openDepartureDB(t), raw, preflight)
				leaver := departureService(t, db, raw, departureEvidenceFault{DepartureRepository: source.accounts, failAck: true})
				request := casino.CreateRoomRequest{Name: "Admission-" + id.New()[:8], GameType: casino.GameTypeIndian, Speed: casino.SpeedFast, MaxPlayers: 2, Rate: 10}
				createdRoom := ""
				t.Cleanup(func() {
					if createdRoom != "" {
						if err := target.rooms.DeleteRoom(context.Background(), createdRoom); err != nil {
							t.Error(err)
						}
					}
				})
				admit := func(s *casino.Service) error {
					switch admission {
					case "join":
						_, err := s.JoinRoom(ctx, target.roomID, mover, "", 0)
						return err
					case "spectate":
						_, err := s.SpectateRoom(ctx, target.roomID, mover, "")
						return err
					default:
						detail, err := s.CreateRoom(ctx, mover, request)
						if detail != nil {
							createdRoom = detail.Room.ID
						}
						return err
					}
				}
				var leaveErr error
				admissionErr, raceErr := testutil.RunRace2(
					func() error { return admit(joiner) },
					func() error {
						defer close(preflight.resume)
						select {
						case <-preflight.checked:
						case <-ctx.Done():
							return ctx.Err()
						}
						leaveErr = leaver.LeaveRoom(ctx, source.roomID, source.actors[0])
						return nil
					},
				)
				if raceErr != nil {
					t.Fatal(raceErr)
				}
				if !errors.Is(leaveErr, errDepartureFault) {
					t.Fatalf("acknowledgement fault was not reached: %v", leaveErr)
				}
				d, err := source.accounts.FindDeparture(ctx, source.roomID)
				if err != nil || !d.Pending {
					t.Fatalf("lost pending receipt: %+v %v", d, err)
				}
				if !errors.Is(admissionErr, casino.ErrDepartureRecovery) {
					t.Fatalf("admission passed empty preflight and succeeded with a pending departure: admission=%v pending=%v", admissionErr, d.Pending)
				}
				if createdRoom != "" {
					t.Fatal("rejected admission published a room")
				}
				if _, err := target.rooms.GetMember(ctx, target.roomID, mover); !errors.Is(err, casino.ErrMemberNotFound) {
					t.Fatalf("rejected admission published membership: %v", err)
				}
				target.assertUnchanged(t, ctx, beforeTarget)
				for _, actor := range source.actors {
					if _, err := source.rooms.GetCharacterRoom(ctx, actor); actor == mover && !errors.Is(err, casino.ErrRoomNotFound) {
						t.Fatalf("rejected admission published a mapping: %v", err)
					}
				}
				// With acknowledgement restored, the next command recovers first and then admits.
				healthy := departureService(t, openDepartureDB(t), raw, nil)
				if err := admit(healthy); err != nil {
					t.Fatal(err)
				}
				assertDeparture(t, ctx, source, 0, 1)
				wantRoom := target.roomID
				if admission == "create" {
					wantRoom = createdRoom
				}
				if mapped, err := target.rooms.GetCharacterRoom(ctx, mover); err != nil || mapped != wantRoom {
					t.Fatalf("admission after recovery: mapping=%q %v", mapped, err)
				}
				ch, err := source.characters.FindByID(ctx, mover)
				if err != nil {
					t.Fatal(err)
				}
				if view, err := healthy.GetCharacterRoomView(ctx, ch.PlayerID, mover); err != nil || view == nil || view.Room.ID != wantRoom {
					t.Fatalf("owned view after recovery: %+v %v", view, err)
				}
				if after := source.state(t, ctx); after.coins != beforeSource.coins || after.wins != beforeSource.wins {
					t.Fatal("admission/departure changed coins or wins")
				}
			})
		}
	}
}

type pausedDepartureCommit struct {
	casino.DepartureRepository
	staged chan struct{}
	resume chan struct{}
}

func (r pausedDepartureCommit) CommitDeparture(ctx context.Context, d casino.Departure, mutate func(context.Context) error) error {
	close(r.staged)
	select {
	case <-r.resume:
	case <-ctx.Done():
		return ctx.Err()
	}
	return r.DepartureRepository.CommitDeparture(ctx, d, mutate)
}

func TestDepartureRecovery_RejectsClaimAfterNewAdmission(t *testing.T) {
	db := openDepartureDB(t)
	raw := openIsolatedTestValkey(t, 0)
	t.Cleanup(raw.Close)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	source := departureFixture(t, ctx, db, raw)
	target := departureFixture(t, ctx, db, raw)
	state := target.state(t, ctx)
	state.room.MaxPlayers = 4
	if err := target.rooms.UpdateRoomDetail(ctx, casino.RoomDetail{Room: state.room, Members: state.members}); err != nil {
		t.Fatal(err)
	}
	pause := pausedDepartureCommit{DepartureRepository: source.accounts, staged: make(chan struct{}), resume: make(chan struct{})}
	leaver := departureService(t, db, raw, pause)
	joiner := departureService(t, openDepartureDB(t), raw, nil)
	mover := source.actors[0]
	leaveErr, joinErr := testutil.RunRace2(
		func() error { return leaver.LeaveRoom(ctx, source.roomID, mover) },
		func() error {
			defer close(pause.resume)
			select {
			case <-pause.staged:
			case <-ctx.Done():
				return ctx.Err()
			}
			// Lose only this fixture's Valkey state before the staged departure commits.
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
	if joinErr != nil || !errors.Is(leaveErr, casino.ErrDepartureRecovery) {
		t.Fatalf("new admission must win over the stale departure: join=%v leave=%v", joinErr, leaveErr)
	}
	if d, err := source.accounts.FindDeparture(ctx, source.roomID); err != nil || d.Version != 0 {
		t.Fatalf("stale departure claimed the new admission: %+v %v", d, err)
	}
	if claim, err := source.accounts.FindPendingDeparture(ctx, mover); err != nil || claim != "" {
		t.Fatalf("new admission stranded by old claim: %q %v", claim, err)
	}
	ch, err := source.characters.FindByID(ctx, mover)
	if err != nil || ch.Tired != 0 {
		t.Fatalf("rejected departure changed fatigue: %+v %v", ch, err)
	}
	if view, err := joiner.GetCharacterRoomView(ctx, ch.PlayerID, mover); err != nil || view == nil || view.Room.ID != target.roomID {
		t.Fatalf("new admission hidden or overwritten: %+v %v", view, err)
	}
	for _, f := range []*publicationFixture{source, target} {
		for _, actor := range f.actors {
			acc, err := f.accounts.GetAccount(ctx, actor)
			if err != nil || acc.Coins != 500 {
				t.Fatalf("coins changed: %+v %v", acc, err)
			}
		}
	}
}
