package inventory

import (
	"context"
	"testing"

	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
)

type repositoryStub struct {
	value coreinventory.Inventory
}

func (r *repositoryStub) Save(_ context.Context, value coreinventory.Inventory) error {
	r.value = value
	return nil
}

func (r *repositoryStub) FindByCharacterID(_ context.Context, _ string) (coreinventory.Inventory, error) {
	return r.value, nil
}

func TestServiceAddAndConsumePersistsInventory(t *testing.T) {
	value, _ := coreinventory.New("character-1")
	instance, _ := item.NewInstance("potion", 2)
	repository := &repositoryStub{value: value}
	service, err := NewService(repository)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := service.Add(context.Background(), "character-1", instance); err != nil {
		t.Fatal(err)
	}
	got, err := service.Consume(context.Background(), "character-1", instance.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.Quantity("potion") != 1 || repository.value.Quantity("potion") != 1 {
		t.Fatalf("inventory = %#v, saved = %#v", got, repository.value)
	}
}
