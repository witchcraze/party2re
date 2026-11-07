package home_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"sync/atomic"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	corejob "github.com/witchcraze/party2re/internal/core/job"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/economy"
	"github.com/witchcraze/party2re/internal/home"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/inventory"
	"github.com/witchcraze/party2re/internal/job"
	"github.com/witchcraze/party2re/internal/testutil"
)

type futureMemoryFixture struct {
	db        *sql.DB
	char      corecharacter.Character
	svc       *home.Service
	jobSvc    *job.Service
	charRepo  *database.CharacterRepository
	invRepo   *database.InventoryRepository
	depotRepo *database.DepotRepository
	jobRepo   *database.CharacterJobRepository
	future    *database.FutureMemoryRepository
	ids       map[string]string
}

// newFutureMemoryFixture creates a character whose current job has an SP record
// while the previous job has none, and owns item-207 in both Home sources.
func newFutureMemoryFixture(t *testing.T, name string, overFuture int, inventoryIDs, depotIDs []string) futureMemoryFixture {
	t.Helper()
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()

	char, err := database.CreateTestCharacter(ctx, db, name)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, q := range []string{
			"DELETE FROM character_future_memories WHERE character_id = ?",
			"DELETE FROM inventory_items WHERE character_id = ?",
			"DELETE FROM depot_items WHERE character_id = ?",
			"DELETE FROM character_job_points WHERE character_id = ?",
			"DELETE FROM character_jobs WHERE character_id = ?",
			"DELETE FROM characters WHERE id = ?",
		} {
			_, _ = db.ExecContext(context.Background(), q, char.ID)
		}
		_, _ = db.ExecContext(context.Background(), "DELETE FROM players WHERE id = ?", char.PlayerID)
	})

	f := futureMemoryFixture{db: db, ids: map[string]string{}}
	for _, label := range append(append([]string{}, inventoryIDs...), depotIDs...) {
		f.ids[label] = id.New()
	}
	if f.charRepo, err = database.NewCharacterRepository(db); err != nil {
		t.Fatal(err)
	}
	if f.invRepo, err = database.NewInventoryRepository(db); err != nil {
		t.Fatal(err)
	}
	if f.depotRepo, err = database.NewDepotRepository(db); err != nil {
		t.Fatal(err)
	}
	if f.jobRepo, err = database.NewCharacterJobRepository(db); err != nil {
		t.Fatal(err)
	}
	if f.future, err = database.NewFutureMemoryRepository(db); err != nil {
		t.Fatal(err)
	}

	char.OverFuture = overFuture
	char.JobID, char.SP = "job-05", 77
	char.OldJobID, char.OldSP = "job-01", 33
	char.Level, char.Experience = 42, 1234
	if err := f.charRepo.Update(ctx, char); err != nil {
		t.Fatal(err)
	}
	f.char = char

	state, err := corejob.NewCharacterJob(char.ID, char.JobID)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.RecordJobSP(char.JobID, 5); err != nil {
		t.Fatal(err)
	}
	if err := f.jobRepo.Save(ctx, state); err != nil {
		t.Fatal(err)
	}

	inv, err := coreinventory.New(char.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, label := range inventoryIDs {
		if err := inv.Add(coreitem.Instance{ID: f.ids[label], DefinitionID: "item-207", Quantity: 1}); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.invRepo.Save(ctx, inv); err != nil {
		t.Fatal(err)
	}

	dp, err := depot.NewDepot(char.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, label := range depotIDs {
		dp.Items = append(dp.Items, coreitem.Instance{ID: f.ids[label], DefinitionID: "item-207", Quantity: 1})
	}
	// Fill the Depot to capacity: a full Depot must not block the save.
	for i := len(dp.Items); i < dp.Capacity; i++ {
		dp.Items = append(dp.Items, coreitem.Instance{ID: id.New(), DefinitionID: "item-018", Quantity: 1})
	}
	if err := f.depotRepo.Save(ctx, dp); err != nil {
		t.Fatal(err)
	}

	eco, err := economy.NewService(f.charRepo, f.invRepo, economy.WithTransactionProvider(database.NewTransactionProvider(db)))
	if err != nil {
		t.Fatal(err)
	}
	if f.jobSvc, err = job.NewService(f.jobRepo,
		job.WithCharacterRepository(f.charRepo),
		job.WithEconomy(eco),
		job.WithFutureMemoryRepository(f.future),
	); err != nil {
		t.Fatal(err)
	}
	invService, err := inventory.NewService(f.invRepo)
	if err != nil {
		t.Fatal(err)
	}
	homeRepo, err := database.NewHomeRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	cat, err := coreitem.InitialCatalog()
	if err != nil {
		t.Fatal(err)
	}
	if f.svc, err = home.NewService(homeRepo, f.charRepo,
		home.WithEconomy(eco),
		home.WithInventoryManager(invService),
		home.WithDepotManager(&testDepotManagerAdapter{repo: f.depotRepo}),
		home.WithItemCatalog(cat),
	); err != nil {
		t.Fatal(err)
	}
	f.svc.SetFutureMemorySaver(f.jobSvc)
	return f
}

// id returns the stored instance ID for a fixture item label.
func (f futureMemoryFixture) id(label string) string { return f.ids[label] }

func (f futureMemoryFixture) memories(t *testing.T) []corecharacter.FutureMemory {
	t.Helper()
	m, err := f.future.FindByCharacterID(context.Background(), f.char.ID)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func (f futureMemoryFixture) owns(t *testing.T, source, instanceID string) bool {
	t.Helper()
	ctx := context.Background()
	if source == "inventory" {
		inv, err := f.invRepo.FindByCharacterID(ctx, f.char.ID)
		if err != nil {
			t.Fatal(err)
		}
		_, ok := inv.Find(instanceID)
		return ok
	}
	dp, err := f.depotRepo.FindByCharacterID(ctx, f.char.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range dp.Items {
		if it.ID == instanceID {
			return true
		}
	}
	return false
}

func (f futureMemoryFixture) jobState(t *testing.T) corejob.CharacterJob {
	t.Helper()
	state, err := f.jobRepo.FindByCharacterID(context.Background(), f.char.ID)
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func TestHomeFutureMemory_Integration_BothSourcesAndOverFutureBoundary(t *testing.T) {
	for _, tc := range []struct {
		name       string
		overFuture int
	}{{"OverFuture0", 0}, {"OverFuture1", 1}} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFutureMemoryFixture(t, "FutMem"+tc.name[len(tc.name)-1:], tc.overFuture,
				[]string{"fm-inv-1", "fm-inv-2"}, []string{"fm-dep-1"})
			ctx := context.Background()

			res, err := f.svc.UseHomeItem(ctx, f.char.ID, f.id("fm-dep-1"), "depot")
			if err != nil {
				t.Fatalf("depot save: %v", err)
			}
			if !res.Consumed || res.Message != "未来のカケラに記憶が写った！" {
				t.Fatalf("unexpected result %+v", res)
			}
			if f.owns(t, "depot", f.id("fm-dep-1")) {
				t.Fatal("depot item-207 was not consumed")
			}
			mems := f.memories(t)
			if len(mems) != 1 {
				t.Fatalf("want 1 snapshot, got %d", len(mems))
			}
			m := mems[0]
			c := f.char
			if m.JobID != c.JobID || m.OldJobID != c.OldJobID || m.Level != c.Level || m.Experience != c.Experience ||
				m.MaxHP != c.Stats.MaxHP || m.MaxMP != c.Stats.MaxMP || m.Attack != c.Stats.Attack ||
				m.Defense != c.Stats.Defense || m.Agility != c.Stats.Agility || m.Gender != c.Gender || m.OverLevel != c.OverLevel {
				t.Fatalf("snapshot %+v does not match character %+v", m, c)
			}
			state := f.jobState(t)
			if sp, _ := state.SavedSP(c.JobID); sp != c.SP {
				t.Fatalf("current job SP record = %d, want %d", sp, c.SP)
			}
			if state.HasRecordedJob(c.OldJobID) {
				t.Fatal("missing previous-job record must not be created")
			}

			// OverFuture=1 allows a second snapshot; after the last slot both are refused.
			uses := []string{"fm-inv-1"}
			if tc.overFuture == 1 {
				if _, err := f.svc.UseHomeItem(ctx, f.char.ID, f.id("fm-inv-1"), "inventory"); err != nil {
					t.Fatalf("inventory save within OverFuture: %v", err)
				}
				uses = []string{"fm-inv-2"}
			}
			for _, label := range uses {
				id := f.id(label)
				_, err := f.svc.UseHomeItem(ctx, f.char.ID, id, "inventory")
				if !errors.Is(err, home.ErrCannotUseHere) {
					t.Fatalf("want slot rejection, got %v", err)
				}
				if !f.owns(t, "inventory", id) {
					t.Fatal("rejected item-207 must remain")
				}
			}
			if got := len(f.memories(t)); got != tc.overFuture+1 {
				t.Fatalf("want %d snapshots, got %d", tc.overFuture+1, got)
			}
		})
	}
}

func TestHomeFutureMemory_Integration_RejectsActiveJobMemory(t *testing.T) {
	for _, kind := range []corecharacter.JobMemoryKind{corecharacter.JobMemoryKindPersistent, corecharacter.JobMemoryKindTemporary} {
		for _, source := range []string{"inventory", "depot"} {
			t.Run(string(kind)+"/"+source, func(t *testing.T) {
				f := newFutureMemoryFixture(t, "FutMemJM", 3, []string{"jm-inv"}, []string{"jm-dep"})
				ctx := context.Background()
				c := f.char
				c.JobMemory = &corecharacter.JobMemory{Kind: kind, JobID: "job-02", SP: 1, OldJobID: "job-03", OldSP: 1}
				if err := f.charRepo.Update(ctx, c); err != nil {
					t.Fatal(err)
				}
				id := f.id(map[string]string{"inventory": "jm-inv", "depot": "jm-dep"}[source])
				if _, err := f.svc.UseHomeItem(ctx, c.ID, id, source); !errors.Is(err, home.ErrCannotUseHere) {
					t.Fatalf("want rejection, got %v", err)
				}
				if !f.owns(t, source, id) || len(f.memories(t)) != 0 {
					t.Fatal("rejected save must keep the item and store nothing")
				}
			})
		}
	}
}

func TestHomeFutureMemory_Integration_ConcurrentLastSlot(t *testing.T) {
	f := newFutureMemoryFixture(t, "FutMemCc", 0, []string{"cc-inv"}, []string{"cc-dep"})
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var saved, rejected int32
	uses := [][2]string{{"cc-inv", "inventory"}, {"cc-dep", "depot"}}
	res := testutil.RunConcurrentStressTest(t, testutil.ConcurrencyStressConfig{Workers: 2, OpsPerWorker: 1}, func(w, _ int) error {
		_, err := f.svc.UseHomeItem(ctx, f.char.ID, f.id(uses[w][0]), uses[w][1])
		switch {
		case err == nil:
			atomic.AddInt32(&saved, 1)
		case errors.Is(err, home.ErrCannotUseHere):
			atomic.AddInt32(&rejected, 1)
		default:
			return err
		}
		return nil
	})
	if res.Failures+res.Deadlocks > 0 {
		t.Fatalf("unexpected failures=%d deadlocks=%d", res.Failures, res.Deadlocks)
	}
	if saved != 1 || rejected != 1 || len(f.memories(t)) != 1 {
		t.Fatalf("saved=%d rejected=%d snapshots=%d", saved, rejected, len(f.memories(t)))
	}
	if f.owns(t, "inventory", f.id("cc-inv")) == f.owns(t, "depot", f.id("cc-dep")) {
		t.Fatal("exactly one item-207 must be consumed")
	}
}

type failingFutureMemorySaver struct{ inner home.FutureMemorySaver }

func (s failingFutureMemorySaver) SaveFutureMemorySnapshot(ctx context.Context, char corecharacter.Character) (corecharacter.FutureMemory, error) {
	if _, err := s.inner.SaveFutureMemorySnapshot(ctx, char); err != nil {
		return corecharacter.FutureMemory{}, err
	}
	return corecharacter.FutureMemory{}, errors.New("injected failure after snapshot write")
}

func TestHomeFutureMemory_Integration_FailureRollsBackAndRetrySucceeds(t *testing.T) {
	for _, source := range []string{"inventory", "depot"} {
		t.Run(source, func(t *testing.T) {
			f := newFutureMemoryFixture(t, "FutMemRb", 0, []string{"rb-inv"}, []string{"rb-dep"})
			ctx := context.Background()
			id := f.id(map[string]string{"inventory": "rb-inv", "depot": "rb-dep"}[source])

			f.svc.SetFutureMemorySaver(failingFutureMemorySaver{inner: f.jobSvc})
			if _, err := f.svc.UseHomeItem(ctx, f.char.ID, id, source); err == nil {
				t.Fatal("expected injected failure")
			}
			if !f.owns(t, source, id) || len(f.memories(t)) != 0 {
				t.Fatal("failure must roll back consumption and snapshot")
			}
			if sp, _ := f.jobState(t).SavedSP(f.char.JobID); sp != 5 {
				t.Fatalf("failure must roll back SP record, got %d", sp)
			}

			f.svc.SetFutureMemorySaver(f.jobSvc)
			if _, err := f.svc.UseHomeItem(ctx, f.char.ID, id, source); err != nil {
				t.Fatalf("retry: %v", err)
			}
			if f.owns(t, source, id) || len(f.memories(t)) != 1 {
				t.Fatal("retry must consume the item and store one snapshot")
			}
		})
	}
}
