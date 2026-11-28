package home_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/character"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/costume"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/economy"
	"github.com/witchcraze/party2re/internal/home"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/inventory"
	"github.com/witchcraze/party2re/internal/notification"
	"github.com/witchcraze/party2re/internal/store"
	"github.com/witchcraze/party2re/internal/testutil"
)

const bookDefinition = "item-058"

// bookCostume wraps the production Store adapter over the active costume
// repository (Valkey when configured, otherwise in-memory), counting calls and
// optionally failing or running hooks around ApplyCostume and its undo closure.
type bookCostume struct {
	store    *store.Service
	repo     costume.Repository
	calls    atomic.Int64
	fail     bool
	mu       sync.Mutex
	onApply  func(characterID string)
	wrapUndo func(characterID string, undo func(context.Context) error) func(context.Context) error
}

func (c *bookCostume) ApplyCostume(ctx context.Context, characterID string, itemNo int, itemName, icon string) (func(context.Context) error, error) {
	c.calls.Add(1)
	if c.fail {
		return nil, errInjectedAppearance
	}
	c.mu.Lock()
	onApply := c.onApply
	wrapUndo := c.wrapUndo
	c.mu.Unlock()
	if onApply != nil {
		onApply(characterID)
	}
	undo, err := c.store.ApplyCostume(ctx, characterID, itemNo, itemName, icon)
	if err != nil {
		return undo, err
	}
	if wrapUndo != nil {
		return wrapUndo(characterID, undo), nil
	}
	return undo, nil
}

