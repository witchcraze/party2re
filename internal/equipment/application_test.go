package equipment

import (
	"context"
	"errors"
	"testing"

	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
	"github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
)

type repositoryStub struct {
	value   coreequipment.Equipment
	findErr error
	saveErr error
}

func (r *repositoryStub) Save(_ context.Context, value coreequipment.Equipment) error {
	if r.saveErr != nil {
		return r.saveErr
	}
	r.value = value
	return nil
}
func (r *repositoryStub) FindByCharacterID(_ context.Context, _ string) (coreequipment.Equipment, error) {
	if r.findErr != nil {
		return coreequipment.Equipment{}, r.findErr
	}
	return r.value, nil
}

func TestNewService_NilRepository(t *testing.T) {
	svc, err := NewService(nil)
	if err == nil {
		t.Fatal("expected error when repository is nil, got nil")
	}
	if svc != nil {
		t.Fatalf("expected nil service, got %#v", svc)
	}
}

func TestServiceEquipAndUnequipPersistsState(t *testing.T) {
	owned, _ := inventory.New("character-1")
	instance, _ := item.NewInstance("sword", 1)
	_ = owned.Add(instance)
	definition, _ := item.NewEquipmentDefinition("sword", "Training Sword", 100, item.SlotMainHand)
	value, _ := coreequipment.New("character-1")
	repository := &repositoryStub{value: value}
	service, _ := NewService(repository)

	if _, err := service.Equip(context.Background(), "character-1", &owned, definition, instance.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Unequip(context.Background(), "character-1", item.SlotMainHand); err != nil {
		t.Fatal(err)
	}
	if len(repository.value.Slots) != 0 {
		t.Fatalf("saved equipment = %#v", repository.value)
	}
}

func TestServiceEquip_Errors(t *testing.T) {
	owned, _ := inventory.New("character-1")
	instance, _ := item.NewInstance("sword", 1)
	_ = owned.Add(instance)
	definition, _ := item.NewEquipmentDefinition("sword", "Training Sword", 100, item.SlotMainHand)
	value, _ := coreequipment.New("character-1")

	// 1. FindByCharacterID error
	findErr := errors.New("db find failed")
	repo := &repositoryStub{value: value, findErr: findErr}
	svc, _ := NewService(repo)
	if _, err := svc.Equip(context.Background(), "character-1", &owned, definition, instance.ID); !errors.Is(err, findErr) {
		t.Fatalf("expected findErr, got %v", err)
	}

	// 2. Equip domain error (not owned / missing instance)
	repo = &repositoryStub{value: value}
	svc, _ = NewService(repo)
	if _, err := svc.Equip(context.Background(), "character-1", &owned, definition, "non-existent-instance"); !errors.Is(err, coreequipment.ErrNotOwned) {
		t.Fatalf("expected ErrNotOwned, got %v", err)
	}

	// 3. Save error
	saveErr := errors.New("db save failed")
	repo = &repositoryStub{value: value, saveErr: saveErr}
	svc, _ = NewService(repo)
	if _, err := svc.Equip(context.Background(), "character-1", &owned, definition, instance.ID); !errors.Is(err, saveErr) {
		t.Fatalf("expected saveErr, got %v", err)
	}
}

func TestServiceUnequip_Errors(t *testing.T) {
	value, _ := coreequipment.New("character-1")

	// 1. FindByCharacterID error
	findErr := errors.New("db find failed")
	repo := &repositoryStub{value: value, findErr: findErr}
	svc, _ := NewService(repo)
	if _, err := svc.Unequip(context.Background(), "character-1", item.SlotMainHand); !errors.Is(err, findErr) {
		t.Fatalf("expected findErr, got %v", err)
	}

	// 2. Unequip domain error (slot is empty)
	repo = &repositoryStub{value: value}
	svc, _ = NewService(repo)
	if _, err := svc.Unequip(context.Background(), "character-1", item.SlotMainHand); !errors.Is(err, coreequipment.ErrSlotEmpty) {
		t.Fatalf("expected ErrSlotEmpty, got %v", err)
	}

	// 3. Save error (equip first, then unequip with save error)
	owned, _ := inventory.New("character-1")
	instance, _ := item.NewInstance("sword", 1)
	_ = owned.Add(instance)
	definition, _ := item.NewEquipmentDefinition("sword", "Training Sword", 100, item.SlotMainHand)
	_, _ = value.Equip(&owned, definition, instance.ID)

	saveErr := errors.New("db save failed")
	repo = &repositoryStub{value: value, saveErr: saveErr}
	svc, _ = NewService(repo)
	if _, err := svc.Unequip(context.Background(), "character-1", item.SlotMainHand); !errors.Is(err, saveErr) {
		t.Fatalf("expected saveErr, got %v", err)
	}
}
