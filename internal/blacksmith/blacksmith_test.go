package blacksmith_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/witchcraze/party2re/internal/blacksmith"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
)

type mockCharRepo struct {
	chars map[string]corecharacter.Character
}

func (m *mockCharRepo) FindByID(ctx context.Context, id string) (corecharacter.Character, error) {
	c, ok := m.chars[id]
	if !ok {
		return corecharacter.Character{}, corecharacter.ErrNotFound
	}
	return c, nil
}

func (m *mockCharRepo) FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error) {
	return m.FindByID(ctx, id)
}

func (m *mockCharRepo) Update(ctx context.Context, character corecharacter.Character) error {
	m.chars[character.ID] = character
	return nil
}

type mockInvRepo struct {
	invs map[string]coreinventory.Inventory
}

func (m *mockInvRepo) FindByCharacterID(ctx context.Context, characterID string) (coreinventory.Inventory, error) {
	inv, ok := m.invs[characterID]
	if !ok {
		inv, _ = coreinventory.New(characterID)
		m.invs[characterID] = inv
	}
	return inv, nil
}

func (m *mockInvRepo) FindByCharacterIDForUpdate(ctx context.Context, characterID string) (coreinventory.Inventory, error) {
	return m.FindByCharacterID(ctx, characterID)
}

func (m *mockInvRepo) Save(ctx context.Context, inventory coreinventory.Inventory) error {
	m.invs[inventory.CharacterID] = inventory
	return nil
}

type mockEquipRepo struct {
	equips map[string]coreequipment.Equipment
}

func (m *mockEquipRepo) FindByCharacterID(ctx context.Context, characterID string) (coreequipment.Equipment, error) {
	eq, ok := m.equips[characterID]
	if !ok {
		eq, _ = coreequipment.New(characterID)
		m.equips[characterID] = eq
	}
	return eq, nil
}

func (m *mockEquipRepo) FindByCharacterIDForUpdate(ctx context.Context, characterID string) (coreequipment.Equipment, error) {
	return m.FindByCharacterID(ctx, characterID)
}

func (m *mockEquipRepo) Save(ctx context.Context, equipment coreequipment.Equipment) error {
	m.equips[equipment.CharacterID] = equipment
	return nil
}

type mockStorageRepo struct {
	storage map[string]map[int]blacksmith.Deposit
}

func newMockStorageRepo() *mockStorageRepo {
	return &mockStorageRepo{
		storage: make(map[string]map[int]blacksmith.Deposit),
	}
}

func (m *mockStorageRepo) ListByCharacterID(ctx context.Context, characterID string) ([]blacksmith.Deposit, error) {
	slots, ok := m.storage[characterID]
	if !ok {
		return nil, nil
	}
	var res []blacksmith.Deposit
	for s := 1; s <= 3; s++ {
		if dep, exists := slots[s]; exists {
			res = append(res, dep)
		}
	}
	return res, nil
}

func (m *mockStorageRepo) ListByCharacterIDForUpdate(ctx context.Context, characterID string) ([]blacksmith.Deposit, error) {
	return m.ListByCharacterID(ctx, characterID)
}

func (m *mockStorageRepo) Save(ctx context.Context, deposit blacksmith.Deposit) error {
	slots, ok := m.storage[deposit.CharacterID]
	if !ok {
		slots = make(map[int]blacksmith.Deposit)
		m.storage[deposit.CharacterID] = slots
	}
	slots[deposit.Slot] = deposit
	return nil
}

func (m *mockStorageRepo) Delete(ctx context.Context, characterID string, slot int) error {
	if slots, ok := m.storage[characterID]; ok {
		delete(slots, slot)
	}
	return nil
}

