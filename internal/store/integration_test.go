package store_test

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/store"
	"github.com/witchcraze/party2re/internal/testutil"
)

type testStoreGuildPoints struct {
	mu     sync.Mutex
	err    error
	points map[string]int
}

func (g *testStoreGuildPoints) AddGuildPoints(_ context.Context, characterID string, points int) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.err != nil {
		return g.err
	}
	if g.points == nil {
		g.points = make(map[string]int)
	}
	g.points[characterID] += points
	return nil
}

type testStoreDelivery struct {
	depots  *database.DepotRepository
	mu      sync.Mutex
	notices []string
}

func (d *testStoreDelivery) HoldPayment(ctx context.Context, recipientID, buyerName string, amount int) error {
	return d.depots.AddRemittance(ctx, depot.Remittance{
		ID:          id.New(),
		CharacterID: recipientID,
		Amount:      amount,
		Source:      depot.RemittanceStorePayment,
		SenderName:  buyerName,
	})
}

func (d *testStoreDelivery) NotifyItem(_ context.Context, recipientID, senderName, itemID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.notices = append(d.notices, fmt.Sprintf("%s:%s:%s", recipientID, senderName, itemID))
	return nil
}

type storeDBFixture struct {
	db          *sql.DB
	repo        *database.StoreRepository
	chars       *database.CharacterRepository
	depots      *database.DepotRepository
	catalog     *item.Catalog
	guildPoints *testStoreGuildPoints
	delivery    *testStoreDelivery
}

