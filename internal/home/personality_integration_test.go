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
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/economy"
	"github.com/witchcraze/party2re/internal/home"
	"github.com/witchcraze/party2re/internal/inventory"
	"github.com/witchcraze/party2re/internal/notification"
)

const bookDefinition = "item-058"

type bookCostume struct {
	itemNo int
	icon   string
	calls  int
}

func (c *bookCostume) ApplyCostume(_ context.Context, _ string, itemNo int, _, icon string, _ time.Time) error {
	c.itemNo, c.icon = itemNo, icon
	c.calls++
	return nil
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
	db       *sql.DB
	svc      *home.Service
	costume  *bookCostume
	invRepo  *database.InventoryRepository
	depots   *database.DepotRepository
	charRepo *database.CharacterRepository
	presence *character.PresenceService
}

func newBookFixture(t *testing.T, failProfileSave bool) *bookFixture {
	t.Helper()
	db := homeIsolatedDB(t)
	f := &bookFixture{db: db, costume: &bookCostume{}}
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
	chars, err := character.NewService(failingProfiles{f.charRepo, failProfileSave},
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
	eco, err := economy.NewService(f.charRepo, f.invRepo, economy.WithTransactionProvider(tx))
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
	f.svc, err = home.NewService(homeRepo, f.charRepo,
		home.WithEconomy(eco),
		home.WithInventoryManager(invService),
		home.WithDepotManager(&testDepotManagerAdapter{repo: f.depots}),
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
	t.Helper()
	ctx := context.Background()
	inst := coreitem.Instance{ID: "book-" + source + "-" + c.ID[:8], DefinitionID: bookDefinition, Quantity: 1}
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
	f := newBookFixture(t, false)
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
		if f.costume.itemNo != 58 || f.costume.icon != "assets/images/placeholders/job/job-default.svg#job-08-"+tc.gender {
			t.Fatalf("%s: appearance %+v", tc.source, f.costume)
		}
	}
}

func TestPersonalityBook_ProfileFailureRollsBack(t *testing.T) {
	f := newBookFixture(t, true)
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