func setupTestCatalog(t *testing.T) *item.Catalog {
	t.Helper()
	sword, _ := item.NewEquipmentDefinition("weapon-01", "ひのきの棒", 10, item.SlotMainHand)
	flameSword, _ := item.NewEquipmentDefinition("weapon-36", "炎の剣", 22000, item.SlotMainHand)
	iceSword, _ := item.NewEquipmentDefinition("weapon-50", "吹雪の剣", 24000, item.SlotMainHand)
	iceBlade, _ := item.NewEquipmentDefinition("weapon-51", "氷の刃", 26000, item.SlotMainHand)
	thunderSword, _ := item.NewEquipmentDefinition("weapon-68", "稲妻の剣", 28000, item.SlotMainHand)
	godspeedSword, _ := item.NewEquipmentDefinition("weapon-29", "隼の剣", 21000, item.SlotMainHand)
	voidAbacus, _ := item.NewEquipmentDefinition("weapon-35", "天上の聖剣", 30000, item.SlotMainHand)
	reasonStaff, _ := item.NewEquipmentDefinition("weapon-34", "理力の杖", 17000, item.SlotMainHand)
	armor, _ := item.NewEquipmentDefinition("armor-01", "皮の鎧", 150, item.SlotBody)

	catalog, err := item.NewCatalog([]item.Definition{
		sword, flameSword, iceSword, iceBlade, thunderSword, godspeedSword, voidAbacus, reasonStaff, armor,
	})
	if err != nil {
		t.Fatalf("setup catalog: %v", err)
	}
	return catalog
}

func TestSealsCatalog(t *testing.T) {
	if len(blacksmith.Seals) != 12 {
		t.Fatalf("expected 12 seals, got %d", len(blacksmith.Seals))
	}

	// 1. Universal seals
	for id := 1; id <= 6; id++ {
		seal, ok := blacksmith.GetSeal(id)
		if !ok {
			t.Errorf("seal %d not found", id)
		}
		if !blacksmith.CanApplySeal("weapon-01", id) {
			t.Errorf("universal seal %d should apply to weapon-01", id)
		}
		if !blacksmith.CanApplySeal("weapon-36", id) {
			t.Errorf("universal seal %d should apply to weapon-36", id)
		}
		if len(seal.ApplicableWeaponIDs) != 0 {
			t.Errorf("universal seal %d should have empty applicable weapon IDs", id)
		}
	}

	// 2. Weapon-specific seals
	cases := []struct {
		sealID      int
		allowedDef  string
		rejectedDef string
	}{
		{7, "weapon-36", "weapon-01"},
		{8, "weapon-50", "weapon-01"},
		{8, "weapon-51", "weapon-01"},
		{9, "weapon-68", "weapon-01"},
		{10, "weapon-29", "weapon-01"},
		{11, "weapon-35", "weapon-01"},
		{12, "weapon-34", "weapon-01"},
	}

	for _, tc := range cases {
		if !blacksmith.CanApplySeal(tc.allowedDef, tc.sealID) {
			t.Errorf("seal %d should apply to %s", tc.sealID, tc.allowedDef)
		}
		if blacksmith.CanApplySeal(tc.rejectedDef, tc.sealID) {
			t.Errorf("seal %d should not apply to %s", tc.sealID, tc.rejectedDef)
		}
	}

	// Invalid seal
	if _, ok := blacksmith.GetSeal(0); ok {
		t.Error("seal 0 should not exist")
	}
	if _, ok := blacksmith.GetSeal(13); ok {
		t.Error("seal 13 should not exist")
	}
	if blacksmith.CanApplySeal("weapon-01", 999) {
		t.Error("nonexistent seal should not be applicable")
	}

	// ListAvailableSeals
	seals01 := blacksmith.ListAvailableSeals("weapon-01")
	if len(seals01) != 6 {
		t.Errorf("expected 6 universal seals for weapon-01, got %d", len(seals01))
	}
	seals36 := blacksmith.ListAvailableSeals("weapon-36")
	if len(seals36) != 7 {
		t.Errorf("expected 7 seals for weapon-36, got %d", len(seals36))
	}
}

func TestValidateCustomName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr error
	}{
		{"empty", "", blacksmith.ErrEmptyName},
		{"half-width space", "Holy Sword", blacksmith.ErrNameWhitespace},
		{"full-width space", "聖なる　剣", blacksmith.ErrNameWhitespace},
		{"tab character", "Sword\tX", blacksmith.ErrNameWhitespace},
		{"comma forbidden", "Sword,One", blacksmith.ErrNameForbiddenChar},
		{"semicolon forbidden", "Sword;One", blacksmith.ErrNameForbiddenChar},
		{"quote forbidden", "Sword\"One", blacksmith.ErrNameForbiddenChar},
		{"single quote forbidden", "Sword'One", blacksmith.ErrNameForbiddenChar},
		{"ampersand forbidden", "Sword&Shield", blacksmith.ErrNameForbiddenChar},
		{"less than forbidden", "Sword<1>", blacksmith.ErrNameForbiddenChar},
		{"greater than forbidden", "Sword>1<", blacksmith.ErrNameForbiddenChar},
		{"half-width at forbidden", "Sword@User", blacksmith.ErrNameAtSymbol},
		{"full-width at forbidden", "Sword＠User", blacksmith.ErrNameAtSymbol},
		{"21 runes too long", strings.Repeat("剣", 21), blacksmith.ErrNameTooLong},
		{"20 runes valid", strings.Repeat("剣", 20), nil},
		{"valid Japanese", "真・聖王の魔剣", nil},
		{"valid English", "ExcaliburMarkII", nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := blacksmith.ValidateCustomName(tc.input)
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("ValidateCustomName(%q) error = %v, want %v", tc.input, err, tc.wantErr)
			}
		})
	}
}

