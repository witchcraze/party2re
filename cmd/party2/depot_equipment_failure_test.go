package main

import (
	"context"
	"errors"
	"reflect"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/depot"
)

var errDepotEquipmentPersistence = errors.New("equipment movement persistence failed")

type depotEquipmentFailure struct {
	*database.EquipmentRepository
	stage string
}

func (r depotEquipmentFailure) FindByCharacterIDForUpdate(ctx context.Context, id string) (coreequipment.Equipment, error) {
	if r.stage == "equipment read" {
		return coreequipment.Equipment{}, errDepotEquipmentPersistence
	}
	return r.EquipmentRepository.FindByCharacterIDForUpdate(ctx, id)
}

func (r depotEquipmentFailure) Save(ctx context.Context, eq coreequipment.Equipment) error {
	if err := r.EquipmentRepository.Save(ctx, eq); err != nil {
		return err
	}
	if r.stage == "equipment save" {
		return errDepotEquipmentPersistence
	}
	return nil
}

type depotMovementInventoryFailure struct{ *database.InventoryRepository }

func (r depotMovementInventoryFailure) Save(ctx context.Context, inv coreinventory.Inventory) error {
	if err := r.InventoryRepository.Save(ctx, inv); err != nil {
		return err
	}
	return errDepotEquipmentPersistence
}

type depotMovementStorageFailure struct{ *database.DepotRepository }

func (r depotMovementStorageFailure) Save(ctx context.Context, dep depot.Depot) error {
	if err := r.DepotRepository.Save(ctx, dep); err != nil {
		return err
	}
	return errDepotEquipmentPersistence
}

type depotMovementCharacterFailure struct{ *database.CharacterRepository }

func (r depotMovementCharacterFailure) Update(ctx context.Context, char corecharacter.Character) error {
	if err := r.CharacterRepository.Update(ctx, char); err != nil {
		return err
	}
	return errDepotEquipmentPersistence
}

type depotEquipmentState struct {
	sender, recipient           corecharacter.Character
	inventory                   coreinventory.Inventory
	equipment                   coreequipment.Equipment
	senderDepot, recipientDepot depot.Depot
}

func (f depotEquipmentFixture) state(t *testing.T) depotEquipmentState {
	t.Helper()
	ctx := context.Background()
	var state depotEquipmentState
	var err error
	state.sender, err = f.core.charRepo.FindByID(ctx, f.sender.ID)
	if err != nil {
		t.Fatal(err)
	}
	state.recipient, err = f.core.charRepo.FindByID(ctx, f.recipient.ID)
	if err != nil {
		t.Fatal(err)
	}
	state.inventory, err = f.core.invRepo.FindByCharacterID(ctx, f.sender.ID)
	if err != nil {
		t.Fatal(err)
	}
	state.equipment, err = f.econ.equipRepo.FindByCharacterID(ctx, f.sender.ID)
	if err != nil {
		t.Fatal(err)
	}
	state.senderDepot, err = f.econ.depotRepo.FindByCharacterID(ctx, f.sender.ID)
	if err != nil {
		t.Fatal(err)
	}
	state.recipientDepot, err = f.econ.depotRepo.FindByCharacterID(ctx, f.recipient.ID)
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func TestWireDepotEquipmentPersistenceRollback_Integration(t *testing.T) {
	for _, send := range []bool{false, true} {
		name := "deposit"
		if send {
			name = "send"
		}
		for _, stage := range []string{"equipment read", "equipment save", "character save", "inventory save", "depot save"} {
			t.Run(name+"/"+stage, func(t *testing.T) {
				f := newDepotEquipmentFixture(t, item.SlotMainHand, true)
				var inv depot.InventoryRepository = f.core.invRepo
				var chars depot.CharacterRepository = f.core.charRepo
				var storage depot.Repository = f.econ.depotRepo
				if stage == "inventory save" {
					inv = depotMovementInventoryFailure{f.core.invRepo}
				}
				if stage == "character save" {
					chars = depotMovementCharacterFailure{f.core.charRepo}
				}
				if stage == "depot save" {
					storage = depotMovementStorageFailure{f.econ.depotRepo}
				}
				svc, err := depot.NewServiceWithTransaction(storage, chars, inv, f.core.txProvider,
					depot.WithItemDefinitionProvider(f.core.itemCatalog),
					depot.WithEquipmentRepository(depotEquipmentFailure{f.econ.equipRepo, stage}))
				if err != nil {
					t.Fatal(err)
				}
				f.econ.depot = svc
				before := f.state(t)
				if _, err := f.move(context.Background(), send); !errors.Is(err, errDepotEquipmentPersistence) {
					t.Fatalf("error = %v", err)
				}
				if after := f.state(t); !reflect.DeepEqual(before, after) {
					t.Fatalf("failed %s changed state:\nbefore=%+v\nafter=%+v", stage, before, after)
				}
				f.econ.depot, err = depot.NewServiceWithTransaction(f.econ.depotRepo, f.core.charRepo, f.core.invRepo, f.core.txProvider,
					depot.WithItemDefinitionProvider(f.core.itemCatalog), depot.WithEquipmentRepository(f.econ.equipRepo))
				if err != nil {
					t.Fatal(err)
				}
				if _, err := f.move(context.Background(), send); err != nil {
					t.Fatalf("retry: %v", err)
				}
				f.assertMoved(t, item.SlotMainHand, true, send)
			})
		}
	}
}
