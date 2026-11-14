package fleamarket_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/fleamarket"
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

type fleaMarketDBFixture struct {
	db       *sql.DB
	repo     *database.FleaMarketRepository
	chars    *database.CharacterRepository
	depots   *database.DepotRepository
	catalog  *item.Catalog
	recorder *testFleaCollectionRecorder
}

func newFleaMarketDBFixture(t *testing.T) *fleaMarketDBFixture {
	t.Helper()
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN not set")
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
	catalog, err := item.DefaultCatalog()
	if err != nil {
		t.Fatal(err)
	}
	return &fleaMarketDBFixture{
		db:       db,
		repo:     repo,
		chars:    chars,
		depots:   depots,
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
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cleanupCancel()
		if err := f.chars.Delete(cleanupCtx, char.ID); err != nil {
			t.Error(err)
		}
	})
	return char
}

func (f *fleaMarketDBFixture) service(t *testing.T) *fleamarket.Service {
	t.Helper()
	svc, err := fleamarket.NewService(
		f.repo,
		f.chars,
		f.depots,
		fleamarket.WithTransactionProvider(database.NewTransactionProvider(f.db)),
		fleamarket.WithItemDefinitionProvider(f.catalog),
		fleamarket.WithCollectionRecorder(f.recorder),
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
	if purchase.BuyerGold != 9400 || purchase.SellerGold != 5600 || purchase.ItemInstance.EnhancementLevel != 7 {
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
	if persistedBuyer.Money != 9400 || persistedSeller.Money != 5600 {
		t.Fatalf("gold after purchase: buyer=%d seller=%d", persistedBuyer.Money, persistedSeller.Money)
	}
	buyerDepot, err := f.depots.FindByCharacterID(ctx, buyer.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(buyerDepot.Items) != 1 || buyerDepot.Items[0].DefinitionID != "weapon-01" || buyerDepot.Items[0].EnhancementLevel != 7 {
		t.Fatalf("buyer depot = %+v", buyerDepot.Items)
	}
	if len(f.recorder.records) == 0 {
		t.Fatal("expected collection discovery recorded")
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
	totalGold := persistedSeller.Money
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
