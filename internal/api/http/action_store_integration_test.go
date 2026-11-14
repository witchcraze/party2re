package http

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"testing"

	"github.com/witchcraze/party2re/internal/character"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/playercontext"
	"github.com/witchcraze/party2re/internal/store"
	"github.com/witchcraze/party2re/internal/testutil"
)

type failingGuildPoints struct{ err error }

func (g *failingGuildPoints) AddGuildPoints(context.Context, string, int) error { return g.err }

type storeDB struct {
	db      *sql.DB
	chars   *database.CharacterRepository
	depots  *database.DepotRepository
	items   *item.Catalog
	guild   *failingGuildPoints
	service *store.Service
}

func newStoreDB(t *testing.T) *storeDB {
	t.Helper()
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	source := db
	t.Cleanup(func() {
		if err := source.Close(); err != nil {
			t.Error(err)
		}
	})
	db = isolatedStoreGatewayDB(t, source)
	f := &storeDB{db: db, guild: &failingGuildPoints{}}
	if f.chars, err = database.NewCharacterRepository(db); err != nil {
		t.Fatal(err)
	}
	if f.depots, err = database.NewDepotRepository(db); err != nil {
		t.Fatal(err)
	}
	repo, err := database.NewStoreRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	if f.items, err = item.InitialCatalog(); err != nil {
		t.Fatal(err)
	}
	f.service = store.NewService(repo, f.chars, f.depots, f.items, database.NewTransactionProvider(db), store.WithGuildPoints(f.guild), store.WithDeliveryService(f))
	return f
}

func (f *storeDB) HoldPayment(ctx context.Context, recipientID, buyerName string, amount int) error {
	return f.depots.AddRemittance(ctx, depot.Remittance{ID: id.New(), CharacterID: recipientID,
		Amount: amount, Source: depot.RemittanceStorePayment, SenderName: buyerName})
}

// Home notices are verified with the production adapter in cmd/party2 tests.
func (*storeDB) NotifyItem(context.Context, string, string, string) error { return nil }

