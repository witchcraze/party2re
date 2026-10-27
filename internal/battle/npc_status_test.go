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
	"github.com/witchcraze/party2re/internal/core/skill"
)

func TestNPCStatusAdapterArmedFactsAndRequesterResetAfterReload(t *testing.T) {
	ctx := context.Background()
	chars, inventory, equipment := newMockCharRepo(), newMockInvRepo(), newMockEquipRepo()
	char := corecharacter.Character{ID: "player", Level: 150, JobID: "job-01", WeaponSeal: 3, Stats: corecharacter.Stats{HP: 50, MaxHP: 100, Attack: 100, Defense: 80, Agility: 100}}
	chars.characters[char.ID] = char
	weapon := coreitem.Instance{ID: "weapon-instance", DefinitionID: "weapon-69", Quantity: 1}
	if err := inventory.Save(ctx, coreinventory.Inventory{CharacterID: char.ID, Items: []coreitem.Instance{weapon}}); err != nil {
		t.Fatal(err)
	}
	if err := equipment.Save(ctx, coreequipment.Equipment{CharacterID: char.ID, Slots: map[coreitem.Slot]string{coreitem.SlotMainHand: weapon.ID}}); err != nil {
		t.Fatal(err)
	}
	svc := battle.NewService(battle.WithCharacterRepository(chars), battle.WithInventoryRepository(inventory), battle.WithEquipmentRepository(equipment), battle.WithRandomSource(mockRNG{val: 45}), battle.WithJobHistoryRepository(&historyReadStub{err: corejob.ErrCharacterJobNotFound}))
	a, err := svc.BuildLifetimeActor(ctx, char.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	a.TeamID, a.Attack = "party", 100
	rng := &resetTableRNG{t: t}
	seals, err := battle.BuildWeaponSealTable(a.Participant, rng)
	if err != nil {
		t.Fatal(err)
	}
	s := corebattle.LifetimeState{Actors: []corebattle.LifetimeActor{a, {Participant: corebattle.Participant{ID: "npc", TeamID: "npc", HP: 1000, MaxHP: 1000, Gear: &corebattle.BattleGearStats{}}, NPC: true, Tension: 1}}}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var loaded corebattle.LifetimeState
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	level := 10
	c := corebattle.NPCStatusContext{DamageContext: corebattle.DamageContext{EffectContext: corebattle.EffectContext{TargetContext: corebattle.TargetContext{ActorID: "player", PartyIDs: []string{"player"}, EnemyIDs: []string{"npc"}}, RequesterLevel: &level}, GuaranteedHit: true, Defeat: func(string) error { t.Fatal("nonlethal claw must not defeat"); return nil }}, Death: &corebattle.DefeatContext{EquipmentResetContext: corebattle.EquipmentResetContext{RequesterLevel: &level, Seals: &seals}}}
	var d skill.BattleDefinitions
	d.RegisterNPCStatus(&c)
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"npc"}}
	// Adapter weapon identity selects AT×0.8, not the unarmed AT×1.5 branch.
	if err := loaded.ApplyInvocations(&f, d.Invocations, []corebattle.CallbackInvocation{{Kind: "entry", EntryID: "npc-job-88-position-1"}}, rng); err != nil {
		t.Fatal(err)
	}
	if loaded.Actors[1].HP != 905 {
		t.Fatal(loaded)
	}
	// Current HP>999 makes the enemy death helper immune. Only self reset runs;
	// weapon69's adjustment uses requester10, not recipient level150.
	loaded.Actors[1].HP = 1000
	if err := loaded.ApplyInvocations(&f, d.Invocations, []corebattle.CallbackInvocation{{Kind: "entry", EntryID: "npc-job-94-position-0"}}, rng); err != nil {
		t.Fatal(err)
	}
	p := loaded.Actors[0]
	if p.HP != 0 || p.MP != 1 || p.Attack != 169 || p.Gear.WeaponAttack == nil || *p.Gear.WeaponAttack != 45 || chars.characters[char.ID].Stats.HP != 50 {
		t.Fatal(loaded)
	}
}
