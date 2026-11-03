package adventure_test

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

	"github.com/witchcraze/party2re/internal/adventure"
	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/economy"
	"github.com/witchcraze/party2re/internal/id"
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

// The explicit combat consumer remains #1329. This test port supplies bounded
// effects, so these tests verify lifecycle persistence rather than skill parity.
type normalCombat struct{ calls atomic.Int64 }

func (*normalCombat) PrepareAdvance(context.Context, *adventure.NormalRun, string) error { return nil }
func (e *normalCombat) ExecuteRunAction(_ context.Context, r *adventure.NormalRun, cmd adventure.RunCommand) (adventure.RunActionEffects, error) {
	e.calls.Add(1)
	fx := adventure.RunActionEffects{RoomActivity: true, ActorActivity: true, ReleaseStock: true}
	actor := slices.IndexFunc(r.Battle.Actors, func(a corebattle.LifetimeActor) bool { return a.ID == cmd.ActorID })
	if actor < 0 {
		return fx, adventure.ErrNotParticipant
	}
	switch cmd.Action {
	case "earn", "kill":
		fx.Earnings = map[string]adventure.RunEarnings{}
		for _, id := range r.Roster {
			fx.Earnings[id] = adventure.RunEarnings{EXP: 3, Gold: 5, Crystals: 1}
		}
		if cmd.Action == "kill" {
			for i := range r.Battle.Actors {
				if slices.Contains(r.Enemies, r.Battle.Actors[i].ID) {
					r.Battle.Actors[i].HP = 0
				}
			}
			fx.EncounterDefeated = true
		}
	case "fall":
		r.Battle.Actors[actor].HP = 0
	case "travel10":
		r.Floor = 10
		for i := range r.Battle.Actors {
			if slices.Contains(r.Enemies, r.Battle.Actors[i].ID) {
				r.Battle.Actors[i].HP = 0
			}
		}
	case "buff":
		r.Battle.Actors[actor].Extended = corebattle.ExtendedIncense
		r.Battle.Actors[actor].ExtendedTurns = 4
		r.Battle.Actors[actor].MP = 7
		r.Battle.Fields.AntiTurns = 2
	case "merchant":
		r.Battle.Actors[actor].Jobs.JobID = "job-07"
	case "comment", "unknown_text", "ignored_skill":
		fx.ActorActivity = false
		fx.ReleaseStock = false
	case "whisper":
		fx.ActorActivity = false
		fx.ReleaseStock = false
	case "screenshot", "invite":
		fx.ActorActivity, fx.RoomActivity = false, false
		fx.ReleaseStock = false
	case "no_effect":
	case "reject":
		return fx, adventure.ErrRunCooldown
	default:
		return fx, adventure.ErrRunUnavailable
	}
	return fx, nil
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
	adventure.NormalRunStore
	failWrite  atomic.Bool
	loseCommit atomic.Bool
}

func (r *normalStore) SaveRunReceipt(ctx context.Context, receipt adventure.RunReceipt) error {
	if err := r.NormalRunStore.SaveRunReceipt(ctx, receipt); err != nil {
		return err
	}
	if r.failWrite.Load() {
		return errors.New("receipt write failure")
	}
	return nil
}
func (r *normalStore) WithRunForUpdate(ctx context.Context, id string, create bool, fn func(context.Context) error) error {
	outer := database.TxFromContext(ctx) == nil
	err := r.NormalRunStore.WithRunForUpdate(ctx, id, create, fn)
	if err == nil && outer && r.loseCommit.CompareAndSwap(true, false) {
		return errors.New("SQL commit acknowledgement lost")
	}
	return err
}

type normalFixture struct {
	db      *sql.DB
	chars   *database.CharacterRepository
	invs    *database.InventoryRepository
	history *normalHistory
	store   *normalStore
	clock   *normalClock
	penalty *normalPenalty
	combat  *normalCombat
	rng     *normalRNG
	cfg     adventure.NormalRunConfig
	svc     *adventure.NormalRunService
	actors  []corecharacter.Character
}

