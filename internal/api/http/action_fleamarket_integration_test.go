package http

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/character"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/fleamarket"
	"github.com/witchcraze/party2re/internal/playercontext"
	"github.com/witchcraze/party2re/internal/testutil"
)

type failingCollectionRecorder struct{ err error }

func (r *failingCollectionRecorder) RecordItemDiscovered(context.Context, string, string, string, string) error {
	return r.err
}

type fleaMarketDB struct {
	db       *sql.DB
	chars    *database.CharacterRepository
	depots   *database.DepotRepository
	repo     *database.FleaMarketRepository
	recorder *failingCollectionRecorder
	seller   corecharacter.Character
	buyer    corecharacter.Character
	service  *fleamarket.Service
}

func newFleaMarketDB(t *testing.T, ctx context.Context) *fleaMarketDB {
	t.Helper()
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	f := &fleaMarketDB{db: db, recorder: &failingCollectionRecorder{}}
	if f.chars, err = database.NewCharacterRepository(db); err != nil {
		t.Fatal(err)
	}
	if f.depots, err = database.NewDepotRepository(db); err != nil {
		t.Fatal(err)
	}
	if f.repo, err = database.NewFleaMarketRepository(db); err != nil {
		t.Fatal(err)
	}
	for _, target := range []*corecharacter.Character{&f.seller, &f.buyer} {
		if *target, err = database.CreateTestCharacterWithFunds(ctx, db, "Flea Gateway", 1000); err != nil {
			t.Fatal(err)
		}
		id := target.ID
		t.Cleanup(func() {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			if err := f.chars.Delete(cleanupCtx, id); err != nil {
				t.Error(err)
			}
		})
		if _, err := database.CreateTestDepot(ctx, db, id, 2, nil); err != nil {
			t.Fatal(err)
		}
	}
	items, err := item.InitialCatalog()
	if err != nil {
		t.Fatal(err)
	}
	f.service, err = fleamarket.NewService(f.repo, f.chars, f.depots, fleamarket.WithTransactionProvider(database.NewTransactionProvider(db)),
		fleamarket.WithItemDefinitionProvider(items), fleamarket.WithCollectionRecorder(f.recorder))
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *fleaMarketDB) request(t *testing.T, ctx context.Context, actor corecharacter.Character, body string) (int, map[string]json.RawMessage) {
	t.Helper()
	characters, err := character.NewService(f.chars)
	if err != nil {
		t.Fatal(err)
	}
	pc := playercontext.NewService(f.chars, emptyActionsReader{}, timer.NewService(nil), playercontext.WithNavigation(&lockedNavigationStore{selection: playercontext.Selection{Destination: "fleamarket"}},
		playercontext.SceneDefinition{ID: "town", Pageable: true}, playercontext.SceneDefinition{ID: "fleamarket", Parent: "town", Pageable: true}))
	h, err := NewHandler(expansionDBPlayer{owner: actor.PlayerID}, characters, &struct{ AdventureService }{}, &struct{ ShopService }{}, WithFleaMarket(f.service), WithPlayerContext(pc))
	if err != nil {
		t.Fatal(err)
	}
	return gatewayRequest(t, h.Router(), actor.ID, "session", "application/json", body, ctx)
}

