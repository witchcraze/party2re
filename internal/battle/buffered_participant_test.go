package battle_test

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/random"
)

func TestBuildParticipantWithBufferedConsumption(t *testing.T) {
	ctx := context.Background()
	chars, invs, equips := newMockCharRepo(), newMockInvRepo(), newMockEquipRepo()
	chars.characters["hero"] = corecharacter.Character{ID: "hero", Stats: corecharacter.Stats{
		HP: 100, MaxHP: 100, MP: 30, MaxMP: 50, Attack: 1000, Defense: 0}}
	inst, err := coreitem.NewInstance(battle.ItemDokuroAmulet, 1)
	if err != nil {
		t.Fatal(err)
	}
	invs.inventories["hero"] = coreinventory.Inventory{CharacterID: "hero", Items: []coreitem.Instance{inst}}
	equips.equipments["hero"] = coreequipment.Equipment{CharacterID: "hero", Slots: map[coreitem.Slot]string{coreitem.SlotAccessory: inst.ID}}
	svc := battle.NewService(battle.WithCharacterRepository(chars), battle.WithInventoryRepository(invs), battle.WithEquipmentRepository(equips))
	mp := 0
	p, err := svc.BuildParticipantWithState(ctx, "hero", 1, &mp, nil)
	if err != nil {
		t.Fatal(err)
	}
	if p.HP != 1 || p.MP != 0 || !slices.Contains(p.Abilities, "dokuro_amulet") {
		t.Fatalf("initial: %+v", p)
	}
	// The concrete amulet revives once when a faster, lethal enemy strikes.
	enemy := corebattle.MustNewParticipant("enemy", 5, 2, 0)
	enemy.Agility = 1000
	res, err := (corebattle.Engine{}).ResolvePartyBattle(corebattle.PartyBattleRequest{Allies: []corebattle.Participant{p}, Enemies: []corebattle.Participant{enemy}, RNG: random.NewDeterministic(1)})
	if err != nil {
		t.Fatal(err)
	}
	consumed := res.ConsumedItems["hero"]
	if res.WinnerSide != "allies" || len(consumed) != 1 || consumed[0].ID != battle.ItemDokuroAmulet {
		t.Fatalf("revival result: %+v", res)
	}
	p, err = svc.BuildParticipantWithState(ctx, "hero", res.RemainingHP["hero"], &mp, consumed)
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(p.Abilities, "dokuro_amulet") || slices.Contains(p.ItemDefinitionIDs, battle.ItemDokuroAmulet) || p.Gear.AccessoryID != "" {
		t.Fatalf("consumed amulet rebuilt: %+v", p)
	}
	if !reflect.DeepEqual(invs.inventories["hero"].Items, []coreitem.Instance{inst}) || equips.equipments["hero"].Slots[coreitem.SlotAccessory] != inst.ID {
		t.Fatal("buffer projection mutated repository state")
	}
	for _, entry := range []corebattle.ConsumedItem{{ID: inst.DefinitionID, InstanceID: "foreign", Quantity: 1}, {ID: inst.DefinitionID, Quantity: 2}, {ID: inst.DefinitionID, Quantity: 0}} {
		_, err := svc.BuildParticipantWithState(ctx, "hero", 5, &mp, []corebattle.ConsumedItem{entry})
		if !errors.Is(err, coreinventory.ErrItemNotFound) && !errors.Is(err, coreinventory.ErrInvalidQuantity) {
			t.Fatalf("invalid consumption %+v: %v", entry, err)
		}
	}
}
