package database_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/depot"
)

func TestDeliverRewardItems_Integration(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	invRepo, err := database.NewInventoryRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	depotRepo, err := database.NewDepotRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()

	// Case 1: Empty inventory delivery -> inventory
	char, err := database.CreateTestCharacter(ctx, db, fmt.Sprintf("RewardChar_%08x", now.UnixNano()%100000000))
	if err != nil {
		t.Fatal(err)
	}

	item1 := coreitem.Instance{
		ID:           fmt.Sprintf("item_%016x", now.UnixNano()),
		DefinitionID: "item-potion",
		Quantity:     1,
	}

	res1, err := database.DeliverRewardItems(ctx, db, char, []coreitem.Instance{item1}, depot.PolicyAbortOnDepotFull)
	if err != nil {
		t.Fatalf("DeliverRewardItems failed: %v", err)
	}
	if len(res1) != 1 || res1[0].DeliveredTo != depot.DeliveredToInventory {
		t.Fatalf("expected 1 delivery to inventory, got %+v", res1)
	}

	inv, err := invRepo.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(inv.Items) != 1 || inv.Items[0].DefinitionID != "item-potion" {
		t.Fatalf("unexpected inventory items: %+v", inv.Items)
	}

	// Case 2: Full inventory overflow -> depot with refreshed capacity
	char.JobLevel = 5 // capacity: 5*5 + 5 = 30
	if err := charRepo.Update(ctx, char); err != nil {
		t.Fatal(err)
	}

	item2 := coreitem.Instance{
		ID:           fmt.Sprintf("item_%016x_2", now.UnixNano()),
		DefinitionID: "item-herb",
		Quantity:     1,
	}

	res2, err := database.DeliverRewardItems(ctx, db, char, []coreitem.Instance{item2}, depot.PolicyAbortOnDepotFull)
	if err != nil {
		t.Fatalf("DeliverRewardItems overflow to depot failed: %v", err)
	}
	if len(res2) != 1 || res2[0].DeliveredTo != depot.DeliveredToDepot {
		t.Fatalf("expected 1 delivery to depot, got %+v", res2)
	}

	dep, err := depotRepo.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if dep.Capacity != 30 {
		t.Errorf("expected refreshed capacity 30, got %d", dep.Capacity)
	}
	if len(dep.Items) != 1 || dep.Items[0].DefinitionID != "item-herb" {
		t.Fatalf("unexpected depot items: %+v", dep.Items)
	}

	// Case 3: Full depot with lost drop policy
	// Create another character with JobLevel 0 (capacity 5)
	char2, err := database.CreateTestCharacter(ctx, db, fmt.Sprintf("FullDepot_%08x", now.UnixNano()%100000000))
	if err != nil {
		t.Fatal(err)
	}
	// Fill inventory (1 item)
	fillInvItem := coreitem.Instance{
		ID:           fmt.Sprintf("item_inv_%016x", now.UnixNano()),
		DefinitionID: "item-inv-full",
		Quantity:     1,
	}
	_, err = database.DeliverRewardItems(ctx, db, char2, []coreitem.Instance{fillInvItem}, depot.PolicyAbortOnDepotFull)
	if err != nil {
		t.Fatal(err)
	}

	// Fill depot to full capacity (5 items for JobLevel 0)
	d, err := depotRepo.FindByCharacterID(ctx, char2.ID)
	if errors.Is(err, depot.ErrNotFound) {
		d, err = depot.NewDepotWithCapacity(char2.ID, 0, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 5; i++ {
		fillDepotItem := coreitem.Instance{
			ID:           fmt.Sprintf("depot_fill_%d_%08x", i, now.UnixNano()%100000000),
			DefinitionID: fmt.Sprintf("item-fill-%d", i),
			Quantity:     1,
		}
		if err := d.AddItem(fillDepotItem); err != nil {
			t.Fatalf("failed to add fill item %d: %v", i, err)
		}
	}
	if err := depotRepo.Save(ctx, d); err != nil {
		t.Fatal(err)
	}

	// Deliver with PolicyTreatOverflowAsLost
	lostItem := coreitem.Instance{
		ID:           fmt.Sprintf("lost_%016x", now.UnixNano()),
		DefinitionID: "item-rare-drop",
		Quantity:     1,
	}
	res3, err := database.DeliverRewardItems(ctx, db, char2, []coreitem.Instance{lostItem}, depot.PolicyTreatOverflowAsLost)
	if err != nil {
		t.Fatalf("DeliverRewardItems with lost policy failed: %v", err)
	}
	if len(res3) != 1 || res3[0].DeliveredTo != depot.DeliveredToLost {
		t.Fatalf("expected delivery to lost, got %+v", res3)
	}

	// Case 4: Full depot with rollback/error policy
	_, err = database.DeliverRewardItems(ctx, db, char2, []coreitem.Instance{lostItem}, depot.PolicyAbortOnDepotFull)
	if !errors.Is(err, depot.ErrDepotFull) {
		t.Fatalf("expected ErrDepotFull on abort policy, got: %v", err)
	}
}
