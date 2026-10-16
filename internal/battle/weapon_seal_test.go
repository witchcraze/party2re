package battle_test

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	corejob "github.com/witchcraze/party2re/internal/core/job"
	"github.com/witchcraze/party2re/internal/core/random"
	"github.com/witchcraze/party2re/internal/core/skill"
)

func TestWeaponSealAdapterPreservesAdmissionFactsOnReload(t *testing.T) {
	ctx := context.Background()
	chars, inventory, equipment := newMockCharRepo(), newMockInvRepo(), newMockEquipRepo()
	char := corecharacter.Character{ID: "self", JobID: "job-1", WeaponSeal: 7, OldJobID: "job-2", OldSP: 90, Stats: corecharacter.Stats{HP: 1000, MaxHP: 1000, MP: 40}}
	chars.characters[char.ID] = char
	weapon := coreitem.Instance{ID: "equipped-weapon", DefinitionID: "weapon-36", Quantity: 1}
	if err := inventory.Save(ctx, coreinventory.Inventory{CharacterID: char.ID, Items: []coreitem.Instance{weapon}}); err != nil {
		t.Fatal(err)
	}
	if err := equipment.Save(ctx, coreequipment.Equipment{CharacterID: char.ID, Slots: map[coreitem.Slot]string{coreitem.SlotMainHand: weapon.ID}}); err != nil {
		t.Fatal(err)
	}
	svc := battle.NewService(battle.WithCharacterRepository(chars), battle.WithInventoryRepository(inventory), battle.WithEquipmentRepository(equipment), battle.WithJobHistoryRepository(&historyReadStub{err: corejob.ErrCharacterJobNotFound}))
	a, err := svc.BuildLifetimeActor(ctx, char.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	a.TeamID = "party"
	state := corebattle.LifetimeState{Actors: []corebattle.LifetimeActor{a, {Participant: corebattle.Participant{ID: "enemy", TeamID: "npc", HP: 1000, Gear: &corebattle.BattleGearStats{}}, NPC: true, Tension: 1}}}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	var reloaded corebattle.LifetimeState
	if err := json.Unmarshal(data, &reloaded); err != nil {
		t.Fatal(err)
	}
	if reloaded.Actors[0].WeaponSeal != 7 || reloaded.Actors[0].Jobs.OldSP != 90 || reloaded.Actors[0].Gear.WeaponID != "weapon-36" || len(reloaded.Actors[0].Skills) != 0 {
		t.Fatal(reloaded)
	}
	c := corebattle.DamageContext{EffectContext: corebattle.EffectContext{TargetContext: corebattle.TargetContext{ActorID: char.ID, PartyIDs: []string{char.ID}, EnemyIDs: []string{"enemy"}}}, Defeat: func(string) error { return nil }}
	var definitions skill.BattleDefinitions
	definitions.RegisterWeaponSeals(&c)
	rng := random.NewDeterministic(1440)
	entry, err := definitions.WeaponSealAction(&reloaded, &c, "seal-7-breath", rng)
	if err != nil || entry.MPCost != 40 {
		t.Fatal(entry, err)
	}
	frame := corebattle.CallbackContext{TargetContext: c.TargetContext}
	if err := reloaded.ApplyInvocations(&frame, definitions.Invocations, []corebattle.CallbackInvocation{{Kind: "entry", EntryID: entry.ID}}, rng); err != nil {
		t.Fatal(err)
	}
	if reloaded.Actors[1].HP >= 1000 || reloaded.Actors[0].MP != 40 || !reflect.DeepEqual(chars.characters[char.ID], char) {
		t.Fatal(reloaded, chars.characters)
	}
}