func (c *bookCostume) active(t *testing.T, characterID string) *costume.ActiveCostume {
	t.Helper()
	got, err := c.repo.GetActiveCostume(context.Background(), characterID)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func sameCostume(a *costume.ActiveCostume, b costume.ActiveCostume) bool {
	return a != nil &&
		a.CharacterID == b.CharacterID &&
		a.ItemNo == b.ItemNo &&
		a.ItemName == b.ItemName &&
		a.Icon == b.Icon &&
		a.RentedAt.Equal(b.RentedAt)
}

// bookFailures injects failures before commit (profile save, the runner's
// character save, Depot consumption) and after commit (appearance store).
type bookFailures struct {
	profileSave, characterSave, depotConsume, appearance bool
}

var (
	errInjectedBook       = errors.New("injected failure")
	errInjectedAppearance = errors.New("injected appearance store failure")
)

type failingEconomyCharacters struct {
	*database.CharacterRepository
	fail *atomic.Bool
}

func (r failingEconomyCharacters) Update(ctx context.Context, c corecharacter.Character) error {
	if r.fail == nil || r.fail.Load() {
		return errInjectedBook
	}
	return r.CharacterRepository.Update(ctx, c)
}

type afterSaveInventories struct {
	*database.InventoryRepository
	afterSave *atomic.Pointer[func(context.Context)]
}

func (r afterSaveInventories) Save(ctx context.Context, inv coreinventory.Inventory) error {
	if err := r.InventoryRepository.Save(ctx, inv); err != nil {
		return err
	}
	if r.afterSave != nil {
		if fn := r.afterSave.Load(); fn != nil && *fn != nil {
			(*fn)(ctx)
		}
	}
	return nil
}

type failingDepotConsume struct {
	*testDepotManagerAdapter
	beforeFail *func(characterID string)
}

func (d failingDepotConsume) ConsumeOne(_ context.Context, characterID, _ string) error {
	if *d.beforeFail != nil {
		(*d.beforeFail)(characterID)
	}
	return errInjectedBook
}

// failingProfiles makes the profile save fail after the login message was written.
type failingProfiles struct {
	character.ExtendedRepository
	fail bool
}

func (r failingProfiles) SaveProfile(ctx context.Context, p character.Profile) error {
	if r.fail {
		return errors.New("injected profile save failure")
	}
	return r.ExtendedRepository.SaveProfile(ctx, p)
}

type bookFixture struct {
	db      *sql.DB
	svc     *home.Service
	costume *bookCostume
	// beforeDepotFail runs inside a failing Depot consumption, after the appearance write.
	beforeDepotFail func(characterID string)
	charSaveFail    *atomic.Bool
	afterInvSave    *atomic.Pointer[func(context.Context)]
	invRepo         *database.InventoryRepository
	depots          *database.DepotRepository
	charRepo        *database.CharacterRepository
	presence        *character.PresenceService
}

func newBookFixture(t *testing.T, fail bookFailures) *bookFixture {
	t.Helper()
	db := homeIsolatedDB(t)
	var repo costume.Repository = costume.NewMemoryCostumeRepository()
	if os.Getenv("PARTY2_VALKEY_ADDR") != "" {
		client := testutil.OpenValkey(t)
		repo = costume.NewValkeyCostumeRepository(client)
	}
	charSaveFail := &atomic.Bool{}
	charSaveFail.Store(fail.characterSave)
	f := &bookFixture{
		db: db,
		costume: &bookCostume{
			store: store.NewService(nil, nil, nil, nil, nil, store.WithCostumeRepository(repo)),
			repo:  repo,
			fail:  fail.appearance,
		},
		charSaveFail: charSaveFail,
		afterInvSave: &atomic.Pointer[func(context.Context)]{},
	}
	var err error
	if f.charRepo, err = database.NewCharacterRepository(db); err != nil {
		t.Fatal(err)
	}
	if f.invRepo, err = database.NewInventoryRepository(db); err != nil {
		t.Fatal(err)
	}
	if f.depots, err = database.NewDepotRepository(db); err != nil {
		t.Fatal(err)
	}
	homeRepo, err := database.NewHomeRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	notifRepo, err := database.NewNotificationRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	news, err := notification.NewService(notifRepo, notifRepo)
	if err != nil {
		t.Fatal(err)
	}
	tx := database.NewTransactionProvider(db)
	chars, err := character.NewService(failingProfiles{f.charRepo, fail.profileSave},
		character.WithTransactionProvider(tx),
		character.WithNewsPublisher(character.NewsPublisherFunc(func(ctx context.Context, cat, title, content, author string, at time.Time) error {
			_, err := news.PublishNews(ctx, cat, title, content, author, at)
			return err
		})))
	if err != nil {
		t.Fatal(err)
	}
	presenceRepo, err := database.NewPresenceRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	if f.presence, err = character.NewPresenceService(presenceRepo, tx, nil); err != nil {
		t.Fatal(err)
	}
	ecoChars := failingEconomyCharacters{f.charRepo, f.charSaveFail}
	ecoInvs := afterSaveInventories{f.invRepo, f.afterInvSave}
	eco, err := economy.NewService(ecoChars, ecoInvs, economy.WithTransactionProvider(tx))
	if err != nil {
		t.Fatal(err)
	}
	invService, err := inventory.NewService(f.invRepo)
	if err != nil {
		t.Fatal(err)
	}
	cat, err := coreitem.InitialCatalog()
	if err != nil {
		t.Fatal(err)
	}
	var depots home.DepotManager = &testDepotManagerAdapter{repo: f.depots}
	if fail.depotConsume {
		depots = failingDepotConsume{&testDepotManagerAdapter{repo: f.depots}, &f.beforeDepotFail}
	}
	f.svc, err = home.NewService(homeRepo, f.charRepo,
		home.WithEconomy(eco),
		home.WithInventoryManager(invService),
		home.WithDepotManager(depots),
		home.WithItemCatalog(cat),
		home.WithCostumeApplier(f.costume),
	)
	if err != nil {
		t.Fatal(err)
	}
	f.svc.SetPersonalityRecorder(struct {
		*character.PresenceService
		*character.Service
	}{f.presence, chars})
	return f
}

func (f *bookFixture) character(t *testing.T, gender string) corecharacter.Character {
	t.Helper()
	c, err := database.CreateTestCharacter(context.Background(), f.db, "Book"+gender)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`UPDATE characters SET gender = ? WHERE id = ?`, gender, c.ID); err != nil {
		t.Fatal(err)
	}
	c.Gender = gender
	t.Cleanup(func() {
		if err := f.costume.repo.ClearActiveCostume(context.Background(), c.ID); err != nil {
			t.Errorf("cleanup ClearActiveCostume %s: %v", c.ID, err)
		}
	})
	return c
}

func (f *bookFixture) stock(t *testing.T, c corecharacter.Character, source string) string {
	return f.stockItem(t, c, source, bookDefinition)
}

func (f *bookFixture) stockItem(t *testing.T, c corecharacter.Character, source, definitionID string) string {
	return f.stockItemWithID(t, c, source, "it-"+source+"-"+c.ID[:8], definitionID)
}

func (f *bookFixture) stockItemWithID(t *testing.T, c corecharacter.Character, source, instID, definitionID string) string {
	t.Helper()
	ctx := context.Background()
	inst := coreitem.Instance{ID: instID, DefinitionID: definitionID, Quantity: 1}
	if source == "inventory" {
		inv, err := f.invRepo.FindByCharacterID(ctx, c.ID)
		if err != nil {
			inv, _ = coreinventory.New(c.ID)
		}
		if err := inv.Add(inst); err != nil {
			t.Fatal(err)
		}
		if err := f.invRepo.Save(ctx, inv); err != nil {
			t.Fatal(err)
		}
		return inst.ID
	}
	dp, err := f.depots.FindByCharacterID(ctx, c.ID)
	if err != nil {
		dp, _ = depot.NewDepot(c.ID)
	}
	if len(dp.Items) == 0 {
		// Overfill the Depot: capacity never blocks Home dispatch.
		for i := range depot.MaxDepotCapacity {
			dp.Items = append(dp.Items, coreitem.Instance{ID: fmt.Sprintf("fill-%s-%d", c.ID[:8], i), DefinitionID: "item-018", Quantity: 1})
		}
	}
	dp.Items = append(dp.Items, inst)
	if err := f.depots.Save(ctx, dp); err != nil {
		t.Fatal(err)
	}
	return inst.ID
}

