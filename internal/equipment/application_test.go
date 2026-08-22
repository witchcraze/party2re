package equipment

import (
	"context"
	"testing"

	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
	"github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
)

type repositoryStub struct{ value coreequipment.Equipment }

func (r *repositoryStub) Save(_ context.Context, value coreequipment.Equipment) error {
	r.value = value
	return nil
}
func (r *repositoryStub) FindByCharacterID(_ context.Context, _ string) (coreequipment.Equipment, error) {
	return r.value, nil
}

func TestServiceEquipAndUnequipPersistsState(t *testing.T) {
	owned, _ := inventory.New("character-1")
	instance, _ := item.NewInstance("sword", 1)
	_ = owned.Add(instance)
	definition, _ := item.NewEquipmentDefinition("sword", "Training Sword", item.SlotMainHand)
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
