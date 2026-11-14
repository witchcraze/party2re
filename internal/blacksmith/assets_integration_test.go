package blacksmith_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/blacksmith"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/testutil"
)

type failingUpdateCharRepo struct {
	*database.CharacterRepository
	failUpdate bool
}

func (r *failingUpdateCharRepo) Update(ctx context.Context, char corecharacter.Character) error {
	if r.failUpdate {
		return errors.New("simulated character update failure")
	}
	return r.CharacterRepository.Update(ctx, char)
}

type blacksmithAssetsFixture struct {
	db      *sql.DB
	chars   *failingUpdateCharRepo
	inv     *database.InventoryRepository
	equip   *database.EquipmentRepository
	storage *database.BlacksmithRepository
	catalog *item.Catalog
}

func newBlacksmithAssetsFixture(t *testing.T) *blacksmithAssetsFixture {
	t.Helper()
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN not set")
	}
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})

	baseChars, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	inv, err := database.NewInventoryRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	equip, err := database.NewEquipmentRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	storage, err := database.NewBlacksmithRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := item.DefaultCatalog()
	if err != nil {
		t.Fatal(err)
	}

	return &blacksmithAssetsFixture{
		db:      db,
		chars:   &failingUpdateCharRepo{CharacterRepository: baseChars},
		inv:     inv,
		equip:   equip,
		storage: storage,
		catalog: catalog,
	}
}

func (f *blacksmithAssetsFixture) createEquippedCharacter(t *testing.T, ctx context.Context, name string, money, crystals int) (corecharacter.Character, item.Instance) {
	t.Helper()
	char, err := database.CreateTestCharacterWithFunds(ctx, f.db, name, money)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cleanupCancel()
		if err := f.chars.CharacterRepository.Delete(cleanupCtx, char.ID); err != nil {
			t.Error(err)
		}
	})

	if crystals > 0 {
		if err := char.AddCrystal(crystals); err != nil {
			t.Fatal(err)
		}
		if err := f.chars.CharacterRepository.Update(ctx, char); err != nil {
			t.Fatal(err)
		}
	}

	wpn, err := item.NewInstance("weapon-01", 1)
	if err != nil {
		t.Fatal(err)
	}
	inv, err := database.CreateTestInventoryWithItems(ctx, f.db, char.ID, []item.Instance{wpn})
	if err != nil {
		t.Fatal(err)
	}
	eq, err := f.equip.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	def, err := f.catalog.FindByID("weapon-01")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := eq.Equip(&inv, def, wpn.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.equip.Save(ctx, eq); err != nil {
		t.Fatal(err)
	}
	return char, wpn
}