func (f *fleaMarketDB) store(t *testing.T, ctx context.Context, level int) item.Instance {
	t.Helper()
	inst, err := item.NewInstanceWithEnhancement("weapon-01", 1, level)
	if err != nil {
		t.Fatal(err)
	}
	dep, err := f.depots.FindByCharacterID(ctx, f.seller.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := dep.AddItem(inst); err != nil {
		t.Fatal(err)
	}
	if err := f.depots.Save(ctx, dep); err != nil {
		t.Fatal(err)
	}
	return inst
}

func (f *fleaMarketDB) list(t *testing.T, ctx context.Context, inst item.Instance, price int) fleamarket.Listing {
	t.Helper()
	status, got := f.request(t, ctx, f.seller, fmt.Sprintf(`{"action":"fleamarket_list","params":{"item_id":%q,"price":%d}}`, inst.ID, price))
	if status != 200 {
		t.Fatalf("list: %d %s", status, got)
	}
	var listing fleamarket.Listing
	if err := json.Unmarshal(got["result"], &listing); err != nil {
		t.Fatal(err)
	}
	return listing
}

func (f *fleaMarketDB) money(t *testing.T, ctx context.Context, id string) int {
	t.Helper()
	c, err := f.chars.FindByID(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	return c.Money
}

func TestFleaMarketGatewayListPurchaseThroughRealService(t *testing.T) {
	ctx := context.Background()
	f := newFleaMarketDB(t, ctx)
	listing := f.list(t, ctx, f.store(t, ctx, 5), 300)
	if listing.EnhancementLevel != 5 || listing.Status != fleamarket.StatusActive {
		t.Fatalf("listing: %+v", listing)
	}
	status, got := f.request(t, ctx, f.buyer, fmt.Sprintf(`{"action":"fleamarket_purchase","params":{"listing_id":%q}}`, listing.ID))
	if status != 200 {
		t.Fatalf("purchase: %d %s", status, got)
	}
	dep, err := f.depots.FindByCharacterID(ctx, f.buyer.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(dep.Items) != 1 || dep.Items[0].DefinitionID != "weapon-01" || dep.Items[0].EnhancementLevel != 5 {
		t.Fatalf("buyer item: %+v", dep.Items)
	}
	if b, s := f.money(t, ctx, f.buyer.ID), f.money(t, ctx, f.seller.ID); b != 700 || s != 1300 {
		t.Fatalf("gold: buyer=%d seller=%d", b, s)
	}
	status, got = f.request(t, ctx, f.buyer, fmt.Sprintf(`{"action":"fleamarket_purchase","params":{"listing_id":%q}}`, listing.ID))
	if status != 400 {
		t.Fatalf("repeat purchase: %d %s", status, got)
	}
	assertGatewayError(t, got, "FLEAMARKET_LISTING_NOT_ACTIVE")
}

func TestFleaMarketGatewayCancelReturnsItemAndLeavesObservation(t *testing.T) {
	ctx := context.Background()
	f := newFleaMarketDB(t, ctx)
	listing := f.list(t, ctx, f.store(t, ctx, 3), 50)
	status, got := f.request(t, ctx, f.buyer, fmt.Sprintf(`{"action":"fleamarket_cancel","params":{"listing_id":%q}}`, listing.ID))
	if status != 403 {
		t.Fatalf("foreign cancel: %d %s", status, got)
	}
	assertGatewayError(t, got, "FLEAMARKET_NOT_SELLER")
	if status, got = f.request(t, ctx, f.seller, fmt.Sprintf(`{"action":"fleamarket_cancel","params":{"listing_id":%q}}`, listing.ID)); status != 200 {
		t.Fatalf("cancel: %d %s", status, got)
	}
	dep, err := f.depots.FindByCharacterID(ctx, f.seller.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(dep.Items) != 1 || dep.Items[0].EnhancementLevel != 3 {
		t.Fatalf("returned item: %+v", dep.Items)
	}
	var ctxResponse struct {
		Scene struct {
			Data FleaMarketSceneData `json:"data"`
		} `json:"scene"`
	}
	if err := json.Unmarshal(got["context"], &ctxResponse); err != nil {
		t.Fatal(err)
	}
	if len(ctxResponse.Scene.Data.OwnedListings) != 0 {
		t.Fatalf("finished listing observed: %+v", ctxResponse.Scene.Data.OwnedListings)
	}
}

func TestFleaMarketGatewayFailedListingRollsBackDepot(t *testing.T) {
	ctx := context.Background()
	f := newFleaMarketDB(t, ctx)
	inst := f.store(t, ctx, 0)
	f.recorder.err = errors.New("private collection failure")
	status, got := f.request(t, ctx, f.seller, fmt.Sprintf(`{"action":"fleamarket_list","params":{"item_id":%q,"price":10}}`, inst.ID))
	if status != 500 {
		t.Fatalf("list failure: %d %s", status, got)
	}
	assertGatewayError(t, got, "EXECUTION_FAILED")
	dep, err := f.depots.FindByCharacterID(ctx, f.seller.ID)
	if err != nil {
		t.Fatal(err)
	}
	owned, err := f.service.GetActiveCharacterListings(ctx, f.seller.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(dep.Items) != 1 || len(owned) != 0 {
		t.Fatalf("partial listing: %+v %+v", dep.Items, owned)
	}
	f.recorder.err = nil
	if listing := f.list(t, ctx, inst, 10); listing.Status != fleamarket.StatusActive {
		t.Fatalf("retry: %+v", listing)
	}
}

func TestFleaMarketGatewayConcurrentPurchasesConserveAssets(t *testing.T) {
	ctx := context.Background()
	f := newFleaMarketDB(t, ctx)
	rival, err := database.CreateTestCharacterWithFunds(ctx, f.db, "Flea Rival", 1000)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		if err := f.chars.Delete(cleanupCtx, rival.ID); err != nil {
			t.Error(err)
		}
	})
	if _, err := database.CreateTestDepot(ctx, f.db, rival.ID, 2, nil); err != nil {
		t.Fatal(err)
	}
	listing := f.list(t, ctx, f.store(t, ctx, 2), 400)
	body := fmt.Sprintf(`{"action":"fleamarket_purchase","params":{"listing_id":%q}}`, listing.ID)
	statuses := make([]int, 2)
	errs := testutil.RunRace(func() error {
		statuses[0], _ = f.request(t, ctx, f.buyer, body)
		return nil
	}, func() error {
		statuses[1], _ = f.request(t, ctx, rival, body)
		return nil
	})
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if !(statuses[0] == 200 && statuses[1] == 400) && !(statuses[0] == 400 && statuses[1] == 200) {
		t.Fatalf("expected one winner: %v", statuses)
	}
	total := f.money(t, ctx, f.buyer.ID) + f.money(t, ctx, rival.ID) + f.money(t, ctx, f.seller.ID)
	if total != 3000 {
		t.Fatalf("gold drift: %d", total)
	}
	items := 0
	for _, id := range []string{f.buyer.ID, rival.ID} {
		dep, err := f.depots.FindByCharacterID(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		items += len(dep.Items)
	}
	if items != 1 {
		t.Fatalf("item drift: %d", items)
	}
}