func TestCharacterCrystalOperations(t *testing.T) {
	char, _ := corecharacter.New("CrystalTester")
	if char.Crystal != 0 {
		t.Fatalf("expected initial crystal 0, got %d", char.Crystal)
	}

	// Add crystal
	if err := char.AddCrystal(500); err != nil {
		t.Fatal(err)
	}
	if char.Crystal != 500 {
		t.Errorf("got crystal %d, want 500", char.Crystal)
	}

	// Add capped
	if err := char.AddCrystal(1_000_000); err != nil {
		t.Fatal(err)
	}
	if char.Crystal != corecharacter.MaxCrystal {
		t.Errorf("got capped crystal %d, want %d", char.Crystal, corecharacter.MaxCrystal)
	}

	// Deduct
	char.Crystal = 100
	if err := char.DeductCrystal(50); err != nil {
		t.Fatal(err)
	}
	if char.Crystal != 50 {
		t.Errorf("got crystal %d, want 50", char.Crystal)
	}

	// Deduct insufficient
	if err := char.DeductCrystal(100); !errors.Is(err, corecharacter.ErrInsufficientCrystals) {
		t.Errorf("expected ErrInsufficientCrystals, got %v", err)
	}
}

func TestApplySeal(t *testing.T) {
	catalog := setupTestCatalog(t)
	char, _ := corecharacter.New("SealHero")
	char.Crystal = 1000

	charRepo := &mockCharRepo{chars: map[string]corecharacter.Character{char.ID: char}}
	invRepo := &mockInvRepo{invs: make(map[string]coreinventory.Inventory)}
	equipRepo := &mockEquipRepo{equips: make(map[string]coreequipment.Equipment)}
	storageRepo := newMockStorageRepo()

	svc, err := blacksmith.NewService(
		charRepo,
		invRepo,
		catalog,
		blacksmith.WithEquipmentRepository(equipRepo),
		blacksmith.WithStorageRepository(storageRepo),
	)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// 1. Error when no weapon equipped
	_, err = svc.ApplySeal(ctx, char.ID, 1)
	if !errors.Is(err, blacksmith.ErrNoWeaponEquipped) {
		t.Errorf("expected ErrNoWeaponEquipped, got %v", err)
	}

	// Equip weapon-01
	inv, _ := invRepo.FindByCharacterID(ctx, char.ID)
	weaInst, _ := item.NewInstance("weapon-01", 1)
	_ = inv.Add(weaInst)
	_ = invRepo.Save(ctx, inv)

	equip, _ := equipRepo.FindByCharacterID(ctx, char.ID)
	def, _ := catalog.FindByID("weapon-01")
	_, _ = equip.Equip(&inv, def, weaInst.ID)
	_ = equipRepo.Save(ctx, equip)

	// 2. Error when seal not applicable to weapon-01 (seal 7: flame sword only)
	_, err = svc.ApplySeal(ctx, char.ID, 7)
	if !errors.Is(err, blacksmith.ErrSealNotApplicable) {
		t.Errorf("expected ErrSealNotApplicable, got %v", err)
	}

	// 3. Error when insufficient crystals (seal 3 costs 5000, char has 1000)
	_, err = svc.ApplySeal(ctx, char.ID, 3)
	if !errors.Is(err, blacksmith.ErrInsufficientCrystals) {
		t.Errorf("expected ErrInsufficientCrystals, got %v", err)
	}

	// 4. Success applying seal 2 (fang: costs 500 crystal)
	seal, err := svc.ApplySeal(ctx, char.ID, 2)
	if err != nil {
		t.Fatalf("ApplySeal failed: %v", err)
	}
	if seal.ID != 2 || seal.Name != "牙" {
		t.Errorf("unexpected seal: %+v", seal)
	}

	updatedChar, _ := charRepo.FindByID(ctx, char.ID)
	if updatedChar.Crystal != 500 {
		t.Errorf("char crystal = %d, want 500", updatedChar.Crystal)
	}
	if updatedChar.WeaponSeal != 2 {
		t.Errorf("char weapon seal = %d, want 2", updatedChar.WeaponSeal)
	}

	// 5. Overwrite seal with seal 1 (claw: costs 50 crystal)
	seal1, err := svc.ApplySeal(ctx, char.ID, 1)
	if err != nil {
		t.Fatalf("ApplySeal overwrite failed: %v", err)
	}
	if seal1.ID != 1 {
		t.Errorf("unexpected seal: %+v", seal1)
	}
	updatedChar2, _ := charRepo.FindByID(ctx, char.ID)
	if updatedChar2.Crystal != 450 {
		t.Errorf("char crystal = %d, want 450", updatedChar2.Crystal)
	}
	if updatedChar2.WeaponSeal != 1 {
		t.Errorf("char weapon seal = %d, want 1", updatedChar2.WeaponSeal)
	}
}

