package battle_test

import (
	"context"
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

func TestNPC402BlessingUsesAdapterEquipmentAndRequesterLevel(t *testing.T) {
	ctx := context.Background()
	chars, inventory, equipment := newMockCharRepo(), newMockInvRepo(), newMockEquipRepo()
	char := corecharacter.Character{ID: "player", Level: 150, JobID: "job-01", Stats: corecharacter.Stats{HP: 50, MaxHP: 100, Attack: 100, Defense: 80, Agility: 100}}
	chars.characters[char.ID] = char
	accessory := coreitem.Instance{ID: "accessory-instance", DefinitionID: "item-205", Quantity: 1}
	if err := inventory.Save(ctx, coreinventory.Inventory{CharacterID: char.ID, Items: []coreitem.Instance{accessory}}); err != nil {
		t.Fatal(err)
	}
	if err := equipment.Save(ctx, coreequipment.Equipment{CharacterID: char.ID, Slots: map[coreitem.Slot]string{coreitem.SlotAccessory: accessory.ID}}); err != nil {
		t.Fatal(err)
	}
	svc := battle.NewService(battle.WithCharacterRepository(chars), battle.WithInventoryRepository(inventory), battle.WithEquipmentRepository(equipment), battle.WithJobHistoryRepository(&historyReadStub{err: corejob.ErrCharacterJobNotFound}))
	a, err := svc.BuildLifetimeActor(ctx, char.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	a.TeamID, a.Tension = "party", 3
	s := corebattle.LifetimeState{Actors: []corebattle.LifetimeActor{a, {
		Participant: corebattle.Participant{ID: "npc", TeamID: "party", HP: 1000, MaxHP: 1000, MP: 500, Gear: &corebattle.BattleGearStats{}},
		NPC:         true, Tension: 1, Jobs: corebattle.BattleJobs{JobID: "job-402"},
	}}}
	level := 10 // The requester differs from the affected player's level150.
	c := corebattle.NPCJob402Context{DamageContext: corebattle.DamageContext{EffectContext: corebattle.EffectContext{
		TargetContext: corebattle.TargetContext{ActorID: "npc", PartyIDs: []string{"npc", "player"}}, ActorNPC: true, RequesterLevel: &level,
	}}}
	var d skill.BattleDefinitions
	d.RegisterNPCJob402(&c)
	f := corebattle.CallbackContext{TargetContext: c.TargetContext}
	rng := &resetTableRNG{t: t}
	ops, err := d.InvocationAction(&s, &f, []string{"npc-job-402-position-1"}, rng)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyInvocations(&f, d.Invocations, ops, rng); err != nil {
		t.Fatal(err)
	}
	if s.Actors[0].Attack != 290 || s.Actors[0].Defense != 120 || s.Actors[0].Agility != 200 || s.Actors[0].Tension != 5 || s.Actors[0].Temporary != corebattle.TemporaryDouble || s.Actors[1].Status != corebattle.StatusDofuu || rng.draws != 0 || chars.characters[char.ID].Stats != char.Stats {
		t.Fatal(s, rng.draws)
	}
}
