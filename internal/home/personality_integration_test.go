package home_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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
	"github.com/witchcraze/party2re/internal/inventory"
	"github.com/witchcraze/party2re/internal/notification"
	"github.com/witchcraze/party2re/internal/store"
)

const bookDefinition = "item-058"

// bookCostume wraps the production Store adapter over the in-memory costume
// repository, counting calls and optionally failing or running a hook first.
type bookCostume struct {
	store   *store.Service
	repo    *costume.MemoryCostumeRepository
	calls   int
	fail    bool
	onApply func(characterID string)
}

func (c *bookCostume) ApplyCostume(ctx context.Context, characterID string, itemNo int, itemName, icon string, exp time.Time) (func(context.Context) error, error) {
	c.calls++
	if c.fail {
		return nil, errors.New("injected appearance store failure")
	}
	if c.onApply != nil {
		c.onApply(characterID)
	}
	return c.store.ApplyCostume(ctx, characterID, itemNo, itemName, icon, exp)
}

func (c *bookCostume) active(t *testing.T, characterID string) *costume.ActiveCostume {
	t.Helper()
	got, err := c.repo.GetActiveCostume(context.Background(), characterID)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// bookFailures injects failures before commit (profile save, the runner's
// character save, Depot consumption) and after commit (appearance store).
type bookFailures struct {
	profileSave, characterSave, depotConsume, appearance bool
}

var errInjectedBook = errors.New("injected failure")

type failingEconomyCharacters struct {
	*database.CharacterRepository
}

func (r failingEconomyCharacters) Update(context.Context, corecharacter.Character) error {
	return errInjectedBook
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
	invRepo         *database.InventoryRepository
	depots          *database.DepotRepository
	charRepo        *database.CharacterRepository
	presence        *character.PresenceService
}

func newBookFixture(t *testing.T, fail bookFailures) *bookFixture {
	t.Helper()
	db := homeIsolatedDB(t)
	mem := costume.NewMemoryCostumeRepository()
	f := &bookFixture{db: db, costume: &bookCostume{
		store: store.NewService(nil, nil, nil, nil, nil, store.WithCostumeRepository(mem)),
		repo:  mem,
		fail:  fail.appearance,
	}}
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
	var ecoChars economy.CharacterRepository = f.charRepo
	if fail.characterSave {
		ecoChars = failingEconomyCharacters{f.charRepo}
	}
	eco, err := economy.NewService(ecoChars, f.invRepo, economy.WithTransactionProvider(tx))
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
	return c
}

func (f *bookFixture) stock(t *testing.T, c corecharacter.Character, source string) string {
	return f.stockItem(t, c, source, bookDefinition)
}

func (f *bookFixture) stockItem(t *testing.T, c corecharacter.Character, source, definitionID string) string {
	t.Helper()
	ctx := context.Background()
	inst := coreitem.Instance{ID: "it-" + source + "-" + c.ID[:8], DefinitionID: definitionID, Quantity: 1}
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
	// Overfill the Depot: capacity never blocks Home dispatch.
	for i := range depot.MaxDepotCapacity {
		dp.Items = append(dp.Items, coreitem.Instance{ID: fmt.Sprintf("fill-%s-%d", c.ID[:8], i), DefinitionID: "item-018", Quantity: 1})
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
			if err := f.charRepo.SaveProfile(ctx, character.Profile{CharacterID: c.ID, Comment: "hi", AvatarURL: "https://i.imgur.com/abc12.gif", AuraEffect: 2, BioData: map[string]string{"hobby": "fishing", "character": "calm"}}); err != nil {
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
		if tc.profile && (p.Comment != "hi" || p.AvatarURL != "https://i.imgur.com/abc12.gif" || p.AuraEffect != 2 || p.BioData["hobby"] != "fishing") {
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
	if f.costume.calls != 0 {
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
				RentedAt: time.Now().UTC().Add(-time.Minute), ExpiresAt: time.Now().UTC().Add(time.Hour)}
			if err := f.costume.repo.SaveActiveCostume(ctx, before, time.Hour); err != nil {
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
			if got := f.costume.active(t, c.ID); got == nil || *got != before {
				t.Fatalf("%s %s: appearance not restored: %+v", tc.name, def, got)
			}
		}
		if f.costume.calls == 0 {
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
		RentedAt: time.Now().UTC(), ExpiresAt: time.Now().UTC().Add(time.Hour)}
	f.beforeDepotFail = func(id string) {
		if err := f.costume.repo.SaveActiveCostume(ctx, newer, time.Hour); err != nil {
			t.Error(err)
		}
	}
	id := f.stock(t, c, "depot")
	if _, err := f.svc.UseHomeItem(ctx, c.ID, id, "depot"); !errors.Is(err, errInjectedBook) {
		t.Fatalf("want injected failure, got %v", err)
	}
	if got := f.costume.active(t, c.ID); got == nil || *got != newer {
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

	f.costume.fail = true
	for _, source := range []string{"inventory", "depot"} {
		c := f.character(t, "f")
		id := f.stock(t, c, source)
		if _, err := f.svc.UseHomeItem(ctx, c.ID, id, source); err == nil {
			t.Fatalf("%s: appearance failure reported as success", source)
		}
		st, _ := f.presence.State(ctx, c.ID)
		if st.LoginMessage != "" || !f.held(t, c, source, id) || f.newsCount(t, c) != 0 {
			t.Fatalf("%s: use not rolled back", source)
		}
	}
}
