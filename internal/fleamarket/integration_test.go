package fleamarket_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/fleamarket"
	"github.com/witchcraze/party2re/internal/home"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/testutil"
)

type testFleaCollectionRecorder struct {
	mu      sync.Mutex
	err     error
	records []string
}

func (r *testFleaCollectionRecorder) RecordItemDiscovered(_ context.Context, characterID, itemID, itemName, category string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return r.err
	}
	r.records = append(r.records, fmt.Sprintf("%s:%s:%s:%s", characterID, itemID, itemName, category))
	return nil
}

type sqlFleaPaymentHolder struct {
	depots *database.DepotRepository
	homes  *home.Service
}

func (h sqlFleaPaymentHolder) HoldPayment(ctx context.Context, recipientID, buyerName string, amount int) error {
	if err := h.depots.AddRemittance(ctx, depot.Remittance{
		ID:          id.New(),
		CharacterID: recipientID,
		Amount:      amount,
		Source:      depot.RemittanceFleaMarketPayment,
		SenderName:  buyerName,
	}); err != nil {
		return err
	}
	return h.homes.AddDeliveryFlag(ctx, recipientID, depot.NoticeMoneyTransfer, "お金が預かり所に届いています")
}

type fleaMarketDBFixture struct {
	db       *sql.DB
	repo     *database.FleaMarketRepository
	chars    *database.CharacterRepository
	depots   *database.DepotRepository
	invs     *database.InventoryRepository
	homes    *home.Service
	catalog  *item.Catalog
	recorder *testFleaCollectionRecorder
}

func newFleaMarketDBFixture(t *testing.T) *fleaMarketDBFixture {
	t.Helper()
	db := database.CreateTestDatabase(t,
		"players",
		"characters",
		"inventory_items",
		"character_depots",
		"depot_items",
		"character_pending_remittances",
		"character_delivery_notices",
		"fleamarket_listings",
		"fleamarket_capacity_lock",
	)

	repo, err := database.NewFleaMarketRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	chars, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	depots, err := database.NewDepotRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	invs, err := database.NewInventoryRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	homeRepo, err := database.NewHomeRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	homes, err := home.NewService(homeRepo, chars)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := item.DefaultCatalog()
	if err != nil {
		t.Fatal(err)
	}
	return &fleaMarketDBFixture{
		db:       db,
		repo:     repo,
		chars:    chars,
		depots:   depots,
		invs:     invs,
		homes:    homes,
		catalog:  catalog,
		recorder: &testFleaCollectionRecorder{},
	}
}

func (f *fleaMarketDBFixture) createCharacter(t *testing.T, ctx context.Context, name string, money int) corecharacter.Character {
	t.Helper()
	char, err := database.CreateTestCharacterWithFunds(ctx, f.db, name, money)
	if err != nil {
		t.Fatal(err)
	}
	return char
}

