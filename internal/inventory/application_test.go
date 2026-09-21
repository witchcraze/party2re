package inventory

import (
	"context"
	"errors"
	"testing"

	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
)

type repositoryStub struct {
	value   coreinventory.Inventory
	findErr error
	saveErr error
}

func (r *repositoryStub) Save(_ context.Context, value coreinventory.Inventory) error {
	if r.saveErr != nil {
		return r.saveErr
	}
	r.value = value
	return nil
}

func (r *repositoryStub) FindByCharacterID(_ context.Context, _ string) (coreinventory.Inventory, error) {
	if r.findErr != nil {
		return coreinventory.Inventory{}, r.findErr
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

func TestServiceFindByCharacterID(t *testing.T) {
	value, _ := coreinventory.New("character-1")
	instance, _ := item.NewInstance("potion", 2)
	_ = value.Add(instance)
	repository := &repositoryStub{value: value}
	service, err := NewService(repository)
	if err != nil {
		t.Fatal(err)
	}

	got, err := service.FindByCharacterID(context.Background(), "character-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Quantity("potion") != 2 {
		t.Fatalf("expected 2 potions, got %d", got.Quantity("potion"))
	}
}

func TestServiceAdd_Errors(t *testing.T) {
	value, _ := coreinventory.New("character-1")
	instance, _ := item.NewInstance("potion", 1)

	// 1. FindByCharacterID error
	findErr := errors.New("db find failed")
	repo := &repositoryStub{value: value, findErr: findErr}
	svc, _ := NewService(repo)
	if _, err := svc.Add(context.Background(), "character-1", instance); !errors.Is(err, findErr) {
		t.Fatalf("expected findErr, got %v", err)
	}

	// 2. Add domain error (invalid instance)
	repo = &repositoryStub{value: value}
	svc, _ = NewService(repo)
	invalidInstance := item.Instance{}
	if _, err := svc.Add(context.Background(), "character-1", invalidInstance); !errors.Is(err, coreinventory.ErrInvalidInventory) {
		t.Fatalf("expected ErrInvalidInventory, got %v", err)
	}

	// 3. Save error
	saveErr := errors.New("db save failed")
	repo = &repositoryStub{value: value, saveErr: saveErr}
	svc, _ = NewService(repo)
	if _, err := svc.Add(context.Background(), "character-1", instance); !errors.Is(err, saveErr) {
		t.Fatalf("expected saveErr, got %v", err)
	}
}

func TestServiceConsume_Errors(t *testing.T) {
	value, _ := coreinventory.New("character-1")
	instance, _ := item.NewInstance("potion", 2)
	_ = value.Add(instance)

	// 1. FindByCharacterID error
	findErr := errors.New("db find failed")
	repo := &repositoryStub{value: value, findErr: findErr}
	svc, _ := NewService(repo)
	if _, err := svc.Consume(context.Background(), "character-1", instance.ID, 1); !errors.Is(err, findErr) {
		t.Fatalf("expected findErr, got %v", err)
	}

	// 2. Consume domain error (item not found / invalid quantity)
	repo = &repositoryStub{value: value}
	svc, _ = NewService(repo)
	if _, err := svc.Consume(context.Background(), "character-1", "non-existent-id", 1); !errors.Is(err, coreinventory.ErrItemNotFound) {
		t.Fatalf("expected ErrItemNotFound, got %v", err)
	}

	// 3. Save error
	saveErr := errors.New("db save failed")
	repo = &repositoryStub{value: value, saveErr: saveErr}
	svc, _ = NewService(repo)
	if _, err := svc.Consume(context.Background(), "character-1", instance.ID, 1); !errors.Is(err, saveErr) {
		t.Fatalf("expected saveErr, got %v", err)
	}
}

func TestServiceFindByCharacterID_Error(t *testing.T) {
	findErr := errors.New("db find failed")
	repo := &repositoryStub{findErr: findErr}
	svc, _ := NewService(repo)

	if _, err := svc.FindByCharacterID(context.Background(), "character-1"); !errors.Is(err, findErr) {
		t.Fatalf("expected findErr, got %v", err)
	}
}
