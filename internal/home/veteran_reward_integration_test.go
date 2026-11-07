package home_test

import (
	"context"
	"errors"
	"os"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/economy"
	"github.com/witchcraze/party2re/internal/home"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/inventory"
)

type veteranFloatRNG struct{ f float64 }

func (r veteranFloatRNG) Intn(int) int     { return 0 }
func (r veteranFloatRNG) IntN(int) int     { return 0 }
func (r veteranFloatRNG) Float64() float64 { return r.f }

type failingConsumeDepot struct{ home.DepotManager }

func (failingConsumeDepot) ConsumeOne(context.Context, string, string) error {
	return errors.New("injected depot write failure")
}

type veteranFixture struct {
	char      corecharacter.Character
	charRepo  *database.CharacterRepository
	invRepo   *database.InventoryRepository
	depotRepo *database.DepotRepository
	ids       map[string]string
	newSvc    func(dm home.DepotManager) *home.Service
}

// newVeteranFixture stores item-237 in inventory and in a full Depot.
func newVeteranFixture(t *testing.T, level, exp, gold int) veteranFixture {
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
	char, err := database.CreateTestCharacter(ctx, db, "Veteran")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, q := range []string{
			"DELETE FROM inventory_items WHERE character_id = ?",
			"DELETE FROM depot_items WHERE character_id = ?",
			"DELETE FROM characters WHERE id = ?",
		} {
			_, _ = db.ExecContext(context.Background(), q, char.ID)
		}
		_, _ = db.ExecContext(context.Background(), "DELETE FROM players WHERE id = ?", char.PlayerID)
	})
	f := veteranFixture{ids: map[string]string{"inventory": id.New(), "depot": id.New()}}
	if f.charRepo, err = database.NewCharacterRepository(db); err != nil {
		t.Fatal(err)
	}
	if f.invRepo, err = database.NewInventoryRepository(db); err != nil {
		t.Fatal(err)
	}
	if f.depotRepo, err = database.NewDepotRepository(db); err != nil {
		t.Fatal(err)
	}
	char.Level, char.Experience, char.Money = level, exp, gold
	if err := f.charRepo.Update(ctx, char); err != nil {
		t.Fatal(err)
	}
	f.char = char
	inv, err := coreinventory.New(char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := inv.Add(coreitem.Instance{ID: f.ids["inventory"], DefinitionID: "item-237", Quantity: 1}); err != nil {
		t.Fatal(err)
	}
	if err := f.invRepo.Save(ctx, inv); err != nil {
		t.Fatal(err)
	}
	dp, err := depot.NewDepot(char.ID)
	if err != nil {
		t.Fatal(err)
	}
	dp.Items = append(dp.Items, coreitem.Instance{ID: f.ids["depot"], DefinitionID: "item-237", Quantity: 1})
	for len(dp.Items) < dp.Capacity {
		dp.Items = append(dp.Items, coreitem.Instance{ID: id.New(), DefinitionID: "item-018", Quantity: 1})
	}
	if err := f.depotRepo.Save(ctx, dp); err != nil {
		t.Fatal(err)
	}
	eco, err := economy.NewService(f.charRepo, f.invRepo, economy.WithTransactionProvider(database.NewTransactionProvider(db)))
	if err != nil {
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
	f.newSvc = func(dm home.DepotManager) *home.Service {
		svc, err := home.NewService(homeRepo, f.charRepo,
			home.WithEconomy(eco),
			home.WithInventoryManager(invService),
			home.WithDepotManager(dm),
			home.WithItemCatalog(cat),
			home.WithRNG(veteranFloatRNG{f: 0}),
		)
		if err != nil {
			t.Fatal(err)
		}
		return svc
	}
	return f
}

func (f veteranFixture) state(t *testing.T, source string) (corecharacter.Character, bool) {
	t.Helper()
	ctx := context.Background()
	c, err := f.charRepo.FindByID(ctx, f.char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if source == "inventory" {
		inv, err := f.invRepo.FindByCharacterID(ctx, f.char.ID)
		if err != nil {
			t.Fatal(err)
		}
		_, ok := inv.Find(f.ids[source])
		return c, ok
	}
	dp, err := f.depotRepo.FindByCharacterID(ctx, f.char.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range dp.Items {
		if it.ID == f.ids[source] {
			return c, true
		}
	}
	return c, false
}

func TestHomeVeteranReward_Integration_PersistsBothSources(t *testing.T) {
	cases := []struct {
		name         string
		exp, gold    int
		wantGold     int
		wantConsumed bool
	}{
		{"keeps item and still mutates", 98000, 100, 90, false},
		{"negative gold persists", 96040, 100, 100 - 1970, false},
		{"consumes at full Depot", 98020, 100, 110, true},
	}
	for _, tc := range cases {
		for _, source := range []string{"inventory", "depot"} {
			t.Run(tc.name+"/"+source, func(t *testing.T) {
				f := newVeteranFixture(t, 99, tc.exp, tc.gold)
				svc := f.newSvc(&testDepotManagerAdapter{repo: f.depotRepo})
				res, err := svc.UseHomeItem(context.Background(), f.char.ID, f.ids[source], source)
				if err != nil {
					t.Fatalf("UseHomeItem: %v", err)
				}
				c, owned := f.state(t, source)
				if res.Consumed != tc.wantConsumed || owned == tc.wantConsumed {
					t.Fatalf("consumed=%v owned=%v, want consumed=%v", res.Consumed, owned, tc.wantConsumed)
				}
				if c.Experience != 98010 || c.Money != tc.wantGold {
					t.Fatalf("stored exp/gold = %d/%d, want 98010/%d", c.Experience, c.Money, tc.wantGold)
				}
			})
		}
	}
}

func TestHomeVeteranReward_Integration_DepotFailureRollsBack(t *testing.T) {
	f := newVeteranFixture(t, 99, 98020, 100)
	svc := f.newSvc(failingConsumeDepot{&testDepotManagerAdapter{repo: f.depotRepo}})
	if _, err := svc.UseHomeItem(context.Background(), f.char.ID, f.ids["depot"], "depot"); err == nil {
		t.Fatal("expected injected failure")
	}
	c, owned := f.state(t, "depot")
	if !owned || c.Experience != 98020 || c.Money != 100 {
		t.Fatalf("failure must roll back: owned=%v exp=%d gold=%d", owned, c.Experience, c.Money)
	}
}