func TestNameEquipment(t *testing.T) {
	catalog := setupTestCatalog(t)
	char, _ := corecharacter.New("NameHero")

	charRepo := &mockCharRepo{chars: map[string]corecharacter.Character{char.ID: char}}
	invRepo := &mockInvRepo{invs: make(map[string]coreinventory.Inventory)}
	equipRepo := &mockEquipRepo{equips: make(map[string]coreequipment.Equipment)}
	storageRepo := newMockStorageRepo()

	svc, err := blacksmith.NewService(
		charRepo,
		invRepo,
		catalog,
		blacksmith.WithEquipmentRepository(equipRepo),
		blacksmith.WithStorageRepository(storageRepo),
	)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// 1. Error when not equipped
	err = svc.NameEquipment(ctx, char.ID, "weapon", "Excalibur")
	if !errors.Is(err, blacksmith.ErrNoWeaponEquipped) {
		t.Errorf("expected ErrNoWeaponEquipped, got %v", err)
	}
	err = svc.NameEquipment(ctx, char.ID, "armor", "DragonMail")
	if !errors.Is(err, blacksmith.ErrNoArmorEquipped) {
		t.Errorf("expected ErrNoArmorEquipped, got %v", err)
	}

	// Equip weapon and armor
	inv, _ := invRepo.FindByCharacterID(ctx, char.ID)
	wInst, _ := item.NewInstance("weapon-01", 1)
	aInst, _ := item.NewInstance("armor-01", 1)
	_ = inv.Add(wInst)
	_ = inv.Add(aInst)
	_ = invRepo.Save(ctx, inv)

	equip, _ := equipRepo.FindByCharacterID(ctx, char.ID)
	wDef, _ := catalog.FindByID("weapon-01")
	aDef, _ := catalog.FindByID("armor-01")
	_, _ = equip.Equip(&inv, wDef, wInst.ID)
	_, _ = equip.Equip(&inv, aDef, aInst.ID)
	_ = equipRepo.Save(ctx, equip)

	// 2. Error on forbidden char
	err = svc.NameEquipment(ctx, char.ID, "weapon", "Excali@bur")
	if !errors.Is(err, blacksmith.ErrNameAtSymbol) {
		t.Errorf("expected ErrNameAtSymbol, got %v", err)
	}

	// 3. Error on invalid target
	err = svc.NameEquipment(ctx, char.ID, "shield", "Aegis")
	if !errors.Is(err, blacksmith.ErrInvalidTarget) {
		t.Errorf("expected ErrInvalidTarget, got %v", err)
	}

	// 4. Success naming weapon
	if err := svc.NameEquipment(ctx, char.ID, "weapon", "聖剣エクスカリバー"); err != nil {
		t.Fatalf("NameEquipment weapon failed: %v", err)
	}
	c1, _ := charRepo.FindByID(ctx, char.ID)
	if c1.WeaponCustomName != "聖剣エクスカリバー" {
		t.Errorf("weapon name = %q, want 聖剣エクスカリバー", c1.WeaponCustomName)
	}

	// 5. Success naming armor
	if err := svc.NameEquipment(ctx, char.ID, "armor", "竜鱗の鎧"); err != nil {
		t.Fatalf("NameEquipment armor failed: %v", err)
	}
	c2, _ := charRepo.FindByID(ctx, char.ID)
	if c2.ArmorCustomName != "竜鱗の鎧" {
		t.Errorf("armor name = %q, want 竜鱗の鎧", c2.ArmorCustomName)
	}

	// 6. Clearing name
	if err := svc.NameEquipment(ctx, char.ID, "weapon", ""); err != nil {
		t.Fatalf("clear weapon name failed: %v", err)
	}
	c3, _ := charRepo.FindByID(ctx, char.ID)
	if c3.WeaponCustomName != "" {
		t.Errorf("weapon name not cleared: %q", c3.WeaponCustomName)
	}
}

