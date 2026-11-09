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

	res1, err := database.DeliverRewardItems(ctx, db, char, []coreitem.Instance{item1}, depot.PolicyDeliverOverCapacity)
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

	res2, err := database.DeliverRewardItems(ctx, db, char, []coreitem.Instance{item2}, depot.PolicyDeliverOverCapacity)
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
	_, err = database.DeliverRewardItems(ctx, db, char2, []coreitem.Instance{fillInvItem}, depot.PolicyDeliverOverCapacity)
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

	// Case 4: NPC delivery appends above capacity and persists the extra slot
	res4, err := database.DeliverRewardItems(ctx, db, char2, []coreitem.Instance{lostItem}, depot.PolicyDeliverOverCapacity)
	if err != nil || len(res4) != 1 || res4[0].DeliveredTo != depot.DeliveredToDepot {
		t.Fatalf("expected over-capacity depot delivery, got %+v, %v", res4, err)
	}
	reloaded, err := depotRepo.FindByCharacterID(ctx, char2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.Items) != 6 || reloaded.Quantity("item-rare-drop") != 1 {
		t.Fatalf("expected 6 persisted slots including the delivery, got %+v", reloaded.Items)
	}
}

func TestDeliverRewardItem_TreasureReproductionVectors_Integration(t *testing.T) {
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
	txProvider := database.NewTransactionProvider(db)

	now := time.Now().UTC()

	// 1. Full depot (5/5), tool hand occupied with item-001, empty weapon/armor hands.
	char, err := database.CreateTestCharacter(ctx, db, fmt.Sprintf("Treas_%08x", now.UnixNano()%100000000))
	if err != nil {
		t.Fatal(err)
	}
	char.JobLevel = 0
	if err := charRepo.Update(ctx, char); err != nil {
		t.Fatal(err)
	}

	toolInst, _ := coreitem.NewInstance("item-001", 1)
	if _, err := database.CreateTestInventoryWithItems(ctx, db, char.ID, []coreitem.Instance{toolInst}); err != nil {
		t.Fatal(err)
	}

	d, err := depot.NewDepotWithCapacity(char.ID, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		fillInst := coreitem.Instance{
			ID:           fmt.Sprintf("wep_fill_%d_%08x", i, now.UnixNano()%100000000),
			DefinitionID: fmt.Sprintf("weapon-fill-%d", i),
			Quantity:     1,
		}
		if err := d.AddItem(fillInst); err != nil {
			t.Fatal(err)
		}
	}
	if err := depotRepo.Save(ctx, d); err != nil {
		t.Fatal(err)
	}

	// Vector 1: weapon-01 into full depot with tool held -> DeliveredToLost (not inventory!)
	err = txProvider.RunInTx(ctx, func(txCtx context.Context) error {
		res, err := depot.DeliverRewardItem(txCtx, invRepo, depotRepo, char, "weapon-01", 1, depot.PolicyTreatOverflowAsLost)
		if err != nil {
			return err
		}
		if res.DeliveredTo != depot.DeliveredToLost {
			return fmt.Errorf("expected DeliveredToLost for weapon-01, got %s", res.DeliveredTo)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Vector 1 failed: %v", err)
	}

	// Check DB state after Vector 1: inventory still has only item-001; depot remains 5/5
	invCheck, err := invRepo.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(invCheck.Items) != 1 || invCheck.Items[0].DefinitionID != "item-001" {
		t.Fatalf("inventory altered after lost drop: %+v", invCheck.Items)
	}
	depotCheck, err := depotRepo.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(depotCheck.Items) != 5 {
		t.Fatalf("depot items count altered: %d", len(depotCheck.Items))
	}

	// Vector 2: armor-01 into full depot with tool held -> DeliveredToLost
	err = txProvider.RunInTx(ctx, func(txCtx context.Context) error {
		res, err := depot.DeliverRewardItem(txCtx, invRepo, depotRepo, char, "armor-01", 1, depot.PolicyTreatOverflowAsLost)
		if err != nil {
			return err
		}
		if res.DeliveredTo != depot.DeliveredToLost {
			return fmt.Errorf("expected DeliveredToLost for armor-01, got %s", res.DeliveredTo)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Vector 2 failed: %v", err)
	}

	// Vector 3 & 4: Depot has space (e.g. 4/5 items).
	// weapon-01 and armor-01 must route to depot, NOT inventory!
	charSpace, err := database.CreateTestCharacter(ctx, db, fmt.Sprintf("TreasSpace_%08x", now.UnixNano()%100000000))
	if err != nil {
		t.Fatal(err)
	}
	charSpace.JobLevel = 0
	if err := charRepo.Update(ctx, charSpace); err != nil {
		t.Fatal(err)
	}
	toolInstSpace, _ := coreitem.NewInstance("item-001", 1)
	if _, err := database.CreateTestInventoryWithItems(ctx, db, charSpace.ID, []coreitem.Instance{toolInstSpace}); err != nil {
		t.Fatal(err)
	}
	dSpace, err := depot.NewDepotWithCapacity(charSpace.ID, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		fillInst := coreitem.Instance{
			ID:           fmt.Sprintf("space_fill_%d_%08x", i, now.UnixNano()%100000000),
			DefinitionID: fmt.Sprintf("weapon-space-%d", i),
			Quantity:     1,
		}
		if err := dSpace.AddItem(fillInst); err != nil {
			t.Fatal(err)
		}
	}
	if err := depotRepo.Save(ctx, dSpace); err != nil {
		t.Fatal(err)
	}

	// Vector 3: weapon-01 with space -> DeliveredToDepot
	err = txProvider.RunInTx(ctx, func(txCtx context.Context) error {
		res, err := depot.DeliverRewardItem(txCtx, invRepo, depotRepo, charSpace, "weapon-01", 1, depot.PolicyTreatOverflowAsLost)
		if err != nil {
			return err
		}
		if res.DeliveredTo != depot.DeliveredToDepot {
			return fmt.Errorf("expected DeliveredToDepot for weapon-01, got %s", res.DeliveredTo)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Vector 3 failed: %v", err)
	}

	// Vector 4: armor-01 with space (now 4/5) -> DeliveredToDepot
	err = txProvider.RunInTx(ctx, func(txCtx context.Context) error {
		res, err := depot.DeliverRewardItem(txCtx, invRepo, depotRepo, charSpace, "armor-01", 1, depot.PolicyTreatOverflowAsLost)
		if err != nil {
			return err
		}
		if res.DeliveredTo != depot.DeliveredToDepot {
			return fmt.Errorf("expected DeliveredToDepot for armor-01, got %s", res.DeliveredTo)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Vector 4 failed: %v", err)
	}

	// Verify charSpace depot now has 5/5 items, and inventory still only has item-001
	invSpaceCheck, err := invRepo.FindByCharacterID(ctx, charSpace.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(invSpaceCheck.Items) != 1 || invSpaceCheck.Items[0].DefinitionID != "item-001" {
		t.Fatalf("inventory altered unexpectedly: %+v", invSpaceCheck.Items)
	}
	depotSpaceCheck, err := depotRepo.FindByCharacterID(ctx, charSpace.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(depotSpaceCheck.Items) != 5 {
		t.Fatalf("expected 5 items in depot, got %d", len(depotSpaceCheck.Items))
	}

	// Healthy contrast: empty tool hand receives item-001 even when depot is full
	charEmpty, err := database.CreateTestCharacter(ctx, db, fmt.Sprintf("TreasEmpty_%08x", now.UnixNano()%100000000))
	if err != nil {
		t.Fatal(err)
	}
	charEmpty.JobLevel = 0
	if err := charRepo.Update(ctx, charEmpty); err != nil {
		t.Fatal(err)
	}
	dEmpty, err := depot.NewDepotWithCapacity(charEmpty.ID, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		fillInst := coreitem.Instance{
			ID:           fmt.Sprintf("empty_fill_%d_%08x", i, now.UnixNano()%100000000),
			DefinitionID: fmt.Sprintf("weapon-empty-%d", i),
			Quantity:     1,
		}
		if err := dEmpty.AddItem(fillInst); err != nil {
			t.Fatal(err)
		}
	}
	if err := depotRepo.Save(ctx, dEmpty); err != nil {
		t.Fatal(err)
	}

	err = txProvider.RunInTx(ctx, func(txCtx context.Context) error {
		res, err := depot.DeliverRewardItem(txCtx, invRepo, depotRepo, charEmpty, "item-001", 1, depot.PolicyTreatOverflowAsLost)
		if err != nil {
			return err
		}
		if res.DeliveredTo != depot.DeliveredToInventory {
			return fmt.Errorf("expected DeliveredToInventory for item-001 with empty tool hand, got %s", res.DeliveredTo)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Healthy contrast failed: %v", err)
	}
	invEmptyCheck, err := invRepo.FindByCharacterID(ctx, charEmpty.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(invEmptyCheck.Items) != 1 || invEmptyCheck.Items[0].DefinitionID != "item-001" {
		t.Fatalf("expected item-001 in inventory, got %+v", invEmptyCheck.Items)
	}

	// Matching stack test: tool hand is now occupied by item-001. Depot has 5 items including a stackable item-002.
	// Delivering item-002 when depot is at full capacity must forfeit the item (DeliveredToLost),
	// never bypassing the full-depot check via stack merge.
	dEmptyReloaded, err := depotRepo.FindByCharacterID(ctx, charEmpty.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Replace one slot with item-002
	dEmptyReloaded.Items[0] = coreitem.Instance{
		ID:           fmt.Sprintf("stack_item_%08x", now.UnixNano()%100000000),
		DefinitionID: "item-002",
		Quantity:     1,
	}
	if err := depotRepo.Save(ctx, dEmptyReloaded); err != nil {
		t.Fatal(err)
	}

	// Try delivering another item-001: tool hand already holds item-001 -> cannot take hand.
	// Depot is 5/5 full. Must result in DeliveredToLost, no addition.
	err = txProvider.RunInTx(ctx, func(txCtx context.Context) error {
		res, err := depot.DeliverRewardItem(txCtx, invRepo, depotRepo, charEmpty, "item-001", 1, depot.PolicyTreatOverflowAsLost)
		if err != nil {
			return err
		}
		if res.DeliveredTo != depot.DeliveredToLost {
			return fmt.Errorf("expected DeliveredToLost for occupied tool hand with full depot, got %s", res.DeliveredTo)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Occupied tool hand with full depot failed: %v", err)
	}

	// Try delivering item-002: tool hand is occupied by item-001 -> cannot take hand.
	// Depot already contains item-002, but is 5/5 full.
	// PolicyTreatOverflowAsLost must forfeit the item even with an eligible stack.
	err = txProvider.RunInTx(ctx, func(txCtx context.Context) error {
		res, err := depot.DeliverRewardItem(txCtx, invRepo, depotRepo, charEmpty, "item-002", 1, depot.PolicyTreatOverflowAsLost)
		if err != nil {
			return err
		}
		if res.DeliveredTo != depot.DeliveredToLost {
			return fmt.Errorf("expected DeliveredToLost for matching stack on full depot, got %s", res.DeliveredTo)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Matching stack full-depot check failed: %v", err)
	}

	// Verify depot quantity of item-002 is still 1 (not merged to 2)
	depotFinal, err := depotRepo.FindByCharacterID(ctx, charEmpty.ID)
	if err != nil {
		t.Fatal(err)
	}
	if q := depotFinal.Quantity("item-002"); q != 1 {
		t.Fatalf("expected quantity 1 for item-002 in depot, got %d", q)
	}
}
