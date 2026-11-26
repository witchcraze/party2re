package runhost_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/economy"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/runhost"
	"github.com/witchcraze/party2re/internal/testutil"
)

type sysRun struct{ runhost.State }

func (r *sysRun) Host() *runhost.State { return &r.State }

// sysRoom is a test system room: capacity, +20 Tired on join and a
// test-controlled retention answer.
type sysRoom struct {
	capacity int
	retain   *atomic.Bool
}

func (sysRoom) Prepare(*runhost.Admission) error                   { return nil }
func (sysRoom) AdmitMember(context.Context, *sysRun, string) error { return nil }
func (sysRoom) Open(context.Context, *sysRun) (func(context.Context) error, error) {
	return nil, nil
}
func (sysRoom) Actions() []string { return nil }
func (sysRoom) Act(context.Context, *sysRun, runhost.Command) (runhost.Effects, error) {
	return runhost.Effects{}, runhost.ErrInvalidAction
}
func (sysRoom) Exit(*sysRun, string) (runhost.Earnings, int, error) {
	return runhost.Earnings{}, 0, nil
}
func (sysRoom) Record(context.Context, *sysRun, string, string) error { return nil }
func (sysRoom) Conditional(*sysRun) bool                              { return false }
func (room sysRoom) Retain(*sysRun) bool                              { return room.retain.Load() }
func (sysRoom) Banish(_ *sysRun, c *corecharacter.Character) error    { c.AddTired(30); return nil }
func (room sysRoom) Join(r *sysRun, c *corecharacter.Character) error {
	if len(r.Roster) >= room.capacity {
		return runhost.ErrRoomFull
	}
	c.AddTired(20)
	return nil
}

type sysClock struct{ nanos atomic.Int64 }

func (c *sysClock) Now() time.Time { return time.Unix(0, c.nanos.Load()).UTC() }

type sysPenalty struct{}

func (sysPenalty) ProjectRunPenalty(context.Context, string, time.Time) error { return nil }

// lossyStore reports one committed run transaction as failed (a lost acknowledgement)
// and can inject mid-transaction save or receipt-lookup failures in tests.
type lossyStore struct {
	runhost.Store[sysRun]
	lose        *atomic.Bool
	failSave    *atomic.Bool
	failReceipt *atomic.Bool
}

func (s lossyStore) WithRunForUpdate(ctx context.Context, id string, create bool, fn func(context.Context) error) error {
	err := s.Store.WithRunForUpdate(ctx, id, create, fn)
	if err == nil && s.lose.CompareAndSwap(true, false) {
		return errors.New("lost acknowledgement")
	}
	return err
}

func (s lossyStore) SaveRun(ctx context.Context, value sysRun, expected int) error {
	if s.failSave != nil && s.failSave.CompareAndSwap(true, false) {
		return errors.New("injected save failure")
	}
	return s.Store.SaveRun(ctx, value, expected)
}

func (s lossyStore) FindRunReceipt(ctx context.Context, id, key string) (*runhost.Receipt, error) {
	if s.failReceipt != nil && s.failReceipt.CompareAndSwap(true, false) {
		return nil, errors.New("injected receipt read failure")
	}
	return s.Store.FindRunReceipt(ctx, id, key)
}

type sysFixture struct {
	db          *sql.DB
	chars       *database.CharacterRepository
	clock       *sysClock
	retain      atomic.Bool
	lose        atomic.Bool
	failSave    atomic.Bool
	failReceipt atomic.Bool
	svc         *runhost.Service[sysRun, *sysRun]
}