func isolatedStoreDB(t *testing.T) *sql.DB {
	t.Helper()
	sourceDSN := os.Getenv("PARTY2_DB_DSN")
	if sourceDSN == "" {
		t.Skip("PARTY2_DB_DSN not set")
	}
	adminDSN := os.Getenv("PARTY2_TEST_DB_ADMIN_DSN")
	if adminDSN == "" {
		t.Fatal("PARTY2_TEST_DB_ADMIN_DSN is required for isolated store tests")
	}

	sourceCfg, err := mysql.ParseDSN(sourceDSN)
	if err != nil {
		t.Fatalf("parse PARTY2_DB_DSN: %v", err)
	}
	adminCfg, err := mysql.ParseDSN(adminDSN)
	if err != nil {
		t.Fatalf("parse PARTY2_TEST_DB_ADMIN_DSN: %v", err)
	}

	sourceDB, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatalf("open source db: %v", err)
	}
	t.Cleanup(func() {
		if err := sourceDB.Close(); err != nil {
			t.Error(err)
		}
	})

	adminDB, err := database.Open(adminDSN)
	if err != nil {
		t.Fatalf("open admin db: %v", err)
	}

	var suffix [6]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}
	dbName := fmt.Sprintf("party2_store_test_%s", hex.EncodeToString(suffix[:]))

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if _, err := adminDB.ExecContext(ctx, fmt.Sprintf("CREATE DATABASE `%s`", dbName)); err != nil {
		if closeErr := adminDB.Close(); closeErr != nil {
			t.Error(closeErr)
		}
		t.Fatalf("create isolated db: %v", err)
	}
	t.Cleanup(func() {
		dropCtx, dropCancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer dropCancel()
		if _, err := adminDB.ExecContext(dropCtx, fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", dbName)); err != nil {
			t.Error(err)
		}
		if err := adminDB.Close(); err != nil {
			t.Error(err)
		}
	})

	isolatedCfg := *adminCfg
	isolatedCfg.DBName = dbName
	isolatedCfg.ParseTime = true
	db, err := database.Open(isolatedCfg.FormatDSN())
	if err != nil {
		t.Fatalf("open isolated db: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})

	tables := []string{
		"players",
		"characters",
		"character_depots",
		"depot_items",
		"character_stores",
		"store_town_capacity_lock",
		"store_name_lock",
		"store_sales",
		"store_interiors",
		"character_pending_remittances",
	}
	for _, tbl := range tables {
		var name, ddl string
		row := sourceDB.QueryRowContext(ctx, fmt.Sprintf("SHOW CREATE TABLE `%s`.`%s`", sourceCfg.DBName, tbl))
		if err := row.Scan(&name, &ddl); err != nil {
			t.Fatalf("SHOW CREATE TABLE %s: %v", tbl, err)
		}
		if _, err := db.ExecContext(ctx, ddl); err != nil {
			t.Fatalf("create table %s: %v", tbl, err)
		}
	}
	return db
}

func newStoreDBFixture(t *testing.T) *storeDBFixture {
	t.Helper()
	db := isolatedStoreDB(t)
	repo, err := database.NewStoreRepository(db)
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
	return &storeDBFixture{
		db:          db,
		repo:        repo,
		chars:       chars,
		depots:      depots,
		catalog:     catalog,
		guildPoints: &testStoreGuildPoints{},
		delivery:    &testStoreDelivery{depots: depots},
	}
}

func (f *storeDBFixture) service() *store.Service {
	return store.NewService(
		f.repo,
		f.chars,
		f.depots,
		f.catalog,
		database.NewTransactionProvider(f.db),
		store.WithGuildPoints(f.guildPoints),
		store.WithDeliveryService(f.delivery),
	)
}

func TestStoreBuildBuyAndTrade_Integration(t *testing.T) {
	f := newStoreDBFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	ownerInitialGold := store.StorePrice + 10000
	owner, err := database.CreateTestCharacterWithFunds(ctx, f.db, "StoreOwn", ownerInitialGold)
	if err != nil {
		t.Fatal(err)
	}
	buyer, err := database.CreateTestCharacterWithFunds(ctx, f.db, "StoreBuy", 10000)
	if err != nil {
		t.Fatal(err)
	}

	goldWeapon, err := item.NewInstance("weapon-01", 1)
	if err != nil {
		t.Fatal(err)
	}
	goldWeapon.EnhancementLevel = 3
	barterWeapon, err := item.NewInstance("weapon-01", 1)
	if err != nil {
		t.Fatal(err)
	}
	barterWeapon.EnhancementLevel = 5
	if _, err := database.CreateTestDepot(ctx, f.db, owner.ID, 0, []item.Instance{goldWeapon, barterWeapon}); err != nil {
		t.Fatal(err)
	}

	herb, err := item.NewInstance("item-001", 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateTestDepot(ctx, f.db, buyer.ID, 0, []item.Instance{herb}); err != nil {
		t.Fatal(err)
	}

	svc := f.service()
	res, err := svc.BuildStore(ctx, owner.ID, "town1", "001")
	if err != nil {
		t.Fatalf("BuildStore failed: %v", err)
	}
	if res.CharacterID != owner.ID || res.TownID != "town1" || res.HouseStyle != "001" {
		t.Fatalf("unexpected store result: %+v", res)
	}

	persistedOwner, err := f.chars.FindByID(ctx, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persistedOwner.Money != ownerInitialGold-store.StorePrice {
		t.Fatalf("owner money after build = %d, want %d", persistedOwner.Money, ownerInitialGold-store.StorePrice)
	}

	goldSale, err := svc.ListGoldItem(ctx, owner.ID, goldWeapon.ID, 400)
	if err != nil {
		t.Fatalf("ListGoldItem failed: %v", err)
	}
	barterSale, err := svc.ListBarterItem(ctx, owner.ID, barterWeapon.ID, "薬草")
	if err != nil {
		t.Fatalf("ListBarterItem failed: %v", err)
	}

	if err := svc.BuyItem(ctx, buyer.ID, goldSale.ID); err != nil {
		t.Fatalf("BuyItem failed: %v", err)
	}
	if err := svc.BuyItem(ctx, buyer.ID, goldSale.ID); !errors.Is(err, store.ErrListingNotFound) {
		t.Fatalf("repeat BuyItem expected ErrListingNotFound, got %v", err)
	}

	persistedBuyer, err := f.chars.FindByID(ctx, buyer.ID)
	if err != nil {
		t.Fatal(err)
	}
	persistedOwner, err = f.chars.FindByID(ctx, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persistedBuyer.Money != 9600 || persistedOwner.Money != ownerInitialGold-store.StorePrice {
		t.Fatalf("balances after buy: buyer=%d owner=%d", persistedBuyer.Money, persistedOwner.Money)
	}

	remittances, err := f.depots.ListRemittances(ctx, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(remittances) != 1 || remittances[0].Amount != 400 || remittances[0].Source != depot.RemittanceStorePayment {
		t.Fatalf("remittances = %+v", remittances)
	}

	if err := svc.TradeItem(ctx, buyer.ID, barterSale.ID, herb.ID); err != nil {
		t.Fatalf("TradeItem failed: %v", err)
	}

	buyerDepot, err := f.depots.FindByCharacterID(ctx, buyer.ID)
	if err != nil {
		t.Fatal(err)
	}
	ownerDepot, err := f.depots.FindByCharacterID(ctx, owner.ID)
	if err != nil {
		t.Fatal(err)
	}

	if len(buyerDepot.Items) != 2 {
		t.Fatalf("expected 2 traded/bought weapons in buyer depot, got %+v", buyerDepot.Items)
	}
	seenEnhancements := map[int]bool{}
	for _, it := range buyerDepot.Items {
		if it.DefinitionID != "weapon-01" {
			t.Fatalf("unexpected item in buyer depot: %+v", it)
		}
		seenEnhancements[it.EnhancementLevel] = true
	}
	if !seenEnhancements[3] || !seenEnhancements[5] {
		t.Fatalf("expected enhancement levels 3 and 5 preserved, got %+v", buyerDepot.Items)
	}
	if len(ownerDepot.Items) != 1 || ownerDepot.Items[0].DefinitionID != "item-001" {
		t.Fatalf("expected herb in owner depot, got %+v", ownerDepot.Items)
	}
	if len(f.delivery.notices) == 0 {
		t.Fatal("expected trade item notification recorded")
	}
}

func TestStoreFailedBuildRollsBack_Integration(t *testing.T) {
	f := newStoreDBFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	owner, err := database.CreateTestCharacterWithFunds(ctx, f.db, "StoreRbk", store.StorePrice+5000)
	if err != nil {
		t.Fatal(err)
	}
	f.guildPoints.err = errors.New("simulated guild point write failure")

	svc := f.service()
	if _, err := svc.BuildStore(ctx, owner.ID, "town1", "001"); err == nil {
		t.Fatal("expected BuildStore to fail when AddGuildPoints fails")
	}

	persistedOwner, err := f.chars.FindByID(ctx, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persistedOwner.Money != store.StorePrice+5000 {
		t.Fatalf("expected gold rolled back, got %d", persistedOwner.Money)
	}
	if _, err := f.repo.GetStoreByCharacterID(ctx, owner.ID); !errors.Is(err, store.ErrStoreNotFound) {
		t.Fatalf("expected ErrStoreNotFound after rollback, got %v", err)
	}

	// Retry succeeds without stale name or town lock residue.
	f.guildPoints.err = nil
	if _, err := svc.BuildStore(ctx, owner.ID, "town1", "001"); err != nil {
		t.Fatalf("retry BuildStore failed: %v", err)
	}
}

func TestStoreConcurrentPurchasesConserveAssets_Integration(t *testing.T) {
	f := newStoreDBFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	owner, err := database.CreateTestCharacterWithFunds(ctx, f.db, "StoreSO", store.StorePrice+1000)
	if err != nil {
		t.Fatal(err)
	}
	wpn, err := item.NewInstance("weapon-01", 1)
	if err != nil {
		t.Fatal(err)
	}
	wpn.EnhancementLevel = 4
	if _, err := database.CreateTestDepot(ctx, f.db, owner.ID, 0, []item.Instance{wpn}); err != nil {
		t.Fatal(err)
	}

	svc := f.service()
	if _, err := svc.BuildStore(ctx, owner.ID, "town1", "001"); err != nil {
		t.Fatal(err)
	}
	const price = 400
	sale, err := svc.ListGoldItem(ctx, owner.ID, wpn.ID, price)
	if err != nil {
		t.Fatal(err)
	}

	const numBuyers = 4
	buyers := make([]string, numBuyers)
	fns := make([]func() error, numBuyers)
	for i := range numBuyers {
		b, err := database.CreateTestCharacterWithFunds(ctx, f.db, fmt.Sprintf("SBuy%d", i), 1000)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := database.CreateTestDepot(ctx, f.db, b.ID, 0, nil); err != nil {
			t.Fatal(err)
		}
		buyers[i] = b.ID
		buyerID := b.ID
		fns[i] = func() error {
			opCtx, opCancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer opCancel()
			return svc.BuyItem(opCtx, buyerID, sale.ID)
		}
	}

	errs := testutil.RunRace(fns...)
	successes := 0
	for _, err := range errs {
		if testutil.IsDeadlockError(err) {
			t.Fatalf("unexpected deadlock during concurrent BuyItem: %v", err)
		}
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("expected exactly 1 purchase success, got %d (errs=%v)", successes, errs)
	}

	totalBuyerGold := 0
	totalBuyerItems := 0
	for _, bID := range buyers {
		b, err := f.chars.FindByID(ctx, bID)
		if err != nil {
			t.Fatal(err)
		}
		totalBuyerGold += b.Money
		dep, err := f.depots.FindByCharacterID(ctx, bID)
		if err != nil {
			t.Fatal(err)
		}
		totalBuyerItems += len(dep.Items)
	}
	remittances, err := f.depots.ListRemittances(ctx, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	heldGold := 0
	for _, r := range remittances {
		heldGold += r.Amount
	}
	if totalBuyerGold+heldGold != numBuyers*1000 {
		t.Fatalf("gold not conserved: buyerGold=%d heldGold=%d", totalBuyerGold, heldGold)
	}
	if totalBuyerItems != 1 {
		t.Fatalf("expected exactly 1 delivered item, got %d", totalBuyerItems)
	}
}