func newNormalFixture(t *testing.T, count int) *normalFixture {
	t.Helper()
	db, chars, invs, history, _ := earnedRewardStorage(t)
	runRepo, err := database.NewNormalAdventureRepository(db)
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
	runner, err := economy.NewService(chars, invs, economy.WithTransactionProvider(database.NewTransactionProvider(db)))
	if err != nil {
		t.Fatal(err)
	}
	adapter := battle.NewService(battle.WithCharacterRepository(chars), battle.WithInventoryRepository(invs), battle.WithEquipmentRepository(equipment), battle.WithJobHistoryRepository(jobs), battle.WithTransactionProvider(database.NewTransactionProvider(db)), battle.WithTransactionRunner(runner), battle.WithMaxInventoryCapacity(30))
	stages, err := adventure.InitialStageCatalog()
	if err != nil {
		t.Fatal(err)
	}
	monsters, err := adventure.InitialMonsterCatalog()
	if err != nil {
		t.Fatal(err)
	}
	f := &normalFixture{db: db, chars: chars, invs: invs, history: &normalHistory{Repository: history}, store: &normalStore{NormalRunStore: runRepo}, clock: &normalClock{}, penalty: &normalPenalty{}, combat: &normalCombat{}, rng: &normalRNG{}}
	f.clock.nanos.Store(time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC).UnixNano())
	for range count {
		c, err := database.CreateTestCharacterWithFunds(context.Background(), db, id.New()[:8], 100)
		if err != nil {
			t.Fatal(err)
		}
		f.actors = append(f.actors, c)
	}
	f.cfg = adventure.NormalRunConfig{Store: f.store, Characters: chars, Inventories: invs, Builder: adapter, Settler: adapter, History: f.history, Stages: stages, Monsters: monsters, Combat: f.combat, Penalty: f.penalty, Clock: f.clock, RNG: f.rng, ExitEffects: func(context.Context, string) error { return nil }}
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

