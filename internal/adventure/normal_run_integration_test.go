package adventure_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/adventure"
	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/job"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/economy"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/runhost"
	"github.com/witchcraze/party2re/internal/testutil"
)

type normalClock struct{ nanos atomic.Int64 }

func (c *normalClock) Now() time.Time      { return time.Unix(0, c.nanos.Load()).UTC() }
func (c *normalClock) add(d time.Duration) { c.nanos.Add(int64(d)) }

type normalRNG struct{ calls atomic.Int64 }

func (r *normalRNG) Intn(int) int     { r.calls.Add(1); return 0 }
func (r *normalRNG) IntN(n int) int   { return r.Intn(n) }
func (r *normalRNG) Float64() float64 { r.calls.Add(1); return 0 }

type normalPenalty struct {
	fail     atomic.Bool
	calls    atomic.Int64
	deadline atomic.Int64
}

func (p *normalPenalty) ProjectRunPenalty(_ context.Context, _ string, until time.Time) error {
	p.calls.Add(1)
	if p.fail.Load() {
		return errors.New("timer projection unavailable")
	}
	p.deadline.Store(until.UnixNano())
	return nil
}

type normalHistory struct {
	adventure.Repository
	fail atomic.Bool
}

func (r *normalHistory) Save(ctx context.Context, v adventure.Adventure) error {
	if err := r.Repository.Save(ctx, v); err != nil {
		return err
	}
	if r.fail.Load() {
		return errors.New("history acknowledgement failed before commit")
	}
	return nil
}

type normalStore struct {
	runhost.Store[adventure.NormalRun]
	failWrite      atomic.Bool
	loseCommit     atomic.Bool
	tamperRetained atomic.Bool
	owned          map[string]bool // when set, ListRuns returns only these runs
}

func (r *normalStore) FindRetainedEarnings(ctx context.Context, actor string) ([]runhost.RetainedEarnings, error) {
	found, err := r.Store.FindRetainedEarnings(ctx, actor)
	if err == nil && r.tamperRetained.Load() && len(found) > 0 {
		found[0].Recipient.Pending.Gold++
	}
	return found, err
}

func (r *normalStore) SaveRunReceipt(ctx context.Context, receipt runhost.Receipt) error {
	if err := r.Store.SaveRunReceipt(ctx, receipt); err != nil {
		return err
	}
	if r.failWrite.Load() {
		return errors.New("receipt write failure")
	}
	return nil
}
func (r *normalStore) WithRunForUpdate(ctx context.Context, id string, create bool, fn func(context.Context) error) error {
	outer := database.TxFromContext(ctx) == nil
	err := r.Store.WithRunForUpdate(ctx, id, create, fn)
	if err == nil && outer && r.loseCommit.CompareAndSwap(true, false) {
		return errors.New("SQL commit acknowledgement lost")
	}
	return err
}

type normalFixture struct {
	db      *sql.DB
	customs *database.CustomSkillRepository
	chars   *database.CharacterRepository
	invs    *database.InventoryRepository
	equips  *database.EquipmentRepository
	jobs    *database.CharacterJobRepository
	depots  *database.DepotRepository
	history *normalHistory
	store   *normalStore
	clock   *normalClock
	penalty *normalPenalty
	rng     *normalRNG
	cfg     adventure.NormalRunConfig
	svc     *adventure.NormalRunService
	actors  []corecharacter.Character
}

