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

	totalItems, err := repo.GetItemCollectionCount(ctx, char.ID)
	if err != nil {
		t.Fatalf("GetItemCollectionCount failed: %v", err)
	}
	if totalItems != 1 {
		t.Errorf("total items = %d, want 1", totalItems)
	}
}
