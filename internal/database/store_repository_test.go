package database

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/store"
)

func TestStoreRepositoryNilDB(t *testing.T) {
	if _, err := NewStoreRepository(nil); err == nil {
		t.Fatal("NewStoreRepository(nil) expected error, got nil")
	}
}

func TestStoreRepositorySaveAndFind(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	storeRepo, err := NewStoreRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	char, err := CreateTestCharacter(ctx, db, "Store DB Test")
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	expiresAt := now.Add(90 * 24 * time.Hour)

	st := store.Store{
		ID:          "st_" + char.ID,
		CharacterID: char.ID,
		TownID:      "town1",
		StoreName:   "テスト店舗",
		HouseStyle:  "001",
		Wallpaper:   "none",
		ExpiresAt:   expiresAt,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := storeRepo.SaveStore(ctx, st); err != nil {
		t.Fatalf("SaveStore() error = %v", err)
	}

	// Find by character ID
	found, err := storeRepo.GetStoreByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatalf("GetStoreByCharacterID() error = %v", err)
	}
	if found.StoreName != "テスト店舗" || found.TownID != "town1" {
		t.Fatalf("GetStoreByCharacterID() mismatch: %+v", found)
	}

	// Count active town stores
	count, err := storeRepo.CountActiveTownStores(ctx, "town1", now)
	if err != nil {
		t.Fatalf("CountActiveTownStores() error = %v", err)
	}
	if count < 1 {
		t.Fatalf("expected count >= 1, got %d", count)
	}

	// List active stores
	list, err := storeRepo.ListActiveStoresInTown(ctx, "town1", now)
	if err != nil {
		t.Fatalf("ListActiveStoresInTown() error = %v", err)
	}
	if len(list) < 1 {
		t.Fatalf("expected at least 1 active store in town")
	}

	// Test Sale CRUD
	sale := store.Sale{
		ID:               "sale_" + char.ID,
		StoreID:          st.ID,
		CharacterID:      char.ID,
		SlotNumber:       1,
		ItemDefinitionID: "wpn_001",
		ItemName:         "ひのきのぼう",
		Quantity:         1,
		EnhancementLevel: 0,
		SaleType:         store.SaleTypeGold,
		Price:            1000,
		WishItemName:     "",
		CreatedAt:        now,
	}
	if err := storeRepo.SaveSale(ctx, sale); err != nil {
		t.Fatalf("SaveSale() error = %v", err)
	}

	sales, err := storeRepo.GetSalesByStoreID(ctx, st.ID)
	if err != nil {
		t.Fatalf("GetSalesByStoreID() error = %v", err)
	}
	if len(sales) != 1 || sales[0].Price != 1000 {
		t.Fatalf("GetSalesByStoreID() mismatch: %+v", sales)
	}

	// Test Interior CRUD
	interior := store.Interior{
		ID:          "int_" + char.ID,
		StoreID:     st.ID,
		CharacterID: char.ID,
		FurnitureID: "001",
		Name:        "机",
		SlotIndex:   0,
		CreatedAt:   now,
	}
	if err := storeRepo.SaveInterior(ctx, interior); err != nil {
		t.Fatalf("SaveInterior() error = %v", err)
	}

	interiors, err := storeRepo.GetInteriorsByStoreID(ctx, st.ID)
	if err != nil {
		t.Fatalf("GetInteriorsByStoreID() error = %v", err)
	}
	if len(interiors) != 1 || interiors[0].Name != "机" {
		t.Fatalf("GetInteriorsByStoreID() mismatch: %+v", interiors)
	}

	if err := storeRepo.UpdateInteriorName(ctx, interior.ID, "かっこいい机"); err != nil {
		t.Fatalf("UpdateInteriorName() error = %v", err)
	}

	// Cleanup
	_ = storeRepo.DeleteSale(ctx, sale.ID)
	_ = storeRepo.DeleteInteriorsByStoreID(ctx, st.ID)
	_ = storeRepo.DeleteStore(ctx, st.ID)
}
