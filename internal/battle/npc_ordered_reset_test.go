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
	"github.com/witchcraze/party2re/internal/core/skill"
)

func TestNPCOrderedResetUsesReloadedAdapterGearAndRequester(t *testing.T) {
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
	a.TeamID, a.Attack, a.Temporary = "party", 999, corebattle.TemporaryOneHitDefense
	rng := &resetTableRNG{t: t}
	seals, err := battle.BuildWeaponSealTable(a.Participant, rng)
	if err != nil {
		t.Fatal(err)
	}
	s := corebattle.LifetimeState{Actors: []corebattle.LifetimeActor{a, {
		Participant: corebattle.Participant{ID: "npc", TeamID: "npc", HP: 1000, MaxHP: 1000, MP: 500, Gear: &corebattle.BattleGearStats{}},
		NPC:         true, Tension: 1, Jobs: corebattle.BattleJobs{JobID: "job-403", SP: 190, OldJobID: "job-83", OldSP: 35},
	}}}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var loaded corebattle.LifetimeState
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	level := 10 // Requester facts are independent of the NPC and target levels.
	c := corebattle.NPCOrderedResetContext{DamageContext: corebattle.DamageContext{
		EffectContext: corebattle.EffectContext{TargetContext: corebattle.TargetContext{ActorID: "npc", PartyIDs: []string{"npc"}, EnemyIDs: []string{"player"}}, ActorNPC: true, RequesterLevel: &level},
		Defeat:        func(string) error { t.Fatal("unexpected defeat"); return nil },
	}, EquipmentReset: &corebattle.EquipmentResetContext{RequesterLevel: &level, Seals: &seals}}
	var d skill.BattleDefinitions
	d.RegisterNPCOrderedReset(&c)
	f := corebattle.CallbackContext{TargetContext: c.TargetContext}
	ops, err := d.InvocationAction(&loaded, &f, []string{"npc-job-403-position-3"}, rng)
	if err != nil || rng.draws != 1 {
		t.Fatal(err, rng.draws)
	}
	if err := loaded.ApplyInvocations(&f, d.Invocations, ops, rng); err != nil {
		t.Fatal(err)
	}
	p := loaded.Actors[0]
	if p.Attack != 169 || p.HP != 21 || p.BaseStats.Attack != 100 || p.Gear.WeaponAttack == nil || *p.Gear.WeaponAttack != 45 || p.Temporary != "" || rng.draws != 2 || loaded.Actors[1].Jobs != s.Actors[1].Jobs || loaded.Actors[1].MP != 500 || !reflect.DeepEqual(chars.characters[char.ID], char) {
		t.Fatal(loaded, rng.draws)
	}
}