func (f *bookFixture) held(t *testing.T, c corecharacter.Character, source, id string) bool {
	t.Helper()
	ctx := context.Background()
	if source == "inventory" {
		inv, err := f.invRepo.FindByCharacterID(ctx, c.ID)
		if err != nil {
			t.Fatal(err)
		}
		_, ok := inv.Find(id)
		return ok
	}
	dp, err := f.depots.FindByCharacterID(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range dp.Items {
		if it.ID == id {
			return true
		}
	}
	return false
}

func (f *bookFixture) newsCount(t *testing.T, c corecharacter.Character) int {
	t.Helper()
	var n int
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM news_articles WHERE content = ?`, c.Name+"がムッツリスケベになりました！").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestPersonalityBook_BothSourcesAndProfiles(t *testing.T) {
	f := newBookFixture(t, bookFailures{})
	ctx := context.Background()
	for _, tc := range []struct {
		source, gender string
		profile        bool
	}{{"inventory", "m", true}, {"depot", "f", false}} {
		c := f.character(t, tc.gender)
		if tc.profile {
			if err := f.charRepo.SaveProfile(ctx, character.Profile{CharacterID: c.ID, AvatarURL: "https://i.imgur.com/abc12.gif", AuraEffect: 2, BioData: map[string]string{"hobby": "fishing", "character": "calm"}}); err != nil {
				t.Fatal(err)
			}
		}
		id := f.stock(t, c, tc.source)
		res, err := f.svc.UseHomeItem(ctx, c.ID, id, tc.source)
		if err != nil {
			t.Fatalf("%s: %v", tc.source, err)
		}
		if !res.Consumed || res.Action != "consumed" || f.held(t, c, tc.source, id) {
			t.Fatalf("%s: book must be consumed once: %+v", tc.source, res)
		}
		p, err := f.charRepo.GetProfile(ctx, c.ID)
		if err != nil || p.BioData["character"] != "ムッツリスケベ" {
			t.Fatalf("%s: personality not saved: %+v %v", tc.source, p, err)
		}
		if tc.profile && (p.AvatarURL != "https://i.imgur.com/abc12.gif" || p.AuraEffect != 2 || p.BioData["hobby"] != "fishing") {
			t.Fatalf("other profile fields changed: %+v", p)
		}
		st, err := f.presence.State(ctx, c.ID)
		if err != nil || st.LoginMessage != "性格：ムッツリスケベ" {
			t.Fatalf("%s: login message %q %v", tc.source, st.LoginMessage, err)
		}
		if f.newsCount(t, c) != 1 {
			t.Fatalf("%s: want exactly one news article, got %d", tc.source, f.newsCount(t, c))
		}
		if got := f.costume.active(t, c.ID); got == nil || got.ItemNo != 58 || got.Icon != "assets/images/placeholders/job/appearance.svg#job-08-"+tc.gender {
			t.Fatalf("%s: appearance %+v", tc.source, got)
		}
	}
}

func TestPersonalityBook_ProfileFailureRollsBack(t *testing.T) {
	f := newBookFixture(t, bookFailures{profileSave: true})
	ctx := context.Background()
	for _, source := range []string{"inventory", "depot"} {
		c := f.character(t, "m")
		id := f.stock(t, c, source)
		if _, err := f.svc.UseHomeItem(ctx, c.ID, id, source); err == nil {
			t.Fatalf("%s: storage failure reported as success", source)
		}
		st, err := f.presence.State(ctx, c.ID)
		if err != nil || st.LoginMessage != "" || !f.held(t, c, source, id) || f.newsCount(t, c) != 0 {
			t.Fatalf("%s: partial commit message=%q held=%v news=%d", source, st.LoginMessage, f.held(t, c, source, id), f.newsCount(t, c))
		}
	}
	if f.costume.calls.Load() != 0 {
		t.Fatal("appearance applied despite rollback")
	}
}

// Database work that fails after the appearance write (runner character save for
// both sources, Depot consumption) restores the previous appearance, for the book
// and for an ordinary costume sharing the same path.
func TestItemAppearance_RestoredWhenTransactionFails(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name   string
		fail   bookFailures
		source string
	}{
		{"character save/inventory", bookFailures{characterSave: true}, "inventory"},
		{"character save/depot", bookFailures{characterSave: true}, "depot"},
		{"depot consume", bookFailures{depotConsume: true}, "depot"},
	} {
		f := newBookFixture(t, tc.fail)
		for _, def := range []string{bookDefinition, "item-044"} {
			c := f.character(t, "f")
			before := costume.ActiveCostume{CharacterID: c.ID, ItemNo: 51, ItemName: "previous", Icon: "chr/003.gif",
				RentedAt: time.Now().UTC().Add(-time.Minute)}
			if err := f.costume.repo.SaveActiveCostume(ctx, before); err != nil {
				t.Fatal(err)
			}
			id := f.stockItem(t, c, tc.source, def)
			if _, err := f.svc.UseHomeItem(ctx, c.ID, id, tc.source); !errors.Is(err, errInjectedBook) {
				t.Fatalf("%s %s: want injected failure, got %v", tc.name, def, err)
			}
			st, err := f.presence.State(ctx, c.ID)
			if err != nil || st.LoginMessage != "" || !f.held(t, c, tc.source, id) || f.newsCount(t, c) != 0 {
				t.Fatalf("%s %s: partial commit message=%q held=%v", tc.name, def, st.LoginMessage, f.held(t, c, tc.source, id))
			}
			if got := f.costume.active(t, c.ID); !sameCostume(got, before) {
				t.Fatalf("%s %s: appearance not restored: %+v", tc.name, def, got)
			}
		}
		if f.costume.calls.Load() == 0 {
			t.Fatalf("%s: the failure must happen after the appearance write", tc.name)
		}
	}
}

// A newer appearance stored before the failed use is compensated (here written
// directly, as a later locked writer or a reset would) is never overwritten.
func TestItemAppearance_UndoKeepsNewerChange(t *testing.T) {
	f := newBookFixture(t, bookFailures{depotConsume: true})
	ctx := context.Background()
	c := f.character(t, "m")
	newer := costume.ActiveCostume{CharacterID: c.ID, ItemNo: 52, ItemName: "newer", Icon: "chr/015.gif",
		RentedAt: time.Now().UTC()}
	f.beforeDepotFail = func(id string) {
		if err := f.costume.repo.SaveActiveCostume(ctx, newer); err != nil {
			t.Error(err)
		}
	}
	id := f.stock(t, c, "depot")
	if _, err := f.svc.UseHomeItem(ctx, c.ID, id, "depot"); !errors.Is(err, errInjectedBook) {
		t.Fatalf("want injected failure, got %v", err)
	}
	if got := f.costume.active(t, c.ID); !sameCostume(got, newer) {
		t.Fatalf("undo clobbered the newer appearance: %+v", got)
	}
	// A reset (cleared costume) is equally kept.
	f.beforeDepotFail = func(id string) {
		if err := f.costume.repo.ClearActiveCostume(ctx, id); err != nil {
			t.Error(err)
		}
	}
	c2 := f.character(t, "f")
	id = f.stock(t, c2, "depot")
	if _, err := f.svc.UseHomeItem(ctx, c2.ID, id, "depot"); !errors.Is(err, errInjectedBook) {
		t.Fatalf("want injected failure, got %v", err)
	}
	if got := f.costume.active(t, c2.ID); got != nil {
		t.Fatalf("undo revived a reset appearance: %+v", got)
	}
}

// The appearance is written while the item transaction holds the character row
// lock, so it is ordered with every other character-locked appearance writer in
// database commit order; an appearance-store failure rolls the use back.
func TestItemAppearance_WrittenUnderCharacterLockAndStoreFailureRollsBack(t *testing.T) {
	f := newBookFixture(t, bookFailures{})
	ctx := context.Background()
	for _, source := range []string{"inventory", "depot"} {
		c := f.character(t, "m")
		locked := false
		f.costume.onApply = func(id string) {
			_, err := f.db.ExecContext(ctx, `SELECT id FROM characters WHERE id = ? FOR UPDATE NOWAIT`, id)
			locked = err != nil
		}
		if _, err := f.svc.UseHomeItem(ctx, c.ID, f.stock(t, c, source), source); err != nil {
			t.Fatal(err)
		}
		if !locked {
			t.Fatalf("%s: appearance written without the character lock held", source)
		}
	}
	f.costume.onApply = nil

	// The store's cause propagates (a 5xx at the Gateway), never ErrCannotUseHere,
	// for the book and an ordinary costume; a healthy retry then consumes it.
	for _, def := range []string{bookDefinition, "item-044"} {
		for _, source := range []string{"inventory", "depot"} {
			f.costume.fail = true
			c := f.character(t, "f")
			id := f.stockItem(t, c, source, def)
			_, err := f.svc.UseHomeItem(ctx, c.ID, id, source)
			if !errors.Is(err, errInjectedAppearance) || errors.Is(err, home.ErrCannotUseHere) {
				t.Fatalf("%s %s: want the store cause, got %v", def, source, err)
			}
			st, _ := f.presence.State(ctx, c.ID)
			if st.LoginMessage != "" || !f.held(t, c, source, id) || f.newsCount(t, c) != 0 || f.costume.active(t, c.ID) != nil {
				t.Fatalf("%s %s: use not rolled back", def, source)
			}
			f.costume.fail = false
			if res, err := f.svc.UseHomeItem(ctx, c.ID, id, source); err != nil || !res.Consumed || f.held(t, c, source, id) {
				t.Fatalf("%s %s: healthy retry %+v %v", def, source, res, err)
			}
		}
	}
}

// Overlapping failed uses cannot observe or resurrect each other's uncommitted
// appearance because rollback hooks finish before the transaction releases the
// character row lock.
func TestItemAppearance_OverlappingFailedUsesPreservePriorState(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name      string
		withPrior bool
	}{
		{"without prior appearance", false},
		{"with committed prior appearance", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newBookFixture(t, bookFailures{characterSave: true})
			c := f.character(t, "m")
			var priorCostume *costume.ActiveCostume
			if tc.withPrior {
				f.charSaveFail.Store(false)
				priorID := f.stockItemWithID(t, c, "inventory", id.New(), "item-044")
				if _, err := f.svc.UseHomeItem(ctx, c.ID, priorID, "inventory"); err != nil {
					t.Fatalf("seed prior costume: %v", err)
				}
				priorCostume = f.costume.active(t, c.ID)
				if priorCostume == nil || priorCostume.ItemNo != 44 {
					t.Fatalf("unexpected seeded costume: %+v", priorCostume)
				}
				f.charSaveFail.Store(true)
			}

			itemA := f.stockItemWithID(t, c, "inventory", id.New(), bookDefinition)
			itemB := f.stockItemWithID(t, c, "depot", id.New(), "item-045")

			undoEntered := make(chan struct{})
			releaseUndo := make(chan struct{})
			bApplied := make(chan struct{})
			var undoHeldLock atomic.Bool
			var hookCalls atomic.Int32

			f.costume.mu.Lock()
			f.costume.onApply = func(charID string) {
				if hookCalls.Add(1) == 2 {
					close(bApplied)
				}
			}
			var once sync.Once
			f.costume.wrapUndo = func(charID string, undo func(context.Context) error) func(context.Context) error {
				return func(rbCtx context.Context) error {
					once.Do(func() {
						_, lockErr := f.db.ExecContext(ctx, `SELECT id FROM characters WHERE id = ? FOR UPDATE NOWAIT`, charID)
						undoHeldLock.Store(lockErr != nil)
						close(undoEntered)
						<-releaseUndo
					})
					return undo(rbCtx)
				}
			}
			f.costume.mu.Unlock()

			errA := make(chan error, 1)
			go func() {
				_, err := f.svc.UseHomeItem(ctx, c.ID, itemA, "inventory")
				errA <- err
			}()

			<-undoEntered
			if !undoHeldLock.Load() {
				close(releaseUndo)
				<-errA
				t.Fatal("A's undo ran after the character row lock was released")
			}

			bStarted := make(chan struct{})
			errB := make(chan error, 1)
			go func() {
				close(bStarted)
				_, err := f.svc.UseHomeItem(ctx, c.ID, itemB, "depot")
				errB <- err
			}()
			<-bStarted

			timer := time.NewTimer(100 * time.Millisecond)
			defer timer.Stop()
			select {
			case <-bApplied:
				close(releaseUndo)
				<-errA
				<-errB
				t.Fatal("B entered ApplyCostume while A's undo still held the character lock")
			case <-timer.C:
			}

			close(releaseUndo)
			if gotErrA := <-errA; !errors.Is(gotErrA, errInjectedBook) {
				t.Fatalf("want A error errInjectedBook, got %v", gotErrA)
			}
			if gotErrB := <-errB; !errors.Is(gotErrB, errInjectedBook) {
				t.Fatalf("want B error errInjectedBook, got %v", gotErrB)
			}
			select {
			case <-bApplied:
			default:
				t.Fatal("B never entered ApplyCostume after A released lock")
			}

			got := f.costume.active(t, c.ID)
			if priorCostume == nil {
				if got != nil {
					t.Fatalf("expected nil appearance after overlapping failures, got %+v", got)
				}
			} else if !sameCostume(got, *priorCostume) {
				t.Fatalf("expected prior costume %+v restored, got %+v", priorCostume, got)
			}

			st, err := f.presence.State(ctx, c.ID)
			if err != nil || st.LoginMessage != "" || !f.held(t, c, "inventory", itemA) || !f.held(t, c, "depot", itemB) || f.newsCount(t, c) != 0 {
				t.Fatalf("SQL state mutated on rollback: message=%q heldA=%v heldB=%v news=%d err=%v",
					st.LoginMessage, f.held(t, c, "inventory", itemA), f.held(t, c, "depot", itemB), f.newsCount(t, c), err)
			}
		})
	}
}

// When UseHomeItem succeeds inside an ambient outer database.RunInTx that
// later rolls back, promoted hooks restore Valkey appearance before releasing
// the SQL lock and propagate any undo errors.
func TestItemAppearance_AmbientTxRollbackRestoresPriorStateAndJoinsUndoError(t *testing.T) {
	ctx := context.Background()
	f := newBookFixture(t, bookFailures{})
	outerErr := errors.New("outer ambient failure")
	undoErr := errors.New("injected undo error")

	for _, withPrior := range []bool{false, true} {
		name := "without prior"
		if withPrior {
			name = "with prior"
		}
		t.Run(name, func(t *testing.T) {
			c := f.character(t, "f")
			var priorCostume *costume.ActiveCostume
			if withPrior {
				priorID := f.stockItemWithID(t, c, "inventory", id.New(), "item-044")
				if _, err := f.svc.UseHomeItem(ctx, c.ID, priorID, "inventory"); err != nil {
					t.Fatalf("seed prior costume: %v", err)
				}
				priorCostume = f.costume.active(t, c.ID)
			}

			bookID := f.stockItemWithID(t, c, "inventory", id.New(), bookDefinition)
			var undoLocked atomic.Bool
			var undoCalls atomic.Int32
			f.costume.mu.Lock()
			f.costume.wrapUndo = func(charID string, undo func(context.Context) error) func(context.Context) error {
				return func(rbCtx context.Context) error {
					undoCalls.Add(1)
					_, lockErr := f.db.ExecContext(ctx, `SELECT id FROM characters WHERE id = ? FOR UPDATE NOWAIT`, charID)
					undoLocked.Store(lockErr != nil)
					return undo(rbCtx)
				}
			}
			f.costume.mu.Unlock()

			err := database.RunInTx(ctx, f.db, func(txCtx context.Context) error {
				if _, err := f.svc.UseHomeItem(txCtx, c.ID, bookID, "inventory"); err != nil {
					return err
				}
				return outerErr
			})
			if !errors.Is(err, outerErr) {
				t.Fatalf("want outerErr, got %v", err)
			}
			if undoCalls.Load() != 1 || !undoLocked.Load() {
				t.Fatalf("expected 1 undo call under lock, calls=%d locked=%v", undoCalls.Load(), undoLocked.Load())
			}
			got := f.costume.active(t, c.ID)
			if priorCostume == nil {
				if got != nil {
					t.Fatalf("want nil after ambient rollback, got %+v", got)
				}
			} else if !sameCostume(got, *priorCostume) {
				t.Fatalf("want prior %+v after ambient rollback, got %+v", priorCostume, got)
			}
			st, err := f.presence.State(ctx, c.ID)
			if err != nil || st.LoginMessage != "" || !f.held(t, c, "inventory", bookID) || f.newsCount(t, c) != 0 {
				t.Fatalf("SQL state not rolled back: msg=%q held=%v news=%d err=%v",
					st.LoginMessage, f.held(t, c, "inventory", bookID), f.newsCount(t, c), err)
			}
		})
	}

	t.Run("undo error joined on ambient rollback", func(t *testing.T) {
		c := f.character(t, "m")
		bookID := f.stockItemWithID(t, c, "inventory", id.New(), bookDefinition)
		f.costume.mu.Lock()
		f.costume.wrapUndo = func(_ string, undo func(context.Context) error) func(context.Context) error {
			return func(rbCtx context.Context) error {
				return errors.Join(undo(rbCtx), undoErr)
			}
		}
		f.costume.mu.Unlock()

		err := database.RunInTx(ctx, f.db, func(txCtx context.Context) error {
			if _, err := f.svc.UseHomeItem(txCtx, c.ID, bookID, "inventory"); err != nil {
				return err
			}
			return outerErr
		})
		if !errors.Is(err, outerErr) || !errors.Is(err, undoErr) {
			t.Fatalf("expected joined outerErr and undoErr, got %v", err)
		}
	})
}

// When a writer's context is canceled after modifying external Valkey state
// (either mid-transaction or right before SQL commit), the SQL row lock remains
// held until compensation finishes so concurrent writers cannot observe or
// inherit the failed appearance.
func TestItemAppearance_ContextCancelHoldsLockUntilUndoAndBlocksConcurrentWriter(t *testing.T) {
	ctx := context.Background()

	for _, tc := range []struct {
		name      string
		withPrior bool
	}{
		{"without prior appearance", false},
		{"with committed prior appearance", true},
	} {
		t.Run("cancel after appearance write/"+tc.name, func(t *testing.T) {
			f := newBookFixture(t, bookFailures{})
			c := f.character(t, "m")
			var priorCostume *costume.ActiveCostume
			if tc.withPrior {
				priorID := f.stockItemWithID(t, c, "inventory", id.New(), "item-044")
				if _, err := f.svc.UseHomeItem(ctx, c.ID, priorID, "inventory"); err != nil {
					t.Fatalf("seed prior costume: %v", err)
				}
				priorCostume = f.costume.active(t, c.ID)
			}

			itemA := f.stockItemWithID(t, c, "inventory", id.New(), bookDefinition)
			itemB := f.stockItemWithID(t, c, "depot", id.New(), "item-045")

			ctxA, cancelA := context.WithCancel(ctx)
			defer cancelA()

			undoEntered := make(chan struct{})
			releaseUndo := make(chan struct{})
			bApplied := make(chan struct{})
			var undoHeldLock atomic.Bool
			var applyCalls atomic.Int32
			var wrapCalls atomic.Int32

			f.costume.mu.Lock()
			f.costume.onApply = func(string) {
				if applyCalls.Add(1) == 2 {
					close(bApplied)
				}
			}
			f.costume.wrapUndo = func(charID string, undo func(context.Context) error) func(context.Context) error {
				callIdx := wrapCalls.Add(1)
				if callIdx != 1 {
					return undo
				}
				// Cancel writer A right after A writes Valkey appearance, before subsequent SQL steps complete.
				cancelA()
				var once sync.Once
				return func(rbCtx context.Context) error {
					once.Do(func() {
						_, lockErr := f.db.ExecContext(ctx, `SELECT id FROM characters WHERE id = ? FOR UPDATE NOWAIT`, charID)
						undoHeldLock.Store(lockErr != nil)
						close(undoEntered)
						<-releaseUndo
					})
					return undo(rbCtx)
				}
			}
			f.costume.mu.Unlock()

			errA := make(chan error, 1)
			go func() {
				_, err := f.svc.UseHomeItem(ctxA, c.ID, itemA, "inventory")
				errA <- err
			}()

			<-undoEntered
			if !undoHeldLock.Load() {
				close(releaseUndo)
				<-errA
				t.Fatal("A's undo ran after the character row lock was released on context cancel")
			}

			f.charSaveFail.Store(true)
			bStarted := make(chan struct{})
			errB := make(chan error, 1)
			go func() {
				close(bStarted)
				_, err := f.svc.UseHomeItem(ctx, c.ID, itemB, "depot")
				errB <- err
			}()
			<-bStarted

			timer := time.NewTimer(100 * time.Millisecond)
			defer timer.Stop()
			select {
			case <-bApplied:
				close(releaseUndo)
				<-errA
				<-errB
				t.Fatal("B entered ApplyCostume while canceled writer A's undo still held the character lock")
			case <-timer.C:
			}

			close(releaseUndo)
			if gotErrA := <-errA; !errors.Is(gotErrA, context.Canceled) {
				t.Fatalf("want A error context.Canceled, got %v", gotErrA)
			}
			if gotErrB := <-errB; !errors.Is(gotErrB, errInjectedBook) {
				t.Fatalf("want B error errInjectedBook, got %v", gotErrB)
			}
			select {
			case <-bApplied:
			default:
				t.Fatal("B never entered ApplyCostume after A released lock")
			}

			got := f.costume.active(t, c.ID)
			if priorCostume == nil {
				if got != nil {
					t.Fatalf("expected nil appearance after cancel + concurrent failure, got %+v", got)
				}
			} else if !sameCostume(got, *priorCostume) {
				t.Fatalf("expected prior costume %+v restored, got %+v", priorCostume, got)
			}

			st, err := f.presence.State(ctx, c.ID)
			if err != nil || st.LoginMessage != "" || !f.held(t, c, "inventory", itemA) || !f.held(t, c, "depot", itemB) || f.newsCount(t, c) != 0 {
				t.Fatalf("SQL state mutated on canceled rollback: message=%q heldA=%v heldB=%v news=%d err=%v",
					st.LoginMessage, f.held(t, c, "inventory", itemA), f.held(t, c, "depot", itemB), f.newsCount(t, c), err)
			}
		})
	}

	t.Run("cancel after final SQL update before commit", func(t *testing.T) {
		f := newBookFixture(t, bookFailures{})
		c := f.character(t, "f")
		priorID := f.stockItemWithID(t, c, "inventory", id.New(), "item-044")
		if _, err := f.svc.UseHomeItem(ctx, c.ID, priorID, "inventory"); err != nil {
			t.Fatalf("seed prior costume: %v", err)
		}
		priorCostume := f.costume.active(t, c.ID)

		bookID := f.stockItemWithID(t, c, "depot", id.New(), bookDefinition)
		ctxA, cancelA := context.WithCancel(ctx)
		defer cancelA()

		var undoCalls atomic.Int32
		var undoLocked atomic.Bool
		f.costume.mu.Lock()
		f.costume.wrapUndo = func(charID string, undo func(context.Context) error) func(context.Context) error {
			return func(rbCtx context.Context) error {
				undoCalls.Add(1)
				_, lockErr := f.db.ExecContext(ctx, `SELECT id FROM characters WHERE id = ? FOR UPDATE NOWAIT`, charID)
				undoLocked.Store(lockErr != nil)
				return undo(rbCtx)
			}
		}
		f.costume.mu.Unlock()

		cancelAfterSQL := func(context.Context) { cancelA() }
		f.afterInvSave.Store(&cancelAfterSQL)
		defer f.afterInvSave.Store(nil)

		_, err := f.svc.UseHomeItem(ctxA, c.ID, bookID, "depot")
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("want context.Canceled, got %v", err)
		}
		if undoCalls.Load() != 1 || !undoLocked.Load() {
			t.Fatalf("expected 1 undo call under lock, calls=%d locked=%v", undoCalls.Load(), undoLocked.Load())
		}
		if got := f.costume.active(t, c.ID); !sameCostume(got, *priorCostume) {
			t.Fatalf("expected prior costume %+v restored, got %+v", priorCostume, got)
		}
		st, err := f.presence.State(ctx, c.ID)
		if err != nil || st.LoginMessage != "" || !f.held(t, c, "depot", bookID) || f.newsCount(t, c) != 0 {
			t.Fatalf("SQL state mutated on pre-commit cancel: message=%q held=%v news=%d err=%v",
				st.LoginMessage, f.held(t, c, "depot", bookID), f.newsCount(t, c), err)
		}
	})
}

// When root SQL commit fails after SQL writes complete, commit outcome is
// indeterminate (or committed) after lock release: Valkey compensation must not
// run and the commit error must propagate directly without being masked as no_effect.
func TestItemAppearance_CommitErrorSkipsRollbackAndPropagatesError(t *testing.T) {
	ctx := context.Background()
	f := newBookFixture(t, bookFailures{})
	c := f.character(t, "m")
	priorID := f.stockItemWithID(t, c, "inventory", id.New(), "item-044")
	if _, err := f.svc.UseHomeItem(ctx, c.ID, priorID, "inventory"); err != nil {
		t.Fatalf("seed prior costume: %v", err)
	}

	bookID := f.stockItemWithID(t, c, "depot", id.New(), bookDefinition)
	var undoCalls atomic.Int32
	f.costume.mu.Lock()
	f.costume.wrapUndo = func(_ string, undo func(context.Context) error) func(context.Context) error {
		return func(rbCtx context.Context) error {
			undoCalls.Add(1)
			return undo(rbCtx)
		}
	}
	f.costume.mu.Unlock()

	forceCommitErr := func(txCtx context.Context) {
		if tx := database.TxFromContext(txCtx); tx != nil {
			_ = tx.Commit()
		}
	}
	f.afterInvSave.Store(&forceCommitErr)
	defer f.afterInvSave.Store(nil)

	res, err := f.svc.UseHomeItem(ctx, c.ID, bookID, "depot")
	if !errors.Is(err, sql.ErrTxDone) || res != nil {
		t.Fatalf("want (nil, sql.ErrTxDone), got res=%+v err=%v", res, err)
	}
	if undoCalls.Load() != 0 {
		t.Fatalf("expected 0 rollback hook invocations on commit error, got %d", undoCalls.Load())
	}
	got := f.costume.active(t, c.ID)
	if got == nil || got.ItemNo != 58 {
		t.Fatalf("expected Valkey appearance retained on committed SQL + commit error, got %+v", got)
	}
	st, err := f.presence.State(ctx, c.ID)
	if err != nil || st.LoginMessage != "性格：ムッツリスケベ" || f.held(t, c, "depot", bookID) || f.newsCount(t, c) != 1 {
		t.Fatalf("committed SQL state unexpected: message=%q held=%v news=%d err=%v",
			st.LoginMessage, f.held(t, c, "depot", bookID), f.newsCount(t, c), err)
	}
}
