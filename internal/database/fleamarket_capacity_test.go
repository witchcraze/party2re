package database_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/fleamarket"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/testutil"
)

// countRendezvous holds each creator after its count so that, without the capacity
// lock, both act on the same 119. With the lock the second creator cannot reach the
// count until the first commits, so the wait ends by timeout.
type countRendezvous struct {
	*database.FleaMarketRepository
	mu      sync.Mutex
	arrived int
	both    chan struct{}
}

func (r *countRendezvous) CountTotalActiveListings(ctx context.Context) (int, error) {
	n, err := r.FleaMarketRepository.CountTotalActiveListings(ctx)
	r.mu.Lock()
	r.arrived++
	if r.arrived == 2 {
		close(r.both)
	}
	r.mu.Unlock()
	select {
	case <-r.both:
	case <-time.After(300 * time.Millisecond):
	case <-ctx.Done():
	}
	return n, err
}

func capacityCount(t *testing.T, db *sql.DB) int {
	t.Helper()
	var n int
	if err := db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM fleamarket_listings WHERE status = 'active'").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// Requires a dedicated DB: the ceiling counts every active listing in the database.
func TestFleaMarketCapacitySerializesSellers(t *testing.T) {
	db := isolatedRepositoryDB(t, "players", "characters", "character_depots", "depot_items", "fleamarket_listings", "fleamarket_capacity_lock")
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, "DELETE FROM fleamarket_capacity_lock"); err != nil {
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
	repo, err := database.NewFleaMarketRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	rendezvous := &countRendezvous{FleaMarketRepository: repo, both: make(chan struct{})}
	svc, err := fleamarket.NewService(rendezvous, chars, depots, fleamarket.WithTransactionProvider(database.NewTransactionProvider(db)))
	if err != nil {
		t.Fatal(err)
	}
	filler, err := database.CreateTestCharacter(ctx, db, "CapFiller")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	for i := range fleamarket.ServerMaxListings - 1 {
		if err := repo.CreateListing(ctx, fleamarket.Listing{ID: id.New(), SellerCharacterID: filler.ID, SellerName: filler.Name, ItemID: "item-001", ItemName: fmt.Sprint(i), ItemCategory: "misc", Price: 1, Status: fleamarket.StatusActive, CreatedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	sellers := make([]string, 2)
	for i := range sellers {
		seller, err := database.CreateTestCharacter(ctx, db, "CapSeller")
		if err != nil {
			t.Fatal(err)
		}
		inst, err := coreitem.NewInstance("item-001", 1)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := database.CreateTestDepot(ctx, db, seller.ID, 1, []coreitem.Instance{inst}); err != nil {
			t.Fatal(err)
		}
		sellers[i] = seller.ID
	}
	errs := testutil.RunRace(
		func() error { _, err := svc.CreateListing(ctx, sellers[0], "item-001", 10, now); return err },
		func() error { _, err := svc.CreateListing(ctx, sellers[1], "item-001", 10, now); return err },
	)
	won := 0
	for i, err := range errs {
		dep, findErr := depots.FindByCharacterID(ctx, sellers[i])
		if findErr != nil {
			t.Fatal(findErr)
		}
		switch {
		case err == nil:
			won++
			if len(dep.Items) != 0 {
				t.Fatalf("winner kept the listed item: %+v", dep.Items)
			}
		case errors.Is(err, fleamarket.ErrServerMaxListingsReached):
			if len(dep.Items) != 1 {
				t.Fatalf("rejected seller lost the item: %+v", dep.Items)
			}
		default:
			t.Fatalf("unexpected race outcome: %v", err)
		}
	}
	if won != 1 || capacityCount(t, db) != fleamarket.ServerMaxListings {
		t.Fatalf("ceiling broken: won=%d active=%d", won, capacityCount(t, db))
	}
	// A rejected seller lists once a slot frees; purchase and cancel release capacity.
	if _, err := db.ExecContext(ctx, "UPDATE fleamarket_listings SET status = 'cancelled' WHERE seller_character_id = ? LIMIT 1", filler.ID); err != nil {
		t.Fatal(err)
	}
	for i, err := range errs {
		if err != nil {
			if _, err := svc.CreateListing(ctx, sellers[i], "item-001", 10, now); err != nil {
				t.Fatalf("freed slot not reusable: %v", err)
			}
		}
	}
}

func TestFleaMarketListingOrderAndEnhancementPersist(t *testing.T) {
	db := isolatedRepositoryDB(t, "players", "characters", "character_depots", "depot_items", "fleamarket_listings")
	ctx := context.Background()
	repo, err := database.NewFleaMarketRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	seller, err := database.CreateTestCharacter(ctx, db, "OrderSeller")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	want := []struct {
		item  string
		price int
	}{{"weapon-02", 500}, {"weapon-10", 1}, {"armor-01", 9}, {"item-002", 5}, {"item-002", 7}, {"item-010", 1}}
	// Insert newest-first in the wanted order reversed, so age alone cannot produce it.
	for i := len(want) - 1; i >= 0; i-- {
		if err := repo.CreateListing(ctx, fleamarket.Listing{ID: id.New(), SellerCharacterID: seller.ID, SellerName: seller.Name, ItemID: want[i].item, ItemName: want[i].item, ItemCategory: "misc", EnhancementLevel: i, Price: want[i].price, Status: fleamarket.StatusActive, CreatedAt: now.Add(time.Duration(-i) * time.Minute)}); err != nil {
			t.Fatal(err)
		}
	}
	got, total, err := repo.ListActiveListings(ctx, 100, 0)
	if err != nil || total != len(want) {
		t.Fatalf("list: %d %v", total, err)
	}
	for i, row := range got {
		if row.ItemID != want[i].item || row.Price != want[i].price || row.EnhancementLevel != i {
			t.Fatalf("row %d: %+v", i, row)
		}
	}
	// Rows written before the column existed read back as unenhanced through its default.
	if _, err := db.ExecContext(ctx, "INSERT INTO fleamarket_listings (id, seller_character_id, seller_name, item_id, item_name, price) VALUES (?, ?, ?, 'item-999', 'x', 1)", id.New(), seller.ID, seller.Name); err != nil {
		t.Fatal(err)
	}
	owned, err := repo.GetActiveListingsBySeller(ctx, seller.ID)
	if err != nil || len(owned) != len(want)+1 {
		t.Fatalf("owned: %d %v", len(owned), err)
	}
	for _, row := range owned {
		if row.ItemID == "item-999" && row.EnhancementLevel != 0 {
			t.Fatalf("default enhancement: %+v", row)
		}
	}
}