func (f *normalFixture) enter(t *testing.T, mode string) adventure.RunReceipt {
	t.Helper()
	c := f.actors[0]
	r, err := f.svc.EnterSolo(context.Background(), c.PlayerID, c.ID, "start", adventure.StarterAdventure, mode, 3)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func (f *normalFixture) send(t *testing.T, runID string, actor int, action, target string) adventure.RunReceipt {
	t.Helper()
	f.clock.add(3 * time.Second)
	r, err := f.store.FindRun(context.Background(), runID)
	if err != nil || r == nil {
		t.Fatal(r, err)
	}
	c := f.actors[actor]
	result, err := f.svc.Command(context.Background(), adventure.RunCommand{PlayerID: c.PlayerID, ActorID: c.ID, RunID: runID, ID: id.New(), Version: r.Version, Action: action, TargetID: target})
	if err != nil {
		t.Fatal(action, err)
	}
	return result
}

func TestNormalRunAdmissionReceiptRestart(t *testing.T) {
	f := newNormalFixture(t, 1)
	entry := f.enter(t, "normal")
	r, err := f.store.FindRun(context.Background(), entry.Command.RunID)
	if err != nil || r.Floor != 0 || len(r.Enemies) != 0 || f.combat.calls.Load() != 0 {
		t.Fatal(r, err)
	}
	f.restart(t)
	if again := f.enter(t, "normal"); !reflect.DeepEqual(entry, again) {
		t.Fatal("admission retry changed receipt")
	}
	c := f.actors[0]
	if _, err := f.svc.EnterSolo(context.Background(), c.PlayerID, c.ID, "new-key", adventure.StarterAdventure, "normal", 3); !errors.Is(err, adventure.ErrRunBusy) {
		t.Fatal(err)
	}
	if _, err := f.svc.GetActorRun(context.Background(), "another-player", c.ID); !errors.Is(err, adventure.ErrRunForbidden) {
		t.Fatal(err)
	}
	before := *r
	for range 3 {
		if _, err := f.svc.GetActorRun(context.Background(), c.PlayerID, c.ID); err != nil {
			t.Fatal(err)
		}
	}
	r, err = f.store.FindRun(context.Background(), r.ID)
	if err != nil || !reflect.DeepEqual(before, *r) {
		t.Fatal("GET mutated state", err)
	}
	cmd := adventure.RunCommand{PlayerID: c.PlayerID, ActorID: c.ID, RunID: r.ID, ID: "lost-response", Version: r.Version, Action: "buff"}
	f.store.loseCommit.Store(true)
	result, err := f.svc.Command(context.Background(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	again, err := f.svc.Command(context.Background(), cmd)
	if err != nil || !reflect.DeepEqual(result, again) || f.combat.calls.Load() != 1 {
		t.Fatal(again, err)
	}
	r, err = f.store.FindRun(context.Background(), r.ID)
	if err != nil || r.Battle.Actors[0].MP != 7 || r.Battle.Actors[0].ExtendedTurns != 4 || r.Battle.Fields.AntiTurns != 2 {
		t.Fatal(r, err)
	}
	cmd.Action = "earn"
	if _, err := f.svc.Command(context.Background(), cmd); !errors.Is(err, adventure.ErrRunConflict) {
		t.Fatal(err)
	}
	for _, key := range []string{"Key", "key", "key "} {
		r, err := f.store.FindRun(context.Background(), cmd.RunID)
		if err != nil {
			t.Fatal(err)
		}
		cmd.ID, cmd.Version, cmd.Action = key, r.Version, "comment"
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
	f.send(t, runID, 0, "earn", "")
	f.send(t, runID, 0, "fall", "")
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
	cmd := adventure.RunCommand{PlayerID: c.PlayerID, ActorID: c.ID, RunID: runID, ID: "exit", Version: r.Version, Action: "exit"}
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
	if err != nil || after.Money != c.Money+5 || after.Tired != before.Tired+1 || after.Stats.HP != 1 {
		t.Fatal(after, err)
	}
}

func TestNormalRunTryExitAndTravelTreasure(t *testing.T) {
	for _, success := range []bool{false, true} {
		t.Run(fmt.Sprint(success), func(t *testing.T) {
			f := newNormalFixture(t, 1)
			entry := f.enter(t, "try")
			runID := entry.Command.RunID
			f.send(t, runID, 0, "earn", "")
			f.send(t, runID, 0, "earn", "")
			if success {
				f.send(t, runID, 0, "travel10", "")
				f.send(t, runID, 0, "advance", "")
				r, err := f.store.FindRun(context.Background(), runID)
				if err != nil || r.Mode != "normal" || r.StageCleared || r.Recipients[f.actors[0].ID].Pending.Gold != 15 {
					t.Fatal(r, err)
				}
				f.restart(t)
				f.send(t, runID, 0, "fall", "")
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
	f.send(t, runID, 0, "merchant", "")
	f.send(t, runID, 0, "travel10", "")
	f.send(t, runID, 0, "advance", "")
	r, err := f.store.FindRun(context.Background(), runID)
	if err != nil || len(r.Boxes) < 1 {
		t.Fatal(r, err)
	}
	c := f.actors[0]
	cmd := adventure.RunCommand{PlayerID: c.PlayerID, ActorID: c.ID, RunID: runID, ID: "chest", Version: r.Version, Action: "chest", TargetID: r.Boxes[0].ID}
	f.clock.add(3 * time.Second)
	f.store.failWrite.Store(true)
	if _, err := f.svc.Command(context.Background(), cmd); err == nil {
		t.Fatal("receipt failure accepted")
	}
	inv, err := f.invs.FindByCharacterID(context.Background(), c.ID)
	if err != nil || len(inv.Items) != 0 {
		t.Fatal("failed chest leaked an item", inv, err)
	}
	f.store.failWrite.Store(false)
	result, err := f.svc.Command(context.Background(), cmd)
	if err != nil || result.Box == nil || result.Box.DeliveredTo != "inventory" {
		t.Fatal(result, err)
	}
	stats := testutil.RunConcurrentStressTest(t, testutil.GetStressConfig(), func(int, int) error { _, err := f.svc.Command(context.Background(), cmd); return err })
	if stats.Failures != 0 {
		t.Fatal(stats)
	}
	inv, err = f.invs.FindByCharacterID(context.Background(), c.ID)
	if err != nil || len(inv.Items) != 1 || inv.Items[0].Quantity != 1 {
		t.Fatal("duplicate chest delivery", inv, err)
	}
	// A real concrete key instance is debited only on an additional inspection.
	key, err := coreitem.NewInstance(adventure.ItemTreasurePocket, 2)
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
	if k, ok := inv.Find(key.ID); !ok || k.Quantity != 2 {
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
	if k, ok := inv.Find(key.ID); !ok || k.Quantity != 1 {
		t.Fatal("additional chest consumed key twice", inv)
	}
}