func TestStorageDepositAndWithdraw(t *testing.T) {
	catalog := setupTestCatalog(t)
	char, _ := corecharacter.New("StorageHero")
	char.Crystal = 2000

	charRepo := &mockCharRepo{chars: map[string]corecharacter.Character{char.ID: char}}
	invRepo := &mockInvRepo{invs: make(map[string]coreinventory.Inventory)}
	equipRepo := &mockEquipRepo{equips: make(map[string]coreequipment.Equipment)}
	storageRepo := newMockStorageRepo()

	svc, err := blacksmith.NewService(
		charRepo,
		invRepo,
		catalog,
		blacksmith.WithEquipmentRepository(equipRepo),
		blacksmith.WithStorageRepository(storageRepo),
	)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// 1. Error depositing when nothing equipped
	_, err = svc.DepositWeapon(ctx, char.ID)
	if !errors.Is(err, blacksmith.ErrNoWeaponEquipped) {
		t.Errorf("expected ErrNoWeaponEquipped, got %v", err)
	}

	// Equip weapon-01 and apply seal 2 (fang) + name
	inv, _ := invRepo.FindByCharacterID(ctx, char.ID)
	w1Inst, _ := item.NewInstance("weapon-01", 1)
	_ = inv.Add(w1Inst)
	_ = invRepo.Save(ctx, inv)

	equip, _ := equipRepo.FindByCharacterID(ctx, char.ID)
	w1Def, _ := catalog.FindByID("weapon-01")
	_, _ = equip.Equip(&inv, w1Def, w1Inst.ID)
	_ = equipRepo.Save(ctx, equip)

	_, _ = svc.ApplySeal(ctx, char.ID, 2)
	_ = svc.NameEquipment(ctx, char.ID, "weapon", "CustomClub")

	// 2. Deposit weapon-01
	dep1, err := svc.DepositWeapon(ctx, char.ID)
	if err != nil {
		t.Fatalf("DepositWeapon failed: %v", err)
	}
	if dep1.Slot != 1 || dep1.ItemDefinitionID != "weapon-01" || dep1.SealID != 2 || dep1.CustomName != "CustomClub" {
		t.Errorf("unexpected dep1: %+v", dep1)
	}

	// Verify unequipped, removed from inventory, and seal/name reset on character
	cState, _ := charRepo.FindByID(ctx, char.ID)
	if cState.WeaponSeal != 0 || cState.WeaponCustomName != "" {
		t.Errorf("character seal/name should be reset, got seal=%d, name=%q", cState.WeaponSeal, cState.WeaponCustomName)
	}
	eState, _ := equipRepo.FindByCharacterID(ctx, char.ID)
	if _, equipped := eState.Equipped(item.SlotMainHand); equipped {
		t.Error("weapon should not be equipped after deposit")
	}
	iState, _ := invRepo.FindByCharacterID(ctx, char.ID)
	if _, found := iState.Find(w1Inst.ID); found {
		t.Error("weapon instance should be consumed from inventory after deposit")
	}

	// 3. Equip weapon-36 and deposit into slot 2
	w2Inst, _ := item.NewInstance("weapon-36", 1)
	_ = iState.Add(w2Inst)
	_ = invRepo.Save(ctx, iState)
	w2Def, _ := catalog.FindByID("weapon-36")
	_, _ = eState.Equip(&iState, w2Def, w2Inst.ID)
	_ = equipRepo.Save(ctx, eState)

	dep2, err := svc.DepositWeapon(ctx, char.ID)
	if err != nil {
		t.Fatalf("DepositWeapon 2 failed: %v", err)
	}
	if dep2.Slot != 2 || dep2.ItemDefinitionID != "weapon-36" {
		t.Errorf("unexpected dep2: %+v", dep2)
	}

	// 4. Test duplicate name collision guard ("同じ名前の武器を預けることはできない")
	// Try depositing another weapon with custom name "CustomClub" (same as dep1)
	w3Inst, _ := item.NewInstance("weapon-50", 1)
	_ = iState.Add(w3Inst)
	_ = invRepo.Save(ctx, iState)
	w3Def, _ := catalog.FindByID("weapon-50")
	_, _ = eState.Equip(&iState, w3Def, w3Inst.ID)
	_ = equipRepo.Save(ctx, eState)
	_ = svc.NameEquipment(ctx, char.ID, "weapon", "CustomClub")

	_, err = svc.DepositWeapon(ctx, char.ID)
	if !errors.Is(err, blacksmith.ErrDuplicateStoredName) {
		t.Errorf("expected ErrDuplicateStoredName, got %v", err)
	}

	// Rename and deposit successfully into slot 3
	_ = svc.NameEquipment(ctx, char.ID, "weapon", "IceSwordCustom")
	dep3, err := svc.DepositWeapon(ctx, char.ID)
	if err != nil {
		t.Fatalf("DepositWeapon 3 failed: %v", err)
	}
	if dep3.Slot != 3 {
		t.Errorf("expected slot 3, got %d", dep3.Slot)
	}

	// 5. Test storage full (3 items already stored)
	w4Inst, _ := item.NewInstance("weapon-68", 1)
	_ = iState.Add(w4Inst)
	_ = invRepo.Save(ctx, iState)
	w4Def, _ := catalog.FindByID("weapon-68")
	_, _ = eState.Equip(&iState, w4Def, w4Inst.ID)
	_ = equipRepo.Save(ctx, eState)

	_, err = svc.DepositWeapon(ctx, char.ID)
	if !errors.Is(err, blacksmith.ErrStorageFull) {
		t.Errorf("expected ErrStorageFull, got %v", err)
	}

	// 6. Test withdrawing when weapon already equipped -> "おっと、武器を外してからにしてくれ！"
	err = svc.WithdrawWeapon(ctx, char.ID, 1)
	if !errors.Is(err, blacksmith.ErrWeaponSlotOccupied) {
		t.Errorf("expected ErrWeaponSlotOccupied, got %v", err)
	}

	// Unequip weapon-68
	_, _ = eState.Unequip(item.SlotMainHand)
	_ = equipRepo.Save(ctx, eState)

	// 7. Withdraw slot 1 successfully
	if err := svc.WithdrawWeapon(ctx, char.ID, 1); err != nil {
		t.Fatalf("WithdrawWeapon slot 1 failed: %v", err)
	}

	// Verify weapon equipped, seal restored, name restored
	cAfterWithdraw, _ := charRepo.FindByID(ctx, char.ID)
	if cAfterWithdraw.WeaponSeal != 2 || cAfterWithdraw.WeaponCustomName != "CustomClub" {
		t.Errorf("restored seal = %d, name = %q (want seal=2, name=CustomClub)",
			cAfterWithdraw.WeaponSeal, cAfterWithdraw.WeaponCustomName)
	}
	eAfterWithdraw, _ := equipRepo.FindByCharacterID(ctx, char.ID)
	instAfter, ok := eAfterWithdraw.Equipped(item.SlotMainHand)
	if !ok {
		t.Fatal("weapon should be equipped after withdraw")
	}
	iAfterWithdraw, _ := invRepo.FindByCharacterID(ctx, char.ID)
	itemAfter, found := iAfterWithdraw.Find(instAfter)
	if !found || itemAfter.DefinitionID != "weapon-01" {
		t.Errorf("unexpected item after withdraw: %+v", itemAfter)
	}

	// 8. Slot 1 should no longer be in storage
	deposits, err := svc.ListDeposits(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(deposits) != 2 {
		t.Fatalf("expected 2 deposits remaining, got %d", len(deposits))
	}
	for _, d := range deposits {
		if d.Slot == 1 {
			t.Errorf("slot 1 should have been deleted from storage")
		}
	}
}
