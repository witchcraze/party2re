package job_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	corejob "github.com/witchcraze/party2re/internal/core/job"
	"github.com/witchcraze/party2re/internal/costume"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/economy"
	"github.com/witchcraze/party2re/internal/god"
	"github.com/witchcraze/party2re/internal/home"
	"github.com/witchcraze/party2re/internal/inventory"
	"github.com/witchcraze/party2re/internal/job"
	"github.com/witchcraze/party2re/internal/store"
	vk "github.com/witchcraze/party2re/internal/valkey"
)

type countingGuildAwarder struct{ calls int }

func (a *countingGuildAwarder) AddGuildPoints(context.Context, string, int) error {
	a.calls++
	return nil
}

func TestCelestialWishSynchronizesJobStateAcrossDatabaseReload(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	chars, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	jobs, err := database.NewCharacterJobRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	inventories, err := database.NewInventoryRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	txProvider := database.NewTransactionProvider(db)
	runner, err := economy.NewService(chars, inventories, economy.WithTransactionProvider(txProvider))
	if err != nil {
		t.Fatal(err)
	}
	// job-70 is ineligible for a normal change: it requires itself.
	catalog, err := corejob.NewCatalog([]corejob.Definition{
		{ID: "job-01", Name: "First", CMPTier: 1, MasterySP: 80},
		{ID: "job-02", Name: "Second", CMPTier: 1, MasterySP: 80},
		{ID: "job-70", Name: "Celestial", CMPTier: 1, MasterySP: 80, RequiredJobIDs: []string{"job-70"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	sentinel := errors.New("job state save failure")

	for _, tc := range []struct {
		name         string
		sp, wantSP   int
		masterTarget bool
		wantMastered bool
		failJobSave  bool
	}{
		{name: "unmastered departure", sp: 30},
		{name: "mastery reached on departure", sp: 90, wantMastered: true},
		{name: "previously mastered target restores SP", sp: 10, masterTarget: true, wantSP: 150},
		{name: "job state failure rolls back character", sp: 30, failJobSave: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var repo job.Repository = jobs
			if tc.failJobSave {
				repo = failAfterJobPointsSave{Repository: jobs, err: sentinel}
			}
			guild := &countingGuildAwarder{}
			jobSvc, err := job.NewService(repo, job.WithCatalog(catalog), job.WithCharacterRepository(chars), job.WithInventoryRepository(inventories), job.WithEconomy(runner), job.WithGuildPointAwarder(guild))
			if err != nil {
				t.Fatal(err)
			}
			godSvc, err := god.NewService(chars, god.WithTransactionProvider(txProvider), god.WithJobChanger(jobSvc))
			if err != nil {
				t.Fatal(err)
			}

			char, err := database.CreateTestCharacter(ctx, db, "Celestial Sync")
			if err != nil {
				t.Fatal(err)
			}
			// Level 5 proves the normal Lv20 gate is not applied.
			char.JobID, char.OldJobID, char.SP, char.OldSP, char.Level = "job-01", "", tc.sp, 0, 5
			char.GodVisit = string(god.RealmHeaven)
			if err := chars.Update(ctx, char); err != nil {
				t.Fatal(err)
			}
			state, err := corejob.NewCharacterJob(char.ID, "job-01")
			if err != nil {
				t.Fatal(err)
			}
			if tc.masterTarget {
				state.RecordMastery("job-70", 150, 80)
				if err := state.RecordJobSP("job-70", 150); err != nil {
					t.Fatal(err)
				}
			}
			if err := jobs.Save(ctx, state); err != nil {
				t.Fatal(err)
			}

			_, wishErr := godSvc.GrantWish(ctx, char.ID, god.WishCelestialDragon)
			stored, err := chars.FindByID(ctx, char.ID)
			if err != nil {
				t.Fatal(err)
			}
			loaded, err := jobs.FindByCharacterID(ctx, char.ID)
			if err != nil {
				t.Fatal(err)
			}
			if tc.failJobSave {
				if !errors.Is(wishErr, sentinel) || stored.JobID != "job-01" || stored.SP != tc.sp || loaded.CurrentJobID != "job-01" || loaded.HasRecordedJob("job-01") {
					t.Fatalf("not rolled back: err=%v char=%+v state=%+v", wishErr, stored, loaded)
				}
				return
			}
			if wishErr != nil {
				t.Fatal(wishErr)
			}
			if stored.JobID != "job-70" || stored.OldJobID != "job-01" || stored.SP != tc.wantSP || stored.OldSP != tc.sp || loaded.CurrentJobID != "job-70" {
				t.Fatalf("job ids/SP out of sync: char=%+v state=%+v", stored, loaded)
			}
			if sp, ok := loaded.SavedSP("job-01"); !ok || sp != tc.sp || loaded.IsMastered("job-01") != tc.wantMastered {
				t.Fatalf("departure record: sp=%d ok=%v state=%+v", sp, ok, loaded)
			}
			if !slices.Contains(loaded.History, corejob.Change{FromJobID: "job-01", ToJobID: "job-70"}) || guild.calls != 0 {
				t.Fatalf("history=%v guildCalls=%d", loaded.History, guild.calls)
			}

			// A following normal change departs job-70, not the stale job-01.
			stored.Level, stored.SP = 20, 40
			if err := chars.Update(ctx, stored); err != nil {
				t.Fatal(err)
			}
			changed, _, err := jobSvc.ChangeJob(ctx, char.ID, "job-02")
			if err != nil {
				t.Fatal(err)
			}
			loaded, err = jobs.FindByCharacterID(ctx, char.ID)
			if err != nil {
				t.Fatal(err)
			}
			if sp, _ := loaded.SavedSP("job-70"); changed.OldJobID != "job-70" || sp != 40 || loaded.CurrentJobID != "job-02" || !slices.Contains(loaded.History, corejob.Change{FromJobID: "job-70", ToJobID: "job-02"}) {
				t.Fatalf("follow-up change: char=%+v state=%+v", changed, loaded)
			}
			if prior, _ := loaded.SavedSP("job-01"); prior != tc.sp {
				t.Fatalf("job-01 record overwritten: %d", prior)
			}
		})
	}
}

func testCostumeRepoFromEnv(t *testing.T) costume.Repository {
	t.Helper()
	if os.Getenv("PARTY2_VALKEY_ADDR") == "" {
		return costume.NewMemoryCostumeRepository()
	}
	client, err := vk.NewClient()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	return costume.NewValkeyCostumeRepository(client)
}

type jobCostumeWrapper struct {
	store   *store.Service
	err     error
	onReset func(ctx context.Context, characterID string)
}

func (w *jobCostumeWrapper) ResetCostume(ctx context.Context, characterID string) error {
	if w.err != nil {
		return w.err
	}
	if err := w.store.ResetCostume(ctx, characterID); err != nil {
		return err
	}
	if w.onReset != nil {
		w.onReset(ctx, characterID)
	}
	return nil
}

type failingUpdateChars struct {
	*database.CharacterRepository
	err error
}

func (r failingUpdateChars) Update(context.Context, corecharacter.Character) error {
	return r.err
}

type hookedHomeCostume struct {
	store   *store.Service
	onApply func(characterID string)
}

func (h *hookedHomeCostume) ApplyCostume(ctx context.Context, characterID string, itemNo int, itemName, icon string, exp time.Time) (func(context.Context) error, error) {
	if h.onApply != nil {
		h.onApply(characterID)
	}
	return h.store.ApplyCostume(ctx, characterID, itemNo, itemName, icon, exp)
}

func newTestHomeService(t *testing.T, db *sql.DB, chars economy.CharacterRepository, invRepo *database.InventoryRepository, tx *database.DefaultTransactionProvider, applier home.CostumeApplier) *home.Service {
	t.Helper()
	homeRepo, err := database.NewHomeRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	eco, err := economy.NewService(chars, invRepo, economy.WithTransactionProvider(tx))
	if err != nil {
		t.Fatal(err)
	}
	invSvc, err := inventory.NewService(invRepo)
	if err != nil {
		t.Fatal(err)
	}
	cat, err := coreitem.InitialCatalog()
	if err != nil {
		t.Fatal(err)
	}
	reader, ok := chars.(home.CharacterReader)
	if !ok {
		t.Fatalf("chars must implement home.CharacterReader")
	}
	svc, err := home.NewService(homeRepo, reader,
		home.WithEconomy(eco),
		home.WithInventoryManager(invSvc),
		home.WithItemCatalog(cat),
		home.WithCostumeApplier(applier),
	)
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func seedActiveCostume(t *testing.T, repo costume.Repository, charID string) costume.ActiveCostume {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	prior := costume.ActiveCostume{
		CharacterID: charID,
		ItemNo:      44,
		ItemName:    "ピンクスカート",
		Icon:        "chr/001.gif",
		RentedAt:    now.Add(-time.Minute),
		ExpiresAt:   now.Add(time.Hour),
	}
	if err := repo.SaveActiveCostume(ctx, prior, time.Hour); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.ClearActiveCostume(context.Background(), charID) })
	return prior
}

func sameCostume(got *costume.ActiveCostume, want costume.ActiveCostume) bool {
	return got != nil &&
		got.CharacterID == want.CharacterID &&
		got.ItemNo == want.ItemNo &&
		got.ItemName == want.ItemName &&
		got.Icon == want.Icon &&
		got.RentedAt.Equal(want.RentedAt) &&
		got.ExpiresAt.Equal(want.ExpiresAt)
}

func TestChangeJob_CostumeResetContract(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	costumeRepo := testCostumeRepoFromEnv(t)
	storeSvc := store.NewService(nil, nil, nil, nil, nil, store.WithCostumeRepository(costumeRepo))
	ctx := context.Background()

	chars, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	jobs, err := database.NewCharacterJobRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	inventories, err := database.NewInventoryRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	txProvider := database.NewTransactionProvider(db)
	runner, err := economy.NewService(chars, inventories, economy.WithTransactionProvider(txProvider))
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := corejob.NewCatalog([]corejob.Definition{
		{ID: "job-01", Name: "First", CMPTier: 1, MasterySP: 80},
		{ID: "job-02", Name: "Second", CMPTier: 1, MasterySP: 80},
	})
	if err != nil {
		t.Fatal(err)
	}

	setupChar := func(t *testing.T, prefix string) (corecharacter.Character, costume.ActiveCostume) {
		t.Helper()
		char, err := database.CreateTestCharacter(ctx, db, prefix)
		if err != nil {
			t.Fatal(err)
		}
		char.JobID, char.OldJobID, char.SP, char.Level = "job-01", "", 10, 20
		if err := chars.Update(ctx, char); err != nil {
			t.Fatal(err)
		}
		state, err := corejob.NewCharacterJob(char.ID, "job-01")
		if err != nil {
			t.Fatal(err)
		}
		if err := jobs.Save(ctx, state); err != nil {
			t.Fatal(err)
		}
		prior := seedActiveCostume(t, costumeRepo, char.ID)
		return char, prior
	}

	t.Run("resets under lock and allows subsequent home costume write", func(t *testing.T) {
		char, _ := setupChar(t, "ResetLockOK")
		var resetLocked atomic.Bool
		wrapper := &jobCostumeWrapper{
			store: storeSvc,
			onReset: func(_ context.Context, id string) {
				_, lockErr := db.ExecContext(ctx, `SELECT id FROM characters WHERE id = ? FOR UPDATE NOWAIT`, id)
				resetLocked.Store(lockErr != nil)
			},
		}
		jobSvc, err := job.NewService(jobs, job.WithCatalog(catalog), job.WithCharacterRepository(chars), job.WithInventoryRepository(inventories), job.WithEconomy(runner), job.WithCostumeResetter(wrapper))
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := jobSvc.ChangeJob(ctx, char.ID, "job-02"); err != nil {
			t.Fatalf("ChangeJob failed: %v", err)
		}
		if !resetLocked.Load() {
			t.Fatal("ResetCostume did not execute under character row lock")
		}
		if got, err := costumeRepo.GetActiveCostume(ctx, char.ID); err != nil || got != nil {
			t.Fatalf("costume not cleared after job change: got=%+v err=%v", got, err)
		}

		// Subsequent Home writer sets a new costume cleanly.
		homeSvc := newTestHomeService(t, db, chars, inventories, txProvider, storeSvc)
		inv, _ := coreinventory.New(char.ID)
		costumeInst := coreitem.Instance{ID: "it-after-job-" + char.ID[:8], DefinitionID: "item-045", Quantity: 1}
		if err := inv.Add(costumeInst); err != nil {
			t.Fatal(err)
		}
		if err := inventories.Save(ctx, inv); err != nil {
			t.Fatal(err)
		}
		if _, err := homeSvc.UseHomeItem(ctx, char.ID, costumeInst.ID, "inventory"); err != nil {
			t.Fatalf("UseHomeItem after ChangeJob failed: %v", err)
		}
		if got, err := costumeRepo.GetActiveCostume(ctx, char.ID); err != nil || got == nil || got.ItemNo != 45 {
			t.Fatalf("subsequent home costume not persisted: got=%+v err=%v", got, err)
		}
	})

	t.Run("reset error fails and rolls back job change", func(t *testing.T) {
		char, prior := setupChar(t, "ResetErr")
		resetSentinel := errors.New("injected reset error")
		wrapper := &jobCostumeWrapper{store: storeSvc, err: resetSentinel}
		jobSvc, err := job.NewService(jobs, job.WithCatalog(catalog), job.WithCharacterRepository(chars), job.WithInventoryRepository(inventories), job.WithEconomy(runner), job.WithCostumeResetter(wrapper))
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := jobSvc.ChangeJob(ctx, char.ID, "job-02"); !errors.Is(err, resetSentinel) {
			t.Fatalf("want resetSentinel, got %v", err)
		}
		stored, err := chars.FindByID(ctx, char.ID)
		if err != nil || stored.JobID != "job-01" {
			t.Fatalf("character not rolled back: stored=%+v err=%v", stored, err)
		}
		got, err := costumeRepo.GetActiveCostume(ctx, char.ID)
		if err != nil || !sameCostume(got, prior) {
			t.Fatalf("costume unexpectedly changed: got=%+v err=%v", got, err)
		}
	})

	t.Run("post-reset failure restores prior costume", func(t *testing.T) {
		char, prior := setupChar(t, "ResetRollback")
		saveSentinel := errors.New("job save failure")
		wrapper := &jobCostumeWrapper{store: storeSvc}
		jobSvc, err := job.NewService(failAfterJobPointsSave{Repository: jobs, err: saveSentinel}, job.WithCatalog(catalog), job.WithCharacterRepository(chars), job.WithInventoryRepository(inventories), job.WithEconomy(runner), job.WithCostumeResetter(wrapper))
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := jobSvc.ChangeJob(ctx, char.ID, "job-02"); !errors.Is(err, saveSentinel) {
			t.Fatalf("want saveSentinel, got %v", err)
		}
		stored, err := chars.FindByID(ctx, char.ID)
		loaded, err2 := jobs.FindByCharacterID(ctx, char.ID)
		if err != nil || err2 != nil || stored.JobID != "job-01" || loaded.CurrentJobID != "job-01" {
			t.Fatalf("SQL not rolled back: char=%+v state=%+v err=%v/%v", stored, loaded, err, err2)
		}
		got, err := costumeRepo.GetActiveCostume(ctx, char.ID)
		if err != nil || !sameCostume(got, prior) {
			t.Fatalf("prior costume not restored: got=%+v err=%v", got, err)
		}
	})

	t.Run("ambient tx rollback restores prior costume", func(t *testing.T) {
		char, prior := setupChar(t, "ResetAmbient")
		outerErr := errors.New("outer ambient failure")
		wrapper := &jobCostumeWrapper{store: storeSvc}
		jobSvc, err := job.NewService(jobs, job.WithCatalog(catalog), job.WithCharacterRepository(chars), job.WithInventoryRepository(inventories), job.WithEconomy(runner), job.WithCostumeResetter(wrapper))
		if err != nil {
			t.Fatal(err)
		}
		err = database.RunInTx(ctx, db, func(txCtx context.Context) error {
			if _, _, err := jobSvc.ChangeJob(txCtx, char.ID, "job-02"); err != nil {
				return err
			}
			return outerErr
		})
		if !errors.Is(err, outerErr) {
			t.Fatalf("want outerErr, got %v", err)
		}
		stored, err := chars.FindByID(ctx, char.ID)
		if err != nil || stored.JobID != "job-01" {
			t.Fatalf("SQL not rolled back: stored=%+v err=%v", stored, err)
		}
		got, err := costumeRepo.GetActiveCostume(ctx, char.ID)
		if err != nil || !sameCostume(got, prior) {
			t.Fatalf("prior costume not restored after ambient rollback: got=%+v err=%v", got, err)
		}
	})

	t.Run("overlapping failed ChangeJob and Home use preserve prior costume", func(t *testing.T) {
		char, prior := setupChar(t, "OverlapJobHome")
		inv, _ := coreinventory.New(char.ID)
		homeInst := coreitem.Instance{ID: "it-overlap-" + char.ID[:8], DefinitionID: "item-045", Quantity: 1}
		if err := inv.Add(homeInst); err != nil {
			t.Fatal(err)
		}
		if err := inventories.Save(ctx, inv); err != nil {
			t.Fatal(err)
		}

		saveSentinel := errors.New("job save failure")
		homeSaveSentinel := errors.New("home character save failure")
		rollbackEntered := make(chan struct{})
		releaseRollback := make(chan struct{})
		homeApplied := make(chan struct{})
		var rollbackLocked atomic.Bool
		var once sync.Once

		// Hook runs AFTER storeSvc.ResetCostume registers its undo, so LIFO invokes
		// this blocker BEFORE storeSvc restores Valkey, while holding the SQL lock.
		wrapper := &jobCostumeWrapper{
			store: storeSvc,
			onReset: func(txCtx context.Context, id string) {
				economy.OnRollback(txCtx, func(context.Context) error {
					once.Do(func() {
						_, lockErr := db.ExecContext(ctx, `SELECT id FROM characters WHERE id = ? FOR UPDATE NOWAIT`, id)
						rollbackLocked.Store(lockErr != nil)
						close(rollbackEntered)
						<-releaseRollback
					})
					return nil
				})
			},
		}
		jobSvc, err := job.NewService(failAfterJobPointsSave{Repository: jobs, err: saveSentinel}, job.WithCatalog(catalog), job.WithCharacterRepository(chars), job.WithInventoryRepository(inventories), job.WithEconomy(runner), job.WithCostumeResetter(wrapper))
		if err != nil {
			t.Fatal(err)
		}

		homeApplier := &hookedHomeCostume{
			store: storeSvc,
			onApply: func(string) {
				close(homeApplied)
			},
		}
		failingChars := failingUpdateChars{CharacterRepository: chars, err: homeSaveSentinel}
		homeSvc := newTestHomeService(t, db, failingChars, inventories, txProvider, homeApplier)

		jobErrCh := make(chan error, 1)
		go func() {
			_, _, err := jobSvc.ChangeJob(ctx, char.ID, "job-02")
			jobErrCh <- err
		}()

		<-rollbackEntered
		if !rollbackLocked.Load() {
			close(releaseRollback)
			<-jobErrCh
			t.Fatal("ChangeJob rollback ran without holding the character lock")
		}

		homeStarted := make(chan struct{})
		homeErrCh := make(chan error, 1)
		go func() {
			close(homeStarted)
			_, err := homeSvc.UseHomeItem(ctx, char.ID, homeInst.ID, "inventory")
			homeErrCh <- err
		}()
		<-homeStarted

		timer := time.NewTimer(100 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-homeApplied:
			close(releaseRollback)
			<-jobErrCh
			<-homeErrCh
			t.Fatal("Home item writer entered ApplyCostume while ChangeJob rollback still held the character lock")
		case <-timer.C:
		}

		close(releaseRollback)
		if gotErr := <-jobErrCh; !errors.Is(gotErr, saveSentinel) {
			t.Fatalf("want saveSentinel, got %v", gotErr)
		}
		if gotErr := <-homeErrCh; !errors.Is(gotErr, homeSaveSentinel) {
			t.Fatalf("want homeSaveSentinel, got %v", gotErr)
		}
		select {
		case <-homeApplied:
		default:
			t.Fatal("Home writer never ran after ChangeJob released the character lock")
		}

		got, err := costumeRepo.GetActiveCostume(ctx, char.ID)
		if err != nil || !sameCostume(got, prior) {
			t.Fatalf("prior costume lost across overlapping failures: got=%+v err=%v", got, err)
		}
	})
}

func TestCelestialWish_CostumeResetContract(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	costumeRepo := testCostumeRepoFromEnv(t)
	storeSvc := store.NewService(nil, nil, nil, nil, nil, store.WithCostumeRepository(costumeRepo))
	ctx := context.Background()

	chars, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	jobs, err := database.NewCharacterJobRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	inventories, err := database.NewInventoryRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	txProvider := database.NewTransactionProvider(db)
	runner, err := economy.NewService(chars, inventories, economy.WithTransactionProvider(txProvider))
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := corejob.NewCatalog([]corejob.Definition{
		{ID: "job-01", Name: "First", CMPTier: 1, MasterySP: 80},
		{ID: "job-70", Name: "Celestial", CMPTier: 1, MasterySP: 80, RequiredJobIDs: []string{"job-70"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	setupWishChar := func(t *testing.T, prefix string) (corecharacter.Character, costume.ActiveCostume) {
		t.Helper()
		char, err := database.CreateTestCharacter(ctx, db, prefix)
		if err != nil {
			t.Fatal(err)
		}
		char.JobID, char.OldJobID, char.SP, char.Level = "job-01", "", 30, 5
		char.GodVisit = string(god.RealmHeaven)
		if err := chars.Update(ctx, char); err != nil {
			t.Fatal(err)
		}
		state, err := corejob.NewCharacterJob(char.ID, "job-01")
		if err != nil {
			t.Fatal(err)
		}
		if err := jobs.Save(ctx, state); err != nil {
			t.Fatal(err)
		}
		prior := seedActiveCostume(t, costumeRepo, char.ID)
		return char, prior
	}

	t.Run("success clears costume and subsequent home write persists", func(t *testing.T) {
		char, _ := setupWishChar(t, "WishClearOK")
		jobSvc, err := job.NewService(jobs, job.WithCatalog(catalog), job.WithCharacterRepository(chars), job.WithInventoryRepository(inventories), job.WithEconomy(runner), job.WithCostumeResetter(storeSvc))
		if err != nil {
			t.Fatal(err)
		}
		godSvc, err := god.NewService(chars, god.WithTransactionProvider(txProvider), god.WithJobChanger(jobSvc))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := godSvc.GrantWish(ctx, char.ID, god.WishCelestialDragon); err != nil {
			t.Fatalf("GrantWish failed: %v", err)
		}
		if got, err := costumeRepo.GetActiveCostume(ctx, char.ID); err != nil || got != nil {
			t.Fatalf("costume not cleared by CelestialWish: got=%+v err=%v", got, err)
		}

		homeSvc := newTestHomeService(t, db, chars, inventories, txProvider, storeSvc)
		inv, _ := coreinventory.New(char.ID)
		homeInst := coreitem.Instance{ID: "it-after-wish-" + char.ID[:8], DefinitionID: "item-045", Quantity: 1}
		if err := inv.Add(homeInst); err != nil {
			t.Fatal(err)
		}
		if err := inventories.Save(ctx, inv); err != nil {
			t.Fatal(err)
		}
		if _, err := homeSvc.UseHomeItem(ctx, char.ID, homeInst.ID, "inventory"); err != nil {
			t.Fatalf("UseHomeItem after GrantWish failed: %v", err)
		}
		if got, err := costumeRepo.GetActiveCostume(ctx, char.ID); err != nil || got == nil || got.ItemNo != 45 {
			t.Fatalf("home costume not saved after wish reset: got=%+v err=%v", got, err)
		}
	})

	t.Run("reset error rolls back celestial wish", func(t *testing.T) {
		char, prior := setupWishChar(t, "WishResetErr")
		resetSentinel := errors.New("wish reset failed")
		wrapper := &jobCostumeWrapper{store: storeSvc, err: resetSentinel}
		jobSvc, err := job.NewService(jobs, job.WithCatalog(catalog), job.WithCharacterRepository(chars), job.WithInventoryRepository(inventories), job.WithEconomy(runner), job.WithCostumeResetter(wrapper))
		if err != nil {
			t.Fatal(err)
		}
		godSvc, err := god.NewService(chars, god.WithTransactionProvider(txProvider), god.WithJobChanger(jobSvc))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := godSvc.GrantWish(ctx, char.ID, god.WishCelestialDragon); !errors.Is(err, resetSentinel) {
			t.Fatalf("want resetSentinel, got %v", err)
		}
		stored, err := chars.FindByID(ctx, char.ID)
		if err != nil || stored.JobID != "job-01" || stored.GodVisit != string(god.RealmHeaven) {
			t.Fatalf("character not rolled back: stored=%+v err=%v", stored, err)
		}
		got, err := costumeRepo.GetActiveCostume(ctx, char.ID)
		if err != nil || !sameCostume(got, prior) {
			t.Fatalf("prior costume altered on reset error: got=%+v err=%v", got, err)
		}
	})

	t.Run("post-reset failure and ambient rollback restore prior costume", func(t *testing.T) {
		for _, ambient := range []bool{false, true} {
			name := "job save failure"
			if ambient {
				name = "ambient tx failure"
			}
			t.Run(name, func(t *testing.T) {
				char, prior := setupWishChar(t, "WishRollback")
				sentinel := errors.New("wish failure")
				var repo job.Repository = jobs
				if !ambient {
					repo = failAfterJobPointsSave{Repository: jobs, err: sentinel}
				}
				jobSvc, err := job.NewService(repo, job.WithCatalog(catalog), job.WithCharacterRepository(chars), job.WithInventoryRepository(inventories), job.WithEconomy(runner), job.WithCostumeResetter(storeSvc))
				if err != nil {
					t.Fatal(err)
				}
				godSvc, err := god.NewService(chars, god.WithTransactionProvider(txProvider), god.WithJobChanger(jobSvc))
				if err != nil {
					t.Fatal(err)
				}
				var gotErr error
				if ambient {
					gotErr = database.RunInTx(ctx, db, func(txCtx context.Context) error {
						if _, err := godSvc.GrantWish(txCtx, char.ID, god.WishCelestialDragon); err != nil {
							return err
						}
						return sentinel
					})
				} else {
					_, gotErr = godSvc.GrantWish(ctx, char.ID, god.WishCelestialDragon)
				}
				if !errors.Is(gotErr, sentinel) {
					t.Fatalf("want sentinel, got %v", gotErr)
				}
				stored, err := chars.FindByID(ctx, char.ID)
				loaded, err2 := jobs.FindByCharacterID(ctx, char.ID)
				if err != nil || err2 != nil || stored.JobID != "job-01" || loaded.CurrentJobID != "job-01" || stored.GodVisit != string(god.RealmHeaven) {
					t.Fatalf("SQL not rolled back: stored=%+v loaded=%+v err=%v/%v", stored, loaded, err, err2)
				}
				got, err := costumeRepo.GetActiveCostume(ctx, char.ID)
				if err != nil || !sameCostume(got, prior) {
					t.Fatalf("prior costume not restored: got=%+v err=%v", got, err)
				}
			})
		}
	})
}