func newNormalFixture(t *testing.T, count int) *normalFixture {
	t.Helper()
	db, chars, invs, history, _ := earnedRewardStorage(t)
	runRepo, err := database.NewRunRepository[adventure.NormalRun](db)
	if err != nil {
		t.Fatal(err)
	}
	jobs, err := database.NewCharacterJobRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	equipment, err := database.NewEquipmentRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	depots, err := database.NewDepotRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	runner, err := economy.NewService(chars, invs, economy.WithTransactionProvider(database.NewTransactionProvider(db)))
	if err != nil {
		t.Fatal(err)
	}
	customs, err := database.NewCustomSkillRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	adapter := battle.NewService(battle.WithCharacterRepository(chars), battle.WithCustomSkillRepository(customs), battle.WithInventoryRepository(invs), battle.WithDepotRepository(depots), battle.WithEquipmentRepository(equipment), battle.WithJobHistoryRepository(jobs), battle.WithTransactionProvider(database.NewTransactionProvider(db)), battle.WithTransactionRunner(runner))
	stages, err := adventure.InitialStageCatalog()
	if err != nil {
		t.Fatal(err)
	}
	monsters, err := adventure.InitialMonsterCatalog()
	if err != nil {
		t.Fatal(err)
	}
	f := &normalFixture{db: db, customs: customs, chars: chars, invs: invs, equips: equipment, jobs: jobs, depots: depots, history: &normalHistory{Repository: history}, store: &normalStore{Store: runRepo}, clock: &normalClock{}, penalty: &normalPenalty{}, rng: &normalRNG{}}
	f.clock.nanos.Store(time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC).UnixNano())
	for range count {
		c, err := database.CreateTestCharacterWithFunds(context.Background(), db, id.New()[:8], 100)
		if err != nil {
			t.Fatal(err)
		}
		f.actors = append(f.actors, c)
	}
	jobCatalog, err := job.InitialCatalog()
	if err != nil {
		t.Fatal(err)
	}
	f.cfg = adventure.NormalRunConfig{Jobs: jobCatalog, Store: f.store, Characters: chars, Inventories: invs, Builder: adapter, Settler: adapter, History: f.history, Stages: stages, Monsters: monsters, Penalty: f.penalty, Clock: f.clock, RNG: f.rng, ExitEffects: func(context.Context, string) error { return nil }}
	f.restart(t)
	return f
}

func (f *normalFixture) restart(t *testing.T) {
	t.Helper()
	var err error
	f.svc, err = adventure.NewNormalRunService(f.cfg)
	if err != nil {
		t.Fatal(err)
	}
}