func newSysFixture(t *testing.T, capacity int) *sysFixture {
	t.Helper()
	f := &sysFixture{db: database.OpenTestDB(t), clock: &sysClock{}}
	f.clock.nanos.Store(time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC).UnixNano())
	f.retain.Store(true)
	var err error
	if f.chars, err = database.NewCharacterRepository(f.db); err != nil {
		t.Fatal(err)
	}
	store, err := database.NewRunRepository[sysRun](f.db)
	if err != nil {
		t.Fatal(err)
	}
	invs, err := database.NewInventoryRepository(f.db)
	if err != nil {
		t.Fatal(err)
	}
	equips, err := database.NewEquipmentRepository(f.db)
	if err != nil {
		t.Fatal(err)
	}
	customs, err := database.NewCustomSkillRepository(f.db)
	if err != nil {
		t.Fatal(err)
	}
	depots, err := database.NewDepotRepository(f.db)
	if err != nil {
		t.Fatal(err)
	}
	history, err := database.NewCharacterJobRepository(f.db)
	if err != nil {
		t.Fatal(err)
	}
	tx := database.NewTransactionProvider(f.db)
	runner, err := economy.NewService(f.chars, invs, economy.WithTransactionProvider(tx))
	if err != nil {
		t.Fatal(err)
	}
	adapter := battle.NewService(battle.WithCharacterRepository(f.chars), battle.WithCustomSkillRepository(customs), battle.WithInventoryRepository(invs), battle.WithDepotRepository(depots), battle.WithEquipmentRepository(equips), battle.WithJobHistoryRepository(history), battle.WithTransactionProvider(tx), battle.WithTransactionRunner(runner))
	room := sysRoom{capacity: capacity, retain: &f.retain}
	f.svc, err = runhost.NewService[sysRun](runhost.Config[sysRun]{
		Kind: "system-test", MaxMembers: 6, Store: lossyStore{Store: store, lose: &f.lose, failSave: &f.failSave, failReceipt: &f.failReceipt}, Characters: f.chars, Inventories: invs,
		Builder: adapter, Settler: adapter, Penalty: sysPenalty{}, Clock: f.clock, Room: room, SystemRoom: room,
		ExitEffects: func(context.Context, string) error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *sysFixture) character(t *testing.T) corecharacter.Character {
	t.Helper()
	c, err := database.CreateTestCharacterWithFunds(context.Background(), f.db, id.New()[:8], 0)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func (f *sysFixture) tired(t *testing.T, characterID string) int {
	t.Helper()
	c, err := f.chars.FindByID(context.Background(), characterID)
	if err != nil {
		t.Fatal(err)
	}
	return c.Tired
}

// open opens a room on a test-owned slot whose single enemy has 7 of 10 HP.
func (f *sysFixture) open(t *testing.T, slot string) string {
	t.Helper()
	runID, err := f.svc.Open(context.Background(), slot, runhost.Admission{StageID: "king-test", Speed: 12}, func(r *sysRun) {
		r.Round = 1
		r.Enemies = []string{"boss"}
		r.Battle.Actors = append(r.Battle.Actors, corebattle.LifetimeActor{Participant: corebattle.Participant{ID: "boss", Name: "@Boss", HP: 7, MaxHP: 10}, NPC: true})
	})
	if err != nil || runID == "" {
		t.Fatal(runID, err)
	}
	return runID
}

func (f *sysFixture) room(t *testing.T, slot string) *sysRun {
	t.Helper()
	r, err := f.svc.FindSlot(context.Background(), slot)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func (f *sysFixture) exit(t *testing.T, c corecharacter.Character, runID string) {
	t.Helper()
	version := f.version(t, runID)
	if _, err := f.svc.Command(context.Background(), runhost.Command{PlayerID: c.PlayerID, ActorID: c.ID, RunID: runID, ID: id.New(), Version: version, Action: "exit"}); err != nil {
		t.Fatal(err)
	}
}

func (f *sysFixture) version(t *testing.T, runID string) int {
	t.Helper()
	var version int
	if err := f.db.QueryRowContext(context.Background(), `SELECT version FROM runs WHERE run_id=?`, runID).Scan(&version); err != nil {
		t.Fatal(err)
	}
	return version
}

func enemyHP(r *sysRun) int {
	if a := r.Actor("boss"); a != nil {
		return a.HP
	}
	return -1
}

func TestSystemRunLifecycleIntegration(t *testing.T) {
	f := newSysFixture(t, 6)
	ctx := context.Background()
	slot := "test:" + id.New()
	runID := f.open(t, slot)
	if again := f.open(t, slot); again != runID {
		t.Fatalf("reopen of a live slot = %s, want %s", again, runID)
	}
	r := f.room(t, slot)
	if r == nil || len(r.Roster) != 0 || r.LeaderID != "" || r.Round != 1 || enemyHP(r) != 7 {
		t.Fatalf("opened room = %+v", r)
	}

	a, b := f.character(t), f.character(t)
	// A lost acknowledgement resolves to the committed join.
	f.lose.Store(true)
	receipt, err := f.svc.Join(ctx, a.PlayerID, a.ID, runID, "join-a")
	if err != nil || receipt.Version != 2 || receipt.Command.Action != "join" {
		t.Fatalf("join = %+v, %v", receipt, err)
	}
	replay, err := f.svc.Join(ctx, a.PlayerID, a.ID, runID, "join-a")
	if err != nil || !reflect.DeepEqual(replay, receipt) {
		t.Fatalf("replay = %+v, %v", replay, err)
	}
	if got := f.tired(t, a.ID); got != a.Tired+20 {
		t.Fatalf("joiner tired = %d, want %d once", got, a.Tired+20)
	}
	if _, err := f.svc.Join(ctx, b.PlayerID, b.ID, runID, "join-a"); !errors.Is(err, runhost.ErrForbidden) {
		t.Fatalf("another actor's key = %v", err)
	}
	r = f.room(t, slot)
	if !slices.Equal(r.Roster, []string{a.ID}) || r.Actor(a.ID) == nil || r.Actor(a.ID).TeamID != "allies" || enemyHP(r) != 7 {
		t.Fatalf("joined room = %+v", r.State)
	}
	if run, err := f.svc.GetActorRun(ctx, a.PlayerID, a.ID); err != nil || run == nil || run.RunID != runID {
		t.Fatalf("joiner's run = %+v, %v", run, err)
	}

	// The last exit keeps a retained room and its enemy state; the joiner can come back.
	f.exit(t, a, runID)
	r = f.room(t, slot)
	if r == nil || len(r.Roster) != 0 || enemyHP(r) != 7 {
		t.Fatalf("retained room = %+v", r)
	}
	if _, err := f.svc.Join(ctx, a.PlayerID, a.ID, runID, "join-a-again"); err != nil {
		t.Fatalf("rejoin after exit: %v", err)
	}

	// Without retention the last exit disposes the room and frees its slot.
	f.retain.Store(false)
	f.exit(t, a, runID)
	if r := f.room(t, slot); r != nil {
		t.Fatalf("disposed room still live: %+v", r.State)
	}
	if next := f.open(t, slot); next == runID {
		t.Fatal("a reopened slot reused the disposed run identity")
	}
}

func TestSystemRunJoinRejectionsIntegration(t *testing.T) {
	f := newSysFixture(t, 1)
	ctx := context.Background()
	first := f.open(t, "test:"+id.New())
	other := f.open(t, "test:"+id.New())
	a, b, tired, fallen, asleep := f.character(t), f.character(t), f.character(t), f.character(t), f.character(t)
	if _, err := f.svc.Join(ctx, a.PlayerID, a.ID, first, "a"); err != nil {
		t.Fatal(err)
	}
	for query, c := range map[string]corecharacter.Character{
		`UPDATE characters SET tired=100 WHERE id=?`:      tired,
		`UPDATE characters SET hp=0 WHERE id=?`:           fallen,
		`UPDATE characters SET pending_wake=1 WHERE id=?`: asleep,
	} {
		if _, err := f.db.ExecContext(ctx, query, c.ID); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name string
		c    corecharacter.Character
		run  string
		want error
	}{
		{"full room", b, first, runhost.ErrRoomFull},
		{"already in another run", a, other, runhost.ErrBusy},
		{"exhausted", tired, other, runhost.ErrCharacterExhausted},
		{"unconscious", fallen, other, runhost.ErrCharacterUnconscious},
		{"pending wake", asleep, other, runhost.ErrBusy},
		{"missing run", b, id.New(), runhost.ErrNotFound},
	} {
		before := f.tired(t, tc.c.ID)
		if _, err := f.svc.Join(ctx, tc.c.PlayerID, tc.c.ID, tc.run, "reject-"+tc.name); !errors.Is(err, tc.want) {
			t.Fatalf("%s: err = %v, want %v", tc.name, err, tc.want)
		}
		if got := f.tired(t, tc.c.ID); got != before {
			t.Fatalf("%s: rejected join changed tired %d -> %d", tc.name, before, got)
		}
	}
	if _, err := f.svc.Join(ctx, b.PlayerID, a.ID, other, "forged"); !errors.Is(err, runhost.ErrForbidden) {
		t.Fatalf("another player's character = %v", err)
	}
}

func TestSystemRunIdleDisposalIntegration(t *testing.T) {
	f := newSysFixture(t, 6)
	ctx := context.Background()
	empty, occupied := "test:"+id.New(), "test:"+id.New()
	emptyID, occupiedID := f.open(t, empty), f.open(t, occupied)
	c := f.character(t)
	if _, err := f.svc.Join(ctx, c.PlayerID, c.ID, occupiedID, "occupy"); err != nil {
		t.Fatal(err)
	}
	f.clock.nanos.Add(int64(29 * time.Minute))
	if err := f.svc.DisposeIdle(ctx, emptyID); !errors.Is(err, runhost.ErrConflict) {
		t.Fatalf("dispose before the idle timeout = %v", err)
	}
	f.clock.nanos.Add(int64(2 * time.Minute))
	if err := f.svc.DisposeIdle(ctx, occupiedID); !errors.Is(err, runhost.ErrConflict) {
		t.Fatalf("dispose of an occupied room = %v", err)
	}
	if err := f.svc.DisposeIdle(ctx, emptyID); err != nil {
		t.Fatal(err)
	}
	if r := f.room(t, empty); r != nil {
		t.Fatalf("idle empty room still live: %+v", r.State)
	}
	if r := f.room(t, occupied); r == nil || !slices.Equal(r.Roster, []string{c.ID}) {
		t.Fatalf("occupied room = %+v", r)
	}
	// A member's idle cleanup deletes a retained room too (auto_delete_quest).
	if _, err := f.svc.Command(ctx, runhost.Command{PlayerID: c.PlayerID, ActorID: c.ID, RunID: occupiedID, ID: "cleanup", Version: f.version(t, occupiedID), Action: "cleanup"}); err != nil {
		t.Fatal(err)
	}
	if r := f.room(t, occupied); r != nil {
		t.Fatalf("timed-out retained room still live: %+v", r.State)
	}
}

// TestSystemRunJoinStressIntegration races every worker's character for a
// two-slot room: capacity holds, and only admitted joiners pay the join fatigue.
func TestSystemRunJoinStressIntegration(t *testing.T) {
	const capacity = 2
	f := newSysFixture(t, capacity)
	ctx := context.Background()
	cfg := testutil.GetStressConfig()
	slot := "test:" + id.New()
	runID := f.open(t, slot)
	chars := make([]corecharacter.Character, cfg.Workers)
	for i := range chars {
		chars[i] = f.character(t)
	}
	result := testutil.RunConcurrentStressTest(t, cfg, func(worker, op int) error {
		c := chars[worker]
		_, err := f.svc.Join(ctx, c.PlayerID, c.ID, runID, fmt.Sprintf("join-%d", worker))
		if err == nil || errors.Is(err, runhost.ErrRoomFull) {
			return nil
		}
		return err
	})
	if result.Failures != 0 {
		t.Fatalf("unexpected join failures: %d", result.Failures)
	}
	r := f.room(t, slot)
	if len(r.Roster) != capacity {
		t.Fatalf("roster = %v, want %d members", r.Roster, capacity)
	}
	enrolled := 0
	for _, c := range chars {
		var runs int
		if err := f.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM run_enrollments WHERE character_id=?`, c.ID).Scan(&runs); err != nil {
			t.Fatal(err)
		}
		want := c.Tired
		if runs == 1 {
			enrolled++
			want += 20
		}
		if runs > 1 || f.tired(t, c.ID) != want {
			t.Fatalf("character %s: runs=%d tired=%d want %d", c.ID, runs, f.tired(t, c.ID), want)
		}
	}
	if enrolled != capacity {
		t.Fatalf("enrolled = %d, want %d", enrolled, capacity)
	}
}

// TestSystemRunJoinTwoRoomsRaceIntegration races one character into two rooms
// with free slots: exactly one join commits, with one enrollment and one +20 Tired.
func TestSystemRunJoinTwoRoomsRaceIntegration(t *testing.T) {
	f := newSysFixture(t, 6)
	ctx := context.Background()
	slots := []string{"test:" + id.New(), "test:" + id.New()}
	rooms := []string{f.open(t, slots[0]), f.open(t, slots[1])}
	for round := range 5 {
		c := f.character(t)
		join := func(i int) func() error {
			return func() error {
				_, err := f.svc.Join(ctx, c.PlayerID, c.ID, rooms[i], fmt.Sprintf("race-%d", round))
				return err
			}
		}
		err0, err1 := testutil.RunRace2(join(0), join(1))
		if (err0 == nil) == (err1 == nil) {
			t.Fatalf("round %d: errs = %v, %v; want exactly one join", round, err0, err1)
		}
		if lost := errors.Join(err0, err1); !errors.Is(lost, runhost.ErrBusy) {
			t.Fatalf("round %d: losing join = %v, want ErrBusy", round, lost)
		}
		var runs int
		if err := f.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM run_enrollments WHERE character_id=?`, c.ID).Scan(&runs); err != nil {
			t.Fatal(err)
		}
		members := 0
		for _, slot := range slots {
			if slices.Contains(f.room(t, slot).Roster, c.ID) {
				members++
			}
		}
		if runs != 1 || members != 1 || f.tired(t, c.ID) != c.Tired+20 {
			t.Fatalf("round %d: enrollments=%d memberships=%d tired=%d, want 1/1/%d", round, runs, members, f.tired(t, c.ID), c.Tired+20)
		}
	}
}

func TestSystemRunConcurrentOpenIntegration(t *testing.T) {
	f := newSysFixture(t, 6)
	slot := "test:" + id.New()
	ids := make(chan string, 8)
	result := testutil.RunConcurrentStressTest(t, testutil.ConcurrencyStressConfig{Workers: 8, OpsPerWorker: 1}, func(int, int) error {
		runID, err := f.svc.Open(context.Background(), slot, runhost.Admission{StageID: "king-test", Speed: 12}, nil)
		ids <- runID
		return err
	})
	close(ids)
	if result.Failures != 0 {
		t.Fatalf("open failures: %d", result.Failures)
	}
	var first string
	for runID := range ids {
		if first == "" {
			first = runID
		}
		if runID != first || runID == "" {
			t.Fatalf("concurrent opens returned %q and %q", first, runID)
		}
	}
}

func TestSystemRunJoinRollbackAndRecoveryIntegration(t *testing.T) {
	f := newSysFixture(t, 6)
	ctx := context.Background()
	slot := "test:" + id.New()
	runID := f.open(t, slot)
	a, b := f.character(t), f.character(t)

	// A SQL failure after Characters.Update rolls back both fatigue and membership.
	f.failSave.Store(true)
	if _, err := f.svc.Join(ctx, a.PlayerID, a.ID, runID, "join-a"); err == nil {
		t.Fatal("expected injected SaveRun error")
	}
	if got := f.tired(t, a.ID); got != a.Tired {
		t.Fatalf("rolled-back join changed tired %d -> %d", a.Tired, got)
	}
	if r := f.room(t, slot); len(r.Roster) != 0 {
		t.Fatalf("rolled-back join left roster %v", r.Roster)
	}

	// Retrying with the same key commits once, and a different key rejects without a second charge.
	receipt, err := f.svc.Join(ctx, a.PlayerID, a.ID, runID, "join-a")
	if err != nil {
		t.Fatalf("retry after rollback = %v", err)
	}
	if _, err := f.svc.Join(ctx, a.PlayerID, a.ID, runID, "join-a-other"); !errors.Is(err, runhost.ErrBusy) {
		t.Fatalf("duplicate join with new key = %v, want ErrBusy", err)
	}
	if got := f.tired(t, a.ID); got != a.Tired+20 {
		t.Fatalf("tired after retry and duplicate = %d, want %d", got, a.Tired+20)
	}

	// When both the commit acknowledgement and receipt lookup fail, ErrRecovery is returned
	// without leaving an extra charge; a subsequent replay returns the committed receipt.
	f.lose.Store(true)
	f.failReceipt.Store(true)
	if _, err := f.svc.Join(ctx, b.PlayerID, b.ID, runID, "join-b"); !errors.Is(err, runhost.ErrRecovery) {
		t.Fatalf("unreadable join outcome = %v, want ErrRecovery", err)
	}
	replay, err := f.svc.Join(ctx, b.PlayerID, b.ID, runID, "join-b")
	if err != nil || replay.Command.ActorID != b.ID || replay.Version == receipt.Version {
		t.Fatalf("replay after ErrRecovery = %+v, %v", replay, err)
	}
	if got := f.tired(t, b.ID); got != b.Tired+20 {
		t.Fatalf("tired after ErrRecovery replay = %d, want %d", got, b.Tired+20)
	}
}
