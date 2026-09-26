package database_test

import (
	"context"
	"os"
	"testing"

	"github.com/witchcraze/party2re/internal/database"
)

func TestCollectionRepository_Integration(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	repo, err := database.NewCollectionRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// 1. Create character
	char, err := database.CreateTestCharacter(ctx, db, "CollectorTester")
	if err != nil {
		t.Fatal(err)
	}

	// 2. Record monster defeats
	if err := repo.RecordMonsterDefeat(ctx, char.ID, "mon_goblin", "Goblin", "Forest"); err != nil {
		t.Fatalf("RecordMonsterDefeat 1 failed: %v", err)
	}
	if err := repo.RecordMonsterDefeat(ctx, char.ID, "mon_goblin", "Goblin", "Forest"); err != nil {
		t.Fatalf("RecordMonsterDefeat 2 failed: %v", err)
	}

	// 3. Get monster book
	monsters, err := repo.GetMonsterBook(ctx, char.ID)
	if err != nil {
		t.Fatalf("GetMonsterBook failed: %v", err)
	}
	if len(monsters) != 1 || monsters[0].DefeatedCount != 2 || monsters[0].MonsterName != "Goblin" {
		t.Errorf("monsters: %+v", monsters)
	}

	bookCount, err := repo.GetMonsterBookCount(ctx, char.ID)
	if err != nil {
		t.Fatalf("GetMonsterBookCount failed: %v", err)
	}
	if bookCount != 1 {
		t.Errorf("expected bookCount 1, got %d", bookCount)
	}

	// 4. Record item discovery
	if err := repo.RecordItemDiscovered(ctx, char.ID, "wea_dagger", "Bronze Dagger", "WEAPON"); err != nil {
		t.Fatalf("RecordItemDiscovered failed: %v", err)
	}
	if err := repo.RecordItemDiscovered(ctx, char.ID, "wea_dagger", "Bronze Dagger", "WEAPON"); err != nil {
		t.Fatalf("RecordItemDiscovered duplicate failed: %v", err)
	}

	// 5. Get item collection
	items, err := repo.GetItemCollection(ctx, char.ID, "WEAPON")
	if err != nil {
		t.Fatalf("GetItemCollection failed: %v", err)
	}
	if len(items) != 1 || items[0].ItemID != "wea_dagger" {
		t.Errorf("items: %+v", items)
	}

	// Record armor and item to verify category filtering in counts
	if err := repo.RecordItemDiscovered(ctx, char.ID, "arm_shield", "Iron Shield", "ARMOR"); err != nil {
		t.Fatalf("RecordItemDiscovered armor failed: %v", err)
	}
	if err := repo.RecordItemDiscovered(ctx, char.ID, "itm_herb", "Medicinal Herb", "ITEM"); err != nil {
		t.Fatalf("RecordItemDiscovered item failed: %v", err)
	}

	// Total across all categories (category == "")
	totalAll, err := repo.GetItemCollectionCount(ctx, char.ID, "")
	if err != nil {
		t.Fatalf("GetItemCollectionCount (all) failed: %v", err)
	}
	if totalAll != 3 {
		t.Errorf("total all items = %d, want 3", totalAll)
	}

	// Filter by WEAPON
	totalWeapons, err := repo.GetItemCollectionCount(ctx, char.ID, "WEAPON")
	if err != nil {
		t.Fatalf("GetItemCollectionCount (WEAPON) failed: %v", err)
	}
	if totalWeapons != 1 {
		t.Errorf("total weapons = %d, want 1", totalWeapons)
	}

	// Filter by ARMOR
	totalArmors, err := repo.GetItemCollectionCount(ctx, char.ID, "ARMOR")
	if err != nil {
		t.Fatalf("GetItemCollectionCount (ARMOR) failed: %v", err)
	}
	if totalArmors != 1 {
		t.Errorf("total armors = %d, want 1", totalArmors)
	}

	// Filter by ITEM
	totalItems, err := repo.GetItemCollectionCount(ctx, char.ID, "ITEM")
	if err != nil {
		t.Fatalf("GetItemCollectionCount (ITEM) failed: %v", err)
	}
	if totalItems != 1 {
		t.Errorf("total items = %d, want 1", totalItems)
	}

	// Filter by non-existent category
	totalNonExistent, err := repo.GetItemCollectionCount(ctx, char.ID, "ACCESSORY")
	if err != nil {
		t.Fatalf("GetItemCollectionCount (ACCESSORY) failed: %v", err)
	}
	if totalNonExistent != 0 {
		t.Errorf("total non-existent = %d, want 0", totalNonExistent)
	}

	// 6. Test MarkCompleted and IsCompleted
	isComp, err := repo.IsCompleted(ctx, char.ID, "monster_book")
	if err != nil {
		t.Fatalf("IsCompleted check failed: %v", err)
	}
	if isComp {
		t.Errorf("expected IsCompleted=false initially")
	}

	newly, err := repo.MarkCompleted(ctx, char.ID, "monster_book")
	if err != nil {
		t.Fatalf("MarkCompleted failed: %v", err)
	}
	if !newly {
		t.Errorf("expected newly=true on initial completion")
	}

	isComp, err = repo.IsCompleted(ctx, char.ID, "monster_book")
	if err != nil {
		t.Fatalf("IsCompleted check after mark failed: %v", err)
	}
	if !isComp {
		t.Errorf("expected IsCompleted=true after mark")
	}

	// Idempotent second MarkCompleted
	newlySecond, err := repo.MarkCompleted(ctx, char.ID, "monster_book")
	if err != nil {
		t.Fatalf("second MarkCompleted failed: %v", err)
	}
	if newlySecond {
		t.Errorf("expected newly=false on duplicate completion")
	}
}
