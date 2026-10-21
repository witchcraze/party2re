package pvp_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"sync/atomic"
	"testing"

	"github.com/valkey-io/valkey-go"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/pvp"
	"github.com/witchcraze/party2re/internal/testutil"
)

var errPublication = errors.New("injected publication failure")

type transitionRooms struct {
	pvp.RoomRepository
	fail  string
	after bool
	armed atomic.Bool
}

func (r *transitionRooms) fault(kind string, apply func() error) error {
	if kind == r.fail && r.armed.CompareAndSwap(true, false) {
		if r.after {
			if err := apply(); err != nil {
				return err
			}
		}
		return errPublication
	}
	return apply()
}
func (r *transitionRooms) SaveRoom(ctx context.Context, room pvp.ColosseumRoom, members []pvp.RoomMember) error {
	return r.fault("save", func() error { return r.RoomRepository.SaveRoom(ctx, room, members) })
}
func (r *transitionRooms) SetCharacterRoom(ctx context.Context, characterID, roomID string) error {
	return r.fault("set", func() error { return r.RoomRepository.SetCharacterRoom(ctx, characterID, roomID) })
}
func (r *transitionRooms) DeleteCharacterRoom(ctx context.Context, characterID, roomID string) error {
	return r.fault("delete-member", func() error { return r.RoomRepository.DeleteCharacterRoom(ctx, characterID, roomID) })
}
func (r *transitionRooms) DeleteRoom(ctx context.Context, roomID string) error {
	return r.fault("delete-room", func() error { return r.RoomRepository.DeleteRoom(ctx, roomID) })
}

type transitionCharacters struct {
	pvp.CharacterRepository
	fail atomic.Bool
}

func (r *transitionCharacters) Update(ctx context.Context, c corecharacter.Character) error {
	if err := r.CharacterRepository.Update(ctx, c); err != nil {
		return err
	}
	if r.fail.CompareAndSwap(true, false) {
		return errors.New("injected SQL failure after update")
	}
	return nil
}

type transitionBattle struct {
	calls   atomic.Int64
	outcome corebattle.Outcome
}

func (b *transitionBattle) ResolvePartyBattle(corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
	b.calls.Add(1)
	return corebattle.PartyBattleResult{Outcome: b.outcome}, nil
}

type transitionFixture struct {
	db             *sql.DB
	client         valkey.Client
	rooms          *transitionRooms
	chars          *transitionCharacters
	store          *database.PvPTransitionRepository
	service        *pvp.Service
	battle         *transitionBattle
	leader, member corecharacter.Character
	room           pvp.RoomDetail
	roomIDs        map[string]bool
}

func newTransitionFixture(t *testing.T) *transitionFixture {
	t.Helper()
	if os.Getenv("PARTY2_DB_DSN") == "" || os.Getenv("PARTY2_VALKEY_ADDR") == "" {
		t.Skip("PARTY2_DB_DSN and PARTY2_VALKEY_ADDR are required")
	}
	ctx := context.Background()
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	client, err := valkey.NewClient(valkey.ClientOption{InitAddress: []string{os.Getenv("PARTY2_VALKEY_ADDR")}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	chars, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	store, err := database.NewPvPTransitionRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	rooms, err := pvp.NewValkeyRoomRepository(client)
	if err != nil {
		t.Fatal(err)
	}
	f := &transitionFixture{db: db, client: client, rooms: &transitionRooms{RoomRepository: rooms}, chars: &transitionCharacters{CharacterRepository: chars}, store: store, battle: &transitionBattle{outcome: corebattle.OutcomeWin}, roomIDs: map[string]bool{}}
	f.leader, err = database.CreateTestCharacterWithFunds(ctx, db, "PvpLeader", 1000)
	if err != nil {
		t.Fatal(err)
	}
	f.member, err = database.CreateTestCharacterWithFunds(ctx, db, "PvpMember", 1000)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, c := range []corecharacter.Character{f.leader, f.member} {
			roomID, err := store.FindCharacterClaim(ctx, c.ID)
			if err != nil {
				t.Error(err)
			} else if roomID != "" {
				f.roomIDs[roomID] = true
			}
		}
		for roomID := range f.roomIDs {
			if err := rooms.DeleteRoom(ctx, roomID); err != nil {
				t.Error(err)
			}
			for _, c := range []corecharacter.Character{f.leader, f.member} {
				if err := rooms.DeleteCharacterRoom(ctx, c.ID, roomID); err != nil {
					t.Error(err)
				}
			}
			if _, err := db.ExecContext(ctx, "DELETE FROM pvp_room_transitions WHERE room_id = ?", roomID); err != nil {
				t.Error(err)
			}
		}
		for _, c := range []corecharacter.Character{f.leader, f.member} {
			if _, err := db.ExecContext(ctx, "DELETE FROM character_depots WHERE character_id = ?", c.ID); err != nil {
				t.Error(err)
			}
			if _, err := db.ExecContext(ctx, "DELETE FROM characters WHERE id = ?", c.ID); err != nil {
				t.Error(err)
			}
			if _, err := db.ExecContext(ctx, "DELETE FROM players WHERE id = ?", c.PlayerID); err != nil {
				t.Error(err)
			}
		}
	})
	f.restart(t)
	return f
}

