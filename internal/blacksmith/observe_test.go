package blacksmith_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/witchcraze/party2re/internal/blacksmith"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
)

type failingStorage struct {
	blacksmith.StorageRepository
	err error
}

func (s failingStorage) ListByCharacterID(context.Context, string) ([]blacksmith.Deposit, error) {
	return nil, s.err
}

func TestObserve(t *testing.T) {
	ctx := context.Background()
	char, err := corecharacter.New("Observer")
	if err != nil {
		t.Fatal(err)
	}
	char.Crystal, char.WeaponSeal, char.ArmorCustomName = 70, 7, "銘鎧"
	charRepo := &mockCharRepo{chars: map[string]corecharacter.Character{char.ID: char}}
	invRepo := &mockInvRepo{invs: map[string]coreinventory.Inventory{}}
	equipRepo := &mockEquipRepo{equips: map[string]coreequipment.Equipment{}}
	storage := newMockStorageRepo()
	svc, err := blacksmith.NewService(charRepo, invRepo, setupTestCatalog(t), blacksmith.WithEquipmentRepository(equipRepo), blacksmith.WithStorageRepository(storage))
	if err != nil {
		t.Fatal(err)
	}

	empty, err := svc.Observe(ctx, char.ID)
	if err != nil || empty.Weapon != nil || empty.Armor != nil || empty.AvailableSealIDs == nil || len(empty.AvailableSealIDs) != 0 || empty.Deposits == nil || len(empty.Deposits) != 0 {
		t.Fatalf("no equipment must not offer seals and arrays must be non-null: %+v, %v", empty, err)
	}

	inv, _ := invRepo.FindByCharacterID(ctx, char.ID)
	equip, _ := equipRepo.FindByCharacterID(ctx, char.ID)
	for _, definition := range []struct {
		id   string
		slot item.Slot
	}{{"weapon-36", item.SlotMainHand}, {"armor-01", item.SlotBody}} {
		instance, err := item.NewInstance(definition.id, 1)
		if err != nil {
			t.Fatal(err)
		}
		if err := inv.Add(instance); err != nil {
			t.Fatal(err)
		}
		equip.Slots[definition.slot] = instance.ID
	}
	_ = invRepo.Save(ctx, inv)
	_ = equipRepo.Save(ctx, equip)
	for _, deposit := range []blacksmith.Deposit{
		{ID: "b", CharacterID: char.ID, Slot: 2, ItemDefinitionID: "weapon-01", CustomName: "愛棒", SealID: 3},
		{ID: "a", CharacterID: char.ID, Slot: 1, ItemDefinitionID: "weapon-01"},
	} {
		if err := storage.Save(ctx, deposit); err != nil {
			t.Fatal(err)
		}
	}
	before, _ := charRepo.FindByID(ctx, char.ID)

	got, err := svc.Observe(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	wantSeals := []int{}
	for _, seal := range blacksmith.ListAvailableSeals("weapon-36") {
		wantSeals = append(wantSeals, seal.ID)
	}
	if got.Crystal != 70 || got.WeaponSealID != 7 ||
		!reflect.DeepEqual(got.Weapon, &blacksmith.EquippedItem{ItemDefinitionID: "weapon-36", Name: "炎の剣"}) ||
		!reflect.DeepEqual(got.Armor, &blacksmith.EquippedItem{ItemDefinitionID: "armor-01", Name: "銘鎧", CustomName: "銘鎧"}) ||
		!reflect.DeepEqual(got.AvailableSealIDs, wantSeals) {
		t.Fatalf("observation: %+v", got)
	}
	if len(got.Deposits) != 2 {
		t.Fatalf("deposits: %+v", got.Deposits)
	}
	names := map[int]string{got.Deposits[0].Slot: got.Deposits[0].Name, got.Deposits[1].Slot: got.Deposits[1].Name}
	if names[1] != "ひのきの棒" || names[2] != "愛棒" {
		t.Fatalf("effective stored names: %+v", names)
	}
	if after, _ := charRepo.FindByID(ctx, char.ID); !reflect.DeepEqual(before, after) {
		t.Fatal("observation mutated the character")
	}
}

func TestObserveSurfacesRequiredReadFailures(t *testing.T) {
	ctx := context.Background()
	char, err := corecharacter.New("Broken")
	if err != nil {
		t.Fatal(err)
	}
	charRepo := &mockCharRepo{chars: map[string]corecharacter.Character{char.ID: char}}
	invRepo := &mockInvRepo{invs: map[string]coreinventory.Inventory{}}
	equipRepo := &mockEquipRepo{equips: map[string]coreequipment.Equipment{}}
	catalog := setupTestCatalog(t)

	want := errors.New("storage down")
	svc, _ := blacksmith.NewService(charRepo, invRepo, catalog, blacksmith.WithEquipmentRepository(equipRepo), blacksmith.WithStorageRepository(failingStorage{err: want}))
	if _, err := svc.Observe(ctx, char.ID); !errors.Is(err, want) {
		t.Fatalf("storage failure: %v", err)
	}

	storage := newMockStorageRepo()
	svc, _ = blacksmith.NewService(charRepo, invRepo, catalog, blacksmith.WithEquipmentRepository(equipRepo), blacksmith.WithStorageRepository(storage))
	equip, _ := equipRepo.FindByCharacterID(ctx, char.ID)
	equip.Slots[item.SlotMainHand] = "dangling"
	_ = equipRepo.Save(ctx, equip)
	if _, err := svc.Observe(ctx, char.ID); err == nil {
		t.Fatal("dangling equipped instance observed as an empty slot")
	}

	equip.Slots = map[item.Slot]string{}
	_ = equipRepo.Save(ctx, equip)
	if err := storage.Save(ctx, blacksmith.Deposit{ID: "x", CharacterID: char.ID, Slot: 1, ItemDefinitionID: "unknown"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Observe(ctx, char.ID); err == nil {
		t.Fatal("unresolved stored definition hidden")
	}
}
