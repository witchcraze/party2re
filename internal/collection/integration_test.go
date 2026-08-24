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
	items, itemProgress, err := svc.GetItemCollection(ctx, char.ID, "")
	if err != nil {
		t.Fatalf("GetItemCollection failed: %v", err)
	}
	if len(items) != 2 || itemProgress.DiscoveredCount != 2 {
		t.Errorf("items count = %d, progress = %+v", len(items), itemProgress)
	}
}