func (f *normalFixture) enter(t *testing.T, mode string) runhost.Receipt {
	t.Helper()
	c := f.actors[0]
	r, err := f.svc.EnterSolo(context.Background(), c.PlayerID, c.ID, "start", "solo-quest", adventure.StarterAdventure, mode, 3)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func (f *normalFixture) send(t *testing.T, runID string, actor int, action, target string) runhost.Receipt {
	t.Helper()
	f.clock.add(3 * time.Second)
	r, err := f.store.FindRun(context.Background(), runID)
	if err != nil || r == nil {
		t.Fatal(r, err)
	}
	c := f.actors[actor]
	result, err := f.svc.Command(context.Background(), runhost.Command{PlayerID: c.PlayerID, ActorID: c.ID, RunID: runID, ID: id.New(), Version: r.Version, Action: action, TargetID: target})
	if err != nil {
		t.Fatal(action, err)
	}
	return result
}

// The quest list reads live runs of this kind only, from the shared runs table.
func TestNormalRunListQuestRooms(t *testing.T) {
	f := newNormalFixture(t, 1)
	entry := f.enter(t, "normal")
	c := f.actors[0]
	rooms, err := f.svc.ListQuestRooms(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := runhost.QuestRoom{Name: "solo-quest", Speed: 3, StageID: adventure.StarterAdventure, Mode: "normal", LeaderName: c.Name, MemberNames: []string{c.Name}, MaxMembers: 1}
	found := 0
	for _, room := range rooms {
		if reflect.DeepEqual(room.MemberNames, []string{c.Name}) {
			found++
			if !reflect.DeepEqual(room, want) {
				t.Fatalf("room %+v, want %+v", room, want)
			}
		}
	}
	if found != 1 {
		t.Fatalf("own run listed %d times", found)
	}
	other, err := f.store.ListRuns(context.Background(), "other-kind")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range other {
		if r.ID == entry.Command.RunID {
			t.Fatal("run listed under another kind")
		}
	}
}

func TestNormalRunAdmissionReceiptRestart(t *testing.T) {
	f := newNormalFixture(t, 1)
	entry := f.enter(t, "normal")
	r, err := f.store.FindRun(context.Background(), entry.Command.RunID)
	if err != nil || r.Round != 0 || len(r.Enemies) != 0 || r.Admission.Name != "solo-quest" {
		t.Fatal(r, err)
	}
	f.restart(t)
	if again := f.enter(t, "normal"); !reflect.DeepEqual(entry, again) {
		t.Fatal("admission retry changed receipt")
	}
	c := f.actors[0]
	if _, err := f.svc.EnterSolo(context.Background(), c.PlayerID, c.ID, "new-key", "solo-quest", adventure.StarterAdventure, "normal", 3); !errors.Is(err, runhost.ErrBusy) {
		t.Fatal(err)
	}
	if _, err := f.svc.GetActorRun(context.Background(), "another-player", c.ID); !errors.Is(err, runhost.ErrForbidden) {
		t.Fatal(err)
	}
	before := *r
	for range 3 {
		if _, err := f.svc.GetActorRun(context.Background(), c.PlayerID, c.ID); err != nil {
			t.Fatal(err)
		}
		if active, err := f.svc.HasActiveRun(context.Background(), c.ID); err != nil || !active {
			t.Fatalf("public admission: %v %v", active, err)
		}
	}
	r, err = f.store.FindRun(context.Background(), r.ID)
	if err != nil || !reflect.DeepEqual(before, *r) {
		t.Fatal("GET mutated state", err)
	}
	cmd := f.earnCommand(t, r.ID, 0)
	cmd.ID = "lost-response"
	f.store.loseCommit.Store(true)
	result, err := f.svc.Command(context.Background(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	again, err := f.svc.Command(context.Background(), cmd)
	if err != nil || !reflect.DeepEqual(result, again) {
		t.Fatal(again, err)
	}
	r, err = f.store.FindRun(context.Background(), r.ID)
	if err != nil || r.Version != result.Version || len(r.Enemies) != 0 {
		t.Fatal("replay re-executed the operation", r, err)
	}
	if paid, err := f.chars.FindByID(context.Background(), c.ID); err != nil || paid.Money != c.Money+5 {
		t.Fatal("replay paid twice", paid.Money, err)
	}
	cmd.Action = "defend"
	if _, err := f.svc.Command(context.Background(), cmd); !errors.Is(err, runhost.ErrConflict) {
		t.Fatal(err)
	}
	for _, key := range []string{"Key", "key", "key "} {
		cmd := f.earnCommand(t, cmd.RunID, 0)
		cmd.ID = key
		if _, err := f.svc.Command(context.Background(), cmd); err != nil {
			t.Fatal("distinct command identity collided", err)
		}
	}
}

func TestNormalRunRollbackDefeatExitAndChronicle(t *testing.T) {
	f := newNormalFixture(t, 1)
	entry := f.enter(t, "normal")
	runID := entry.Command.RunID
	c := f.actors[0]
	f.earn(t, runID, 0)
	f.stage(t, runID, fall(c.ID))
	before, err := f.chars.FindByID(context.Background(), c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if before.Money != c.Money+5 || before.Experience != c.Experience+3 {
		t.Fatal("ordinary earnings not applied at action", before)
	}
	f.history.fail.Store(true)
	r, err := f.store.FindRun(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	f.clock.add(3 * time.Second)
	cmd := runhost.Command{PlayerID: c.PlayerID, ActorID: c.ID, RunID: runID, ID: "exit", Version: r.Version, Action: "exit"}
	if _, err := f.svc.Command(context.Background(), cmd); err == nil {
		t.Fatal("history failure accepted")
	}
	after, err := f.chars.FindByID(context.Background(), c.ID)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("history failure leaked assets", err)
	}
	if got, err := f.svc.GetRunReceipt(context.Background(), c.PlayerID, c.ID, runID, cmd.ID); err != nil || got != nil {
		t.Fatal(got, err)
	}
	f.history.fail.Store(false)
	result, err := f.svc.Command(context.Background(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	if result.Disposition != "exit" {
		t.Fatal(result)
	}
	if active, err := f.svc.GetActorRun(context.Background(), c.PlayerID, c.ID); err != nil || active != nil {
		t.Fatal(active, err)
	}
	if state, err := f.store.FindRun(context.Background(), runID); err != nil || state != nil {
		t.Fatal("last exit retained active state", state, err)
	}
	if _, err := f.svc.Command(context.Background(), cmd); err != nil {
		t.Fatal("receipt lost after disposal", err)
	}
	stats, err := f.history.GetAggregatedStats(context.Background(), c.ID)
	if err != nil || stats.TotalAdventures != 1 || stats.TotalDefeats != 1 || stats.TotalGoldEarned != 5 {
		t.Fatal(stats, err)
	}
	after, err = f.chars.FindByID(context.Background(), c.ID)
	if err != nil || after.Money != c.Money+5 || after.Tired != before.Tired+1 || after.Stats.HP != 0 {
		t.Fatal(after, err)
	}
}

func TestNormalRunTryExitAndTravelTreasure(t *testing.T) {
	for _, success := range []bool{false, true} {
		t.Run(fmt.Sprint(success), func(t *testing.T) {
			f := newNormalFixture(t, 1)
			entry := f.enter(t, "try")
			runID := entry.Command.RunID
			f.earn(t, runID, 0)
			f.earn(t, runID, 0)
			if success {
				f.stage(t, runID, travel10)
				f.send(t, runID, 0, "advance", "")
				r, err := f.store.FindRun(context.Background(), runID)
				if err != nil || r.Mode != "normal" || r.StageCleared || r.Recipients[f.actors[0].ID].Pending.Gold != 15 {
					t.Fatal(r, err)
				}
				f.restart(t)
				f.stage(t, runID, fall(f.actors[0].ID))
			}
			f.send(t, runID, 0, "exit", "")
			c, err := f.chars.FindByID(context.Background(), f.actors[0].ID)
			want := 0
			if success {
				want = 15
			}
			if err != nil || c.Money != f.actors[0].Money+want {
				t.Fatal(c, err)
			}
		})
	}
}

func TestNormalRunChestReceiptAndSQLRollback(t *testing.T) {
	f := newNormalFixture(t, 1)
	entry := f.enter(t, "normal")
	runID := entry.Command.RunID
	f.stage(t, runID, withActor(f.actors[0].ID, func(a *corebattle.LifetimeActor) { a.Jobs.JobID = "job-07" }))
	f.stage(t, runID, travel10)
	f.send(t, runID, 0, "advance", "")
	r, err := f.store.FindRun(context.Background(), runID)
	if err != nil || len(r.Boxes) < 1 {
		t.Fatal(r, err)
	}
	c := f.actors[0]
	cmd := runhost.Command{PlayerID: c.PlayerID, ActorID: c.ID, RunID: runID, ID: "chest", Version: r.Version, Action: "chest", TargetID: r.Boxes[0].ID}
	f.clock.add(3 * time.Second)
	f.store.failWrite.Store(true)
	if _, err := f.svc.Command(context.Background(), cmd); err == nil {
		t.Fatal("receipt failure accepted")
	}
	inv, err := f.invs.FindByCharacterID(context.Background(), c.ID)
	if err != nil || len(inv.Items) != 0 {
		t.Fatal("failed chest leaked an item", inv, err)
	}
	if dep, err := f.depots.FindByCharacterID(context.Background(), c.ID); err != nil || len(dep.Items) != 0 {
		t.Fatal("failed chest leaked a depot item", dep, err)
	}
	f.store.failWrite.Store(false)
	result, err := f.svc.Command(context.Background(), cmd)
	// The fixed RNG rolls a weapon, which bypasses the empty hand and goes to the Depot.
	if err != nil || result.Box == nil || result.Box.DeliveredTo != "depot" {
		t.Fatal(result, err)
	}
	stats := testutil.RunConcurrentStressTest(t, testutil.GetStressConfig(), func(int, int) error { _, err := f.svc.Command(context.Background(), cmd); return err })
	if stats.Failures != 0 {
		t.Fatal(stats)
	}
	inv, err = f.invs.FindByCharacterID(context.Background(), c.ID)
	if err != nil || len(inv.Items) != 0 {
		t.Fatal("weapon chest entered the hand", inv, err)
	}
	if dep, err := f.depots.FindByCharacterID(context.Background(), c.ID); err != nil || len(dep.Items) != 1 || dep.Items[0].Quantity != 1 {
		t.Fatal("duplicate chest delivery", dep, err)
	}
	// A real concrete key instance is debited only on an additional inspection.
	key, err := coreitem.NewInstance(runhost.ItemTreasurePocket, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := inv.Add(key); err != nil {
		t.Fatal(err)
	}
	if err := f.invs.Save(context.Background(), inv); err != nil {
		t.Fatal(err)
	}
	r, err = f.store.FindRun(context.Background(), runID)
	if err != nil || len(r.Boxes) < 2 {
		t.Fatal(r, err)
	}
	f.clock.add(3 * time.Second)
	cmd.ID, cmd.Version, cmd.TargetID = "second", r.Version, r.Boxes[1].ID
	f.store.failWrite.Store(true)
	if _, err := f.svc.Command(context.Background(), cmd); err == nil {
		t.Fatal("additional chest failure accepted")
	}
	inv, err = f.invs.FindByCharacterID(context.Background(), c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if k, ok := inv.Find(key.ID); !ok || k.Quantity != 1 {
		t.Fatal("rollback lost a key", inv)
	}
	f.store.failWrite.Store(false)
	if _, err := f.svc.Command(context.Background(), cmd); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Command(context.Background(), cmd); err != nil {
		t.Fatal(err)
	}
	inv, err = f.invs.FindByCharacterID(context.Background(), c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := inv.Find(key.ID); ok {
		t.Fatal("additional chest did not consume the key", inv)
	}
}
