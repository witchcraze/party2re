package battle_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	corejob "github.com/witchcraze/party2re/internal/core/job"
)

func TestPlayerTailEquipmentAdapterReloadAndNativeBindings(t *testing.T) {
	ctx := context.Background()
	chars, inventory, equipment := newMockCharRepo(), newMockInvRepo(), newMockEquipRepo()
	char := corecharacter.Character{ID: "self", Level: 30, JobID: "job-01", Stats: corecharacter.Stats{HP: 100, MaxHP: 1000, MP: 100, MaxMP: 1000, Attack: 100, Defense: 100, Agility: 100}}
	chars.characters[char.ID] = char
	items := []coreitem.Instance{
		{ID: "weapon", DefinitionID: "weapon-35", Quantity: 1},
		{ID: "armor", DefinitionID: "armor-37", Quantity: 1},
		{ID: "accessory", DefinitionID: "item-202", Quantity: 1},
		{ID: "bag", DefinitionID: "item-189", Quantity: 1},
	}
	if err := inventory.Save(ctx, coreinventory.Inventory{CharacterID: char.ID, Items: items}); err != nil {
		t.Fatal(err)
	}
	if err := equipment.Save(ctx, coreequipment.Equipment{CharacterID: char.ID, Slots: map[coreitem.Slot]string{coreitem.SlotMainHand: "weapon", coreitem.SlotBody: "armor", coreitem.SlotAccessory: "accessory"}}); err != nil {
		t.Fatal(err)
	}
	svc := battle.NewService(battle.WithCharacterRepository(chars), battle.WithInventoryRepository(inventory), battle.WithEquipmentRepository(equipment), battle.WithRandomSource(mockRNG{}), battle.WithJobHistoryRepository(&historyReadStub{err: corejob.ErrCharacterJobNotFound}))
	a, err := svc.BuildLifetimeActor(ctx, char.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	state := corebattle.LifetimeState{Actors: []corebattle.LifetimeActor{a, {Participant: corebattle.Participant{ID: "enemy", TeamID: "enemy", HP: 1000}}}}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	var loaded corebattle.LifetimeState
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	if a := loaded.Actors[0]; a.Gear.WeaponID != "weapon-35" || a.Gear.ArmorID != "armor-37" || a.Gear.AccessoryID != "item-202" || a.Gear.WeaponAttack == nil || len(a.ItemDefinitionIDs) != 3 || a.BaseStats.Attack != 100 {
		t.Fatal(a)
	}
	c := corebattle.PlayerTailContext{DamageContext: corebattle.DamageContext{EffectContext: corebattle.EffectContext{TargetContext: corebattle.TargetContext{ActorID: "self", PartyIDs: []string{"self"}, EnemyIDs: []string{"enemy"}}}}, Completion: corebattle.LifetimeCompletion{Accepted: true, ActorID: "self", EnemiesRemain: true}, NormalAction: true}
	rng := &resetTableRNG{t: t}
	// Bag-only189 must not execute.202 uses its temporary hook; the weapon set
	// independently uses an unmodified tool heal at the current low HP.
	loaded.Actors[0].Temporary = corebattle.TemporaryDouble
	for _, id := range []string{"item-189-normal-tail", "item-202-combat-tail", "weapon-35-armor-37-tail"} {
		if err := loaded.ApplyPlayerTail(&c, id, rng); err != nil {
			t.Fatal(err)
		}
	}
	if loaded.Actors[0].HP != 314 || loaded.Actors[0].MP != 100 || rng.draws != 2 || chars.characters[char.ID].Stats.HP != 100 {
		t.Fatal(loaded.Actors[0], rng.draws)
	}
}