func (f *blacksmithAssetsFixture) service(t *testing.T) *blacksmith.Service {
	t.Helper()
	svc, err := blacksmith.NewService(
		f.chars,
		f.inv,
		f.catalog,
		blacksmith.WithEquipmentRepository(f.equip),
		blacksmith.WithStorageRepository(f.storage),
		blacksmith.WithTransactionProvider(database.NewTransactionProvider(f.db)),
	)
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func TestBlacksmithStateAndReset_Integration(t *testing.T) {
	f := newBlacksmithAssetsFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Insufficient crystals for seal 2 (costs 500) leaves real-DB state unchanged.
	char, wpn := f.createEquippedCharacter(t, ctx, "BSMoney", 0, 50)
	svc := f.service(t)

	if _, err := svc.ApplySeal(ctx, char.ID, 2); !errors.Is(err, blacksmith.ErrInsufficientCrystals) {
		t.Fatalf("expected ErrInsufficientCrystals, got %v", err)
	}
	unchanged, err := f.chars.FindByID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.Crystal != 50 || unchanged.WeaponSeal != 0 || unchanged.Money != 0 {
		t.Fatalf("state mutated on insufficient crystals: %+v", unchanged)
	}

	// Apply seal 1 (costs 50) -> Crystal=0, Money still 0.
	if _, err := svc.ApplySeal(ctx, char.ID, 1); err != nil {
		t.Fatalf("ApplySeal failed: %v", err)
	}
	if err := svc.NameEquipment(ctx, char.ID, "weapon", "BladeOne"); err != nil {
		t.Fatalf("NameEquipment failed: %v", err)
	}
	sealedChar, err := f.chars.FindByID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if sealedChar.Crystal != 0 || sealedChar.Money != 0 || sealedChar.WeaponSeal != 1 || sealedChar.WeaponCustomName != "BladeOne" {
		t.Fatalf("unexpected sealed/named state: %+v", sealedChar)
	}

	// Explicit name reset ("") persists empty WeaponCustomName while keeping seal and Money==0.
	if err := svc.NameEquipment(ctx, char.ID, "weapon", ""); err != nil {
		t.Fatalf("NameEquipment reset failed: %v", err)
	}
	resetChar, err := f.chars.FindByID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if resetChar.WeaponCustomName != "" || resetChar.WeaponSeal != 1 || resetChar.Money != 0 {
		t.Fatalf("expected empty custom name after reset, got %+v", resetChar)
	}

	// Rename again before deposit so SealID + CustomName round-trip through storage.
	if err := svc.NameEquipment(ctx, char.ID, "weapon", "SealedBlade"); err != nil {
		t.Fatal(err)
	}

	dep, err := svc.DepositWeapon(ctx, char.ID)
	if err != nil {
		t.Fatalf("DepositWeapon failed: %v", err)
	}
	if dep.SealID != 1 || dep.CustomName != "SealedBlade" {
		t.Fatalf("deposit record = %+v", dep)
	}

	invAfterDep, err := f.inv.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	eqAfterDep, err := f.equip.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	charAfterDep, err := f.chars.FindByID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(invAfterDep.Items) != 0 || len(eqAfterDep.Slots) != 0 || charAfterDep.WeaponSeal != 0 || charAfterDep.WeaponCustomName != "" || charAfterDep.Money != 0 {
		t.Fatalf("deposit state mismatch: inv=%+v eq=%+v char=%+v", invAfterDep.Items, eqAfterDep.Slots, charAfterDep)
	}

	if err := svc.WithdrawWeapon(ctx, char.ID, dep.Slot); err != nil {
		t.Fatalf("WithdrawWeapon failed: %v", err)
	}
	invAfterWith, err := f.inv.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	eqAfterWith, err := f.equip.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	charAfterWith, err := f.chars.FindByID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	equippedID, ok := eqAfterWith.Equipped(item.SlotMainHand)
	if !ok || len(invAfterWith.Items) != 1 || charAfterWith.Crystal != 0 || charAfterWith.Money != 0 ||
		charAfterWith.WeaponSeal != 1 || charAfterWith.WeaponCustomName != "SealedBlade" {
		t.Fatalf("withdraw state mismatch: eq=%v(%s) inv=%+v char=%+v (origID=%s)", ok, equippedID, invAfterWith.Items, charAfterWith, wpn.ID)
	}
}

func TestBlacksmithRollback_Integration(t *testing.T) {
	f := newBlacksmithAssetsFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	char, wpn := f.createEquippedCharacter(t, ctx, "BSRbk", 500, 100)
	svc := f.service(t)

	// 1. ApplySeal rollback.
	f.chars.failUpdate = true
	if _, err := svc.ApplySeal(ctx, char.ID, 1); err == nil {
		t.Fatal("expected ApplySeal failure")
	}
	c, err := f.chars.FindByID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if c.Crystal != 100 || c.WeaponSeal != 0 {
		t.Fatalf("ApplySeal rollback leaked: %+v", c)
	}

	// 2. NameEquipment rollback.
	if err := svc.NameEquipment(ctx, char.ID, "weapon", "FailName"); err == nil {
		t.Fatal("expected NameEquipment failure")
	}
	c, err = f.chars.FindByID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if c.WeaponCustomName != "" {
		t.Fatalf("NameEquipment rollback leaked: %+v", c)
	}

	// Apply seal and name normally before deposit rollback check.
	f.chars.failUpdate = false
	if _, err := svc.ApplySeal(ctx, char.ID, 1); err != nil {
		t.Fatal(err)
	}
	if err := svc.NameEquipment(ctx, char.ID, "weapon", "KeepName"); err != nil {
		t.Fatal(err)
	}

	// 3. DepositWeapon rollback.
	f.chars.failUpdate = true
	if _, err := svc.DepositWeapon(ctx, char.ID); err == nil {
		t.Fatal("expected DepositWeapon failure")
	}
	deposits, err := svc.ListDeposits(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	eq, err := f.equip.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	inv, err := f.inv.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	eqID, equipped := eq.Equipped(item.SlotMainHand)
	if len(deposits) != 0 || !equipped || eqID != wpn.ID || len(inv.Items) != 1 {
		t.Fatalf("DepositWeapon rollback leaked: deposits=%+v equipped=%v inv=%+v", deposits, equipped, inv.Items)
	}

	// Deposit normally before withdraw rollback check.
	f.chars.failUpdate = false
	dep, err := svc.DepositWeapon(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}

	// 4. WithdrawWeapon rollback.
	f.chars.failUpdate = true
	if err := svc.WithdrawWeapon(ctx, char.ID, dep.Slot); err == nil {
		t.Fatal("expected WithdrawWeapon failure")
	}
	deposits, err = svc.ListDeposits(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	eq, err = f.equip.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	inv, err = f.inv.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	c, err = f.chars.FindByID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(deposits) != 1 || len(eq.Slots) != 0 || len(inv.Items) != 0 || c.WeaponSeal != 0 || c.WeaponCustomName != "" {
		t.Fatalf("WithdrawWeapon rollback leaked: deposits=%+v eq=%+v inv=%+v char=%+v", deposits, eq.Slots, inv.Items, c)
	}

	// Retry withdraw succeeds cleanly.
	f.chars.failUpdate = false
	if err := svc.WithdrawWeapon(ctx, char.ID, dep.Slot); err != nil {
		t.Fatalf("retry WithdrawWeapon failed: %v", err)
	}
}

func TestBlacksmithConcurrentAssets_Integration(t *testing.T) {
	f := newBlacksmithAssetsFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Concurrent seal: enough crystals for exactly 1 seal (50 crystals, 3 racers).
	charSeal, _ := f.createEquippedCharacter(t, ctx, "BSRaceS", 0, 50)
	svc := f.service(t)

	sealErrs := testutil.RunRace(
		func() error {
			opCtx, opCancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer opCancel()
			_, err := svc.ApplySeal(opCtx, charSeal.ID, 1)
			return err
		},
		func() error {
			opCtx, opCancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer opCancel()
			_, err := svc.ApplySeal(opCtx, charSeal.ID, 1)
			return err
		},
		func() error {
			opCtx, opCancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer opCancel()
			_, err := svc.ApplySeal(opCtx, charSeal.ID, 1)
			return err
		},
	)
	sealSuccesses := 0
	for _, err := range sealErrs {
		if testutil.IsDeadlockError(err) {
			t.Fatalf("unexpected deadlock during ApplySeal race: %v", err)
		}
		if err == nil {
			sealSuccesses++
		}
	}
	if sealSuccesses != 1 {
		t.Fatalf("expected exactly 1 ApplySeal success, got %d (errs=%v)", sealSuccesses, sealErrs)
	}
	cSeal, err := f.chars.FindByID(ctx, charSeal.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cSeal.Crystal != 0 || cSeal.WeaponSeal != 1 {
		t.Fatalf("concurrent seal state mismatch: %+v", cSeal)
	}

	// Concurrent deposit: 3 callers race to deposit the single equipped weapon.
	charStore, _ := f.createEquippedCharacter(t, ctx, "BSRaceD", 0, 0)
	var depositedSlot int
	depErrs := testutil.RunRace(
		func() error {
			opCtx, opCancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer opCancel()
			d, err := svc.DepositWeapon(opCtx, charStore.ID)
			if err == nil {
				depositedSlot = d.Slot
			}
			return err
		},
		func() error {
			opCtx, opCancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer opCancel()
			d, err := svc.DepositWeapon(opCtx, charStore.ID)
			if err == nil {
				depositedSlot = d.Slot
			}
			return err
		},
		func() error {
			opCtx, opCancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer opCancel()
			d, err := svc.DepositWeapon(opCtx, charStore.ID)
			if err == nil {
				depositedSlot = d.Slot
			}
			return err
		},
	)
	depSuccesses := 0
	for _, err := range depErrs {
		if testutil.IsDeadlockError(err) {
			t.Fatalf("unexpected deadlock during DepositWeapon race: %v", err)
		}
		if err == nil {
			depSuccesses++
		}
	}
	if depSuccesses != 1 {
		t.Fatalf("expected exactly 1 DepositWeapon success, got %d (errs=%v)", depSuccesses, depErrs)
	}
	deposits, err := svc.ListDeposits(ctx, charStore.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(deposits) != 1 {
		t.Fatalf("expected 1 deposit after concurrent deposit, got %+v", deposits)
	}

	// Concurrent withdraw: 3 callers race to withdraw the single deposited weapon.
	withErrs := testutil.RunRace(
		func() error {
			opCtx, opCancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer opCancel()
			return svc.WithdrawWeapon(opCtx, charStore.ID, depositedSlot)
		},
		func() error {
			opCtx, opCancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer opCancel()
			return svc.WithdrawWeapon(opCtx, charStore.ID, depositedSlot)
		},
		func() error {
			opCtx, opCancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer opCancel()
			return svc.WithdrawWeapon(opCtx, charStore.ID, depositedSlot)
		},
	)
	withSuccesses := 0
	for _, err := range withErrs {
		if testutil.IsDeadlockError(err) {
			t.Fatalf("unexpected deadlock during WithdrawWeapon race: %v", err)
		}
		if err == nil {
			withSuccesses++
		}
	}
	if withSuccesses != 1 {
		t.Fatalf("expected exactly 1 WithdrawWeapon success, got %d (errs=%v)", withSuccesses, withErrs)
	}
	depositsAfter, err := svc.ListDeposits(ctx, charStore.ID)
	if err != nil {
		t.Fatal(err)
	}
	eqAfter, err := f.equip.FindByCharacterID(ctx, charStore.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, equipped := eqAfter.Equipped(item.SlotMainHand); !equipped || len(depositsAfter) != 0 {
		t.Fatalf("concurrent withdraw state mismatch: equipped=%v deposits=%+v", equipped, depositsAfter)
	}
}
