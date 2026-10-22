package battle_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	corejob "github.com/witchcraze/party2re/internal/core/job"
	"github.com/witchcraze/party2re/internal/economy"
)

type failingInventoryRead struct {
	battle.InventoryRepository
	err error
}

func (r failingInventoryRead) FindByCharacterID(context.Context, string) (coreinventory.Inventory, error) {
	return coreinventory.Inventory{}, r.err
}

type failingEquipmentRead struct {
	battle.EquipmentRepository
	err error
}

func (r failingEquipmentRead) FindByCharacterID(context.Context, string) (coreequipment.Equipment, error) {
	return coreequipment.Equipment{}, r.err
}

func TestParticipantRepositoryReads(t *testing.T) {
	storageErr := errors.New("storage unavailable")
	for _, entry := range []string{"participant", "current HP", "buffered state", "lifetime"} {
		for _, scenario := range []string{"optional omitted", "empty", "equipped", "inventory error", "equipment error", "inventory canceled", "equipment canceled"} {
			t.Run(entry+"/"+scenario, func(t *testing.T) {
				chars, invs, equips := newMockCharRepo(), newMockInvRepo(), newMockEquipRepo()
				chars.characters["hero"] = corecharacter.Character{ID: "hero", JobID: "job-01", Stats: corecharacter.Stats{HP: 100, MaxHP: 100, MP: 30, MaxMP: 50}}
				opts := []battle.Option{battle.WithCharacterRepository(chars), battle.WithJobHistoryRepository(&historyReadStub{err: corejob.ErrCharacterJobNotFound})}
				var wantErr error
				if scenario != "optional omitted" {
					opts = append(opts, battle.WithInventoryRepository(invs), battle.WithEquipmentRepository(equips))
				}
				switch scenario {
				case "equipped":
					inst := coreitem.Instance{ID: "amulet", DefinitionID: battle.ItemDokuroAmulet, Quantity: 1}
					invs.inventories["hero"] = coreinventory.Inventory{CharacterID: "hero", Items: []coreitem.Instance{inst}}
					equips.equipments["hero"] = coreequipment.Equipment{CharacterID: "hero", Slots: map[coreitem.Slot]string{coreitem.SlotAccessory: inst.ID}}
				case "inventory error", "inventory canceled":
					wantErr = storageErr
					if scenario == "inventory canceled" {
						wantErr = context.Canceled
					}
					opts = append(opts, battle.WithInventoryRepository(failingInventoryRead{invs, wantErr}))
				case "equipment error", "equipment canceled":
					wantErr = storageErr
					if scenario == "equipment canceled" {
						wantErr = context.Canceled
					}
					opts = append(opts, battle.WithEquipmentRepository(failingEquipmentRead{equips, wantErr}))
				}
				svc := battle.NewService(opts...)
				ctx := context.Background()
				var p corebattle.Participant
				var err error
				switch entry {
				case "participant":
					p, err = svc.BuildParticipant(ctx, "hero")
				case "current HP":
					p, err = svc.BuildParticipantWithCurrentHP(ctx, "hero", 10)
				case "buffered state":
					mp := 0
					p, err = svc.BuildParticipantWithState(ctx, "hero", 10, &mp, nil)
				case "lifetime":
					var actor corebattle.LifetimeActor
					actor, err = svc.BuildLifetimeActor(ctx, "hero", "")
					p = actor.Participant
				}
				if !errors.Is(err, wantErr) {
					t.Fatalf("error=%v, want %v", err, wantErr)
				}
				if wantErr != nil {
					if !reflect.DeepEqual(p, corebattle.Participant{}) {
						t.Fatalf("failed read returned a participant: %+v", p)
					}
					return
				}
				accessory := ""
				if scenario == "equipped" {
					accessory = battle.ItemDokuroAmulet
				}
				if p.Gear == nil || p.Gear.AccessoryID != accessory {
					t.Fatalf("gear=%+v, want accessory %q", p.Gear, accessory)
				}
			})
		}
	}
}

func TestPostBattleEquipmentReads(t *testing.T) {
	for _, route := range []string{"single runner", "multi provider"} {
		for _, scenario := range []string{"optional omitted", "empty", "error", "canceled"} {
			t.Run(route+"/"+scenario, func(t *testing.T) {
				chars, invs, equips := newMockCharRepo(), newMockInvRepo(), newMockEquipRepo()
				ids := []string{"hero"}
				if route == "multi provider" {
					ids = append(ids, "member")
				}
				for _, id := range ids {
					chars.characters[id] = corecharacter.Character{ID: id, Stats: corecharacter.Stats{HP: 100, MaxHP: 100}}
				}
				opts := []battle.Option{battle.WithCharacterRepository(chars), battle.WithInventoryRepository(invs)}
				provider := &mockTxProvider{}
				if route == "single runner" {
					runner, err := economy.NewService(chars, invs, economy.WithTransactionProvider(provider))
					if err != nil {
						t.Fatal(err)
					}
					opts = append(opts, battle.WithTransactionRunner(runner))
				} else {
					opts = append(opts, battle.WithTransactionProvider(provider))
				}
				var wantErr error
				switch scenario {
				case "empty":
					opts = append(opts, battle.WithEquipmentRepository(equips))
				case "error", "canceled":
					wantErr = errors.New("equipment unavailable")
					if scenario == "canceled" {
						wantErr = context.Canceled
					}
					opts = append(opts, battle.WithEquipmentRepository(failingEquipmentRead{equips, wantErr}))
				}
				_, err := battle.NewService(opts...).ApplyPostBattleResult(context.Background(), battle.ApplyPostBattleRequest{
					CharacterIDs: ids, BattleResult: corebattle.PartyBattleResult{RemainingHP: map[string]int{"hero": 5, "member": 10}},
				})
				if !errors.Is(err, wantErr) {
					t.Fatalf("error=%v, want %v", err, wantErr)
				}
				wantHP := 5
				if wantErr != nil {
					wantHP = 100
				}
				if chars.characters["hero"].Stats.HP != wantHP {
					t.Fatalf("HP=%d, want %d", chars.characters["hero"].Stats.HP, wantHP)
				}
			})
		}
	}
}