func TestPvPSeparateRoomAdmissionIntegration(t *testing.T) {
	f := newTransitionFixture(t)
	ctx := context.Background()
	if err := f.create(t); err != nil {
		t.Fatal(err)
	}
	other, err := f.service.CreateRoom(ctx, f.member.ID, pvp.CreateRoomRequest{Name: "Other", Bet: 100})
	if err != nil {
		t.Fatal(err)
	}
	f.roomIDs[other.Room.ID] = true
	joiner, err := database.CreateTestCharacterWithFunds(ctx, f.db, "PvpJoiner", 1000)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for roomID := range f.roomIDs {
			if err := f.rooms.DeleteCharacterRoom(ctx, joiner.ID, roomID); err != nil {
				t.Error(err)
			}
		}
		if _, err := f.db.ExecContext(ctx, "DELETE FROM character_depots WHERE character_id = ?", joiner.ID); err != nil {
			t.Error(err)
		}
		if _, err := f.db.ExecContext(ctx, "DELETE FROM characters WHERE id = ?", joiner.ID); err != nil {
			t.Error(err)
		}
		if _, err := f.db.ExecContext(ctx, "DELETE FROM players WHERE id = ?", joiner.PlayerID); err != nil {
			t.Error(err)
		}
	})
	errs := testutil.RunRace(
		func() error { _, err := f.service.JoinRoom(ctx, joiner.ID, f.room.Room.ID, "secret"); return err },
		func() error { _, err := f.service.JoinRoom(ctx, joiner.ID, other.Room.ID, ""); return err },
	)
	successes := 0
	for _, err := range errs {
		if err == nil {
			successes++
		} else if !errors.Is(err, pvp.ErrAlreadyInRoom) {
			t.Fatal(err)
		}
	}
	if successes != 1 {
		t.Fatalf("admitted %d rooms", successes)
	}
	c, err := f.chars.FindByID(ctx, joiner.ID)
	if err != nil {
		t.Fatal(err)
	}
	a, err := f.rooms.GetRoom(ctx, f.room.Room.ID)
	if err != nil {
		t.Fatal(err)
	}
	b, err := f.rooms.GetRoom(ctx, other.Room.ID)
	if err != nil {
		t.Fatal(err)
	}
	if c.Money != 900 || a.Room.PrizePool+b.Room.PrizePool != 300 {
		t.Fatalf("money=%d pools=%d/%d", c.Money, a.Room.PrizePool, b.Room.PrizePool)
	}
	// Leaving a completed older room must not erase a newer room's mapping.
	if err := f.rooms.SetCharacterRoom(ctx, joiner.ID, other.Room.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.rooms.DeleteCharacterRoom(ctx, joiner.ID, f.room.Room.ID); err != nil {
		t.Fatal(err)
	}
	if roomID, err := f.rooms.GetCharacterRoom(ctx, joiner.ID); err != nil || roomID != other.Room.ID {
		t.Fatalf("removed newer membership: %s %v", roomID, err)
	}
}
func (f *transitionFixture) restart(t *testing.T) {
	t.Helper()
	// New owner instances must recover solely from SQL and Valkey.
	store, err := database.NewPvPTransitionRepository(f.db)
	if err != nil {
		t.Fatal(err)
	}
	svc, err := pvp.NewService(f.rooms, f.chars, f.battle, pvp.WithTransactionProvider(database.NewTransactionProvider(f.db)), pvp.WithTransitionRepository(store))
	if err != nil {
		t.Fatal(err)
	}
	f.service = svc
}
func (f *transitionFixture) create(t *testing.T) error {
	t.Helper()
	detail, err := f.service.CreateRoom(context.Background(), f.leader.ID, pvp.CreateRoomRequest{Name: "Recovery", Bet: 100, MaxMembers: 2, TargetWins: 1, Password: "secret"})
	if err == nil {
		f.room = detail
		f.roomIDs[detail.Room.ID] = true
	}
	return err
}
func (f *transitionFixture) prepare(t *testing.T, kind string) {
	t.Helper()
	ctx := context.Background()
	if kind == "create" {
		return
	}
	if err := f.create(t); err != nil {
		t.Fatal(err)
	}
	if kind == "join" {
		return
	}
	if _, err := f.service.JoinRoom(ctx, f.member.ID, f.room.Room.ID, "secret"); err != nil {
		t.Fatal(err)
	}
	if kind == "settle" || kind == "draw" {
		if _, err := f.service.SelectTeam(ctx, f.leader.ID, f.room.Room.ID, pvp.ColorRed); err != nil {
			t.Fatal(err)
		}
		if _, err := f.service.SelectTeam(ctx, f.member.ID, f.room.Room.ID, pvp.ColorBlue); err != nil {
			t.Fatal(err)
		}
		detail, err := f.service.StartMatch(ctx, f.leader.ID, f.room.Room.ID)
		if err != nil {
			t.Fatal(err)
		}
		if kind == "draw" {
			f.battle.outcome = corebattle.OutcomeDraw
			detail.Room.Round = pvp.MaxRounds
			if err := f.rooms.SaveRoom(ctx, detail.Room, detail.Members); err != nil {
				t.Fatal(err)
			}
		}
	}
}
func (f *transitionFixture) act(t *testing.T, kind string) error {
	t.Helper()
	ctx := context.Background()
	switch kind {
	case "create":
		return f.create(t)
	case "join":
		_, err := f.service.JoinRoom(ctx, f.member.ID, f.room.Room.ID, "secret")
		return err
	case "leave":
		return f.service.LeaveRoom(ctx, f.member.ID, f.room.Room.ID)
	case "disband":
		return f.service.LeaveRoom(ctx, f.leader.ID, f.room.Room.ID)
	default:
		_, err := f.service.AdvanceRound(ctx, f.leader.ID, f.room.Room.ID)
		return err
	}
}
func (f *transitionFixture) assertFunds(t *testing.T, leaderMoney, memberMoney, wins, pool int) {
	t.Helper()
	ctx := context.Background()
	leader, err := f.chars.FindByID(ctx, f.leader.ID)
	if err != nil {
		t.Fatal(err)
	}
	member, err := f.chars.FindByID(ctx, f.member.ID)
	if err != nil {
		t.Fatal(err)
	}
	if leader.Money != leaderMoney || member.Money != memberMoney || leader.PvPWins != wins {
		t.Fatalf("money=%d/%d wins=%d; want %d/%d wins=%d", leader.Money, member.Money, leader.PvPWins, leaderMoney, memberMoney, wins)
	}
	roomID := f.room.Room.ID
	if roomID == "" {
		roomID, err = f.store.FindCharacterClaim(ctx, f.leader.ID)
		if err != nil {
			t.Fatal(err)
		}
	}
	state, err := f.store.FindTransition(ctx, roomID)
	if err != nil {
		t.Fatal(err)
	}
	actualPool := state.Detail.Room.PrizePool
	if actualPool != pool {
		t.Fatalf("durable escrow=%d; want %d", actualPool, pool)
	}
	if leader.Money+member.Money+actualPool != 2000 {
		t.Fatal("assets not conserved")
	}
}