func (f *fleaMarketDBFixture) service(t *testing.T) *fleamarket.Service {
	t.Helper()
	svc, err := fleamarket.NewService(
		f.repo,
		f.chars,
		f.depots,
		fleamarket.WithTransactionProvider(database.NewTransactionProvider(f.db)),
		fleamarket.WithInventoryRepository(f.invs),
		fleamarket.WithItemDefinitionProvider(f.catalog),
		fleamarket.WithCollectionRecorder(f.recorder),
		fleamarket.WithPaymentHolder(sqlFleaPaymentHolder{depots: f.depots, homes: f.homes}),
	)
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func TestFleaMarketListPurchaseCancel_Integration(t *testing.T) {
	f := newFleaMarketDBFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	seller := f.createCharacter(t, ctx, "FMListS", 5000)
	buyer := f.createCharacter(t, ctx, "FMListB", 10000)
	stranger := f.createCharacter(t, ctx, "FMListX", 5000)

	soldWeapon, err := item.NewInstance("weapon-01", 1)
	if err != nil {
		t.Fatal(err)
	}
	soldWeapon.EnhancementLevel = 7
	cancelledWeapon, err := item.NewInstance("weapon-01", 1)
	if err != nil {
		t.Fatal(err)
	}
	cancelledWeapon.EnhancementLevel = 4
	if _, err := database.CreateTestDepot(ctx, f.db, seller.ID, 0, []item.Instance{soldWeapon, cancelledWeapon}); err != nil {
		t.Fatal(err)
	}
	heldArmor, err := item.NewInstance("armor-01", 1)
	if err != nil {
		t.Fatal(err)
	}
	sellerInv, err := coreinventory.New(seller.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := sellerInv.Add(heldArmor); err != nil {
		t.Fatal(err)
	}
	if err := f.invs.Save(ctx, sellerInv); err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateTestDepot(ctx, f.db, buyer.ID, 0, nil); err != nil {
		t.Fatal(err)
	}

	svc := f.service(t)
	now := time.Now().UTC()

	listing1, err := svc.CreateListing(ctx, seller.ID, soldWeapon.ID, 600, now)
	if err != nil {
		t.Fatalf("CreateListing 1 failed: %v", err)
	}
	if listing1.EnhancementLevel != 7 {
		t.Fatalf("listing1 enhancement = %d, want 7", listing1.EnhancementLevel)
	}

	listing2, err := svc.CreateListing(ctx, seller.ID, cancelledWeapon.ID, 300, now)
	if err != nil {
		t.Fatalf("CreateListing 2 failed: %v", err)
	}

	purchase, err := svc.PurchaseListing(ctx, buyer.ID, listing1.ID, now)
	if err != nil {
		t.Fatalf("PurchaseListing failed: %v", err)
	}
	if purchase.BuyerGold != 9400 || purchase.ItemInstance.EnhancementLevel != 7 {
		t.Fatalf("unexpected purchase result: %+v", purchase)
	}

	if _, err := svc.PurchaseListing(ctx, buyer.ID, listing1.ID, now); !errors.Is(err, fleamarket.ErrListingNotActive) {
		t.Fatalf("repeat purchase expected ErrListingNotActive, got %v", err)
	}

	persistedBuyer, err := f.chars.FindByID(ctx, buyer.ID)
	if err != nil {
		t.Fatal(err)
	}
	persistedSeller, err := f.chars.FindByID(ctx, seller.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persistedBuyer.Money != 9400 || persistedSeller.Money != 5000 {
		t.Fatalf("gold after purchase: buyer=%d seller=%d", persistedBuyer.Money, persistedSeller.Money)
	}
	remittances, err := f.depots.ListRemittances(ctx, seller.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(remittances) != 1 || remittances[0].Amount != 600 || remittances[0].Source != depot.RemittanceFleaMarketPayment || remittances[0].SenderName != buyer.Name {
		t.Fatalf("seller remittances = %+v", remittances)
	}
	notices, err := f.homes.ListDeliveryNotices(ctx, seller.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(notices) != 1 || notices[0].NoticeType != depot.NoticeMoneyTransfer || notices[0].Message != "お金が預かり所に届いています" {
		t.Fatalf("seller notices = %+v", notices)
	}
	buyerDepot, err := f.depots.FindByCharacterID(ctx, buyer.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(buyerDepot.Items) != 1 || buyerDepot.Items[0].DefinitionID != "weapon-01" || buyerDepot.Items[0].EnhancementLevel != 7 {
		t.Fatalf("buyer depot = %+v", buyerDepot.Items)
	}
	if len(f.recorder.records) == 0 {
		t.Fatal("expected collection discovery recorded for held hand item")
	}
	for _, rec := range f.recorder.records {
		if rec != fmt.Sprintf("%s:armor-01:布の服:armor", seller.ID) {
			t.Fatalf("unexpected collection record %q (listed weapon-01 must not be registered)", rec)
		}
	}

	if _, err := svc.CancelListing(ctx, stranger.ID, listing2.ID); !errors.Is(err, fleamarket.ErrUnauthorizedSeller) {
		t.Fatalf("foreign cancel expected ErrUnauthorizedSeller, got %v", err)
	}

	cancelled, err := svc.CancelListing(ctx, seller.ID, listing2.ID)
	if err != nil {
		t.Fatalf("owner CancelListing failed: %v", err)
	}
	if cancelled.Status != fleamarket.StatusCancelled {
		t.Fatalf("expected StatusCancelled, got %s", cancelled.Status)
	}
	owned, err := svc.GetActiveCharacterListings(ctx, seller.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(owned) != 0 {
		t.Fatalf("expected 0 active seller listings, got %+v", owned)
	}
	sellerDepot, err := f.depots.FindByCharacterID(ctx, seller.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(sellerDepot.Items) != 1 || sellerDepot.Items[0].DefinitionID != "weapon-01" || sellerDepot.Items[0].EnhancementLevel != 4 {
		t.Fatalf("seller depot after cancel = %+v", sellerDepot.Items)
	}
}

func TestFleaMarketFailedListingRollsBackDepot_Integration(t *testing.T) {
	f := newFleaMarketDBFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	seller := f.createCharacter(t, ctx, "FMRbk", 5000)
	wpn, err := item.NewInstance("weapon-01", 1)
	if err != nil {
		t.Fatal(err)
	}
	wpn.EnhancementLevel = 5
	if _, err := database.CreateTestDepot(ctx, f.db, seller.ID, 0, []item.Instance{wpn}); err != nil {
		t.Fatal(err)
	}
	heldArmor, err := item.NewInstance("armor-01", 1)
	if err != nil {
		t.Fatal(err)
	}
	sellerInv, err := coreinventory.New(seller.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := sellerInv.Add(heldArmor); err != nil {
		t.Fatal(err)
	}
	if err := f.invs.Save(ctx, sellerInv); err != nil {
		t.Fatal(err)
	}

	f.recorder.err = errors.New("simulated collection recorder failure")
	svc := f.service(t)
	now := time.Now().UTC()

	if _, err := svc.CreateListing(ctx, seller.ID, wpn.ID, 500, now); err == nil {
		t.Fatal("expected CreateListing to fail when collection recorder fails")
	}

	sellerDepot, err := f.depots.FindByCharacterID(ctx, seller.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(sellerDepot.Items) != 1 || sellerDepot.Items[0].ID != wpn.ID || sellerDepot.Items[0].EnhancementLevel != 5 {
		t.Fatalf("depot item lost on rollback: %+v", sellerDepot.Items)
	}
	owned, err := svc.GetActiveCharacterListings(ctx, seller.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(owned) != 0 {
		t.Fatalf("listing persisted after rollback: %+v", owned)
	}

	f.recorder.err = nil
	if _, err := svc.CreateListing(ctx, seller.ID, wpn.ID, 500, now); err != nil {
		t.Fatalf("retry CreateListing failed: %v", err)
	}
}

func TestFleaMarketConcurrentPurchasesConserveAssets_Integration(t *testing.T) {
	f := newFleaMarketDBFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	seller := f.createCharacter(t, ctx, "FMRaceS", 5000)
	wpn, err := item.NewInstance("weapon-01", 1)
	if err != nil {
		t.Fatal(err)
	}
	wpn.EnhancementLevel = 6
	if _, err := database.CreateTestDepot(ctx, f.db, seller.ID, 0, []item.Instance{wpn}); err != nil {
		t.Fatal(err)
	}
	heldArmor, err := item.NewInstance("armor-01", 1)
	if err != nil {
		t.Fatal(err)
	}
	sellerInv, err := coreinventory.New(seller.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := sellerInv.Add(heldArmor); err != nil {
		t.Fatal(err)
	}
	if err := f.invs.Save(ctx, sellerInv); err != nil {
		t.Fatal(err)
	}

	svc := f.service(t)
	now := time.Now().UTC()
	const price = 450
	listing, err := svc.CreateListing(ctx, seller.ID, wpn.ID, price, now)
	if err != nil {
		t.Fatal(err)
	}

	const numBuyers = 4
	buyers := make([]string, numBuyers)
	fns := make([]func() error, numBuyers)
	for i := range numBuyers {
		b := f.createCharacter(t, ctx, fmt.Sprintf("FMBuy%d", i), 2000)
		if _, err := database.CreateTestDepot(ctx, f.db, b.ID, 0, nil); err != nil {
			t.Fatal(err)
		}
		buyers[i] = b.ID
		buyerID := b.ID
		fns[i] = func() error {
			opCtx, opCancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer opCancel()
			_, err := svc.PurchaseListing(opCtx, buyerID, listing.ID, now)
			return err
		}
	}

	errs := testutil.RunRace(fns...)
	successes := 0
	for _, err := range errs {
		if testutil.IsDeadlockError(err) {
			t.Fatalf("unexpected deadlock during concurrent PurchaseListing: %v", err)
		}
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("expected exactly 1 purchase success, got %d (errs=%v)", successes, errs)
	}

	persistedSeller, err := f.chars.FindByID(ctx, seller.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persistedSeller.Money != 5000 {
		t.Fatalf("seller wallet changed after purchase: %d", persistedSeller.Money)
	}
	held, err := f.depots.ListRemittances(ctx, seller.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(held) != 1 || held[0].Amount != price {
		t.Fatalf("seller remittances = %+v", held)
	}
	totalGold := persistedSeller.Money + held[0].Amount
	totalItems := 0
	for _, bID := range buyers {
		b, err := f.chars.FindByID(ctx, bID)
		if err != nil {
			t.Fatal(err)
		}
		totalGold += b.Money
		dep, err := f.depots.FindByCharacterID(ctx, bID)
		if err != nil {
			t.Fatal(err)
		}
		totalItems += len(dep.Items)
	}
	if totalGold != 5000+numBuyers*2000 {
		t.Fatalf("gold not conserved: total=%d", totalGold)
	}
	if totalItems != 1 {
		t.Fatalf("expected exactly 1 item transferred, got %d", totalItems)
	}
	active, err := svc.GetActiveCharacterListings(ctx, seller.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 0 {
		t.Fatalf("expected 0 active listings after purchase, got %+v", active)
	}
}