func (f *storeDB) heldGold(t *testing.T, ctx context.Context, actor string) int {
	t.Helper()
	values, err := f.depots.ListRemittances(ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	var total int
	for _, value := range values {
		total += value.Amount
	}
	return total
}

// character creates a character in the fixture-owned database with the given Depot items.
func (f *storeDB) character(t *testing.T, ctx context.Context, gold int, items ...item.Instance) corecharacter.Character {
	t.Helper()
	c, err := database.CreateTestCharacterWithFunds(ctx, f.db, "Store Gateway", gold)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateTestDepot(ctx, f.db, c.ID, 2, items); err != nil {
		t.Fatal(err)
	}
	return c
}

func (f *storeDB) request(t *testing.T, ctx context.Context, actor corecharacter.Character, action, params string) (int, map[string]json.RawMessage) {
	t.Helper()
	characters, err := character.NewService(f.chars)
	if err != nil {
		t.Fatal(err)
	}
	selection := playercontext.Selection{Destination: "town1"}
	if action != "store_build" {
		// Store commands belong to a selected store; the services check the sale's own store.
		selection.Subject = playercontext.Subject{Kind: "store", ID: "selected"}
	}
	pc := playercontext.NewService(f.chars, emptyActionsReader{}, timer.NewService(nil), playercontext.WithNavigation(&lockedNavigationStore{selection: selection},
		playercontext.SceneDefinition{ID: "town", Pageable: true},
		playercontext.SceneDefinition{ID: "town1", Parent: "town", SubjectKind: "store", SubjectAvailable: func(context.Context, string, string) (bool, error) { return true, nil },
			// The request helper stands in the store its command belongs to.
			SubjectOwned: func(context.Context, string, string) (bool, error) {
				return slices.Contains(storeOwnerCommands, action), nil
			}}))
	h, err := NewHandler(expansionDBPlayer{owner: actor.PlayerID}, characters, &struct{ AdventureService }{}, &struct{ ShopService }{}, WithStore(f.service), WithPlayerContext(pc))
	if err != nil {
		t.Fatal(err)
	}
	return gatewayRequest(t, h.Router(), actor.ID, "session", "application/json", saleGatewayBody(action, params), ctx)
}

func (f *storeDB) money(t *testing.T, ctx context.Context, id string) int {
	t.Helper()
	c, err := f.chars.FindByID(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	return c.Money
}

func (f *storeDB) depot(t *testing.T, ctx context.Context, id string) []item.Instance {
	t.Helper()
	dep, err := f.depots.FindByCharacterID(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	return dep.Items
}

func instance(t *testing.T, def string, level int) item.Instance {
	t.Helper()
	inst, err := item.NewInstanceWithEnhancement(def, 1, level)
	if err != nil {
		t.Fatal(err)
	}
	return inst
}

func (f *storeDB) build(t *testing.T, ctx context.Context, owner corecharacter.Character) {
	t.Helper()
	if status, got := f.request(t, ctx, owner, "store_build", `{"town_id":"town1","house_style":"001"}`); status != 200 {
		t.Fatalf("build: %d %s", status, got)
	}
}

func (f *storeDB) list(t *testing.T, ctx context.Context, owner corecharacter.Character, action, params string) store.Sale {
	t.Helper()
	status, got := f.request(t, ctx, owner, action, params)
	if status != 200 {
		t.Fatalf("%s: %d %s", action, status, got)
	}
	var sale store.Sale
	if err := json.Unmarshal(got["result"], &sale); err != nil {
		t.Fatal(err)
	}
	return sale
}

func TestStoreGatewayBuyAndTradeThroughRealService(t *testing.T) {
	ctx := context.Background()
	f := newStoreDB(t)
	sword, shield, herb := instance(t, "weapon-01", 4), instance(t, "armor-01", 0), instance(t, "item-001", 0)
	herbDef, err := f.items.FindByID(herb.DefinitionID)
	if err != nil {
		t.Fatal(err)
	}
	owner := f.character(t, ctx, store.StorePrice+1000, sword, shield)
	buyer := f.character(t, ctx, 1000, herb)

	f.build(t, ctx, owner)
	sale := f.list(t, ctx, owner, "store_list_gold", fmt.Sprintf(`{"item_id":%q,"price":300}`, sword.ID))
	barter := f.list(t, ctx, owner, "store_list_barter", fmt.Sprintf(`{"item_id":%q,"wish_item_name":%q}`, shield.ID, herbDef.Name))
	if status, got := f.request(t, ctx, buyer, "store_buy", fmt.Sprintf(`{"sale_id":%q}`, sale.ID)); status != 200 {
		t.Fatalf("buy: %d %s", status, got)
	}
	if o, b := f.money(t, ctx, owner.ID), f.money(t, ctx, buyer.ID); o != 1000 || b != 700 {
		t.Fatalf("gold: owner=%d buyer=%d", o, b)
	}
	if held := f.heldGold(t, ctx, owner.ID); held != 300 {
		t.Fatalf("held payment: %d", held)
	}
	status, got := f.request(t, ctx, buyer, "store_buy", fmt.Sprintf(`{"sale_id":%q}`, sale.ID))
	if status != 404 {
		t.Fatalf("repeat buy: %d %s", status, got)
	}
	assertGatewayError(t, got, "STORE_LISTING_NOT_FOUND")

	if status, got := f.request(t, ctx, buyer, "store_trade", fmt.Sprintf(`{"sale_id":%q,"item_id":%q}`, barter.ID, herb.ID)); status != 200 {
		t.Fatalf("trade: %d %s", status, got)
	}
	buyerItems := f.depot(t, ctx, buyer.ID)
	ownerItems := f.depot(t, ctx, owner.ID)
	if len(buyerItems) != 2 || len(ownerItems) != 1 || ownerItems[0].DefinitionID != herb.DefinitionID {
		t.Fatalf("depots: buyer=%+v owner=%+v", buyerItems, ownerItems)
	}
	for _, it := range buyerItems {
		if it.DefinitionID == sword.DefinitionID && it.EnhancementLevel != 4 {
			t.Fatalf("enhancement lost: %+v", it)
		}
	}
}

func TestStoreGatewayFailedBuildRollsBack(t *testing.T) {
	ctx := context.Background()
	f := newStoreDB(t)
	owner := f.character(t, ctx, store.StorePrice)
	f.guild.err = errors.New("private guild failure")
	status, got := f.request(t, ctx, owner, "store_build", `{"town_id":"town1","house_style":"001"}`)
	if status != 500 {
		t.Fatalf("build failure: %d %s", status, got)
	}
	assertGatewayError(t, got, "EXECUTION_FAILED")
	var stores int
	if err := f.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM character_stores WHERE character_id = ?`, owner.ID).Scan(&stores); err != nil {
		t.Fatal(err)
	}
	if stores != 0 || f.money(t, ctx, owner.ID) != store.StorePrice {
		t.Fatalf("partial build: stores=%d gold=%d", stores, f.money(t, ctx, owner.ID))
	}
	f.guild.err = nil
	f.build(t, ctx, owner)
}

func TestStoreGatewayConcurrentPurchasesConserveAssets(t *testing.T) {
	ctx := context.Background()
	f := newStoreDB(t)
	sword := instance(t, "weapon-01", 2)
	owner := f.character(t, ctx, store.StorePrice, sword)
	buyer, rival := f.character(t, ctx, 1000), f.character(t, ctx, 1000)
	f.build(t, ctx, owner)
	sale := f.list(t, ctx, owner, "store_list_gold", fmt.Sprintf(`{"item_id":%q,"price":400}`, sword.ID))
	body := fmt.Sprintf(`{"sale_id":%q}`, sale.ID)
	statuses := make([]int, 2)
	errs := testutil.RunRace(func() error {
		statuses[0], _ = f.request(t, ctx, buyer, "store_buy", body)
		return nil
	}, func() error {
		statuses[1], _ = f.request(t, ctx, rival, "store_buy", body)
		return nil
	})
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if !(statuses[0] == 200 && statuses[1] == 404) && !(statuses[0] == 404 && statuses[1] == 200) {
		t.Fatalf("expected one winner: %v", statuses)
	}
	if total := f.money(t, ctx, owner.ID) + f.money(t, ctx, buyer.ID) + f.money(t, ctx, rival.ID) + f.heldGold(t, ctx, owner.ID); total != 2000 {
		t.Fatalf("gold drift: %d", total)
	}
	if items := len(f.depot(t, ctx, buyer.ID)) + len(f.depot(t, ctx, rival.ID)); items != 1 {
		t.Fatalf("item drift: %d", items)
	}
}