func TestPvPFinancialPublicationIntegration(t *testing.T) {
	for _, kind := range []string{"create", "join", "leave", "disband", "settle", "draw"} {
		faults := []string{"save", "set"}
		if kind == "leave" {
			faults = append(faults, "delete-member")
		}
		if kind == "disband" {
			faults = []string{"delete-room", "delete-member"}
		}
		for _, fault := range faults {
			for _, after := range []bool{false, true} {
				name := kind + "/" + fault + map[bool]string{false: "/before", true: "/lost-reply"}[after]
				t.Run(name, func(t *testing.T) {
					f := newTransitionFixture(t)
					f.prepare(t, kind)
					f.rooms.fail = fault
					f.rooms.after = after
					f.rooms.armed.Store(true)
					if err := f.act(t, kind); !errors.Is(err, errPublication) {
						t.Fatalf("expected injected failure: %v", err)
					}
					f.restart(t)
					if err := f.act(t, kind); err != nil {
						t.Fatal(err)
					}
					switch kind {
					case "create":
						f.assertFunds(t, 900, 1000, 0, 100)
					case "join":
						f.assertFunds(t, 900, 900, 0, 200)
					case "leave":
						f.assertFunds(t, 900, 1000, 0, 100)
					case "disband", "draw":
						f.assertFunds(t, 1000, 1000, 0, 0)
					case "settle":
						f.assertFunds(t, 1100, 900, 1, 0)
					}
					if kind == "settle" || kind == "draw" {
						if err := f.act(t, kind); err != nil {
							t.Fatal(err)
						}
						if f.battle.calls.Load() != 1 {
							t.Fatalf("battle reran: %d", f.battle.calls.Load())
						}
					}
					if kind == "create" || kind == "join" {
						if _, err := f.service.JoinRoom(context.Background(), f.member.ID, f.room.Room.ID, "wrong"); kind == "create" && !errors.Is(err, pvp.ErrInvalidPassword) {
							t.Fatalf("recovery lost password: %v", err)
						}
					}
				})
			}
		}
	}
}

