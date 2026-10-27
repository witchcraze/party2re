package battle_test

import (
	"context"
	"encoding/json"
	"fmt"
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

func TestNPC97ResetUsesAdapterGearRequesterAndSavedTension(t *testing.T) {
	testNPCResetAdapter(t, 97)
}

func TestNPC98ResetUsesAdapterGearRequesterAndSavedTension(t *testing.T) {
	testNPCResetAdapter(t, 98)
}

func testNPCResetAdapter(t *testing.T, jobID int) {
	t.Helper()

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
	a.TeamID, a.Attack, a.Temporary, a.Tension = "party", 999, corebattle.TemporaryOneHitDefense, 5
	rng := &resetTableRNG{t: t}
	seals, err := battle.BuildWeaponSealTable(a.Participant, rng)
	if err != nil {
		t.Fatal(err)
	}
	s := corebattle.LifetimeState{Actors: []corebattle.LifetimeActor{a, {
		Participant: corebattle.Participant{ID: "npc", TeamID: "npc", HP: 1000, MaxHP: 1000, MP: 500, Gear: &corebattle.BattleGearStats{}},
		NPC:         true, Tension: 3, Jobs: corebattle.BattleJobs{JobID: fmt.Sprintf("job-%d", jobID)},
	}}}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var loaded corebattle.LifetimeState
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	level := 10 // Requester level differs from the reset recipient's level150.
	c := corebattle.NPCJob98Context{DamageContext: corebattle.DamageContext{
		EffectContext: corebattle.EffectContext{TargetContext: corebattle.TargetContext{ActorID: "npc", PartyIDs: []string{"npc"}, EnemyIDs: []string{"player"}}, ActorNPC: true, RequesterLevel: &level},
	}, EquipmentReset: &corebattle.EquipmentResetContext{RequesterLevel: &level, Seals: &seals}}
	var d skill.BattleDefinitions
	position := 1
	if jobID == 97 {
		d.RegisterNPCJob97(&corebattle.NPCJob97Context{DamageContext: c.DamageContext, EquipmentReset: c.EquipmentReset})
		position = 2
	} else {
		d.RegisterNPCJob98(&c)
	}
	f := corebattle.CallbackContext{TargetContext: c.TargetContext}
	ops, err := d.InvocationAction(&loaded, &f, []string{fmt.Sprintf("npc-job-%d-position-%d", jobID, position)}, rng)
	if err != nil || rng.draws != 1 {
		t.Fatal(err, rng.draws)
	}
	if err := loaded.ApplyInvocations(&f, d.Invocations, ops, rng); err != nil {
		t.Fatal(err)
	}
	p := loaded.Actors[0]
	if p.Attack != 169 || p.HP != 50 || p.Tension != 5 || p.BaseStats.Attack != 100 || p.Gear.WeaponAttack == nil || *p.Gear.WeaponAttack != 45 || p.Temporary != "" || rng.draws != 1 || loaded.Actors[1].Tension != 3 || loaded.Actors[1].MP != 500 || chars.characters[char.ID].Stats.HP != 50 {
		t.Fatal(loaded, rng.draws)
	}
}
