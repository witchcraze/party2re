package blacksmith_test

import (
	"context"
	"os"
	"testing"

	"github.com/witchcraze/party2re/internal/blacksmith"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/database"
)

func TestBlacksmithIntegration(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	invRepo, err := database.NewInventoryRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	equipRepo, err := database.NewEquipmentRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	bsRepo, err := database.NewBlacksmithRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	txProvider := database.NewTransactionProvider(db)

	char, err := database.CreateTestCharacter(ctx, db, "Blacksmith Live Integrator")
	if err != nil {
		t.Fatal(err)
	}
	char.Crystal = 2000
	if err := charRepo.Update(ctx, char); err != nil {
		t.Fatal(err)
	}

	catalog, err := item.InitialCatalog()
	if err != nil {
		t.Fatal(err)
	}

	svc, err := blacksmith.NewService(
		charRepo,
		invRepo,
		catalog,
		blacksmith.WithEquipmentRepository(equipRepo),
		blacksmith.WithStorageRepository(bsRepo),
		blacksmith.WithTransactionProvider(txProvider),
	)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Equip weapon (weapon-01) and armor (armor-01)
	inv, err := invRepo.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	weaInst, err := item.NewInstance("weapon-01", 1)
	if err != nil {
		t.Fatal(err)
	}
	armInst, err := item.NewInstance("armor-01", 1)
	if err != nil {
		t.Fatal(err)
	}
	_ = inv.Add(weaInst)
	_ = inv.Add(armInst)
	if err := invRepo.Save(ctx, inv); err != nil {
		t.Fatal(err)
	}

	equip, err := equipRepo.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	weaDef, _ := catalog.FindByID("weapon-01")
	armDef, _ := catalog.FindByID("armor-01")
	_, err = equip.Equip(&inv, weaDef, weaInst.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = equip.Equip(&inv, armDef, armInst.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := equipRepo.Save(ctx, equip); err != nil {
		t.Fatal(err)
	}

	// 2. Apply Seal 2 (牙: 500 crystals)
	seal, err := svc.ApplySeal(ctx, char.ID, 2)
	if err != nil {
		t.Fatalf("ApplySeal failed: %v", err)
	}
	if seal.ID != 2 {
		t.Errorf("seal ID = %d, want 2", seal.ID)
	}

	restoredChar, err := charRepo.FindByID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if restoredChar.Crystal != 1500 {
		t.Errorf("char crystal = %d, want 1500", restoredChar.Crystal)
	}
	if restoredChar.WeaponSeal != 2 {
		t.Errorf("char weapon seal = %d, want 2", restoredChar.WeaponSeal)
	}

	// 3. Name weapon and armor
	if err := svc.NameEquipment(ctx, char.ID, "weapon", "DragonSlayer"); err != nil {
		t.Fatalf("NameEquipment weapon: %v", err)
	}
	if err := svc.NameEquipment(ctx, char.ID, "armor", "IronPlate"); err != nil {
		t.Fatalf("NameEquipment armor: %v", err)
	}

	namedChar, err := charRepo.FindByID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if namedChar.WeaponCustomName != "DragonSlayer" {
		t.Errorf("weapon name = %q, want DragonSlayer", namedChar.WeaponCustomName)
	}
	if namedChar.ArmorCustomName != "IronPlate" {
		t.Errorf("armor name = %q, want IronPlate", namedChar.ArmorCustomName)
	}

	// 4. Deposit weapon into storage
	dep, err := svc.DepositWeapon(ctx, char.ID)
	if err != nil {
		t.Fatalf("DepositWeapon: %v", err)
	}
	if dep.Slot != 1 || dep.ItemDefinitionID != "weapon-01" || dep.SealID != 2 || dep.CustomName != "DragonSlayer" {
		t.Errorf("unexpected deposit: %+v", dep)
	}

	// Verify weapon unequipped and reset in DB
	charAfterDep, _ := charRepo.FindByID(ctx, char.ID)
	if charAfterDep.WeaponSeal != 0 || charAfterDep.WeaponCustomName != "" {
		t.Errorf("character seal/name not reset: seal=%d, name=%q", charAfterDep.WeaponSeal, charAfterDep.WeaponCustomName)
	}
	equipAfterDep, _ := equipRepo.FindByCharacterID(ctx, char.ID)
	if _, ok := equipAfterDep.Equipped(item.SlotMainHand); ok {
		t.Error("weapon slot still occupied in equipment repository")
	}

	// 5. Withdraw weapon from storage
	if err := svc.WithdrawWeapon(ctx, char.ID, 1); err != nil {
		t.Fatalf("WithdrawWeapon: %v", err)
	}

	charAfterWithdraw, _ := charRepo.FindByID(ctx, char.ID)
	if charAfterWithdraw.WeaponSeal != 2 || charAfterWithdraw.WeaponCustomName != "DragonSlayer" {
		t.Errorf("restored seal=%d, name=%q (want 2, DragonSlayer)", charAfterWithdraw.WeaponSeal, charAfterWithdraw.WeaponCustomName)
	}
	equipAfterWithdraw, _ := equipRepo.FindByCharacterID(ctx, char.ID)
	if _, ok := equipAfterWithdraw.Equipped(item.SlotMainHand); !ok {
		t.Error("weapon not equipped in equipment repository after withdraw")
	}

	// 6. Verify storage slot 1 is deleted
	storedList, err := svc.ListDeposits(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(storedList) != 0 {
		t.Errorf("expected 0 deposits, got %d", len(storedList))
	}
}