func TestPvPFinancialRollbackIntegration(t *testing.T) {
	for _, kind := range []string{"create", "join", "leave", "disband", "settle", "draw"} {
		t.Run(kind, func(t *testing.T) {
			f := newTransitionFixture(t)
			f.prepare(t, kind)
			before, err := f.store.FindTransition(context.Background(), f.room.Room.ID)
			if err != nil {
				t.Fatal(err)
			}
			f.chars.fail.Store(true)
			if err := f.act(t, kind); err == nil {
				t.Fatal("expected SQL failure")
			}
			switch kind {
			case "create":
				f.assertFunds(t, 1000, 1000, 0, 0)
			case "join":
				f.assertFunds(t, 900, 1000, 0, 100)
			default:
				f.assertFunds(t, 900, 900, 0, 200)
			}
			after, err := f.store.FindTransition(context.Background(), f.room.Room.ID)
			if err != nil {
				t.Fatal(err)
			}
			if after.Version != before.Version || after.Pending {
				t.Fatalf("rollback changed transition: %+v", after)
			}
			if err := f.act(t, kind); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPvPFinancialConcurrentIntegration(t *testing.T) {
	for _, kind := range []string{"create", "join", "leave", "disband", "settle", "draw"} {
		t.Run(kind, func(t *testing.T) {
			f := newTransitionFixture(t)
			f.prepare(t, kind)
			// Concurrent create reads may race; only the SQL claim may admit a debit.
			cfg := testutil.GetStressConfig()
			result := testutil.RunConcurrentStressTest(t, cfg, func(worker, op int) error {
				ctx := context.Background()
				switch kind {
				case "create":
					_, err := f.service.CreateRoom(ctx, f.leader.ID, pvp.CreateRoomRequest{Name: "Concurrent", Bet: 100})
					return err
				case "join":
					_, err := f.service.JoinRoom(ctx, f.member.ID, f.room.Room.ID, "secret")
					return err
				case "leave":
					return f.service.LeaveRoom(ctx, f.member.ID, f.room.Room.ID)
				case "disband":
					return f.service.LeaveRoom(ctx, f.leader.ID, f.room.Room.ID)
				default:
					_, err := f.service.AdvanceRound(ctx, f.leader.ID, f.room.Room.ID)
					return err
				}
			})
			if result.Successes < 1 {
				t.Fatalf("no operation succeeded: %+v", result)
			}
			switch kind {
			case "create", "leave":
				f.assertFunds(t, 900, 1000, 0, 100)
			case "join":
				f.assertFunds(t, 900, 900, 0, 200)
			case "disband", "draw":
				f.assertFunds(t, 1000, 1000, 0, 0)
			case "settle":
				f.assertFunds(t, 1100, 900, 1, 0)
			}
		})
	}
}
