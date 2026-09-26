package collection_test

import (
	"context"
	"os"
	"testing"

	"github.com/witchcraze/party2re/internal/collection"
	"github.com/witchcraze/party2re/internal/database"
)

func TestCollectionServiceDatabaseIntegration(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	collectionRepo, err := database.NewCollectionRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	svc, err := collection.NewService(collectionRepo, 286, 150)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// 1. Create character
	char, err := database.CreateTestCharacter(ctx, db, "FullCollector")
	if err != nil {
		t.Fatal(err)
	}

	// 2. Record monster defeats
	_ = svc.RecordMonsterDefeat(ctx, char.ID, "mon_dragon", "Red Dragon", "Volcano")
	_ = svc.RecordMonsterDefeat(ctx, char.ID, "mon_phoenix", "Phoenix", "Peak")

	// 3. Get Monster Book
	book, progress, err := svc.GetMonsterBook(ctx, char.ID)
	if err != nil {
		t.Fatalf("GetMonsterBook failed: %v", err)
	}
	if len(book) != 2 || progress.DiscoveredCount != 2 {
		t.Errorf("book count = %d, progress = %+v", len(book), progress)
	}

	// 4. Record Items
	_ = svc.RecordItemDiscovered(ctx, char.ID, "wea_excalibur", "Holy Excalibur", "WEAPON")
	_ = svc.RecordItemDiscovered(ctx, char.ID, "arm_aegis", "Aegis Shield", "SHIELD")

	// 5. Get Item Collection
	// Querying "item" category should have 0 items even though weapon and armor were recorded
	itemsOnly, itemProgressOnly, err := svc.GetItemCollection(ctx, char.ID, "item")
	if err != nil {
		t.Fatalf("GetItemCollection (item) failed: %v", err)
	}
	if len(itemsOnly) != 0 || itemProgressOnly.DiscoveredCount != 0 {
		t.Errorf("expected 0 items, got len=%d discovered=%d", len(itemsOnly), itemProgressOnly.DiscoveredCount)
	}

	// Record an item
	_ = svc.RecordItemDiscovered(ctx, char.ID, "itm_potion", "Potion", "ITEM")

	// Now "item" category should have 1 item
	itemsOnly, itemProgressOnly, err = svc.GetItemCollection(ctx, char.ID, "item")
	if err != nil {
		t.Fatalf("GetItemCollection (item after add) failed: %v", err)
	}
	if len(itemsOnly) != 1 || itemProgressOnly.DiscoveredCount != 1 {
		t.Errorf("expected 1 item, got len=%d discovered=%d", len(itemsOnly), itemProgressOnly.DiscoveredCount)
	}

	// Unfiltered ("") should include all 3 discovered entries
	items, itemProgress, err := svc.GetItemCollection(ctx, char.ID, "")
	if err != nil {
		t.Fatalf("GetItemCollection (all) failed: %v", err)
	}
	if len(items) != 3 || itemProgress.DiscoveredCount != 3 {
		t.Errorf("items count = %d, progress = %+v", len(items), itemProgress)
	}
}
